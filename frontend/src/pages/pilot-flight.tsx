import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";

import { useAuth0 } from "@auth0/auth0-react";
import { getApiUrl } from "@/lib/api-url";

const STORAGE_KEY = "pilot-flight-callsign";

type FlightInfo = {
  callsign: string;
  origin: string;
  destination: string;
  is_departure: boolean;
  cleared: boolean;
  pdc_available: boolean;
  pdc_can_submit: boolean;
  pdc_state: string;
  pdc_clearance_text?: string;
  pdc_request_remarks?: string;
  pdc_acknowledged_at?: string;
  pdc_requires_pilot_action: boolean;

  // Optional pilot-facing CDM/ground fields.
  // The backend can start returning these from /api/pilot/flight without breaking older clients.
  eobt?: string | null;
  tobt?: string | null;
  ctot?: string | null;
  pushback_point?: string | null;
  pushback_instruction?: string | null;
};

type PilotProfileResponse = {
  cid: string;
  online_callsign?: string;
  callsign_locked: boolean;
  live_mode: boolean;
};

type MockArrivalHolding = {
  id: string;
  arrival: string;
  hold_fix: string;
  active: boolean;
  eat: string;
  delay: string;
  affected_callsigns: string[];
  updated_at: string;
  remarks: string;
};

// Holding is intentionally mock-only for now because FlightStrips does not yet
// have a holding definition/domain model. Replace this array with an API call
// when the backend model exists.
const MOCK_ARRIVAL_HOLDING_BOARD: MockArrivalHolding[] = [
  {
    id: "tespi-rosbi",
    arrival: "TESPI arrivals",
    hold_fix: "ROSBI",
    active: true,
    eat: "1920Z",
    delay: "+12 min",
    affected_callsigns: ["RYR72KM", "SAS455", "BAW816"],
    updated_at: "1910Z",
    remarks: "Holding established for the north-west inbound stream.",
  },
  {
    id: "tudlo-lugas",
    arrival: "TUDLO arrivals",
    hold_fix: "LUGAS",
    active: true,
    eat: "1925Z",
    delay: "+15 min",
    affected_callsigns: ["DLH3EK", "KLM1127"],
    updated_at: "1910Z",
    remarks: "Expect one to two circuits before onward clearance.",
  },
  {
    id: "ernov-ernov",
    arrival: "ERNOV arrivals",
    hold_fix: "ERNOV",
    active: false,
    eat: "No delay",
    delay: "0 min",
    affected_callsigns: [],
    updated_at: "1910Z",
    remarks: "No mock hold established on this arrival.",
  },
];

const pdcStateLabels: Record<string, { label: string; tone: string }> = {
  REQUESTED: {
    label: "Waiting on controller",
    tone: "bg-amber-100 text-amber-900 border-amber-300 dark:bg-amber-950/40 dark:text-amber-100 dark:border-amber-800",
  },
  CLEARED: {
    label: "Clearance available",
    tone: "bg-emerald-100 text-emerald-900 border-emerald-300 dark:bg-emerald-950/40 dark:text-emerald-100 dark:border-emerald-800",
  },
  CONFIRMED: {
    label: "Clearance acknowledged",
    tone: "bg-sky-100 text-sky-900 border-sky-300 dark:bg-sky-950/40 dark:text-sky-100 dark:border-sky-800",
  },
  FAILED: {
    label: "Unable via web PDC",
    tone: "bg-rose-100 text-rose-900 border-rose-300 dark:bg-rose-950/40 dark:text-rose-100 dark:border-rose-800",
  },
  REVERT_TO_VOICE: {
    label: "Contact ATC by voice",
    tone: "bg-rose-100 text-rose-900 border-rose-300 dark:bg-rose-950/40 dark:text-rose-100 dark:border-rose-800",
  },
  NO_RESPONSE: {
    label: "PDC expired, contact ATC by voice",
    tone: "bg-rose-100 text-rose-900 border-rose-300 dark:bg-rose-950/40 dark:text-rose-100 dark:border-rose-800",
  },
};

function pdcHasBeenAttempted(state: string): boolean {
  return state !== "" && state !== "NONE";
}

function shouldPoll(state: string): boolean {
  return state === "REQUESTED" || state === "CLEARED";
}

function normalizeCallsign(value: string): string {
  return value.trim().toUpperCase();
}

function displayValue(
  value?: string | null,
  fallback = "Not assigned",
): string {
  const trimmed = value?.trim();
  return trimmed ? trimmed : fallback;
}

function getMockHoldingForCallsign(
  callsign: string,
): MockArrivalHolding | null {
  const normalized = normalizeCallsign(callsign);

  if (!normalized) return null;

  return (
    MOCK_ARRIVAL_HOLDING_BOARD.find(
      (holding) =>
        holding.active && holding.affected_callsigns.includes(normalized),
    ) ?? null
  );
}

export default function PilotFlightPage() {
  const { getAccessTokenSilently } = useAuth0();
  const storedCallsign =
    typeof window !== "undefined"
      ? (window.sessionStorage.getItem(STORAGE_KEY) ?? "")
      : "";

  const [inputCallsign, setInputCallsign] = useState(storedCallsign);
  const [activeCallsign, setActiveCallsign] = useState(storedCallsign);
  const [pilotProfile, setPilotProfile] = useState<PilotProfileResponse | null>(
    null,
  );
  const [flightInfo, setFlightInfo] = useState<FlightInfo | null>(null);
  const [flightError, setFlightError] = useState<{
    status: number;
    message: string;
  } | null>(null);
  const [aircraftType, setAircraftType] = useState("");
  const [atis, setAtis] = useState("");
  const [stand, setStand] = useState("");
  const [remarks, setRemarks] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  // PDC acknowledge state
  const [isAcknowledging, setIsAcknowledging] = useState(false);
  const [acknowledgeError, setAcknowledgeError] = useState<string | null>(null);
  const [isUnable, setIsUnable] = useState(false);
  const [unableError, setUnableError] = useState<string | null>(null);
  const [profileLoaded, setProfileLoaded] = useState(false);

  const isLiveMode = pilotProfile?.live_mode === true;

  // Derived loading state: we consider ourselves loading when there is an active callsign
  // but no result yet (avoids calling setState synchronously inside an effect).
  const isLoadingFlight =
    !!activeCallsign && profileLoaded && !flightInfo && !flightError;

  const authorizedFetch = useCallback(
    async (path: string, init?: RequestInit) => {
      const token = await getAccessTokenSilently();
      const response = await fetch(getApiUrl(path), {
        ...init,
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
          ...(init?.headers ?? {}),
        },
      });

      if (!response.ok) {
        const contentType = response.headers.get("content-type") ?? "";
        const fallbackMessage = `Request failed (${response.status} ${response.statusText})`;
        let message = fallbackMessage;

        if (contentType.includes("application/json")) {
          const payload = (await response.json().catch(() => null)) as {
            error?: string;
          } | null;
          message = payload?.error ?? fallbackMessage;
        } else {
          const text = (await response.text().catch(() => "")).trim();
          message = text || fallbackMessage;
        }

        const err = new Error(message) as Error & { status: number };
        err.status = response.status;
        throw err;
      }

      return response.json().catch(() => null);
    },
    [getAccessTokenSilently],
  );

  // Load pilot profile once.
  useEffect(() => {
    let active = true;

    const load = async () => {
      try {
        const payload = (await authorizedFetch(
          "/api/pilot/me",
        )) as PilotProfileResponse;
        if (!active || !payload) return;

        setPilotProfile(payload);

        if (payload.live_mode) {
          if (payload.online_callsign) {
            setInputCallsign(payload.online_callsign);
            setActiveCallsign(payload.online_callsign);
            window.sessionStorage.setItem(STORAGE_KEY, payload.online_callsign);
          } else {
            // Live mode but not currently online: clear any stale test callsign.
            setActiveCallsign("");
            setFlightInfo(null);
            setFlightError(null);
          }
        }
      } catch {
        if (!active) return;
        setPilotProfile(null);
      } finally {
        if (active) setProfileLoaded(true);
      }
    };

    void load();

    return () => {
      active = false;
    };
  }, [authorizedFetch]);

  const fetchFlight = useCallback(
    async (callsign: string) => {
      if (!callsign) return;

      try {
        const payload = (await authorizedFetch(
          `/api/pilot/flight?callsign=${encodeURIComponent(callsign)}`,
        )) as FlightInfo;

        setFlightInfo(payload);
        setFlightError(null);
      } catch (error) {
        setFlightInfo(null);
        const status = (error as Error & { status?: number }).status ?? 0;
        const message =
          error instanceof Error
            ? error.message
            : "Failed to load flight information";
        setFlightError({ status, message });
      }
    },
    [authorizedFetch],
  );

  const hasFlightInfo = flightInfo !== null;
  const pdcPollActive = shouldPoll(flightInfo?.pdc_state ?? "");

  // Fetch + poll flight info when activeCallsign changes. PDC-active flights poll faster,
  // but operational data such as TOBT and pushback point should keep updating too.
  useEffect(() => {
    if (!activeCallsign || !profileLoaded) return;

    let active = true;

    // Capture whether we already have data at effect creation. Used to decide whether
    // transient errors should kill the UI or silently retry while keeping stale data.
    const isPollingMode = pdcPollActive;
    const hasLoadedFlight = hasFlightInfo;

    const poll = async () => {
      try {
        const payload = (await authorizedFetch(
          `/api/pilot/flight?callsign=${encodeURIComponent(activeCallsign)}`,
        )) as FlightInfo;

        if (!active) return;
        setFlightInfo(payload);
        setFlightError(null);
      } catch (error) {
        if (!active) return;

        const status = (error as Error & { status?: number }).status ?? 0;
        const message =
          error instanceof Error
            ? error.message
            : "Failed to load flight information";
        const isTransient = status === 0 || status >= 500;

        if (!isTransient || (!isPollingMode && !hasLoadedFlight)) {
          // Definitive error (4xx) or initial load failure: clear state and surface the error.
          setFlightInfo(null);
          setFlightError({ status, message });
        }

        // Transient error while polling an already loaded flight: keep stale data and retry.
      }
    };

    void poll();

    const id = window.setInterval(
      () => void poll(),
      pdcPollActive ? 3000 : 10000,
    );

    return () => {
      active = false;
      window.clearInterval(id);
    };
  }, [
    authorizedFetch,
    activeCallsign,
    hasFlightInfo,
    pdcPollActive,
    profileLoaded,
  ]);

  const canSubmitPdc = Boolean(
    flightInfo?.pdc_can_submit && !flightInfo?.cleared,
  );

  const handleCallsignSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    const normalized = normalizeCallsign(inputCallsign);
    if (!normalized) return;

    window.sessionStorage.setItem(STORAGE_KEY, normalized);
    setActiveCallsign(normalized);
    setFlightInfo(null);
    setFlightError(null);
    setSubmitError(null);
    setAcknowledgeError(null);
  };

  const handlePdcSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    if (!activeCallsign || !canSubmitPdc) return;

    setIsSubmitting(true);
    setSubmitError(null);

    try {
      await authorizedFetch("/api/pdc/request", {
        method: "POST",
        body: JSON.stringify({
          callsign: activeCallsign,
          aircraft_type: aircraftType.trim().toUpperCase(),
          atis: atis.trim().toUpperCase(),
          stand: stand.trim(),
          remarks: remarks.trim(),
        }),
      });

      await fetchFlight(activeCallsign);
    } catch (error) {
      setSubmitError(
        error instanceof Error ? error.message : "Failed to submit PDC request",
      );
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleAcknowledge = async () => {
    if (!activeCallsign) return;

    setIsAcknowledging(true);
    setAcknowledgeError(null);

    try {
      await authorizedFetch("/api/pdc/acknowledge", {
        method: "POST",
        body: JSON.stringify({ callsign: activeCallsign }),
      });

      await fetchFlight(activeCallsign);
    } catch (error) {
      setAcknowledgeError(
        error instanceof Error
          ? error.message
          : "Failed to acknowledge clearance",
      );
    } finally {
      setIsAcknowledging(false);
    }
  };

  const handleUnable = async () => {
    if (!activeCallsign) return;

    setIsUnable(true);
    setUnableError(null);

    try {
      await authorizedFetch("/api/pdc/unable", {
        method: "POST",
        body: JSON.stringify({ callsign: activeCallsign }),
      });

      await fetchFlight(activeCallsign);
    } catch (error) {
      setUnableError(
        error instanceof Error ? error.message : "Failed to send unable",
      );
    } finally {
      setIsUnable(false);
    }
  };

  const pdcStateMeta = useMemo(() => {
    const state = flightInfo?.pdc_state ?? "";

    return (
      pdcStateLabels[state] ?? {
        label: state || "No active request",
        tone: "bg-slate-100 text-slate-900 border-slate-300 dark:bg-slate-900/50 dark:text-slate-100 dark:border-slate-700",
      }
    );
  }, [flightInfo?.pdc_state]);

  const departureCdmItems = useMemo(
    () => [
      {
        label: "EOBT",
        value: displayValue(flightInfo?.eobt, "Not filed"),
        help: "Estimated off-block time from filed or assigned data.",
      },
      {
        label: "TOBT",
        value: displayValue(flightInfo?.tobt),
        help: "Target off-block time from EKCH flow/sequence.",
      },
      {
        label: "CTOT",
        value: displayValue(flightInfo?.ctot, "Not regulated"),
        help: "Calculated take-off time when a regulation exists.",
      },
    ],
    [flightInfo?.ctot, flightInfo?.eobt, flightInfo?.tobt],
  );

  const activeMockHolding = useMemo(
    () => getMockHoldingForCallsign(activeCallsign),
    [activeCallsign],
  );

  const activeHoldingCount = useMemo(
    () => MOCK_ARRIVAL_HOLDING_BOARD.filter((holding) => holding.active).length,
    [],
  );

  return (
    <div className="space-y-6">
      {/* Callsign lookup bar - testing only, hidden on live server */}
      {profileLoaded && !isLiveMode && (
        <form className="flex gap-3" onSubmit={handleCallsignSubmit}>
          <input
            value={inputCallsign}
            onChange={(e) => setInputCallsign(e.target.value.toUpperCase())}
            className="flex-1 rounded-sm border border-neutral-300/90 bg-white px-4 py-3 text-sm outline-none transition focus:border-[#003d48] placeholder:text-neutral-400 dark:border-white/10 dark:bg-[#101010] dark:placeholder:text-neutral-500"
            placeholder="SAS123"
            autoComplete="off"
          />
          <button
            type="submit"
            disabled={!inputCallsign.trim()}
            className="rounded-sm bg-primary px-5 py-3 text-sm font-semibold text-white dark:text-navy transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            Look up
          </button>
        </form>
      )}

      {/* Profile loading */}
      {!profileLoaded && (
        <div className="rounded-sm border border-neutral-300/90 bg-white shadow-sm dark:border-white/10 dark:bg-[#101010] px-6 py-12 text-center text-sm text-neutral-500 dark:text-neutral-400 shadow-sm">
          Loading...
        </div>
      )}

      {/* Active flight card - always shown once profile is loaded */}
      {profileLoaded && (
        <div className="rounded-sm border border-neutral-300/90 bg-white shadow-sm dark:border-white/10 dark:bg-[#101010] p-6 shadow-sm">
          <p className="text-sm font-semibold uppercase tracking-[0.2em] text-primary">
            Active flight
          </p>

          {/* Empty state - testing mode, no callsign entered yet */}
          {!isLiveMode && !activeCallsign && (
            <p className="mt-4 py-8 text-center text-sm text-neutral-500 dark:text-neutral-400">
              Enter a callsign above to look up flight status.
            </p>
          )}

          {/* Loading */}
          {isLoadingFlight && (
            <p className="mt-4 py-8 text-center text-sm text-neutral-500 dark:text-neutral-400">
              Loading flight information...
            </p>
          )}

          {/* No active flight: 404 from API, or live mode with no online callsign */}
          {!isLoadingFlight &&
            (flightError?.status === 404 ||
              (isLiveMode && !activeCallsign)) && (
              <p className="mt-4 py-8 text-center text-sm text-neutral-500 dark:text-neutral-400">
                No active flight found.
              </p>
            )}

          {/* Other errors */}
          {flightError && flightError.status !== 404 && (
            <div className="mt-4 rounded-sm border border-rose-200 dark:border-rose-800 bg-rose-50 dark:bg-rose-950/40 px-4 py-3 text-sm text-rose-900 dark:text-rose-100">
              {flightError.message}
            </div>
          )}

          {/* Flight details */}
          {flightInfo && (
            <>
              <div className="mt-4 flex items-start justify-between gap-4">
                <div>
                  <p className="text-2xl font-bold tracking-tight">
                    {flightInfo.callsign}
                  </p>
                  <p className="mt-2 text-base font-medium text-neutral-800 dark:text-neutral-200">
                    {flightInfo.origin}
                    <span className="mx-2 text-navy/40 dark:text-muted-foreground">
                      -&gt;
                    </span>
                    {flightInfo.destination}
                  </p>
                </div>

                <span
                  className={`mt-1 shrink-0 rounded-lg border px-3 py-1 text-xs font-semibold uppercase tracking-wide ${
                    flightInfo.is_departure
                      ? "border-primary/20 bg-primary/10 text-primary"
                      : "border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-800 dark:bg-sky-950/40 dark:text-sky-300"
                  }`}
                >
                  {flightInfo.is_departure ? "Departure" : "Arrival"}
                </span>
              </div>

              {flightInfo.cleared && (
                <p className="mt-3 text-sm font-medium text-emerald-700 dark:text-emerald-400">
                  Strip cleared
                </p>
              )}
            </>
          )}
        </div>
      )}

      {/* Departure operational data */}
      {flightInfo?.is_departure && (
        <div className="rounded-sm border border-neutral-300/90 bg-white shadow-sm dark:border-white/10 dark:bg-[#101010] p-6 shadow-sm">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-sm font-semibold uppercase tracking-[0.2em] text-primary">
                EKCH departure sequence
              </p>
              <p className="mt-2 text-sm text-neutral-600 dark:text-neutral-400">
                Pilot-facing CDM and pushback information from the flight status
                response.
              </p>
            </div>
            <span className="rounded-lg border border-primary/20 bg-primary/10 px-3 py-1 text-xs font-semibold uppercase tracking-wide text-primary">
              Departure
            </span>
          </div>

          <div className="mt-6 grid gap-3 sm:grid-cols-3">
            {departureCdmItems.map((item) => (
              <div
                key={item.label}
                className="rounded-sm border border-neutral-200 bg-neutral-50 px-4 py-4 dark:border-white/10 dark:bg-white/5"
              >
                <p className="text-xs font-semibold uppercase tracking-[0.18em] text-neutral-500 dark:text-neutral-400">
                  {item.label}
                </p>
                <p className="mt-2 text-xl font-bold text-neutral-950 dark:text-neutral-50">
                  {item.value}
                </p>
                <p className="mt-2 text-xs leading-5 text-neutral-500 dark:text-neutral-400">
                  {item.help}
                </p>
              </div>
            ))}
          </div>

          <div className="mt-4 rounded-sm border border-neutral-200 bg-neutral-50 px-4 py-4 dark:border-white/10 dark:bg-white/5">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p className="text-xs font-semibold uppercase tracking-[0.18em] text-neutral-500 dark:text-neutral-400">
                  Pushback point
                </p>
                <p className="mt-2 text-xl font-bold text-neutral-950 dark:text-neutral-50">
                  {displayValue(flightInfo.pushback_point)}
                </p>
              </div>
              {!flightInfo.pushback_point && (
                <span className="rounded-lg border border-amber-300 bg-amber-100 px-3 py-1 text-xs font-semibold uppercase tracking-wide text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-100">
                  Awaiting ATC
                </span>
              )}
            </div>
            <p className="mt-2 text-sm leading-6 text-neutral-600 dark:text-neutral-400">
              {flightInfo.pushback_point?.trim()
                ? `Pushback point ${flightInfo.pushback_point.trim()}`
                : "No pushback point has been assigned yet."}
            </p>
          </div>
        </div>
      )}

      {/* Arrival holding data - mock only until FlightStrips has a holding model */}
      {flightInfo && !flightInfo.is_departure && (
        <div className="rounded-sm border border-neutral-300/90 bg-white shadow-sm dark:border-white/10 dark:bg-[#101010] p-6 shadow-sm">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-sm font-semibold uppercase tracking-[0.2em] text-primary">
                EKCH arrival holding
              </p>
              <p className="mt-2 text-sm text-neutral-600 dark:text-neutral-400">
                Mock holding board. Replace with a backend source once holding
                is defined in FlightStrips.
              </p>
            </div>
            <span
              className={`rounded-lg border px-3 py-1 text-xs font-semibold uppercase tracking-wide ${
                activeHoldingCount > 0
                  ? "border-amber-300 bg-amber-100 text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-100"
                  : "border-emerald-300 bg-emerald-100 text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-100"
              }`}
            >
              {activeHoldingCount > 0
                ? `${activeHoldingCount} active`
                : "No holds"}
            </span>
          </div>

          <div className="mt-6 rounded-sm border border-neutral-200 bg-neutral-50 px-4 py-4 dark:border-white/10 dark:bg-white/5">
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-neutral-500 dark:text-neutral-400">
              Your EAT
            </p>

            {activeMockHolding ? (
              <>
                <p className="mt-2 text-xl font-bold text-neutral-950 dark:text-neutral-50">
                  {activeMockHolding.eat}
                </p>
                <p className="mt-2 text-sm leading-6 text-neutral-600 dark:text-neutral-400">
                  Hold established at {activeMockHolding.hold_fix} on{" "}
                  {activeMockHolding.arrival}. Current mock delay{" "}
                  {activeMockHolding.delay}. {activeMockHolding.remarks}
                </p>
              </>
            ) : (
              <>
                <p className="mt-2 text-xl font-bold text-neutral-950 dark:text-neutral-50">
                  No callsign-specific mock EAT
                </p>
                <p className="mt-2 text-sm leading-6 text-neutral-600 dark:text-neutral-400">
                  No mock holding assignment is attached to{" "}
                  {flightInfo.callsign}. Check the board below and follow ATC
                  instructions.
                </p>
              </>
            )}
          </div>

          <div className="mt-6 overflow-x-auto">
            <table className="min-w-full divide-y divide-neutral-200 text-sm dark:divide-white/10">
              <thead>
                <tr className="text-left text-xs font-semibold uppercase tracking-[0.18em] text-neutral-500 dark:text-neutral-400">
                  <th className="py-3 pr-4">Arrival</th>
                  <th className="py-3 pr-4">Hold</th>
                  <th className="py-3 pr-4">Status</th>
                  <th className="py-3 pr-4">EAT</th>
                  <th className="py-3 pr-4">Delay</th>
                  <th className="py-3 pr-4">Updated</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-200 dark:divide-white/10">
                {MOCK_ARRIVAL_HOLDING_BOARD.map((holding) => (
                  <tr key={holding.id}>
                    <td className="py-3 pr-4 font-medium text-neutral-950 dark:text-neutral-50">
                      {holding.arrival}
                    </td>
                    <td className="py-3 pr-4 text-neutral-700 dark:text-neutral-300">
                      {holding.hold_fix}
                    </td>
                    <td className="py-3 pr-4">
                      <span
                        className={`rounded-lg border px-2.5 py-1 text-xs font-semibold uppercase tracking-wide ${
                          holding.active
                            ? "border-amber-300 bg-amber-100 text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-100"
                            : "border-emerald-300 bg-emerald-100 text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-100"
                        }`}
                      >
                        {holding.active ? "Established" : "Not active"}
                      </span>
                    </td>
                    <td className="py-3 pr-4 font-semibold text-neutral-950 dark:text-neutral-50">
                      {holding.eat}
                    </td>
                    <td className="py-3 pr-4 text-neutral-700 dark:text-neutral-300">
                      {holding.delay}
                    </td>
                    <td className="py-3 pr-4 text-neutral-700 dark:text-neutral-300">
                      {holding.updated_at}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* PDC card - shown when available or when a PDC has been attempted (status persists after strip is cleared) */}
      {flightInfo &&
        (flightInfo.pdc_available ||
          pdcHasBeenAttempted(flightInfo.pdc_state)) && (
          <div className="rounded-sm border border-neutral-300/90 bg-white shadow-sm dark:border-white/10 dark:bg-[#101010] p-6 shadow-sm">
            <p className="text-sm font-semibold uppercase tracking-[0.2em] text-primary">
              Pre-departure clearance
            </p>

            {canSubmitPdc ? (
              /* Submit form */
              <form
                className="mt-6 grid gap-4 sm:grid-cols-2"
                onSubmit={handlePdcSubmit}
              >
                <label className="grid gap-2 text-sm font-medium">
                  Aircraft type
                  <input
                    value={aircraftType}
                    onChange={(e) =>
                      setAircraftType(e.target.value.toUpperCase())
                    }
                    className="rounded-sm border border-neutral-300/90 bg-white px-4 py-3 outline-none transition focus:border-[#003d48] placeholder:text-neutral-400 dark:border-white/10 dark:bg-[#101010] dark:placeholder:text-neutral-500"
                    placeholder="A320"
                    autoComplete="off"
                    required
                  />
                </label>

                <label className="grid gap-2 text-sm font-medium">
                  ATIS letter
                  <input
                    value={atis}
                    onChange={(e) =>
                      setAtis(e.target.value.toUpperCase().slice(0, 1))
                    }
                    className="rounded-sm border border-neutral-300/90 bg-white px-4 py-3 outline-none transition focus:border-[#003d48] placeholder:text-neutral-400 dark:border-white/10 dark:bg-[#101010] dark:placeholder:text-neutral-500"
                    placeholder="A"
                    autoComplete="off"
                    required
                  />
                </label>

                <label className="grid gap-2 text-sm font-medium">
                  Stand
                  <input
                    value={stand}
                    onChange={(e) => setStand(e.target.value.toUpperCase())}
                    className="rounded-sm border border-neutral-300/90 bg-white px-4 py-3 outline-none transition focus:border-[#003d48] placeholder:text-neutral-400 dark:border-white/10 dark:bg-[#101010] dark:placeholder:text-neutral-500"
                    placeholder="A12"
                    autoComplete="off"
                  />
                </label>

                <label className="sm:col-span-2 grid gap-2 text-sm font-medium">
                  Remarks for ATC
                  <textarea
                    value={remarks}
                    onChange={(e) => setRemarks(e.target.value)}
                    className="min-h-20 rounded-sm border border-neutral-300/90 bg-white px-4 py-3 outline-none transition focus:border-[#003d48] placeholder:text-neutral-400 dark:border-white/10 dark:bg-[#101010] dark:placeholder:text-neutral-500"
                    placeholder="Optional remarks for manual review."
                  />
                </label>

                {submitError && (
                  <div className="sm:col-span-2 rounded-sm border border-rose-200 dark:border-rose-800 bg-rose-50 dark:bg-rose-950/40 px-4 py-3 text-sm text-rose-900 dark:text-rose-100">
                    {submitError}
                  </div>
                )}

                <div className="sm:col-span-2">
                  <button
                    type="submit"
                    disabled={isSubmitting || !canSubmitPdc}
                    className="rounded-sm bg-primary px-5 py-3 text-sm font-semibold text-white dark:text-navy transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {isSubmitting ? "Submitting..." : "Submit PDC request"}
                  </button>
                </div>
              </form>
            ) : (
              /* Status panel */
              <div className="mt-6 space-y-4">
                <div
                  className={`rounded-xl border px-4 py-3 text-sm font-medium ${pdcStateMeta.tone}`}
                >
                  {pdcStateMeta.label}
                </div>

                {flightInfo.pdc_request_remarks && (
                  <div className="rounded-sm border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/40 px-4 py-3 text-sm text-amber-950 dark:text-amber-100">
                    <p className="font-semibold">Remarks sent to ATC</p>
                    <p className="mt-1 whitespace-pre-wrap">
                      {flightInfo.pdc_request_remarks}
                    </p>
                  </div>
                )}

                {flightInfo.pdc_clearance_text && (
                  <div className="rounded-sm border border-primary/15 dark:border-primary/30 bg-primary/5 dark:bg-primary/10 px-4 py-3">
                    <p className="text-sm font-semibold text-primary">
                      Clearance
                    </p>
                    <pre className="mt-3 whitespace-pre-wrap text-sm leading-6 text-neutral-800 dark:text-neutral-200">
                      {flightInfo.pdc_clearance_text}
                    </pre>
                  </div>
                )}

                {flightInfo.pdc_acknowledged_at && (
                  <p className="text-sm text-neutral-600 dark:text-neutral-400">
                    Acknowledged at{" "}
                    {new Date(flightInfo.pdc_acknowledged_at).toLocaleString()}.
                  </p>
                )}

                {acknowledgeError && (
                  <div className="rounded-sm border border-rose-200 dark:border-rose-800 bg-rose-50 dark:bg-rose-950/40 px-4 py-3 text-sm text-rose-900 dark:text-rose-100">
                    {acknowledgeError}
                  </div>
                )}

                {unableError && (
                  <div className="rounded-sm border border-rose-200 dark:border-rose-800 bg-rose-50 dark:bg-rose-950/40 px-4 py-3 text-sm text-rose-900 dark:text-rose-100">
                    {unableError}
                  </div>
                )}

                <div className="flex flex-wrap gap-3">
                  <button
                    type="button"
                    onClick={() => void fetchFlight(activeCallsign)}
                    className="rounded-sm border border-neutral-300/90 px-4 py-2 text-sm font-semibold text-neutral-800 transition hover:bg-neutral-100 dark:border-white/10 dark:text-neutral-200 dark:hover:bg-white/10"
                  >
                    Refresh
                  </button>

                  {flightInfo.pdc_requires_pilot_action &&
                    !flightInfo.pdc_acknowledged_at && (
                      <>
                        <button
                          type="button"
                          onClick={() => void handleAcknowledge()}
                          disabled={isAcknowledging || isUnable}
                          className="rounded-sm bg-primary px-4 py-2 text-sm font-semibold text-white dark:text-navy transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                        >
                          {isAcknowledging
                            ? "Acknowledging..."
                            : "Acknowledge clearance"}
                        </button>

                        <button
                          type="button"
                          onClick={() => void handleUnable()}
                          disabled={isUnable || isAcknowledging}
                          className="rounded-sm border border-rose-300 dark:border-rose-700 bg-rose-50 dark:bg-rose-950/40 px-4 py-2 text-sm font-semibold text-rose-700 dark:text-rose-300 transition hover:bg-rose-100 dark:hover:bg-rose-900/40 disabled:cursor-not-allowed disabled:opacity-60"
                        >
                          {isUnable ? "Sending..." : "Unable"}
                        </button>
                      </>
                    )}
                </div>
              </div>
            )}
          </div>
        )}
    </div>
  );
}

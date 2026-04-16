import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
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
};

type PilotProfileResponse = {
  cid: string;
  online_callsign?: string;
  callsign_locked: boolean;
  live_mode: boolean;
};

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

export default function PilotFlightPage() {
  const { getAccessTokenSilently } = useAuth0();

  const storedCallsign =
    typeof window !== "undefined"
      ? (window.sessionStorage.getItem(STORAGE_KEY) ?? "")
      : "";

  const [inputCallsign, setInputCallsign] = useState(storedCallsign);
  const [activeCallsign, setActiveCallsign] = useState(storedCallsign);
  const [pilotProfile, setPilotProfile] = useState<PilotProfileResponse | null>(null);
  const [flightInfo, setFlightInfo] = useState<FlightInfo | null>(null);
  const [flightError, setFlightError] = useState<{ status: number; message: string } | null>(null);
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

  // Derived loading state: we consider ourselves loading when there's an active callsign
  // but no result yet (avoids calling setState synchronously inside an effect)
  const isLoadingFlight = !!activeCallsign && profileLoaded && !flightInfo && !flightError;

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
          const payload = (await response.json().catch(() => null)) as { error?: string } | null;
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

  // Load pilot profile once
  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const payload = (await authorizedFetch("/api/pilot/me")) as PilotProfileResponse;
        if (!active || !payload) return;
        setPilotProfile(payload);
        if (payload.live_mode) {
          if (payload.online_callsign) {
            setInputCallsign(payload.online_callsign);
            setActiveCallsign(payload.online_callsign);
            window.sessionStorage.setItem(STORAGE_KEY, payload.online_callsign);
          } else {
            // Live mode but not currently online — clear any stale test callsign
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
          error instanceof Error ? error.message : "Failed to load flight information";
        setFlightError({ status, message });
      }
    },
    [authorizedFetch],
  );

  // Fetch + poll flight info when activeCallsign changes or pdc state needs polling
  useEffect(() => {
    if (!activeCallsign || !profileLoaded) return;

    let active = true;

    // Capture whether we're already in active polling (have flight data) at effect creation.
    // Used to decide whether transient errors should kill the UI or silently retry.
    const isPollingMode = shouldPoll(flightInfo?.pdc_state ?? "");

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
          error instanceof Error ? error.message : "Failed to load flight information";
        const isTransient = status === 0 || status >= 500;
        if (!isTransient || !isPollingMode) {
          // Definitive error (4xx) or initial load failure: clear state and surface the error
          setFlightInfo(null);
          setFlightError({ status, message });
        }
        // Transient error during active polling: keep stale flight data and let the interval retry
      }
    };

    void poll();

    if (!shouldPoll(flightInfo?.pdc_state ?? "")) {
      return () => {
        active = false;
      };
    }

    const id = window.setInterval(() => void poll(), 3000);
    return () => {
      active = false;
      window.clearInterval(id);
    };
  }, [authorizedFetch, activeCallsign, flightInfo?.pdc_state, profileLoaded]);

  const handleCallsignSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalized = inputCallsign.trim().toUpperCase();
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
    if (!activeCallsign) return;
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
      setSubmitError(error instanceof Error ? error.message : "Failed to submit PDC request");
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
        error instanceof Error ? error.message : "Failed to acknowledge clearance",
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
      setUnableError(error instanceof Error ? error.message : "Failed to send unable");
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

  return (
    <div className="space-y-6">
      {/* Callsign lookup bar — testing only, hidden on live server */}
      {profileLoaded && !isLiveMode && (
        <form className="flex gap-3" onSubmit={handleCallsignSubmit}>
          <input
            value={inputCallsign}
            onChange={(e) => setInputCallsign(e.target.value.toUpperCase())}
            className="flex-1 rounded-xl border border-navy/15 dark:border-border bg-white dark:bg-background px-4 py-3 text-sm outline-none transition focus:border-primary placeholder:text-navy/40 dark:placeholder:text-muted-foreground"
            placeholder="SAS123"
            autoComplete="off"
          />
          <button
            type="submit"
            disabled={!inputCallsign.trim()}
            className="rounded-xl bg-primary px-5 py-3 text-sm font-semibold text-white dark:text-navy transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            Look up
          </button>
        </form>
      )}

      {/* Profile loading */}
      {!profileLoaded && (
        <div className="rounded-2xl border border-navy/10 dark:border-border bg-white dark:bg-card px-6 py-12 text-center text-sm text-navy/50 dark:text-muted-foreground shadow-sm">
          Loading…
        </div>
      )}

      {/* Active flight card — always shown once profile is loaded */}
      {profileLoaded && (
        <div className="rounded-2xl border border-navy/10 dark:border-border bg-white dark:bg-card p-6 shadow-sm">
          <p className="text-sm font-semibold uppercase tracking-[0.2em] text-primary">
            Active flight
          </p>

          {/* Empty state — testing mode, no callsign entered yet */}
          {!isLiveMode && !activeCallsign && (
            <p className="mt-4 py-8 text-center text-sm text-navy/50 dark:text-muted-foreground">
              Enter a callsign above to look up flight status.
            </p>
          )}

          {/* Loading */}
          {isLoadingFlight && (
            <p className="mt-4 py-8 text-center text-sm text-navy/50 dark:text-muted-foreground">
              Loading flight information…
            </p>
          )}

          {/* No active flight: 404 from API, or live mode with no online callsign */}
          {!isLoadingFlight && (flightError?.status === 404 || (isLiveMode && !activeCallsign)) && (
            <p className="mt-4 py-8 text-center text-sm text-navy/50 dark:text-muted-foreground">
              No active flight found.
            </p>
          )}

          {/* Other errors */}
          {flightError && flightError.status !== 404 && (
            <div className="mt-4 rounded-xl border border-rose-200 dark:border-rose-800 bg-rose-50 dark:bg-rose-950/40 px-4 py-3 text-sm text-rose-900 dark:text-rose-100">
              {flightError.message}
            </div>
          )}

          {/* Flight details */}
          {flightInfo && (
            <>
              <div className="mt-4 flex items-start justify-between gap-4">
                <div>
                  <p className="text-2xl font-bold tracking-tight">{flightInfo.callsign}</p>
                  <p className="mt-2 text-base font-medium text-navy/80 dark:text-foreground/80">
                    {flightInfo.origin}
                    <span className="mx-2 text-navy/40 dark:text-muted-foreground">→</span>
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
                  ✓ Strip cleared
                </p>
              )}
            </>
          )}
        </div>
      )}

      {/* PDC card — shown when available or when a PDC has been attempted (status persists after strip is cleared) */}
      {flightInfo && (flightInfo.pdc_available || pdcHasBeenAttempted(flightInfo.pdc_state)) && (
            <div className="rounded-2xl border border-navy/10 dark:border-border bg-white dark:bg-card p-6 shadow-sm">
              <p className="text-sm font-semibold uppercase tracking-[0.2em] text-primary">
                Pre-departure clearance
              </p>

              {flightInfo.pdc_can_submit ? (
                /* Submit form */
                <form className="mt-6 grid gap-4 sm:grid-cols-2" onSubmit={handlePdcSubmit}>
                  <label className="grid gap-2 text-sm font-medium">
                    Aircraft type
                    <input
                      value={aircraftType}
                      onChange={(e) => setAircraftType(e.target.value.toUpperCase())}
                      className="rounded-xl border border-navy/15 dark:border-border bg-white dark:bg-background px-4 py-3 outline-none transition focus:border-primary placeholder:text-navy/40 dark:placeholder:text-muted-foreground"
                      placeholder="A320"
                      autoComplete="off"
                      required
                    />
                  </label>
                  <label className="grid gap-2 text-sm font-medium">
                    ATIS letter
                    <input
                      value={atis}
                      onChange={(e) => setAtis(e.target.value.toUpperCase().slice(0, 1))}
                      className="rounded-xl border border-navy/15 dark:border-border bg-white dark:bg-background px-4 py-3 outline-none transition focus:border-primary placeholder:text-navy/40 dark:placeholder:text-muted-foreground"
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
                      className="rounded-xl border border-navy/15 dark:border-border bg-white dark:bg-background px-4 py-3 outline-none transition focus:border-primary placeholder:text-navy/40 dark:placeholder:text-muted-foreground"
                      placeholder="A12"
                      autoComplete="off"
                    />
                  </label>
                  <label className="sm:col-span-2 grid gap-2 text-sm font-medium">
                    Remarks for ATC
                    <textarea
                      value={remarks}
                      onChange={(e) => setRemarks(e.target.value)}
                      className="min-h-20 rounded-xl border border-navy/15 dark:border-border bg-white dark:bg-background px-4 py-3 outline-none transition focus:border-primary placeholder:text-navy/40 dark:placeholder:text-muted-foreground"
                      placeholder="Optional remarks for manual review."
                    />
                  </label>

                  {submitError && (
                    <div className="sm:col-span-2 rounded-xl border border-rose-200 dark:border-rose-800 bg-rose-50 dark:bg-rose-950/40 px-4 py-3 text-sm text-rose-900 dark:text-rose-100">
                      {submitError}
                    </div>
                  )}

                  <div className="sm:col-span-2">
                    <button
                      type="submit"
                      disabled={isSubmitting}
                      className="rounded-xl bg-primary px-5 py-3 text-sm font-semibold text-white dark:text-navy transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      {isSubmitting ? "Submitting…" : "Submit PDC request"}
                    </button>
                  </div>
                </form>
              ) : (
                /* Status panel */
                <div className="mt-6 space-y-4">
                  <div className={`rounded-xl border px-4 py-3 text-sm font-medium ${pdcStateMeta.tone}`}>
                    {pdcStateMeta.label}
                  </div>

                  {flightInfo.pdc_request_remarks && (
                    <div className="rounded-xl border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/40 px-4 py-3 text-sm text-amber-950 dark:text-amber-100">
                      <p className="font-semibold">Remarks sent to ATC</p>
                      <p className="mt-1 whitespace-pre-wrap">{flightInfo.pdc_request_remarks}</p>
                    </div>
                  )}

                  {flightInfo.pdc_clearance_text && (
                    <div className="rounded-xl border border-primary/15 dark:border-primary/30 bg-primary/5 dark:bg-primary/10 px-4 py-3">
                      <p className="text-sm font-semibold text-primary">Clearance</p>
                      <pre className="mt-3 whitespace-pre-wrap text-sm leading-6 text-navy dark:text-foreground">
                        {flightInfo.pdc_clearance_text}
                      </pre>
                    </div>
                  )}

                  {flightInfo.pdc_acknowledged_at && (
                    <p className="text-sm text-navy/70 dark:text-muted-foreground">
                      Acknowledged at {new Date(flightInfo.pdc_acknowledged_at).toLocaleString()}.
                    </p>
                  )}

                  {acknowledgeError && (
                    <div className="rounded-xl border border-rose-200 dark:border-rose-800 bg-rose-50 dark:bg-rose-950/40 px-4 py-3 text-sm text-rose-900 dark:text-rose-100">
                      {acknowledgeError}
                    </div>
                  )}

                  {unableError && (
                    <div className="rounded-xl border border-rose-200 dark:border-rose-800 bg-rose-50 dark:bg-rose-950/40 px-4 py-3 text-sm text-rose-900 dark:text-rose-100">
                      {unableError}
                    </div>
                  )}

                  <div className="flex flex-wrap gap-3">
                    <button
                      type="button"
                      onClick={() => void fetchFlight(activeCallsign)}
                      className="rounded-xl border border-navy/15 dark:border-border px-4 py-2 text-sm font-semibold text-navy dark:text-foreground transition hover:bg-navy/5 dark:hover:bg-white/10"
                    >
                      Refresh
                    </button>
                    {flightInfo.pdc_requires_pilot_action && !flightInfo.pdc_acknowledged_at && (
                      <>
                        <button
                          type="button"
                          onClick={() => void handleAcknowledge()}
                          disabled={isAcknowledging || isUnable}
                          className="rounded-xl bg-primary px-4 py-2 text-sm font-semibold text-white dark:text-navy transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                        >
                          {isAcknowledging ? "Acknowledging…" : "Acknowledge clearance"}
                        </button>
                        <button
                          type="button"
                          onClick={() => void handleUnable()}
                          disabled={isUnable || isAcknowledging}
                          className="rounded-xl border border-rose-300 dark:border-rose-700 bg-rose-50 dark:bg-rose-950/40 px-4 py-2 text-sm font-semibold text-rose-700 dark:text-rose-300 transition hover:bg-rose-100 dark:hover:bg-rose-900/40 disabled:cursor-not-allowed disabled:opacity-60"
                        >
                          {isUnable ? "Sending…" : "Unable"}
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

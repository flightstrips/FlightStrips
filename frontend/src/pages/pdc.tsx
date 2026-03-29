import { useAuth0 } from "@auth0/auth0-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { PdcClearancePanel } from "@/components/pdc/PdcClearancePanel";

const PDC_REQUEST_STORAGE_KEY = "flightstrips_pdc_request_id";

function readStoredRequestId(): number | null {
  if (typeof sessionStorage === "undefined") return null;
  try {
    const v = sessionStorage.getItem(PDC_REQUEST_STORAGE_KEY);
    if (!v) return null;
    const n = parseInt(v, 10);
    return Number.isFinite(n) && n > 0 ? n : null;
  } catch {
    return null;
  }
}

function persistRequestId(id: number) {
  if (typeof sessionStorage === "undefined") return;
  try {
    sessionStorage.setItem(PDC_REQUEST_STORAGE_KEY, String(id));
  } catch {
    /* ignore quota */
  }
}

function clearStoredRequestId() {
  if (typeof sessionStorage === "undefined") return;
  try {
    sessionStorage.removeItem(PDC_REQUEST_STORAGE_KEY);
  } catch {
    /* ignore */
  }
}

function apiUrl(path: string): string {
  const base = window.__APP_CONFIG__?.apiBaseUrl?.replace(/\/$/, "") ?? "";
  return base ? `${base}${path}` : path;
}

type PdcStatus = {
  status: string;
  clearance_text?: string | null;
  error_message?: string | null;
  pilot_acknowledged_at?: string | null;
};

export default function PdcPage() {
  const { user, getAccessTokenSilently, logout } = useAuth0();
  const audience = window.__APP_CONFIG__?.audience ?? "backend-dev";

  const [callsign, setCallsign] = useState("");
  const [atis, setAtis] = useState("");
  const [stand, setStand] = useState("");
  const [remarks, setRemarks] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [requestId, setRequestId] = useState<number | null>(readStoredRequestId);
  const [statusPayload, setStatusPayload] = useState<PdcStatus | null>(null);
  const [ackConfirm, setAckConfirm] = useState(false);
  const [ackSubmitting, setAckSubmitting] = useState(false);
  const [ackError, setAckError] = useState<string | null>(null);

  const getToken = useCallback(async () => {
    return getAccessTokenSilently({
      authorizationParams: { audience },
    });
  }, [audience, getAccessTokenSilently]);

  const pollStatus = useCallback(
    async (rid: number) => {
      const token = await getToken();
      const res = await fetch(
        apiUrl(`/api/pdc/status?request_id=${encodeURIComponent(String(rid))}`),
        { headers: { Authorization: `Bearer ${token}` } }
      );
      if (res.status === 404) {
        clearStoredRequestId();
        setRequestId(null);
        setStatusPayload(null);
        setError("Saved request is no longer available. Submit a new request.");
        return "stop" as const;
      }
      if (res.status === 410) {
        clearStoredRequestId();
        setRequestId(null);
        setStatusPayload(null);
        setError("This request has expired. Submit a new request.");
        return "stop" as const;
      }
      if (!res.ok) {
        const t = await res.text();
        setError(t || `Status ${res.status}`);
        return "stop" as const;
      }
      const data = (await res.json()) as PdcStatus;
      setStatusPayload(data);
      // Keep polling while pending or faults so we see ATC clearance when it arrives.
      if (data.status === "cleared" || data.status === "error") {
        return "stop" as const;
      }
      return "continue" as const;
    },
    [getToken]
  );

  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (requestId == null) return;
    let cancelled = false;

    const tick = async () => {
      if (cancelled) return;
      const next = await pollStatus(requestId);
      if (cancelled) return;
      if (next === "stop" && pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current);
        pollIntervalRef.current = null;
      }
    };

    void tick();
    pollIntervalRef.current = setInterval(() => void tick(), 2500);

    return () => {
      cancelled = true;
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current);
        pollIntervalRef.current = null;
      }
    };
  }, [requestId, pollStatus]);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setStatusPayload(null);
    setAckConfirm(false);
    setAckError(null);
    setSubmitting(true);
    try {
      const token = await getToken();
      const res = await fetch(apiUrl("/api/pdc/request"), {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          callsign: callsign.trim(),
          atis: atis.trim(),
          stand: stand.trim(),
          remarks: remarks.trim(),
        }),
      });
      if (!res.ok) {
        const t = await res.text();
        throw new Error(t || `Request failed (${res.status})`);
      }
      const data = (await res.json()) as { request_id: number };
      setRequestId(data.request_id);
      persistRequestId(data.request_id);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Request failed");
    } finally {
      setSubmitting(false);
    }
  }

  function startNewRequest() {
    clearStoredRequestId();
    setRequestId(null);
    setStatusPayload(null);
    setError(null);
    setAckConfirm(false);
    setAckError(null);
  }

  async function onAcknowledge() {
    if (requestId == null || !ackConfirm) return;
    setAckError(null);
    setAckSubmitting(true);
    try {
      const token = await getToken();
      const res = await fetch(apiUrl("/api/pdc/acknowledge"), {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ request_id: requestId }),
      });
      if (!res.ok) {
        const t = await res.text();
        throw new Error(t || `Failed (${res.status})`);
      }
      await pollStatus(requestId);
    } catch (err: unknown) {
      setAckError(err instanceof Error ? err.message : "Could not confirm");
    } finally {
      setAckSubmitting(false);
    }
  }

  const displayCid =
    (user as { vatsim_cid?: string } | undefined)?.vatsim_cid ??
    (user as { "vatsim/cid"?: string } | undefined)?.["vatsim/cid"];

  const isCleared = statusPayload?.status === "cleared";
  const clearanceBody =
    statusPayload?.clearance_text?.trim() ||
    (isCleared
      ? "(Clearance text unavailable — contact ATC if you need the full text.)"
      : null);
  const showAckForm =
    isCleared && !statusPayload?.pilot_acknowledged_at;
  const showAckDone = isCleared && !!statusPayload?.pilot_acknowledged_at;

  return (
    <div className="min-h-svh bg-zinc-950 text-zinc-100 flex flex-col">
      <header className="border-b border-zinc-800 px-4 sm:px-6 py-3 sm:py-4 flex flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div className="min-w-0">
          <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">
            FlightStrips
          </p>
          <h1 className="text-base sm:text-lg font-semibold tracking-tight truncate">
            Web PDC request
          </h1>
        </div>
        <div className="flex shrink-0 items-center gap-3 text-sm">
          <Link
            to="/"
            className="text-teal-400/90 hover:text-teal-300 transition-colors"
          >
            Main site
          </Link>
          <button
            type="button"
            onClick={() =>
              void logout({ logoutParams: { returnTo: window.location.origin } })
            }
            className="text-zinc-400 hover:text-zinc-200"
          >
            Log out
          </button>
        </div>
      </header>

      <main className="flex-1 w-full min-h-0 max-w-7xl mx-auto px-4 sm:px-6 py-6 sm:py-8 md:py-10 pb-10 sm:pb-12">
        <div
          className={
            requestId != null
              ? "grid grid-cols-1 md:grid-cols-2 md:gap-8 lg:gap-10 items-start"
              : "w-full max-w-lg mx-auto"
          }
        >
          <section className="min-w-0 space-y-6">
            <p className="text-zinc-400 text-sm leading-relaxed">
              Request a pre-departure clearance when your aircraft add-on does
              not support datalink. You must be logged in with VATSIM. Clearance
              is delivered here when ATC issues it.
            </p>

            {displayCid && (
              <p className="text-xs text-zinc-500 font-mono break-all">
                VATSIM CID: {displayCid}
              </p>
            )}

        <form onSubmit={(e) => void onSubmit(e)} className="space-y-5">
          <label className="block space-y-1.5">
            <span className="text-xs uppercase tracking-wider text-zinc-500">
              Callsign
            </span>
            <input
              required
              value={callsign}
              onChange={(e) => setCallsign(e.target.value.toUpperCase())}
              className="w-full rounded-md bg-zinc-900 border border-zinc-700 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-teal-600/50"
              placeholder="SAS123"
              autoComplete="off"
            />
          </label>
          <label className="block space-y-1.5">
            <span className="text-xs uppercase tracking-wider text-zinc-500">
              ATIS information letter
            </span>
            <input
              required
              maxLength={1}
              value={atis}
              onChange={(e) =>
                setAtis(e.target.value.toUpperCase().replace(/[^A-Z]/g, ""))
              }
              className="w-full rounded-md bg-zinc-900 border border-zinc-700 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-teal-600/50"
              placeholder="A"
            />
          </label>
          <label className="block space-y-1.5">
            <span className="text-xs uppercase tracking-wider text-zinc-500">
              Stand number
            </span>
            <input
              required
              value={stand}
              onChange={(e) => setStand(e.target.value.toUpperCase())}
              className="w-full rounded-md bg-zinc-900 border border-zinc-700 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-teal-600/50"
              placeholder="A12"
            />
          </label>
          <label className="block space-y-1.5">
            <span className="text-xs uppercase tracking-wider text-zinc-500">
              Remarks (optional)
            </span>
            <textarea
              value={remarks}
              onChange={(e) => setRemarks(e.target.value)}
              rows={2}
              className="w-full rounded-md bg-zinc-900 border border-zinc-700 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-teal-600/50 resize-y min-h-[72px]"
            />
          </label>
          <button
            type="submit"
            disabled={submitting}
            className="w-full rounded-md bg-teal-700 hover:bg-teal-600 disabled:opacity-50 text-white font-medium py-2.5 text-sm transition-colors"
          >
            {submitting ? "Submitting…" : "Request clearance"}
          </button>
        </form>

            {error && (
              <div
                className="rounded-md border border-red-900/60 bg-red-950/40 px-3 py-2 text-sm text-red-200 break-words"
                role="alert"
              >
                {error}
              </div>
            )}
          </section>

          {requestId != null && (
            <section className="min-w-0 w-full mt-8 md:mt-0 md:sticky md:top-6 md:self-start md:max-h-[calc(100svh-5.5rem)] md:overflow-y-auto overscroll-contain">
              <div className="space-y-4 rounded-md border border-zinc-800 bg-zinc-900/40 px-4 py-4 sm:px-5 sm:py-5 overflow-x-hidden break-words">
            <div className="flex items-start justify-between gap-3">
              <h2 className="text-xs font-medium uppercase tracking-wider text-zinc-500">
                Your request
              </h2>
              <button
                type="button"
                onClick={() => startNewRequest()}
                className="text-xs text-zinc-500 hover:text-zinc-300 shrink-0"
              >
                New request
              </button>
            </div>

            {isCleared ? (
              <div className="space-y-4">
                <PdcClearancePanel
                  clearanceText={statusPayload?.clearance_text ?? ""}
                  displayFallback={clearanceBody ?? ""}
                />

                {showAckDone && (
                  <p
                    className="text-sm text-teal-300/95"
                    role="status"
                  >
                    You have confirmed receipt of this clearance. Thank you.
                  </p>
                )}

                {showAckForm && (
                  <div className="rounded-md border border-zinc-700 bg-zinc-900/60 p-4 space-y-4">
                    <label className="flex items-start gap-3 text-sm text-zinc-300 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={ackConfirm}
                        onChange={(e) => setAckConfirm(e.target.checked)}
                        className="mt-1 rounded border-zinc-600"
                      />
                      <span>
                        I confirm I have received, read, and understood this
                        clearance.
                      </span>
                    </label>
                    {ackError && (
                      <p className="text-sm text-red-300" role="alert">
                        {ackError}
                      </p>
                    )}
                    <button
                      type="button"
                      disabled={!ackConfirm || ackSubmitting}
                      onClick={() => void onAcknowledge()}
                      className="w-full rounded-md bg-teal-800 hover:bg-teal-700 disabled:opacity-40 text-white text-sm font-medium py-2.5"
                    >
                      {ackSubmitting ? "Confirming…" : "Confirm receipt"}
                    </button>
                  </div>
                )}
              </div>
            ) : statusPayload?.status === "faults" ? (
              <div className="space-y-3 text-sm text-zinc-200 leading-relaxed">
                <p>
                  Request received (reference{" "}
                  <span className="font-mono text-zinc-300">#{requestId}</span>
                  ).
                </p>
                <p className="text-zinc-400">
                  Stay on your assigned frequency or contact delivery as
                  instructed. Air traffic control will coordinate your clearance.
                </p>
              </div>
            ) : statusPayload?.status === "error" ? (
              <p className="text-sm text-zinc-400 leading-relaxed">
                Something went wrong processing this request. You may submit a
                new request or contact ATC by voice.
              </p>
            ) : (
              <p className="text-sm text-zinc-200 leading-relaxed">
                Request received (reference{" "}
                <span className="font-mono text-zinc-300">#{requestId}</span>
                ). Awaiting clearance from ATC. This page will update when your
                clearance is available.
              </p>
            )}
              </div>
            </section>
          )}
        </div>
      </main>
    </div>
  );
}

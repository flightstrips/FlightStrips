import { useAuth0 } from "@auth0/auth0-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { getApiUrl } from "@/lib/api-url";
import { normalizeCdmTime } from "@/lib/cdmTime";

const POLL_INTERVAL_MS = 10_000;

type SequenceReason = {
  kind: string;
  message: string;
  against_callsign?: string;
};

type SequenceRow = {
  position?: number;
  callsign: string;
  origin: string;
  destination: string;
  runway: string;
  sid: string;
  wake_category: string;
  state: string;
  eobt: string;
  tobt: string;
  req_tobt: string;
  tobt_confirmed: boolean;
  tobt_confirmed_by: string;
  tsat: string;
  ttot: string;
  natural_ttot: string;
  taxi_minutes?: number;
  taxi_runway: string;
  ctot: string;
  base_time: string;
  base_source: string;
  phase: string;
  invalid_reason: string;
  reasons: SequenceReason[];
};

type SequenceSession = {
  session_id: number;
  name: string;
  airport: string;
  cdm_master: boolean;
  departure_runways: string[];
  arrival_runways: string[];
  rows: SequenceRow[];
};

type SequenceResponse = {
  generated_at: string;
  sessions: SequenceSession[];
};

function formatTime(value: string): string {
  const normalized = normalizeCdmTime(value);
  if (!normalized) {
    return "—";
  }

  return `${normalized.slice(0, 2)}:${normalized.slice(2, 4)}`;
}

function formatBaseSource(value: string): string {
  switch (value) {
    case "TOBT":
      return "TOBT";
    case "REQ_TOBT":
      return "REQ TOBT";
    case "EOBT":
      return "EOBT";
    default:
      return value || "—";
  }
}

function formatGeneratedAt(value: string): string {
  if (!value) {
    return "Unknown";
  }

  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }

  return parsed.toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function formatConfirmation(row: SequenceRow): string {
  if (!row.tobt_confirmed) {
    return "No";
  }

  return row.tobt_confirmed_by ? `Yes (${row.tobt_confirmed_by})` : "Yes";
}

function formatOriginalTtot(row: SequenceRow): string {
  const tobt = normalizeCdmTime(row.tobt);
  if (!tobt || typeof row.taxi_minutes !== "number" || row.taxi_minutes < 0) {
    return "—";
  }

  const hours = Number.parseInt(tobt.slice(0, 2), 10);
  const minutes = Number.parseInt(tobt.slice(2, 4), 10);
  if (Number.isNaN(hours) || Number.isNaN(minutes)) {
    return "—";
  }

  const totalMinutes = (hours * 60) + minutes + row.taxi_minutes;
  const wrappedMinutes = ((totalMinutes % (24 * 60)) + (24 * 60)) % (24 * 60);
  const displayHours = Math.floor(wrappedMinutes / 60);
  const displayMinutes = wrappedMinutes % 60;
  return `${displayHours.toString().padStart(2, "0")}:${displayMinutes.toString().padStart(2, "0")}`;
}

function rowTone(row: SequenceRow): string {
  if (row.phase === "I") {
    return "bg-red-50/80 dark:bg-red-950/20";
  }

  if (!row.position) {
    return "bg-amber-50/70 dark:bg-amber-950/20";
  }

  return "";
}

export default function CdmPage() {
  const { getAccessTokenSilently } = useAuth0();
  const [data, setData] = useState<SequenceResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const authorizedFetch = useCallback(async (path: string) => {
    const token = await getAccessTokenSilently();
    const response = await fetch(getApiUrl(path), {
      headers: {
        Authorization: `Bearer ${token}`,
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

      throw new Error(message);
    }

    return (await response.json()) as SequenceResponse;
  }, [getAccessTokenSilently]);

  useEffect(() => {
    let active = true;

    const poll = async () => {
      try {
        const payload = await authorizedFetch("/api/cdm/sequence");
        if (!active) {
          return;
        }
        setData(payload);
        setError(null);
      } catch (fetchError) {
        if (!active) {
          return;
        }
        const message =
          fetchError instanceof Error ? fetchError.message : "Failed to load CDM sequence.";
        setError(message);
      } finally {
        if (active) {
          setLoading(false);
        }
      }
    };

    void poll();
    const intervalId = window.setInterval(() => void poll(), POLL_INTERVAL_MS);

    return () => {
      active = false;
      window.clearInterval(intervalId);
    };
  }, [authorizedFetch]);

  const totalAircraft = useMemo(() => {
    return data?.sessions.reduce((sum, session) => sum + session.rows.length, 0) ?? 0;
  }, [data]);

  if (loading && !data) {
    return (
      <div className="p-6 md:p-8">
        <div className="rounded-xl border bg-card p-6 text-sm text-muted-foreground">
          Loading CDM sequence...
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen overflow-y-auto bg-background p-6 text-foreground md:p-8">
      <div className="mx-auto max-w-[1800px] space-y-6">
      <header className="space-y-2">
        <h1 className="text-3xl font-semibold tracking-tight">CDM Sequence</h1>
        <p className="text-sm text-muted-foreground">
          {totalAircraft} aircraft across {data?.sessions.length ?? 0} sessions. Last updated{" "}
          {formatGeneratedAt(data?.generated_at ?? "")}.
        </p>
      </header>

      {error ? (
        <div className="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-900 dark:border-red-900 dark:bg-red-950/30 dark:text-red-100">
          {error}
        </div>
      ) : null}

      {data && data.sessions.length === 0 ? (
        <div className="rounded-xl border bg-card p-6 text-sm text-muted-foreground">
          No sequenced aircraft are currently available.
        </div>
      ) : null}

      {data?.sessions.map((session) => (
        <section key={session.session_id} className="rounded-2xl border bg-card shadow-sm">
          <div className="border-b px-6 py-4">
            <div className="flex flex-col gap-2 md:flex-row md:items-baseline md:justify-between">
              <div>
                <h2 className="text-xl font-semibold">
                  {session.airport} - {session.name}
                </h2>
                <p className="text-sm text-muted-foreground">
                  Departure runways {session.departure_runways.join(", ") || "—"} | Arrival runways{" "}
                  {session.arrival_runways.join(", ") || "—"} | CDM{" "}
                  {session.cdm_master ? "Master" : "Slave"}
                </p>
              </div>
              <div className="text-sm text-muted-foreground">
                {session.rows.length} aircraft
              </div>
            </div>
          </div>

          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead className="bg-muted/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-4 py-3 font-medium">Pos</th>
                  <th className="px-4 py-3 font-medium">Callsign</th>
                  <th className="px-4 py-3 font-medium">Route</th>
                  <th className="px-4 py-3 font-medium">TOBT</th>
                  <th className="px-4 py-3 font-medium">Confirmed</th>
                  <th className="px-4 py-3 font-medium">TSAT</th>
                  <th className="px-4 py-3 font-medium">TTOT</th>
                  <th className="px-4 py-3 font-medium">Original TTOT</th>
                  <th className="px-4 py-3 font-medium">Taxi</th>
                  <th className="px-4 py-3 font-medium">CTOT</th>
                  <th className="px-4 py-3 font-medium">Base</th>
                  <th className="px-4 py-3 font-medium">Reason</th>
                </tr>
              </thead>
              <tbody>
                {session.rows.map((row) => (
                  <tr key={row.callsign} className={`border-t align-top ${rowTone(row)}`}>
                    <td className="px-4 py-3 font-medium">{row.position ?? "—"}</td>
                    <td className="px-4 py-3">
                      <div className="font-medium">{row.callsign}</div>
                      <div className="text-xs text-muted-foreground">
                        {row.runway || "—"} / {row.sid || "—"} / {row.wake_category || "—"}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div>{row.origin} - {row.destination}</div>
                      <div className="text-xs text-muted-foreground">
                        State {row.state || "—"}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div>{formatTime(row.tobt)}</div>
                      <div className="text-xs text-muted-foreground">
                        Req {formatTime(row.req_tobt)} / EOBT {formatTime(row.eobt)}
                      </div>
                    </td>
                    <td className="px-4 py-3">{formatConfirmation(row)}</td>
                    <td className="px-4 py-3">{formatTime(row.tsat)}</td>
                    <td className="px-4 py-3">
                      <div>{formatTime(row.ttot)}</div>
                      <div className="text-xs text-muted-foreground">
                        Stored TTOT
                      </div>
                    </td>
                    <td className="px-4 py-3">{formatOriginalTtot(row)}</td>
                    <td className="px-4 py-3">
                      <div>{typeof row.taxi_minutes === "number" ? `${row.taxi_minutes} min` : "—"}</div>
                      <div className="text-xs text-muted-foreground">
                        {row.taxi_runway || row.runway || "—"}
                      </div>
                    </td>
                    <td className="px-4 py-3">{formatTime(row.ctot)}</td>
                    <td className="px-4 py-3">
                      <div>{formatBaseSource(row.base_source)}</div>
                      <div className="text-xs text-muted-foreground">
                        {formatTime(row.base_time)}
                        {row.invalid_reason ? ` / ${row.invalid_reason}` : ""}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      {row.reasons.length > 0 ? (
                        <ul className="space-y-1 text-sm">
                          {row.reasons.map((reason, index) => (
                            <li key={`${reason.kind}-${index}`} className="text-foreground">
                              {reason.message}
                            </li>
                          ))}
                        </ul>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      ))}
      </div>
    </div>
  );
}

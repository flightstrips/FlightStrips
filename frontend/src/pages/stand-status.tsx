import { useAuth0 } from "@auth0/auth0-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { getApiUrl } from "@/lib/api-url";

const POLL_INTERVAL_MS = 10_000;

type StandSystem = {
  enabled: boolean;
  ready: boolean;
  status: string;
  reason?: string;
};

type StandConfiguration = {
  aircraft_types: number;
  stands: number;
  stand_variants: number;
  airline_rules: number;
  stand_groups: number;
  fallback_rules: number;
};

type StandFeed = {
  status: string;
  snapshot_at?: string;
  last_error?: string;
  flights: number;
  online: number;
  prefiles: number;
};

type StandAssignment = {
  id: number;
  callsign: string;
  stand: string;
  direction: string;
  stage: string;
  source: string;
  rule_id?: string;
  tier?: number;
  matched_variant?: string;
  conflict_reason?: string;
  eta?: string;
  eta_source?: string;
  assigned_at?: string;
  expires_at?: string;
  manual: boolean;
  acknowledged: boolean;
  acknowledged_at?: string;
  acknowledged_by?: string;
  vatsim_cid?: number;
  vatsim_revision?: number;
  version: number;
  created_at: string;
  updated_at: string;
};

type StandBlock = {
  id: number;
  stand: string;
  block_type: string;
  source: string;
  reason?: string;
  callsign?: string;
  created_by?: string;
  expires_at?: string;
  manual: boolean;
  version: number;
  created_at: string;
  updated_at: string;
};

type StandAllocationFailure = {
  id: number;
  occurred_at: string;
  session_id: number;
  airport: string;
  callsign: string;
  command: string;
  outcome: string;
  reason: string;
  direction: string;
  stage: string;
  attempted_stand?: string;
  aircraft_type?: string;
  engine_type?: string;
  wtc?: string;
  border_status?: string;
  attempts: number;
};

type StandSession = {
  session_id: number;
  name: string;
  airport: string;
  assignments: StandAssignment[];
  blocks: StandBlock[];
};

type StandStatusResponse = {
  generated_at: string;
  system: StandSystem;
  configuration: StandConfiguration;
  feed: StandFeed;
  failures: StandAllocationFailure[];
  sessions: StandSession[];
};

function formatStatus(value: string): string {
  return value.replace(/_/g, " ").replace(/\b\w/g, (letter: string) => letter.toUpperCase());
}

function statusTone(status: string): string {
  if (status === "ready") {
    return "border-emerald-300 bg-emerald-50 text-emerald-950 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-100";
  }
  if (status === "disabled") {
    return "border-slate-300 bg-slate-50 text-slate-900 dark:border-slate-700 dark:bg-slate-900/50 dark:text-slate-100";
  }
  return "border-amber-300 bg-amber-50 text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-100";
}

function formatTimestamp(value?: string): string {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    day: "2-digit",
    month: "short",
  });
}

function formatAge(timestamp?: string): string {
  if (!timestamp) {
    return "—";
  }
  const timestampMs = new Date(timestamp).getTime();
  if (Number.isNaN(timestampMs)) {
    return "—";
  }
  const seconds = Math.max(0, (Date.now() - timestampMs) / 1000);
  if (seconds < 60) {
    return `${Math.round(seconds)} sec`;
  }
  return `${Math.floor(seconds / 60)} min ${Math.round(seconds % 60)} sec`;
}

function configurationEntries(configuration: StandConfiguration) {
  return [
    ["Aircraft types", configuration.aircraft_types],
    ["Physical stands", configuration.stands],
    ["Stand variants", configuration.stand_variants],
    ["Airline rules", configuration.airline_rules],
    ["Stand groups", configuration.stand_groups],
    ["Fallback rules", configuration.fallback_rules],
  ] as const;
}

export default function StandStatusPage() {
  const { getAccessTokenSilently } = useAuth0();
  const [data, setData] = useState<StandStatusResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const authorizedFetch = useCallback(async () => {
    const token = await getAccessTokenSilently();
    const response = await fetch(getApiUrl("/api/stand/status"), {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!response.ok) {
      const payload = (await response.json().catch(() => null)) as { error?: string } | null;
      throw new Error(payload?.error ?? `Request failed (${response.status} ${response.statusText})`);
    }
    return (await response.json()) as StandStatusResponse;
  }, [getAccessTokenSilently]);

  useEffect(() => {
    let active = true;
    const poll = async () => {
      try {
        const payload = await authorizedFetch();
        if (!active) {
          return;
        }
        setData(payload);
        setError(null);
      } catch (fetchError) {
        if (active) {
          setError(fetchError instanceof Error ? fetchError.message : "Failed to load stand system status.");
        }
      } finally {
        if (active) {
          setLoading(false);
        }
      }
    };

    void poll();
    const interval = window.setInterval(() => void poll(), POLL_INTERVAL_MS);
    return () => {
      active = false;
      window.clearInterval(interval);
    };
  }, [authorizedFetch]);

  const totals = useMemo(() => {
    return (data?.sessions ?? []).reduce(
      (total, session) => ({
        assignments: total.assignments + session.assignments.length,
        blocks: total.blocks + session.blocks.length,
      }),
      { assignments: 0, blocks: 0 },
    );
  }, [data]);

  if (loading && !data) {
    return (
      <div className="p-6 md:p-8">
        <div className="rounded-xl border bg-card p-6 text-sm text-muted-foreground">
          Loading stand system status...
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen overflow-y-auto bg-background p-6 text-foreground md:p-8">
      <div className="mx-auto max-w-[1800px] space-y-6">
        <header className="space-y-2">
          <h1 className="text-3xl font-semibold tracking-tight">Stand System Status</h1>
          <p className="text-sm text-muted-foreground">
            {totals.assignments} assignments and {totals.blocks} active blocks across{" "}
            {data?.sessions.length ?? 0} sessions, with {data?.failures.length ?? 0} recent failures.
            Last updated {formatTimestamp(data?.generated_at)}.
          </p>
        </header>

        {error ? (
          <div className="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-900 dark:border-red-900 dark:bg-red-950/30 dark:text-red-100">
            {error}
          </div>
        ) : null}

        {data ? (
          <>
            <div className="grid gap-4 lg:grid-cols-3">
              <section className={`rounded-xl border p-5 ${statusTone(data.system.status)}`}>
                <div className="text-xs font-medium uppercase tracking-wide opacity-70">System</div>
                <div className="mt-1 text-2xl font-semibold">{formatStatus(data.system.status)}</div>
                <div className="mt-2 text-sm">
                  Feature {data.system.enabled ? "enabled" : "disabled"} · Runtime{" "}
                  {data.system.ready ? "ready" : "not ready"}
                </div>
                {data.system.reason ? <div className="mt-2 text-sm">{data.system.reason}</div> : null}
              </section>

              <section className={`rounded-xl border p-5 ${statusTone(data.feed.status)}`}>
                <div className="text-xs font-medium uppercase tracking-wide opacity-70">VATSIM feed</div>
                <div className="mt-1 text-2xl font-semibold">{formatStatus(data.feed.status)}</div>
                <div className="mt-2 text-sm">
                  {data.feed.online} online · {data.feed.prefiles} prefiles · {data.feed.flights} total
                </div>
                <div className="mt-1 text-sm">
                  Snapshot {formatTimestamp(data.feed.snapshot_at)} ({formatAge(data.feed.snapshot_at)} old)
                </div>
                {data.feed.last_error ? <div className="mt-2 text-sm">{data.feed.last_error}</div> : null}
              </section>

              <section className="rounded-xl border bg-card p-5">
                <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Runtime state</div>
                <div className="mt-1 text-2xl font-semibold">{totals.assignments} assignments</div>
                <div className="mt-2 text-sm text-muted-foreground">
                  {totals.blocks} blocks · {data.failures.length} recent failures · {data.sessions.length} sessions
                </div>
              </section>
            </div>

            <section className="rounded-2xl border bg-card shadow-sm">
              <div className="border-b px-6 py-4">
                <h2 className="text-xl font-semibold">Loaded configuration</h2>
                <p className="text-sm text-muted-foreground">Counts from the validated SAT startup configuration.</p>
              </div>
              <div className="grid grid-cols-2 gap-px bg-border sm:grid-cols-3 lg:grid-cols-6">
                {configurationEntries(data.configuration).map(([label, value]) => (
                  <div key={label} className="bg-card px-5 py-4">
                    <div className="text-2xl font-semibold">{value.toLocaleString()}</div>
                    <div className="text-xs text-muted-foreground">{label}</div>
                  </div>
                ))}
              </div>
            </section>

            <section className="rounded-2xl border bg-card shadow-sm">
              <div className="border-b px-6 py-4">
                <div className="flex flex-col gap-2 md:flex-row md:items-baseline md:justify-between">
                  <div>
                    <h2 className="text-xl font-semibold">Recent assignment failures</h2>
                    <p className="text-sm text-muted-foreground">
                      The newest failed allocation and reallocation attempts retained by this server.
                    </p>
                  </div>
                  <div className="text-sm text-muted-foreground">
                    {data.failures.length} failures · maximum 100 since startup
                  </div>
                </div>
              </div>
              <div className="overflow-x-auto">
                <table className="min-w-full text-sm">
                  <thead className="bg-muted/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
                    <tr>
                      <th className="px-4 py-3 font-medium">Time</th>
                      <th className="px-4 py-3 font-medium">Flight</th>
                      <th className="px-4 py-3 font-medium">Request</th>
                      <th className="px-4 py-3 font-medium">Failure</th>
                      <th className="px-4 py-3 font-medium">Flight facts</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.failures.length === 0 ? (
                      <tr className="border-t">
                        <td colSpan={5} className="px-4 py-6 text-center text-muted-foreground">
                          No stand assignment failures have been recorded since this server started.
                        </td>
                      </tr>
                    ) : null}
                    {data.failures.map((failure) => (
                      <tr key={failure.id} className="border-t bg-amber-50/50 align-top dark:bg-amber-950/10">
                        <td className="whitespace-nowrap px-4 py-3">{formatTimestamp(failure.occurred_at)}</td>
                        <td className="px-4 py-3">
                          <div className="font-semibold">{failure.callsign || "—"}</div>
                          <div className="text-xs text-muted-foreground">
                            {failure.airport || "—"} · session {failure.session_id || "—"}
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <div>{formatStatus(failure.command)}</div>
                          <div className="text-xs text-muted-foreground">
                            {failure.direction || "—"} · {failure.stage || "—"}
                          </div>
                          <div className="text-xs text-muted-foreground">
                            Stand {failure.attempted_stand || "automatic"} · {failure.attempts} attempt
                            {failure.attempts === 1 ? "" : "s"}
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <div className="font-medium text-amber-800 dark:text-amber-200">
                            {formatStatus(failure.outcome)}
                          </div>
                          <div className="mt-1 max-w-xl text-xs">{failure.reason}</div>
                        </td>
                        <td className="px-4 py-3">
                          <div>{failure.aircraft_type || "Unknown aircraft"}</div>
                          <div className="text-xs text-muted-foreground">
                            Engine {failure.engine_type || "—"} · WTC {failure.wtc || "—"} · Border{" "}
                            {failure.border_status || "—"}
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>

            {data.sessions.length === 0 ? (
              <div className="rounded-xl border bg-card p-6 text-sm text-muted-foreground">
                No stand-system session state is currently available.
              </div>
            ) : null}

            {data.sessions.map((session) => (
              <section key={session.session_id} className="rounded-2xl border bg-card shadow-sm">
                <div className="border-b px-6 py-4">
                  <div className="flex flex-col gap-2 md:flex-row md:items-baseline md:justify-between">
                    <div>
                      <h2 className="text-xl font-semibold">{session.airport} - {session.name}</h2>
                      <p className="text-sm text-muted-foreground">Session {session.session_id}</p>
                    </div>
                    <div className="text-sm text-muted-foreground">
                      {session.assignments.length} assignments · {session.blocks.length} blocks
                    </div>
                  </div>
                </div>

                <div className="overflow-x-auto">
                  <table className="min-w-full text-sm">
                    <thead className="bg-muted/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
                      <tr>
                        <th className="px-4 py-3 font-medium">Flight</th>
                        <th className="px-4 py-3 font-medium">Assignment</th>
                        <th className="px-4 py-3 font-medium">Decision</th>
                        <th className="px-4 py-3 font-medium">Timing</th>
                        <th className="px-4 py-3 font-medium">Acknowledgement</th>
                        <th className="px-4 py-3 font-medium">VATSIM / Version</th>
                      </tr>
                    </thead>
                    <tbody>
                      {session.assignments.length === 0 ? (
                        <tr className="border-t">
                          <td colSpan={6} className="px-4 py-6 text-center text-muted-foreground">
                            No active stand assignments.
                          </td>
                        </tr>
                      ) : null}
                      {session.assignments.map((assignment) => (
                        <tr key={assignment.id} className="border-t align-top">
                          <td className="px-4 py-3">
                            <div className="font-semibold">{assignment.callsign}</div>
                            <div className="text-xs text-muted-foreground">{assignment.direction || "—"}</div>
                          </td>
                          <td className="px-4 py-3">
                            <div className="font-semibold">{assignment.stand || "—"}</div>
                            <div className="text-xs text-muted-foreground">
                              {assignment.stage || "—"} · {assignment.source || "—"}
                              {assignment.manual ? " · manual" : ""}
                            </div>
                          </td>
                          <td className="px-4 py-3">
                            <div>{assignment.rule_id || "No policy rule"}</div>
                            <div className="text-xs text-muted-foreground">
                              Tier {assignment.tier ?? "—"} · Variant {assignment.matched_variant || "—"}
                            </div>
                            {assignment.conflict_reason ? (
                              <div className="mt-1 text-xs text-amber-700 dark:text-amber-300">
                                Override: {assignment.conflict_reason}
                              </div>
                            ) : null}
                          </td>
                          <td className="px-4 py-3">
                            <div>ETA {formatTimestamp(assignment.eta)}</div>
                            <div className="text-xs text-muted-foreground">
                              {assignment.eta_source || "No ETA source"} · expires {formatTimestamp(assignment.expires_at)}
                            </div>
                            <div className="text-xs text-muted-foreground">
                              Assigned {formatTimestamp(assignment.assigned_at)}
                            </div>
                          </td>
                          <td className="px-4 py-3">
                            <div>{assignment.acknowledged ? "Acknowledged" : "Pending"}</div>
                            <div className="text-xs text-muted-foreground">
                              {assignment.acknowledged_by || "—"} · {formatTimestamp(assignment.acknowledged_at)}
                            </div>
                          </td>
                          <td className="px-4 py-3">
                            <div>CID {assignment.vatsim_cid ?? "—"}</div>
                            <div className="text-xs text-muted-foreground">
                              Revision {assignment.vatsim_revision ?? "—"} · record v{assignment.version}
                            </div>
                            <div className="text-xs text-muted-foreground">
                              Updated {formatTimestamp(assignment.updated_at)}
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>

                {session.blocks.length > 0 ? (
                  <div className="border-t">
                    <div className="px-6 py-3 text-sm font-semibold">Active stand blocks</div>
                    <div className="overflow-x-auto">
                      <table className="min-w-full text-sm">
                        <thead className="bg-muted/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
                          <tr>
                            <th className="px-4 py-3 font-medium">Stand</th>
                            <th className="px-4 py-3 font-medium">Type</th>
                            <th className="px-4 py-3 font-medium">Source</th>
                            <th className="px-4 py-3 font-medium">Reason / Flight</th>
                            <th className="px-4 py-3 font-medium">Expiry</th>
                            <th className="px-4 py-3 font-medium">Version</th>
                          </tr>
                        </thead>
                        <tbody>
                          {session.blocks.map((block) => (
                            <tr key={block.id} className="border-t">
                              <td className="px-4 py-3 font-semibold">{block.stand}</td>
                              <td className="px-4 py-3">{block.block_type || "—"}</td>
                              <td className="px-4 py-3">
                                {block.source || "—"}{block.manual ? " · manual" : ""}
                              </td>
                              <td className="px-4 py-3">
                                <div>{block.reason || "—"}</div>
                                <div className="text-xs text-muted-foreground">
                                  {block.callsign || "No callsign"} · {block.created_by || "system"}
                                </div>
                              </td>
                              <td className="px-4 py-3">{formatTimestamp(block.expires_at)}</td>
                              <td className="px-4 py-3">
                                v{block.version}
                                <div className="text-xs text-muted-foreground">
                                  Updated {formatTimestamp(block.updated_at)}
                                </div>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                ) : null}
              </section>
            ))}
          </>
        ) : null}
      </div>
    </div>
  );
}

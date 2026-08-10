import { useAuth0 } from "@auth0/auth0-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { getApiUrl } from "@/lib/api-url";
import { assignmentTimelineTiming, latestTimelineDate, timelineRangeEnd } from "./stand-status-timeline";

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
  departure_tobt?: string;
  departure_tsat?: string;
  planned_release_at?: string;
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

type StandSelectionCandidate = {
  stand: string;
  rule_id: string;
  tier: number;
  tier_name: string;
  original_weight: number;
  normalized_weight: number;
  fallback_used: boolean;
  selectable: boolean;
};

type StandAllocationPreview = {
  callsign: string;
  airport: string;
  fallback_used: boolean;
  compatible_stands: number;
  available_stands: number;
  selection: {
    rule_id: string;
    fallback_used: boolean;
    candidates: StandSelectionCandidate[];
  };
};

type SelectedFlight = {
  sessionId: number;
  airport: string;
  callsign: string;
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

function formatPercentage(value: number): string {
  const percentage = value * 100;
  return `${percentage.toFixed(percentage >= 10 ? 0 : 1)}%`;
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

type StandTimelineItem = {
  id: string;
  stand: string;
  label: string;
  detail: string;
  secondary: string;
  start: Date;
  end?: Date;
  plannedEnd?: Date;
  active: boolean;
  tone: string;
};

const comfortableTimelineLayout = {
  width: "min-w-[1280px]",
  height: "max-h-[34rem]",
  rowHeight: 76,
  laneHeight: 68,
};

type StandSelectionTier = {
  ruleID: string;
  tier: number;
  tierName: string;
  selectable: boolean;
  candidates: StandSelectionCandidate[];
};

function groupPreviewCandidates(candidates: StandSelectionCandidate[]): StandSelectionTier[] {
  const groups = new Map<string, StandSelectionTier>();
  for (const candidate of candidates) {
    const key = `${candidate.rule_id}:${candidate.tier}:${candidate.tier_name}`;
    const group = groups.get(key) ?? {
      ruleID: candidate.rule_id,
      tier: candidate.tier,
      tierName: candidate.tier_name,
      selectable: candidate.selectable,
      candidates: [],
    };
    group.selectable ||= candidate.selectable;
    group.candidates.push(candidate);
    groups.set(key, group);
  }
  return [...groups.values()].sort((left, right) => left.tier - right.tier || left.ruleID.localeCompare(right.ruleID));
}

function clockValue(value?: string): string {
  if (!value) return "—";
  const digits = value.replace(/\D/g, "");
  return digits.length === 4 ? `${digits.slice(0, 2)}:${digits.slice(2)}` : value;
}

function timelineDate(value?: string): Date | undefined {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

function entryIsActive(expiresAt: string | undefined, now: Date): boolean {
  const expiry = timelineDate(expiresAt);
  return !expiry || expiry > now;
}

function activeAssignments(session: StandSession, now: Date): StandAssignment[] {
  return session.assignments.filter((assignment) => entryIsActive(assignment.expires_at, now));
}

function activeBlocks(session: StandSession, now: Date): StandBlock[] {
  return session.blocks.filter((block) => entryIsActive(block.expires_at, now));
}

function buildTimelineItems(session: StandSession, now: Date): StandTimelineItem[] {
  const assignments = session.assignments.flatMap((assignment) => {
    // An arrival is a future reservation until it reaches its ETA. Displaying
    // it from the assignment timestamp falsely makes its stand look occupied
    // hours before the aircraft is due to arrive.
    const start = assignment.direction === "ARRIVAL"
      ? timelineDate(assignment.eta) ?? timelineDate(assignment.assigned_at) ?? timelineDate(assignment.created_at)
      : timelineDate(assignment.assigned_at) ?? timelineDate(assignment.created_at);
    if (!start || !assignment.stand) return [];
    const timelineTiming = assignmentTimelineTiming(assignment);
    const displayEnd = timelineTiming.end;
    const active = entryIsActive(assignment.expires_at, now);
    const timing = assignment.direction === "DEPARTURE"
      ? `TOBT ${clockValue(assignment.departure_tobt)} · TSAT ${clockValue(assignment.departure_tsat)}`
      : `ETA ${formatTimestamp(assignment.eta)}`;
    return [{
      id: `assignment-${assignment.id}`,
      stand: assignment.stand.toUpperCase(),
      label: assignment.callsign,
      detail: `${formatStatus(assignment.direction)} · ${formatStatus(assignment.stage)}${assignment.rule_id ? ` · ${assignment.rule_id}` : ""}`,
      secondary: `${timing} · ${displayEnd
        ? `ends ${formatTimestamp(displayEnd.toISOString())}`
        : timelineTiming.plannedRelease
          ? `planned release ${formatTimestamp(timelineTiming.plannedRelease.toISOString())}`
          : "open-ended"}`,
      start,
      end: displayEnd,
      plannedEnd: timelineTiming.plannedRelease,
      active,
      tone: !active
        ? "border-slate-400/70 bg-slate-500/70 text-slate-50 dark:border-slate-500 dark:bg-slate-600/70"
        : assignment.direction === "ARRIVAL"
          ? "border-sky-600/60 bg-sky-600/85 text-white dark:border-sky-300/60 dark:bg-sky-400/80 dark:text-slate-950"
          : "border-emerald-700/60 bg-emerald-600/85 text-white dark:border-emerald-300/60 dark:bg-emerald-400/80 dark:text-slate-950",
    } satisfies StandTimelineItem];
  });
  const blocks = session.blocks.flatMap((block) => {
    const start = timelineDate(block.created_at);
    if (!start || !block.stand) return [];
    const end = timelineDate(block.expires_at);
    const active = entryIsActive(block.expires_at, now);
    return [{
      id: `block-${block.id}`,
      stand: block.stand.toUpperCase(),
      label: block.callsign || formatStatus(block.block_type || "block"),
      detail: `${formatStatus(block.block_type || "block")} · ${formatStatus(block.source || "system")}${block.manual ? " · manual" : ""}`,
      secondary: `${block.reason || "No reason supplied"}${block.callsign ? ` · ${block.callsign}` : ""} · ${end ? `ends ${formatTimestamp(end.toISOString())}` : "open-ended"}`,
      start,
      end,
      active,
      tone: !active
        ? "border-violet-400/70 bg-violet-500/70 text-white dark:border-violet-300/60 dark:bg-violet-400/70 dark:text-slate-950"
        : "border-amber-600/60 bg-amber-500/90 text-amber-950 dark:border-amber-300/60 dark:bg-amber-400/80 dark:text-slate-950",
    } satisfies StandTimelineItem];
  });
  return [...assignments, ...blocks].sort((left, right) => left.stand.localeCompare(right.stand) || left.start.getTime() - right.start.getTime());
}

function StandAllocationTimeline({ session, selectedStand, onSelectStand }: {
  session: StandSession;
  selectedStand: string;
  onSelectStand: (stand: string) => void;
}) {
  const layout = comfortableTimelineLayout;
  const now = new Date();
  const items = buildTimelineItems(session, now);
  const requestedStand = selectedStand.trim().toUpperCase();
  const stands = [...new Set(items.map((item) => item.stand))];
  const visibleStands = requestedStand ? [requestedStand] : stands;
  const earliest = items.reduce<Date | undefined>((value, item) => !value || item.start < value ? item.start : value, undefined);
  const latest = latestTimelineDate(items, now);
  const rangeStart = new Date(Math.min(earliest?.getTime() ?? now.getTime(), now.getTime() - 60 * 60 * 1000));
  const rangeEnd = timelineRangeEnd(latest, now);
  const rangeMs = Math.max(1, rangeEnd.getTime() - rangeStart.getTime());
  const offset = (date: Date) => Math.max(0, Math.min(100, ((date.getTime() - rangeStart.getTime()) / rangeMs) * 100));

  return (
    <div className="border-t px-6 py-5">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h3 className="text-base font-semibold">Stand occupancy timeline</h3>
          <p className="text-sm text-muted-foreground">Active cards are coloured by type; violet/slate cards are expired history. Open-ended cards remain active.</p>
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Find stand
            <input
              value={selectedStand}
              onChange={(event) => onSelectStand(event.target.value.toUpperCase())}
              placeholder="e.g. A31"
              className="h-9 w-40 rounded-md border bg-background px-3 text-sm font-normal normal-case text-foreground outline-none focus:ring-2 focus:ring-ring"
            />
          </label>
        </div>
      </div>
      <div className="mt-4 overflow-x-auto rounded-lg border">
        <div className={layout.width}>
          <div className="grid grid-cols-[7rem_1fr] border-b bg-muted/40 text-xs text-muted-foreground">
            <div className="px-3 py-2 font-medium uppercase tracking-wide">Stand</div>
            <div className="flex justify-between px-3 py-2">
              <span>{formatTimestamp(rangeStart.toISOString())}</span>
              <span>{formatTimestamp(rangeEnd.toISOString())}</span>
            </div>
          </div>
          <div className={`${layout.height} overflow-y-auto`}>
            {visibleStands.length === 0 ? (
              <div className="px-4 py-6 text-sm text-muted-foreground">No stands have active assignments or blocks.</div>
            ) : visibleStands.map((stand) => {
              const standItems = items.filter((item) => item.stand === stand);
              const rowHeight = Math.max(layout.rowHeight, standItems.length * layout.laneHeight + 12);
              return (
                <div key={stand} className="grid grid-cols-[7rem_1fr] border-b last:border-b-0">
                  <div className="flex items-center px-3 py-3 font-semibold">{stand}</div>
                  <div className="relative overflow-hidden bg-background" style={{ minHeight: `${rowHeight}px` }}>
                    <div className="absolute inset-y-0 z-10 border-l-2 border-red-500/90" style={{ left: `${offset(now)}%` }} />
                    {standItems.length === 0 ? <div className="px-3 py-4 text-xs text-muted-foreground">No current allocation or block.</div> : null}
                    {standItems.map((item, index) => {
                      const start = offset(item.start);
                      const end = offset(item.end ?? rangeEnd);
                      return (
                        <div key={item.id}>
                          <div
                            title={`${item.label}: ${item.detail}. ${item.secondary}`}
                            className={`absolute overflow-hidden rounded-md border px-2 py-1 shadow-sm ${item.tone}`}
                            style={{ left: `${start}%`, width: `${Math.max(7, end - start)}%`, top: `${6 + index * layout.laneHeight}px`, height: `${layout.laneHeight - 6}px` }}
                          >
                            <div className="flex items-center justify-between gap-2 text-xs font-semibold leading-4">
                              <span className="truncate">{item.label}</span>
                              {!item.active ? <span className="rounded bg-black/15 px-1 py-px text-[9px] uppercase tracking-wide">History</span> : null}
                            </div>
                            <div className="truncate text-[11px] leading-4 opacity-90">{item.detail}</div>
                            <div className="truncate text-[10px] leading-4 opacity-80">{item.secondary}</div>
                          </div>
                          {item.plannedEnd ? (
                            <div
                              title={`Planned release ${formatTimestamp(item.plannedEnd.toISOString())}`}
                              className="absolute z-20 border-l-2 border-dashed border-emerald-950/70 dark:border-emerald-100/80"
                              style={{ left: `${offset(item.plannedEnd)}%`, top: `${6 + index * layout.laneHeight}px`, height: `${layout.laneHeight - 6}px` }}
                            />
                          ) : null}
                        </div>
                      );
                    })}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}

export default function StandStatusPage() {
  const { getAccessTokenSilently } = useAuth0();
  const [data, setData] = useState<StandStatusResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [selectedStand, setSelectedStand] = useState("");
  const [selectedFlight, setSelectedFlight] = useState<SelectedFlight | null>(null);
  const [preview, setPreview] = useState<StandAllocationPreview | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);

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

  useEffect(() => {
    if (!selectedFlight) {
      setPreview(null);
      setPreviewError(null);
      return;
    }
    let active = true;
    setPreviewLoading(true);
    setPreview(null);
    setPreviewError(null);
    void (async () => {
      try {
        const token = await getAccessTokenSilently();
        const query = new URLSearchParams({
          session_id: String(selectedFlight.sessionId),
          airport: selectedFlight.airport,
          callsign: selectedFlight.callsign,
        });
        const response = await fetch(getApiUrl(`/api/stand/preview?${query.toString()}`), {
          headers: { Authorization: `Bearer ${token}` },
        });
        if (!response.ok) {
          const payload = (await response.json().catch(() => null)) as { error?: string } | null;
          throw new Error(payload?.error ?? `Preview failed (${response.status})`);
        }
        const payload = (await response.json()) as StandAllocationPreview;
        if (active) setPreview(payload);
      } catch (previewFetchError) {
        if (active) setPreviewError(previewFetchError instanceof Error ? previewFetchError.message : "Failed to load stand possibilities.");
      } finally {
        if (active) setPreviewLoading(false);
      }
    })();
    return () => { active = false; };
  }, [getAccessTokenSilently, selectedFlight]);

  const totals = useMemo(() => {
    const now = new Date();
    return (data?.sessions ?? []).reduce(
      (total, session) => ({
        assignments: total.assignments + activeAssignments(session, now).length,
        blocks: total.blocks + activeBlocks(session, now).length,
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

            {data.sessions.map((session) => {
              const now = new Date();
              const sessionAssignments = activeAssignments(session, now);
              const sessionBlocks = activeBlocks(session, now);
              return (
              <section key={session.session_id} className="rounded-2xl border bg-card shadow-sm">
                <div className="border-b px-6 py-4">
                  <div className="flex flex-col gap-2 md:flex-row md:items-baseline md:justify-between">
                    <div>
                      <h2 className="text-xl font-semibold">{session.airport} - {session.name}</h2>
                      <p className="text-sm text-muted-foreground">Session {session.session_id}</p>
                    </div>
                    <div className="text-sm text-muted-foreground">
                      {sessionAssignments.length} assignments · {sessionBlocks.length} blocks
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
                      {sessionAssignments.length === 0 ? (
                        <tr className="border-t">
                          <td colSpan={6} className="px-4 py-6 text-center text-muted-foreground">
                            No active stand assignments.
                          </td>
                        </tr>
                      ) : null}
                      {sessionAssignments.map((assignment) => (
                        <tr
                          key={assignment.id}
                          onClick={() => setSelectedFlight({ sessionId: session.session_id, airport: session.airport, callsign: assignment.callsign })}
                          className={`cursor-pointer border-t align-top hover:bg-muted/50 ${selectedFlight?.sessionId === session.session_id && selectedFlight.callsign === assignment.callsign ? "bg-primary/5" : ""}`}
                        >
                          <td className="px-4 py-3">
                            <div className="font-semibold">{assignment.callsign}</div>
                            <div className="text-xs text-muted-foreground">{assignment.direction || "—"}</div>
                          </td>
                          <td className="px-4 py-3">
                            <div className="font-semibold">{assignment.stand || "—"}</div>
                            <div className="text-xs text-muted-foreground">
                              {assignment.stage || "—"} · {assignment.source || "—"}
                              {assignment.manual && assignment.source !== "MANUAL" ? " · manual" : ""}
                            </div>
                          </td>
                          <td className="px-4 py-3">
                            <div>{assignment.rule_id || "No policy rule"}</div>
                            <div className="text-xs text-muted-foreground">
                              Tier {assignment.tier ?? "—"} · Variant {assignment.matched_variant || "—"}
                            </div>
                            {assignment.conflict_reason ? (
                              <div className="mt-1 text-xs text-amber-700 dark:text-amber-300">
                                {assignment.conflict_reason.startsWith("automatic fallback:") ? "Fallback" : "Override"}: {assignment.conflict_reason}
                              </div>
                            ) : null}
                          </td>
                          <td className="px-4 py-3">
                            {assignment.direction === "DEPARTURE" ? (
                              <>
                                <div>TOBT {clockValue(assignment.departure_tobt)} · TSAT {clockValue(assignment.departure_tsat)}</div>
                                <div className="text-xs text-muted-foreground">
                                  {assignment.expires_at
                                    ? `Expires ${formatTimestamp(assignment.expires_at)}`
                                    : assignment.planned_release_at
                                      ? `Scheduled expiry ${formatTimestamp(assignment.planned_release_at)}; held while observed on stand`
                                      : "Held while observed on stand; no TOBT or TSAT"}
                                </div>
                              </>
                            ) : (
                              <>
                                <div>ETA {formatTimestamp(assignment.eta)}</div>
                                <div className="text-xs text-muted-foreground">
                                  {assignment.eta_source || "No ETA source"} · expires {formatTimestamp(assignment.expires_at)}
                                </div>
                              </>
                            )}
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

                <StandAllocationTimeline session={session} selectedStand={selectedStand} onSelectStand={setSelectedStand} />

                {sessionBlocks.length > 0 ? (
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
                          {sessionBlocks.map((block) => (
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
              );
            })}

            {selectedFlight ? (
              <div
                className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
                onClick={() => setSelectedFlight(null)}
              >
                <section
                  role="dialog"
                  aria-modal="true"
                  aria-labelledby="stand-possibilities-title"
                  className="max-h-[90vh] w-full max-w-5xl overflow-y-auto rounded-xl border bg-background shadow-2xl"
                  onClick={(event) => event.stopPropagation()}
                >
                  <header className="sticky top-0 z-10 flex items-start justify-between gap-4 border-b bg-background px-6 py-5">
                    <div>
                      <h2 id="stand-possibilities-title" className="text-xl font-semibold">Stand possibilities · {selectedFlight.callsign}</h2>
                      <p className="mt-1 text-sm text-muted-foreground">Current compatible and available policy candidates, grouped by allocation tier.</p>
                    </div>
                    <button
                      type="button"
                      onClick={() => setSelectedFlight(null)}
                      className="rounded-md border px-3 py-1.5 text-sm font-medium hover:bg-muted"
                    >
                      Close
                    </button>
                  </header>
                  <div className="space-y-5 px-6 py-5">
                    {preview ? (
                      <div className="text-sm text-muted-foreground">
                        {preview.compatible_stands} compatible · {preview.available_stands} available
                        {preview.fallback_used ? " · using fallback pool" : ""}
                      </div>
                    ) : null}
                    {previewLoading ? <div className="text-sm text-muted-foreground">Calculating current stand possibilities…</div> : null}
                    {previewError ? <div className="text-sm text-red-700 dark:text-red-300">{previewError}</div> : null}
                    {preview && !previewLoading ? (
                      preview.selection.candidates.length === 0 ? (
                        <div className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-100">No currently available stand appears in this flight’s configured policy pool.</div>
                      ) : (
                        <div className="space-y-4">
                          {groupPreviewCandidates(preview.selection.candidates).map((tier) => (
                            <section key={`${tier.ruleID}-${tier.tier}`} className="overflow-hidden rounded-lg border bg-card">
                              <div className="flex flex-col gap-2 border-b bg-muted/30 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                                <div>
                                  <h3 className="font-semibold">{tier.tierName || `Tier ${tier.tier}`}</h3>
                                  <p className="text-xs text-muted-foreground">Rule {tier.ruleID}</p>
                                </div>
                                <span className={`w-fit rounded-full px-2 py-1 text-xs font-medium ${tier.selectable ? "bg-emerald-100 text-emerald-900 dark:bg-emerald-950/50 dark:text-emerald-100" : "bg-muted text-muted-foreground"}`}>
                                  {tier.selectable ? "Selectable now" : "Used if higher tiers have no stand"}
                                </span>
                              </div>
                              <div className="overflow-x-auto">
                                <table className="min-w-full text-sm">
                                  <thead className="bg-muted/20 text-left text-xs uppercase tracking-wide text-muted-foreground">
                                    <tr><th className="px-4 py-2 font-medium">Stand</th><th className="px-4 py-2 font-medium">Weight</th><th className="px-4 py-2 font-medium">Chance</th></tr>
                                  </thead>
                                  <tbody>
                                    {tier.candidates.map((candidate) => (
                                      <tr key={`${candidate.rule_id}-${candidate.tier}-${candidate.stand}`} className="border-t">
                                        <td className="px-4 py-3 font-semibold">{candidate.stand}</td>
                                        <td className="px-4 py-3">{candidate.original_weight}</td>
                                        <td className="px-4 py-3 font-semibold">{formatPercentage(candidate.normalized_weight)}</td>
                                      </tr>
                                    ))}
                                  </tbody>
                                </table>
                              </div>
                            </section>
                          ))}
                        </div>
                      )
                    ) : null}
                  </div>
                </section>
              </div>
            ) : null}
          </>
        ) : null}
      </div>
    </div>
  );
}

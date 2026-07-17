import { useCallback, useEffect, useMemo, useState } from "react";
import { useAuth0 } from "@auth0/auth0-react";
import { AlertTriangle, Clock3, FlaskConical, Plane, RefreshCw, Trash2 } from "lucide-react";
import { Toaster, toast } from "sonner";
import { Button } from "@/components/ui/button";
import { getApiUrl } from "@/lib/api-url";

type Session = { id: number; name: string; airport: string };
type Assignment = {
  stand: string;
  direction: string;
  stage: string;
  source: string;
  version: number;
  eta?: string;
  expires_at?: string;
  conflict_reason?: string;
};
type Scenario = {
  id: string;
  session_id: number;
  preset: "departure" | "arrival" | "wrong_stand";
  step: number;
  callsign: string;
  cid: string;
  aircraft_type: string;
  origin: string;
  destination: string;
  route: string;
  feed_state: string;
  altitude: number;
  groundspeed: number;
  observed_stand?: string;
  strip_bay?: string;
  assignment?: Assignment;
  last_action?: string;
  generated_message?: string;
  error?: string;
};
type StandBlock = { id: number; session_id: number; stand: string; reason?: string; version: number };
type Status = {
  enabled: boolean;
  simulated_time: string;
  sessions: Session[];
  sat: { enabled: boolean; ready: boolean; reason?: string };
};
type ScenarioState = { scenarios: Scenario[]; blocks: StandBlock[]; simulated_time: string };

const emptyForm = {
  preset: "departure" as Scenario["preset"],
  callsign: "",
  aircraft_type: "A320",
  origin: "EKCH",
  destination: "EGLL",
  route: "DCT",
  initial_state: "prefile",
  eobt: "",
  enroute_time: "0045",
  altitude: 0,
  groundspeed: 0,
  observed_stand: "",
};

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="grid gap-1 text-sm text-slate-300">
      <span>{label}</span>
      {children}
    </label>
  );
}

const inputClass = "h-10 rounded-md border border-slate-700 bg-slate-950 px-3 text-white outline-none focus:border-cyan-500";

class ApiError extends Error {
  constructor(readonly status: number, message: string) {
    super(message);
  }
}

function emptyFormForPreset(preset: Scenario["preset"]) {
  return {
    ...emptyForm,
    preset,
    origin: preset === "arrival" ? "EGLL" : "EKCH",
    destination: preset === "arrival" ? "EKCH" : "EGLL",
  };
}

export default function TestToolsPage() {
  const { getAccessTokenSilently } = useAuth0();
  const [status, setStatus] = useState<Status | null>(null);
  const [notAvailable, setNotAvailable] = useState(false);
  const [statusError, setStatusError] = useState("");
  const [sessionID, setSessionID] = useState(0);
  const [state, setState] = useState<ScenarioState>({ scenarios: [], blocks: [], simulated_time: "" });
  const [form, setForm] = useState(emptyForm);
  const [blockStand, setBlockStand] = useState("");
  const [blockReason, setBlockReason] = useState("Local test console");
  const [manualStands, setManualStands] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  const authorizedFetch = useCallback(async (path: string, init?: RequestInit) => {
    const token = await getAccessTokenSilently();
    const response = await fetch(getApiUrl(path), {
      ...init,
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
        ...init?.headers,
      },
    });
    if (!response.ok) {
      const payload = await response.json().catch(() => ({ error: response.statusText }));
      throw new ApiError(response.status, payload.error || response.statusText);
    }
    if (response.status === 204) return null;
    return response.json();
  }, [getAccessTokenSilently]);

  const loadStatus = useCallback(async () => {
    try {
      const value = await authorizedFetch("/api/test/status") as Status;
      setStatus(value);
      setNotAvailable(false);
      setStatusError("");
      setSessionID(current => current || value.sessions[0]?.id || 0);
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) {
        setNotAvailable(true);
        setStatusError("");
      } else {
        setNotAvailable(false);
        setStatusError(error instanceof Error ? error.message : "Unable to load test-console status");
      }
    }
  }, [authorizedFetch]);

  const loadState = useCallback(async () => {
    if (!sessionID || !status?.sat.ready) return;
    try {
      const value = await authorizedFetch(`/api/test/sat/scenarios?session_id=${sessionID}`) as ScenarioState;
      setState(value);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Unable to load test scenarios");
    }
  }, [authorizedFetch, sessionID, status?.sat.ready]);

  useEffect(() => {
    const timer = window.setTimeout(() => void loadStatus(), 0);
    return () => window.clearTimeout(timer);
  }, [loadStatus]);

  useEffect(() => {
    if (!sessionID || !status?.sat.ready) return;
    const initial = window.setTimeout(() => void loadState(), 0);
    const timer = window.setInterval(() => void loadState(), 2000);
    return () => {
      window.clearTimeout(initial);
      window.clearInterval(timer);
    };
  }, [loadState, sessionID, status?.sat.ready]);

  const run = useCallback(async (action: () => Promise<unknown>, success: string) => {
    setBusy(true);
    try {
      await action();
      toast.success(success);
      await Promise.all([loadStatus(), loadState()]);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Test action failed");
    } finally {
      setBusy(false);
    }
  }, [loadState, loadStatus]);

  const setPreset = (preset: Scenario["preset"]) => {
    setForm(current => ({
      ...current,
      preset,
      origin: preset === "arrival" ? "EGLL" : "EKCH",
      destination: preset === "arrival" ? "EKCH" : "EGLL",
      altitude: 0,
      groundspeed: 0,
    }));
  };

  const createScenario = () => run(async () => {
    await authorizedFetch("/api/test/sat/scenarios", {
      method: "POST",
      body: JSON.stringify({ ...form, session_id: sessionID }),
    });
    setForm(current => emptyFormForPreset(current.preset));
  }, "Scenario created");

  const command = (scenario: Scenario, commandName: string, extra: Record<string, unknown> = {}) =>
    run(
      () => authorizedFetch(`/api/test/sat/scenarios/${scenario.id}/commands`, {
        method: "POST",
        body: JSON.stringify({ command: commandName, ...extra }),
      }),
      `Updated ${scenario.callsign}`,
    );

  const simulatedTime = state.simulated_time || status?.simulated_time;
  const selectedSession = useMemo(() => status?.sessions.find(session => session.id === sessionID), [sessionID, status?.sessions]);

  if (notAvailable) {
    return <div className="p-8 text-xl">404 Not Found</div>;
  }
  if (statusError) {
    return (
      <div className="grid gap-3 p-8">
        <h1 className="text-xl font-semibold">Unable to load test console</h1>
        <p className="text-red-500">{statusError}</p>
        <Button className="w-fit" variant="outline" onClick={() => void loadStatus()}>Retry</Button>
      </div>
    );
  }
  if (!status) {
    return <div className="p-8 text-slate-500">Checking local test-tools availability…</div>;
  }

  return (
    <>
      <Toaster richColors position="top-right" />
      <div className="h-full w-full overflow-y-auto bg-slate-950 text-white">
        <div className="mx-auto grid min-h-full w-full max-w-7xl gap-6 p-6 md:p-8">
        <header className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-3">
              <FlaskConical className="size-8 text-cyan-400" />
              <h1 className="text-3xl font-semibold">Local Test Console</h1>
            </div>
            <p className="mt-1 text-slate-400">Synthetic SAT scenarios using the real reconciliation and allocation paths.</p>
          </div>
          <div className="flex items-center gap-3 rounded-lg border border-slate-800 bg-slate-900 px-4 py-3">
            <Clock3 className="size-5 text-cyan-400" />
            <div>
              <div className="text-xs uppercase tracking-wide text-slate-500">Simulated UTC</div>
              <div className="font-mono">{simulatedTime ? new Date(simulatedTime).toISOString().replace(".000Z", "Z") : "—"}</div>
            </div>
            <Button variant="outline" size="icon" disabled={busy} onClick={() => void loadState()} aria-label="Refresh scenarios">
              <RefreshCw className="size-4" />
            </Button>
          </div>
        </header>

        {!status.sat.ready && (
          <div className="flex gap-3 rounded-lg border border-amber-700 bg-amber-950/50 p-4 text-amber-100">
            <AlertTriangle className="mt-0.5 size-5 shrink-0" />
            <div>
              <div className="font-semibold">Stand assignment is unavailable</div>
              <div className="text-sm">{status.sat.enabled ? status.sat.reason || "SAT configuration did not become ready." : "Set ENABLE_STAND_ASSIGNMENT=true and restart the backend."}</div>
            </div>
          </div>
        )}

        <section className="grid gap-4 rounded-xl border border-slate-800 bg-slate-900 p-5">
          <div className="flex flex-wrap items-end justify-between gap-4">
            <Field label="Target EKCH session">
              <select className={inputClass} value={sessionID} onChange={event => setSessionID(Number(event.target.value))}>
                <option value={0}>Select a connected session</option>
                {status.sessions.map(session => <option key={session.id} value={session.id}>{session.name} · {session.airport}</option>)}
              </select>
            </Field>
            <div className="text-sm text-slate-400">
              {selectedSession ? `Changes publish to ${selectedSession.name}.` : "Connect the local EuroScope plugin first to create a session."}
            </div>
          </div>
        </section>

        <section className="grid gap-5 rounded-xl border border-slate-800 bg-slate-900 p-5">
          <div>
            <h2 className="text-xl font-semibold">Create SAT scenario</h2>
            <p className="text-sm text-slate-400">Presets supply lifecycle transitions; every feed field remains editable.</p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Field label="Preset">
              <select className={inputClass} value={form.preset} onChange={event => setPreset(event.target.value as Scenario["preset"])}>
                <option value="departure">Departure lifecycle</option>
                <option value="arrival">Arrival lifecycle</option>
                <option value="wrong_stand">Wrong stand</option>
              </select>
            </Field>
            <Field label="Callsign"><input className={inputClass} value={form.callsign} onChange={event => setForm({ ...form, callsign: event.target.value.toUpperCase() })} placeholder="TST101" /></Field>
            <Field label="Aircraft"><input className={inputClass} value={form.aircraft_type} onChange={event => setForm({ ...form, aircraft_type: event.target.value.toUpperCase() })} /></Field>
            <Field label="Initial state">
              <select className={inputClass} value={form.initial_state} onChange={event => setForm({ ...form, initial_state: event.target.value })}>
                <option value="prefile">Prefile</option>
                <option value="online">Online</option>
              </select>
            </Field>
            <Field label="Origin"><input className={inputClass} value={form.origin} onChange={event => setForm({ ...form, origin: event.target.value.toUpperCase() })} /></Field>
            <Field label="Destination"><input className={inputClass} value={form.destination} onChange={event => setForm({ ...form, destination: event.target.value.toUpperCase() })} /></Field>
            <Field label="Route"><input className={inputClass} value={form.route} onChange={event => setForm({ ...form, route: event.target.value.toUpperCase() })} /></Field>
            <Field label="Observed stand"><input className={inputClass} value={form.observed_stand} onChange={event => setForm({ ...form, observed_stand: event.target.value.toUpperCase() })} placeholder="Optional" /></Field>
            <Field label="EOBT"><input className={inputClass} value={form.eobt} onChange={event => setForm({ ...form, eobt: event.target.value })} placeholder="Current UTC" /></Field>
            <Field label="Enroute time"><input className={inputClass} value={form.enroute_time} onChange={event => setForm({ ...form, enroute_time: event.target.value })} /></Field>
            <Field label="Altitude"><input className={inputClass} type="number" value={form.altitude} onChange={event => setForm({ ...form, altitude: Number(event.target.value) })} /></Field>
            <Field label="Groundspeed"><input className={inputClass} type="number" value={form.groundspeed} onChange={event => setForm({ ...form, groundspeed: Number(event.target.value) })} /></Field>
          </div>
          <Button className="w-fit" disabled={busy || !sessionID || !status.sat.ready} onClick={() => void createScenario()}>
            <Plane className="mr-2 size-4" /> Create scenario
          </Button>
        </section>

        <section className="grid gap-4 rounded-xl border border-slate-800 bg-slate-900 p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 className="text-xl font-semibold">Test-owned stand blocks</h2>
              <p className="text-sm text-slate-400">Blocks created here are tagged and removed by console reset.</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <input className={`${inputClass} w-28`} value={blockStand} onChange={event => setBlockStand(event.target.value.toUpperCase())} placeholder="Stand" />
              <input className={`${inputClass} w-64`} value={blockReason} onChange={event => setBlockReason(event.target.value)} placeholder="Reason" />
              <Button variant="outline" disabled={busy || !sessionID || !blockStand} onClick={() => void run(
                () => authorizedFetch("/api/test/sat/blocks", { method: "POST", body: JSON.stringify({ session_id: sessionID, stand: blockStand, reason: blockReason }) }),
                `Blocked ${blockStand}`,
              )}>Block stand</Button>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            {state.blocks.length === 0 && <span className="text-sm text-slate-500">No test-owned blocks.</span>}
            {state.blocks.map(block => (
              <div key={block.id} className="flex items-center gap-2 rounded-full border border-amber-700 bg-amber-950/40 px-3 py-1 text-sm">
                <span>{block.stand} · {block.reason}</span>
                <button aria-label={`Remove block ${block.stand}`} onClick={() => void run(
                  () => authorizedFetch(`/api/test/sat/blocks?session_id=${block.session_id}&id=${block.id}&version=${block.version}`, { method: "DELETE" }),
                  `Unblocked ${block.stand}`,
                )}><Trash2 className="size-3.5" /></button>
              </div>
            ))}
          </div>
        </section>

        <section className="grid gap-4">
          <div className="flex items-center justify-between">
            <div>
              <h2 className="text-xl font-semibold">Active scenarios</h2>
              <p className="text-sm text-slate-400">Next follows the selected preset; manual controls remain available.</p>
            </div>
            <Button variant="destructive" disabled={busy || (state.scenarios.length === 0 && state.blocks.length === 0)} onClick={() => {
              if (window.confirm("Reset all test-console SAT data?")) {
                void run(() => authorizedFetch("/api/test/sat/scenarios", { method: "DELETE" }), "Test data reset");
              }
            }}><Trash2 className="mr-2 size-4" /> Reset all</Button>
          </div>
          {state.scenarios.length === 0 && <div className="rounded-xl border border-dashed border-slate-800 p-10 text-center text-slate-500">No active scenarios.</div>}
          <div className="grid gap-4 lg:grid-cols-2">
            {state.scenarios.map(scenario => (
              <article key={scenario.id} className="grid gap-4 rounded-xl border border-slate-800 bg-slate-900 p-5">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-xl font-semibold">{scenario.callsign}</span>
                      <span className="rounded bg-cyan-950 px-2 py-0.5 text-xs uppercase text-cyan-300">{scenario.preset.replace("_", " ")}</span>
                    </div>
                    <div className="text-sm text-slate-400">{scenario.aircraft_type} · {scenario.origin} → {scenario.destination} · CID {scenario.cid}</div>
                  </div>
                  <button aria-label={`Delete ${scenario.callsign}`} onClick={() => void run(
                    () => authorizedFetch(`/api/test/sat/scenarios/${scenario.id}`, { method: "DELETE" }),
                    `Deleted ${scenario.callsign}`,
                  )}><Trash2 className="size-4 text-slate-400 hover:text-red-400" /></button>
                </div>
                <div className="grid grid-cols-2 gap-3 text-sm md:grid-cols-4">
                  <div><div className="text-slate-500">Feed</div><div>{scenario.feed_state}</div></div>
                  <div><div className="text-slate-500">Strip bay</div><div>{scenario.strip_bay || "—"}</div></div>
                  <div><div className="text-slate-500">Stage</div><div>{scenario.assignment?.stage || "—"}</div></div>
                  <div><div className="text-slate-500">Stand</div><div>{scenario.assignment?.stand || "—"}</div></div>
                  <div><div className="text-slate-500">Source</div><div>{scenario.assignment?.source || "—"}</div></div>
                  <div><div className="text-slate-500">Version</div><div>{scenario.assignment?.version ?? "—"}</div></div>
                  <div><div className="text-slate-500">ETA</div><div>{scenario.assignment?.eta ? new Date(scenario.assignment.eta).toISOString().slice(11, 16) : "—"}</div></div>
                  <div><div className="text-slate-500">Expires</div><div>{scenario.assignment?.expires_at ? new Date(scenario.assignment.expires_at).toISOString().slice(11, 16) : "—"}</div></div>
                </div>
                {(scenario.last_action || scenario.generated_message || scenario.assignment?.conflict_reason || scenario.error) && (
                  <div className="grid gap-1 rounded-md bg-slate-950 p-3 text-sm">
                    {scenario.last_action && <div><span className="text-slate-500">Last action:</span> {scenario.last_action}</div>}
                    {scenario.generated_message && <div><span className="text-slate-500">Message:</span> {scenario.generated_message}</div>}
                    {scenario.assignment?.conflict_reason && <div className="text-amber-300">{scenario.assignment.conflict_reason}</div>}
                    {scenario.error && <div className="text-red-400">{scenario.error}</div>}
                  </div>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button disabled={busy} onClick={() => void command(scenario, "advance")}>Next</Button>
                  {[5, 15, 30].map(minutes => <Button key={minutes} variant="outline" disabled={busy} onClick={() => void command(scenario, "advance_time", { minutes })}>+{minutes} min</Button>)}
                  <input
                    aria-label={`Move ${scenario.callsign} to stand`}
                    className={`${inputClass} w-24`}
                    value={manualStands[scenario.id] ?? scenario.observed_stand ?? ""}
                    onChange={event => setManualStands(current => ({ ...current, [scenario.id]: event.target.value.toUpperCase() }))}
                    onKeyDown={event => {
                      if (event.key === "Enter") void command(scenario, "move_to_stand", { stand: event.currentTarget.value });
                    }}
                    placeholder="Stand"
                  />
                  <Button
                    variant="outline"
                    disabled={busy || !(manualStands[scenario.id] ?? scenario.observed_stand)}
                    onClick={() => void command(scenario, "move_to_stand", { stand: manualStands[scenario.id] ?? scenario.observed_stand })}
                  >
                    Move
                  </Button>
                  <Button variant="outline" disabled={busy || scenario.feed_state === "removed"} onClick={() => void command(scenario, "remove")}>Remove feed</Button>
                </div>
              </article>
            ))}
          </div>
        </section>
        </div>
      </div>
    </>
  );
}

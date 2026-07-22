import {useMemo, useState} from "react";

import {
  getAMANMutationBlockReason,
  type AMANCommandIntent,
  type AMANCommandRejection,
  type AMANConnectionState,
  type AMANFlight,
  type AMANMutationBlockReason,
  type AMANPendingCommand,
  type AMANState,
} from "@/api/aman";
import {useWebSocketStore} from "@/store/store-hooks";

const blockReasonLabels: Record<AMANMutationBlockReason, string> = {
  no_state: "Waiting for AMAN state",
  disconnected: "Disconnected — controls are unavailable",
  observer: "Observer session — controls are read-only",
  unauthorized: "FMP authority is required",
  not_authoritative: "AMAN is not authoritative — controls are read-only",
  not_ready: "AMAN is technically degraded — controls are unavailable",
};

const controlClass = "rounded border border-slate-500 bg-slate-700 px-2 py-1 text-sm text-white disabled:cursor-not-allowed disabled:opacity-40";
const inputClass = "rounded border border-slate-500 bg-slate-950 px-2 py-1 text-sm text-white";

export interface AMANControlsViewProps {
  state: AMANState | null;
  connectionState: AMANConnectionState;
  readOnly: boolean;
  hasFMPAuthority: boolean;
  pendingCommands: Record<string, AMANPendingCommand>;
  commandRejections: Record<string, AMANCommandRejection>;
  onCommand: (intent: AMANCommandIntent) => void;
  onDismissRejection?: (commandID: string) => void;
}

function displayTime(value: string | null): string {
  if (value === null) return "Unavailable";
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? value : parsed.toISOString().slice(11, 16);
}

function toWireTimestamp(value: string): string | null {
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? null : parsed.toISOString();
}

function componentWarning(label: string, status: string, reason: string | null): string | null {
  if (status === "ready" || status === "disabled") return null;
  return `${label}: ${status}${reason ? ` (${reason})` : ""}`;
}

function FlightStatus({flight}: {flight: AMANFlight}) {
  const direct = flight.route_fact === null
    ? "No accepted direct"
    : flight.route_fact.state === "active"
      ? `Accepted direct ${flight.route_fact.fix}`
      : `Direct ${flight.route_fact.fix} expired`;

  return (
    <section aria-label={`${flight.callsign} AMAN status`} className="grid gap-2 rounded border border-slate-600 bg-slate-900 p-3 text-sm text-slate-100">
      <div className="flex flex-wrap gap-x-4 gap-y-1">
        <strong>{flight.callsign}</strong>
        <span>Data: <b className={flight.data_status === "fresh" ? "text-emerald-300" : "text-amber-300"}>{flight.data_status}</b></span>
        <span>Confidence: {flight.confidence ?? "unavailable"}</span>
        <span>Input age: {flight.input_age_seconds === null ? "unavailable" : `${flight.input_age_seconds}s`}</span>
      </div>

      <div className="grid gap-1 sm:grid-cols-2">
        <span>Operational TETA: <b>{displayTime(flight.operational_teta)}</b></span>
        <span>Operational slot: <b>{displayTime(flight.slot?.time ?? null)}{flight.freeze_reason !== "none" ? ` (fixed by ${flight.freeze_reason} freeze)` : ""}</b></span>
        <span className={flight.raw_teta !== flight.operational_teta ? "text-amber-300" : ""}>Raw TETA (informational): <b>{displayTime(flight.raw_teta)}</b></span>
        <span>Freeze: <b className={flight.freeze_reason === "superstable" ? "text-cyan-300" : flight.freeze_reason === "manual" ? "text-fuchsia-300" : ""}>{flight.freeze_reason}</b></span>
        <span>Holding fix: <b>{flight.holding_fix ?? "Unavailable"}</b></span>
        <span>Geometry: <b>{flight.geometry_version ?? "Unavailable"}</b></span>
        <span>{direct}</span>
      </div>

      {flight.provenance && (
        <div className="text-xs text-slate-300">
          Provenance: model {flight.provenance.model_version}, config {flight.provenance.config_version}, sources {flight.provenance.sources.join(", ") || "none"}, weather {flight.provenance.weather_source ?? "unavailable"}
        </div>
      )}

      {flight.eta_review && (
        <div className="rounded border border-amber-500 bg-amber-950/60 p-2 text-amber-100">
          Discrepancy review: {flight.eta_review.status}. Initial/FPL {displayTime(flight.eta_review.initial_baseline_teta)}; calculated {displayTime(flight.eta_review.calculated_operational_teta)}; selected {displayTime(flight.eta_review.selected_teta)}.
        </div>
      )}
    </section>
  );
}

export function AMANControlsView({
  state,
  connectionState,
  readOnly,
  hasFMPAuthority,
  pendingCommands,
  commandRejections,
  onCommand,
  onDismissRejection,
}: AMANControlsViewProps) {
  const [flightID, setFlightID] = useState("");
  const [targetFlightID, setTargetFlightID] = useState("");
  const [rate, setRate] = useState("30");
  const [rateEffectiveAt, setRateEffectiveAt] = useState("");
  const [manualETA, setManualETA] = useState("");
  const [goAroundAt, setGoAroundAt] = useState("");

  const flights = state?.flights ?? [];
  const selectedFlight = flights.find((flight) => flight.flight_id === flightID) ?? flights[0] ?? null;
  const selectedFlightID = selectedFlight?.flight_id ?? "";
  const runwayGroupID = selectedFlight?.runway_group_id ?? state?.runway_groups[0]?.id ?? null;
  const gateReason = getAMANMutationBlockReason({
    state,
    connection_state: connectionState,
    read_only: readOnly,
    has_fmp_authority: hasFMPAuthority,
  });
  const disabled = gateReason !== null;
  const pending = Object.values(pendingCommands);
  const rejections = Object.values(commandRejections);

  const healthWarnings = useMemo(() => state ? [
    componentWarning("Geometry/navigation", state.technical_health.navigation.status, state.technical_health.navigation.reason),
    componentWarning("Holding/navigation", state.technical_health.navigation.status, state.technical_health.navigation.reason),
    componentWarning("Weather", state.technical_health.weather.status, state.technical_health.weather.reason),
  ].filter((warning): warning is string => warning !== null) : [], [state]);

  const sendFlightCommand = (type: "aman.lock_flight" | "aman.unlock_flight" | "aman.accept_teta" | "aman.keep_fpl_eta" | "aman.reset_teta_override") => {
    if (selectedFlight) onCommand({type, flight_id: selectedFlight.flight_id});
  };

  const sendMove = (placement: "before" | "after") => {
    if (!selectedFlight || !targetFlightID || !runwayGroupID) return;
    onCommand(placement === "before"
      ? {type: "aman.move_flight", flight_id: selectedFlight.flight_id, runway_group_id: runwayGroupID, before_flight_id: targetFlightID}
      : {type: "aman.move_flight", flight_id: selectedFlight.flight_id, runway_group_id: runwayGroupID, after_flight_id: targetFlightID});
  };

  return (
    <aside aria-label="AMAN FMP controls" className="grid gap-3 rounded bg-slate-800 p-4 text-white">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-lg font-semibold">AMAN FMP</h2>
        <span>Mode: <b>{state?.effective_mode ?? "unavailable"}</b> · Health: <b>{state?.technical_health.status ?? "unavailable"}</b> · Revision: <b>{state?.revision ?? "—"}</b></span>
      </header>

      {gateReason && <div role="status" className="rounded border border-amber-500 bg-amber-950 p-2 text-amber-100">{blockReasonLabels[gateReason]}</div>}
      {state?.technical_health.blocked_reasons.map((reason) => <div key={reason} className="rounded bg-red-950 p-2 text-red-100">Blocked: {reason}</div>)}
      {healthWarnings.map((warning) => <div key={warning} className="rounded bg-amber-950 p-2 text-amber-100">{warning}</div>)}

      {pending.length > 0 && (
        <div aria-live="polite" className="rounded border border-sky-600 bg-sky-950 p-2 text-sm">
          Pending: {pending.map((command) => `${command.type} (${command.command_id})`).join(", ")}
        </div>
      )}
      {rejections.map((rejection) => (
        <div role="alert" key={rejection.command_id} className="flex items-start justify-between gap-2 rounded border border-red-500 bg-red-950 p-2 text-sm">
          <span>Command rejected: {rejection.message} ({rejection.code}, server revision {rejection.current_revision})</span>
          {onDismissRejection && <button className={controlClass} onClick={() => onDismissRejection(rejection.command_id)}>Dismiss</button>}
        </div>
      ))}

      {flights.length === 0 ? (
        <div>No AMAN flights available.</div>
      ) : (
        <>
          <label className="grid gap-1 text-sm">
            Flight
            <select aria-label="Flight" className={inputClass} value={selectedFlightID} onChange={(event) => setFlightID(event.target.value)}>
              {flights.map((flight) => <option key={flight.flight_id} value={flight.flight_id}>{flight.callsign}</option>)}
            </select>
          </label>

          {selectedFlight && <FlightStatus flight={selectedFlight} />}

          <div className="grid gap-2 rounded border border-slate-600 p-3">
            <h3 className="font-semibold">Sequence and freeze</h3>
            <select aria-label="Move target" className={inputClass} value={targetFlightID} onChange={(event) => setTargetFlightID(event.target.value)}>
              <option value="">Select target flight</option>
              {flights.filter((flight) => flight.flight_id !== selectedFlightID).map((flight) => <option key={flight.flight_id} value={flight.flight_id}>{flight.callsign}</option>)}
            </select>
            <div className="flex flex-wrap gap-2">
              <button className={controlClass} disabled={disabled || !targetFlightID || !runwayGroupID} onClick={() => sendMove("before")}>Move before</button>
              <button className={controlClass} disabled={disabled || !targetFlightID || !runwayGroupID} onClick={() => sendMove("after")}>Move after</button>
              <button className={controlClass} disabled={disabled || selectedFlight?.freeze_reason === "manual"} onClick={() => sendFlightCommand("aman.lock_flight")}>Apply manual freeze</button>
              <button className={controlClass} disabled={disabled || selectedFlight?.freeze_reason !== "manual"} onClick={() => sendFlightCommand("aman.unlock_flight")}>Release manual freeze</button>
            </div>
          </div>

          <div className="grid gap-2 rounded border border-slate-600 p-3">
            <h3 className="font-semibold">Arrival rate</h3>
            <div className="flex flex-wrap gap-2">
              <input aria-label="Arrivals per hour" className={inputClass} type="number" min="1" value={rate} onChange={(event) => setRate(event.target.value)} />
              <input aria-label="Rate effective at" className={inputClass} type="datetime-local" value={rateEffectiveAt} onChange={(event) => setRateEffectiveAt(event.target.value)} />
              <button className={controlClass} disabled={disabled || !runwayGroupID || !toWireTimestamp(rateEffectiveAt) || Number(rate) < 1} onClick={() => {
                const effectiveAt = toWireTimestamp(rateEffectiveAt);
                if (runwayGroupID && effectiveAt) onCommand({type: "aman.set_rate", runway_group_id: runwayGroupID, arrivals_per_hour: Number(rate), effective_at: effectiveAt});
              }}>Set arrival rate</button>
            </div>
          </div>

          <div className="grid gap-2 rounded border border-slate-600 p-3">
            <h3 className="font-semibold">ETA review and override</h3>
            <div className="flex flex-wrap gap-2">
              <button className={controlClass} disabled={disabled} onClick={() => sendFlightCommand("aman.accept_teta")}>Accept calculated TETA</button>
              <button className={controlClass} disabled={disabled} onClick={() => sendFlightCommand("aman.keep_fpl_eta")}>Keep initial/FPL ETA</button>
              <button className={controlClass} disabled={disabled} onClick={() => sendFlightCommand("aman.reset_teta_override")}>Reset ETA override</button>
            </div>
            <div className="flex flex-wrap gap-2">
              <input aria-label="Manual ETA" className={inputClass} type="datetime-local" value={manualETA} onChange={(event) => setManualETA(event.target.value)} />
              <button className={controlClass} disabled={disabled || !toWireTimestamp(manualETA)} onClick={() => {
                const value = toWireTimestamp(manualETA);
                if (selectedFlight && value) onCommand({type: "aman.set_manual_eta", flight_id: selectedFlight.flight_id, manual_eta: value});
              }}>Set manual ETA</button>
            </div>
          </div>

          <div className="grid gap-2 rounded border border-slate-600 p-3">
            <h3 className="font-semibold">Go-around</h3>
            <div className="flex flex-wrap gap-2">
              <input aria-label="Go-around detected at" className={inputClass} type="datetime-local" value={goAroundAt} onChange={(event) => setGoAroundAt(event.target.value)} />
              <button className={controlClass} disabled={disabled || !toWireTimestamp(goAroundAt)} onClick={() => {
                const value = toWireTimestamp(goAroundAt);
                if (selectedFlight && value) onCommand({type: "aman.report_go_around", flight_id: selectedFlight.flight_id, detected_at: value});
              }}>Report go-around</button>
            </div>
          </div>
        </>
      )}
    </aside>
  );
}

export function AMANControls({hasFMPAuthority}: {hasFMPAuthority: boolean}) {
  const state = useWebSocketStore((value) => value.amanState);
  const connectionState = useWebSocketStore((value) => value.amanConnectionState);
  const readOnly = useWebSocketStore((value) => value.readOnly);
  const pendingCommands = useWebSocketStore((value) => value.amanPendingCommands);
  const commandRejections = useWebSocketStore((value) => value.amanCommandRejections);
  const sendCommand = useWebSocketStore((value) => value.sendAMANCommand);
  const dismissRejection = useWebSocketStore((value) => value.dismissAMANCommandRejection);

  return (
    <AMANControlsView
      state={state}
      connectionState={connectionState}
      readOnly={readOnly}
      hasFMPAuthority={hasFMPAuthority}
      pendingCommands={pendingCommands}
      commandRejections={commandRejections}
      onCommand={(intent) => { sendCommand(intent, hasFMPAuthority); }}
      onDismissRejection={dismissRejection}
    />
  );
}

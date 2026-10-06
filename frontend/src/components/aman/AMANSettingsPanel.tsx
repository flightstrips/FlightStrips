import {getAMANMutationBlockReason, type AMANCommandIntent, type AMANCommandRejection, type AMANConnectionState, type AMANMutationBlockReason, type AMANPendingCommand, type AMANState} from "@/api/aman";

interface AMANSettingsPanelProps {
  state: AMANState | null;
  connectionState: AMANConnectionState;
  hasFMPAuthority: boolean;
  readOnly: boolean;
  pendingCommands: Record<string, AMANPendingCommand>;
  commandRejections: Record<string, AMANCommandRejection>;
  onCommand: (intent: AMANCommandIntent) => void;
}

const mutationBlockLabels: Record<AMANMutationBlockReason, string> = {
  no_state: "waiting for AMAN state", disconnected: "disconnected", observer: "observer session",
  unauthorized: "FMP authority is required", not_authoritative: "AMAN is not authoritative", not_ready: "AMAN is technically degraded",
};

export function AMANSettingsPanel({state, connectionState, hasFMPAuthority, readOnly, pendingCommands, commandRejections, onCommand}: AMANSettingsPanelProps) {
  const blockReason = getAMANMutationBlockReason({state, connection_state: connectionState, read_only: readOnly, has_fmp_authority: hasFMPAuthority});
  const pending = Object.values(pendingCommands).some(command => command.type === "aman.set_holding_eat_writeback");
  const rejection = Object.values(commandRejections).find(command => command.command_type === "aman.set_holding_eat_writeback");

  return <section aria-label="AMAN settings" className="grid gap-3 rounded border border-slate-500 bg-slate-800 p-3 text-sm text-slate-100">
    <h2 className="font-display font-bold">AMAN settings</h2>
    <label className="flex items-center justify-between gap-3 font-bold">
      Write holding EAT to EuroScope
      <input checked={state?.holding_eat_writeback_enabled ?? false} disabled={blockReason !== null || pending || state?.holding_eat_writeback_available !== true} onChange={event => onCommand({type: "aman.set_holding_eat_writeback", enabled: event.target.checked})} type="checkbox" />
    </label>
    <p>Applies to this AMAN session. FMP can change this setting. Switching it off stops EuroScope updates and preserves existing EAT values.</p>
    {state !== null && state.holding_eat_writeback_available !== true && <p role="status">EAT writeback is disabled by the deployment feature flag.</p>}
    {pending && <p role="status">Waiting for server confirmation.</p>}
    {rejection && <p role="alert" className="text-amber-200">Rejected: {rejection.message}</p>}
    {blockReason && <p role="status">Read-only: {mutationBlockLabels[blockReason]}.</p>}
  </section>;
}

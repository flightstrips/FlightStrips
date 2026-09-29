import {CommandOutcome_Status} from "@/api/generated/cluster/v1/storage_pb";
import {useWebSocketStore} from "@/store/store-hooks";

function description(status: CommandOutcome_Status | "checking" | "not-confirmed", label: string): string {
  switch (status) {
    case CommandOutcome_Status.ACCEPTED: return "Accepted; waiting for plugin result";
    case CommandOutcome_Status.SUCCEEDED: return label === "send_private_message" ? "Accepted by local EuroScope send; pilot receipt is not confirmed" : "Succeeded";
    case CommandOutcome_Status.FAILED: return "Failed";
    case CommandOutcome_Status.EXPIRED: return "Expired before dispatch";
    case CommandOutcome_Status.UNKNOWN: return "Execution unknown; check the operational state before retrying";
    case "not-confirmed": return "Not confirmed; retry manually";
    default: return "Checking command status";
  }
}

export function ActionResults() {
  const statuses = useWebSocketStore(state => state.actionStatuses);
  const dismiss = useWebSocketStore(state => state.dismissActionStatus);
  const visible = Object.values(statuses).filter(item => item.status !== CommandOutcome_Status.SUCCEEDED || item.label === "send_private_message");
  if (!visible.length) return null;
  return <section aria-label="Action results" className="fixed bottom-4 right-4 z-50 max-h-64 w-80 overflow-y-auto rounded border border-slate-500 bg-slate-950 p-3 text-sm text-white shadow-xl">
    <h2 className="mb-2 font-semibold">Action results</h2>
    {visible.map(item => <div key={item.requestId} className="mb-2 border-t border-slate-700 pt-2" role="status">
      <div className="flex justify-between gap-2"><strong>{item.label.replace(/[._]/g, " ")}</strong>
        <button aria-label={`Dismiss ${item.label}`} onClick={() => dismiss(item.requestId)} type="button">×</button></div>
      <p>{description(item.status, item.label)}</p>
      {item.status === CommandOutcome_Status.FAILED && <p>{item.detail || item.reasonCode}</p>}
    </div>)}
  </section>;
}

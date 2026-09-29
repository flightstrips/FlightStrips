import {CommandOutcome_Status} from "./generated/cluster/v1/storage_pb";

const PREFIX = "flightstrips.pending-actions.v2.";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export type ActionStatus = {
  requestId: string;
  label: string;
  status: CommandOutcome_Status | "checking" | "not-confirmed";
  reasonCode?: string;
  detail?: string;
};

export function pendingKey(actorId: string): string {
  return PREFIX + actorId;
}

// Only IDs and fixed action labels are persisted. Never pass a command body to
// this module: it may contain a private message or other sensitive content.
export function readPending(actorId: string): ActionStatus[] {
  try {
    const value: unknown = JSON.parse(sessionStorage.getItem(pendingKey(actorId)) ?? "[]");
    if (!Array.isArray(value)) return [];
    return value.filter((item): item is {requestId: string; label: string} =>
      typeof item === "object" && item !== null && UUID.test(item.requestId) &&
      typeof item.label === "string" && /^[a-z._]+$/.test(item.label)).slice(-100)
      .map(item => ({...item, status: "checking"}));
  } catch {
    return [];
  }
}

export function writePending(actorId: string, values: Iterable<ActionStatus>): void {
  try {
    const pending = [...values].filter(item =>
      item.status === "checking" || item.status === "not-confirmed" ||
      item.status === CommandOutcome_Status.ACCEPTED);
    sessionStorage.setItem(pendingKey(actorId), JSON.stringify(pending.slice(-100).map(
      ({requestId, label}) => ({requestId, label}))));
  } catch {
    // Storage can be disabled; live command tracking still works in memory.
  }
}

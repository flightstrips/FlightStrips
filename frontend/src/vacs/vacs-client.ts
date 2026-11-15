import { ActionType, EventType } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import type { ClientInfo, VacsActions, VacsRelayAction, VacsRelayResult, VacsRelaySnapshot, VacsState } from "./types";

export function relayState(snapshot: VacsRelaySnapshot): VacsState {
  const common = { actionable: snapshot.actionable ?? false, pending: snapshot.pending ?? false,
    tooltip: snapshot.tooltip, waiting: snapshot.waiting ?? 0 };
  const clients = (snapshot.clients ?? []).map(peer => ({id: peer.id, displayName: peer.display_name ?? "",
    frequency: peer.frequency ?? "", positionId: peer.position_id}));
  const own = {clients, ownPositionId: snapshot.own_position_id ?? "", ownClientId: snapshot.own_client_id ?? null};
  const peer: ClientInfo = {id: "", displayName: snapshot.label ?? "controller", frequency: ""};
  const callId = snapshot.call_id ?? "";
  switch (snapshot.status) {
    case "idle": return {...common, ...own, status: "idle"};
    case "incoming": return {...common, ...own, status: "incoming", calls: [{callId,
      source: {clientId: "", positionId: snapshot.label}, target: {}, prio: false}]};
    case "outgoing": return {...common, ...own, status: "outgoing", callId, peer};
    case "connecting": case "connected": return {...common, status: snapshot.status, callId, peer};
    case "disabled": case "reconnecting": case "synchronizing": case "unauthenticated": case "disconnected": case "ambiguous":
      return {...common, status: snapshot.status};
    default: return {...common, status: "unavailable"};
  }
}

interface PendingRequest {
  action: VacsRelayAction;
  resolve: () => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
}

/** One relay per board socket; never opens a browser-to-VACS connection. */
export class VacsClient implements VacsActions {
  private state: VacsState = {status: "unavailable", actionable: false};
  private listeners = new Set<() => void>();
  private pending = new Map<string, PendingRequest>();
  private sequence = 0;
  private actionPending = false;
  private generation = 0;
  private interval: ReturnType<typeof setInterval> | null = null;
  private unsubscribe?: () => void;

  constructor(private readonly transport: Pick<WebSocketClient, "send" | "on">) {}

  getState = (): VacsState => this.state;
  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };
  private publish(state: VacsState): void {
    this.state = state;
    for (const listener of this.listeners) listener();
  }
  start(): void {
    if (this.interval !== null) return;
    this.unsubscribe = this.transport.on(EventType.FrontendVacsResult, result => this.receive(result));
    this.refresh();
    // Native VACS remains the single owner of event subscriptions and recovery.
    this.interval = setInterval(() => this.refresh(), 1000);
  }
  stop(): void {
    if (this.interval !== null) clearInterval(this.interval);
    this.interval = null;
    this.unsubscribe?.();
    this.unsubscribe = undefined;
    this.invalidate();
  }
  invalidate(): void {
    this.generation++;
    for (const request of this.pending.values()) {
      clearTimeout(request.timer);
      request.reject(new Error("VACS relay disconnected. No call action was retried."));
    }
    this.pending.clear();
    this.sequence = 0;
    this.actionPending = false;
    this.publish({status: "unavailable", actionable: false, tooltip: "Connect your own EuroScope plugin to use VACS."});
  }
  private refresh(): void {
    if ([...this.pending.values()].some(request => request.action === "status")) return;
    const generation = this.generation;
    void this.request("status").catch(error => {
      if (this.interval !== null && generation === this.generation) this.publish({status: "unavailable", actionable: false, tooltip: error.message});
    });
  }
  private receive(result: VacsRelayResult): void {
    const pending = this.pending.get(result.request_id);
    if (!pending) return; // Disconnected, timed out, or another board/tab.
    this.pending.delete(result.request_id);
    clearTimeout(pending.timer);
    if (result.state && result.state.sequence > this.sequence) {
      this.sequence = result.state.sequence;
      const state = relayState(result.state);
      this.publish(this.actionPending && pending.action === "status" ? {...state, actionable: false} : state);
    }
    if (result.error) pending.reject(new Error(result.error));
    else if (!result.state) pending.reject(new Error("The EuroScope plugin returned no VACS state."));
    else pending.resolve();
  }
  private request(action: VacsRelayAction, call_id?: string, client_id?: string): Promise<void> {
    if (!this.unsubscribe) return Promise.reject(new Error("Enable VACS voice integration first."));
    return new Promise((resolve, reject) => {
      const request_id = crypto.randomUUID();
      const timer = setTimeout(() => {
        this.pending.delete(request_id);
        if (action !== "status") this.publish({status: "synchronizing", actionable: false,
          tooltip: "Call action unconfirmed. Check VACS; the action was not retried."});
        reject(new Error("VACS relay timed out. Check the EuroScope plugin and VACS; no call action was retried."));
      }, 8000);
      this.pending.set(request_id, {action, resolve, reject, timer});
      if (!this.transport.send({type: ActionType.FrontendVacsRequest, request_id, action, call_id, client_id})) {
        clearTimeout(timer);
        this.pending.delete(request_id);
        reject(new Error("Connect your own EuroScope plugin to use VACS."));
      }
    });
  }
  private async execute(action: VacsRelayAction, callId?: string, clientId?: string): Promise<void> {
    if (this.actionPending || this.state.actionable !== true) throw new Error("VACS controls are unavailable. Check the current call state.");
    this.actionPending = true;
    const generation = this.generation;
    this.publish({...this.state, actionable: false, pending: true});
    try { await this.request(action, callId, clientId); }
    finally { if (generation === this.generation) this.actionPending = false; }
  }
  acceptCall(callId: string): Promise<void> { return this.execute("answer", callId); }
  rejectCall(callId: string): Promise<void> { return this.execute("reject", callId); }
  endCall(callId: string): Promise<void> { return this.execute(this.state.status === "outgoing" ? "cancel" : "end", callId); }
  dialClient(client: ClientInfo): Promise<void> { return this.execute("dial", undefined, client.id); }
}

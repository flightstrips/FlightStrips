import { toast } from "sonner";
import { useVacsStore } from "./vacs-store";
import { VACS_SUBSCRIPTIONS } from "./subscriptions";
import type {
  CallInvite,
  ClientInfo,
  SessionStateSnapshot,
  VacsActions,
  VacsClientMessage,
  VacsServerMessage,
  VacsState,
} from "./types";

const VACS_WS_URL = "ws://localhost:9600/ws";
const INVOKE_TIMEOUT_MS = 10_000;
const PING_INTERVAL_MS = 20_000;
const MAX_BACKOFF_MS = 30_000;

export type VacsStateListener = (state: VacsState) => void;

export interface VacsClientOptions {
  url?: string;
  createWebSocket?: (url: string) => WebSocket;
  onStateChange?: VacsStateListener;
}

interface PendingRequest {
  resolve: (data: unknown) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
}

export class VacsClient implements VacsActions {
  private readonly url: string;
  private readonly createWebSocket: (url: string) => WebSocket;
  private readonly onStateChange?: VacsStateListener;

  private ws: WebSocket | null = null;
  private started = false;
  private nextId = 1;
  private backoffMs = 1_000;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private pingTimer: ReturnType<typeof setInterval> | null = null;
  private readonly pending = new Map<string, PendingRequest>();

  private wsConnected = false;
  private authenticated = false;
  private clientId: string | null = null;
  private connectionState: SessionStateSnapshot["connectionState"] = "disconnected";
  private ownPositionId = "";
  private clients: ClientInfo[] = [];
  private pendingIncoming: CallInvite[] = [];
  private activeCallId: string | null = null;
  private activePeer: ClientInfo | null = null;
  private ambiguous = false;
  private outgoingCallId: string | null = null;
  private inviteByCallId = new Map<string, CallInvite>();

  constructor(options: VacsClientOptions = {}) {
    this.url = options.url ?? VACS_WS_URL;
    this.createWebSocket =
      options.createWebSocket ??
      ((url) => new WebSocket(url));
    this.onStateChange = options.onStateChange;
  }

  getState(): VacsState {
    return this.deriveState();
  }

  start(): void {
    if (this.started) {
      return;
    }
    this.started = true;
    this.connect();
  }

  stop(): void {
    this.started = false;
    this.clearReconnect();
    this.clearPing();
    this.cancelAllPending();
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.wsConnected = false;
    this.resetSessionState();
    this.emitState();
  }

  acceptCall(callId: string): Promise<void> {
    return this.invoke("signaling_accept_call", { callId }).then(() => undefined);
  }

  rejectCall(callId: string): Promise<void> {
    return this.invoke("signaling_end_call", { callId }).then(() => undefined);
  }

  endCall(callId: string): Promise<void> {
    return this.invoke("signaling_end_call", { callId }).then(() => undefined);
  }

  dial(targetCid: string): Promise<void> {
    return this.startCall(targetCid);
  }

  dialByPosition(position: string): Promise<void> {
    return this.startCall({ Position: position });
  }

  /** Test hook: feed a server message without a live socket. */
  handleMessageForTest(raw: string): void {
    this.handleMessage(raw);
  }

  /** Test hook: seed connected idle state without a live WebSocket. */
  simulateOpenForTest(snapshot?: SessionStateSnapshot): void {
    this.wsConnected = true;
    this.backoffMs = 1_000;
    this.applySnapshot(
      snapshot ?? {
        connectionState: "connected",
        sessionInfo: {
          client: {
            id: "1000001",
            displayName: "EKCH_TWR",
            frequency: "118.300",
            positionId: "EKCH_TWR",
          },
          profile: { type: "Unchanged" },
        },
        stations: [],
        clients: [
          {
            id: "1000002",
            displayName: "EKCH_APP",
            frequency: "120.200",
            positionId: "EKCH_APP",
          },
        ],
        clientId: "1000001",
        callConfig: {},
        clientPageSettings: {},
        capabilities: {},
        incomingCalls: [],
        outgoingCall: null,
      },
    );
    this.emitState();
  }

  private connect(): void {
    if (!this.started) {
      return;
    }
    this.clearReconnect();
    try {
      const ws = this.createWebSocket(this.url);
      this.ws = ws;
      ws.addEventListener("open", () => this.onOpen());
      ws.addEventListener("message", (event) => this.handleMessage(String(event.data)));
      ws.addEventListener("close", () => this.onClose());
      ws.addEventListener("error", () => this.onClose());
    } catch {
      this.scheduleReconnect();
    }
  }

  private onOpen(): void {
    this.wsConnected = true;
    this.backoffMs = 1_000;
    void this.bootstrap();
    this.startPing();
  }

  private onClose(): void {
    this.wsConnected = false;
    this.clearPing();
    this.cancelAllPending();
    this.ws = null;
    if (!this.started) {
      return;
    }
    this.resetSessionState();
    this.emitState();
    this.scheduleReconnect();
  }

  private scheduleReconnect(): void {
    if (!this.started) {
      return;
    }
    this.clearReconnect();
    const delay = this.backoffMs;
    this.backoffMs = Math.min(this.backoffMs * 2, MAX_BACKOFF_MS);
    this.reconnectTimer = setTimeout(() => this.connect(), delay);
  }

  private clearReconnect(): void {
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private startPing(): void {
    this.clearPing();
    this.pingTimer = setInterval(() => {
      this.send({ type: "ping" });
    }, PING_INTERVAL_MS);
  }

  private clearPing(): void {
    if (this.pingTimer !== null) {
      clearInterval(this.pingTimer);
      this.pingTimer = null;
    }
  }

  private async bootstrap(): Promise<void> {
    if (!this.started) {
      return;
    }
    try {
      for (const event of VACS_SUBSCRIPTIONS) {
        this.send({ type: "subscribe", event });
      }
      await this.invoke("app_frontend_ready", {});
      if (!this.started) {
        return;
      }
      const snapshot = (await this.invoke("remote_get_session_state", {})) as SessionStateSnapshot;
      if (!this.started) {
        return;
      }
      this.applySnapshot(snapshot);
      this.emitState();
    } catch {
      if (this.started) {
        this.emitState();
      }
    }
  }

  private applySnapshot(snapshot: SessionStateSnapshot): void {
    this.connectionState = snapshot.connectionState;
    this.clientId = snapshot.clientId;
    this.authenticated = snapshot.clientId !== null;
    this.clients = [...snapshot.clients];
    this.pendingIncoming = [...snapshot.incomingCalls];
    for (const invite of snapshot.incomingCalls) {
      this.inviteByCallId.set(invite.callId, invite);
    }
    this.ownPositionId = snapshot.sessionInfo?.client.positionId ?? "";
    this.ambiguous = false;
    this.activeCallId = null;
    this.activePeer = null;
    this.outgoingCallId = snapshot.outgoingCall?.callId ?? null;
    if (snapshot.outgoingCall) {
      this.inviteByCallId.set(snapshot.outgoingCall.callId, snapshot.outgoingCall);
    }
  }

  private resetSessionState(): void {
    this.authenticated = false;
    this.clientId = null;
    this.connectionState = "disconnected";
    this.ownPositionId = "";
    this.clients = [];
    this.pendingIncoming = [];
    this.activeCallId = null;
    this.activePeer = null;
    this.ambiguous = false;
    this.outgoingCallId = null;
    this.inviteByCallId.clear();
  }

  private deriveState(): VacsState {
    if (!this.wsConnected) {
      return { status: "unavailable" };
    }
    if (!this.authenticated || !this.clientId) {
      return { status: "unauthenticated" };
    }
    if (this.ambiguous) {
      return { status: "ambiguous" };
    }
    if (this.connectionState !== "connected") {
      return { status: "disconnected" };
    }
    if (this.activeCallId) {
      return {
        status: "connected",
        callId: this.activeCallId,
        peer: this.activePeer,
      };
    }
    if (this.pendingIncoming.length > 0 && this.ownPositionId) {
      return {
        status: "incoming",
        calls: [...this.pendingIncoming],
        clients: [...this.clients],
        ownPositionId: this.ownPositionId,
      };
    }
    if (this.ownPositionId) {
      return {
        status: "idle",
        clients: [...this.clients],
        ownPositionId: this.ownPositionId,
      };
    }
    return { status: "disconnected" };
  }

  private emitState(): void {
    this.onStateChange?.(this.getState());
  }

  private send(message: VacsClientMessage): void {
    if (this.ws && this.ws.readyState === 1) {
      this.ws.send(JSON.stringify(message));
    }
  }

  private invoke(cmd: string, args: Record<string, unknown>): Promise<unknown> {
    const id = String(this.nextId++);
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error("VACS invoke timeout"));
      }, INVOKE_TIMEOUT_MS);
      this.pending.set(id, { resolve, reject, timer });
      this.send({ type: "invoke", id, cmd, args });
    });
  }

  private handleMessage(raw: string): void {
    let message: VacsServerMessage;
    try {
      message = JSON.parse(raw) as VacsServerMessage;
    } catch {
      return;
    }

    if (message.type === "response") {
      this.handleResponse(message);
      return;
    }
    if (message.type === "event") {
      this.handleEvent(message.name, message.payload);
    }
  }

  private handleResponse(response: Extract<VacsServerMessage, { type: "response" }>): void {
    const pending = this.pending.get(response.id);
    if (!pending) {
      return;
    }
    clearTimeout(pending.timer);
    this.pending.delete(response.id);

    if (response.ok) {
      pending.resolve(response.data ?? null);
      return;
    }

    const errorType = response.error?.type ?? "";
    if (errorType === "urn:vacs:error:remote:desktop-only") {
      console.warn("VACS desktop-only command rejected", response);
      pending.resolve(null);
      return;
    }
    if (errorType === "urn:vacs:error:remote:invalid-argument") {
      console.warn("VACS invalid argument", response);
      toast.error("Could not complete voice action — invalid target.");
    } else if (errorType === "urn:vacs:error:remote:timeout") {
      toast.error("VACS did not respond — please check that it is running.");
    }
    pending.reject(new Error(response.error?.detail ?? response.error?.title ?? "VACS error"));
  }

  private handleEvent(name: string, payload: unknown): void {
    switch (name) {
      case "auth:authenticated":
        this.authenticated = true;
        this.clientId = String(payload);
        void this.refetchSession();
        break;
      case "auth:unauthenticated":
        this.authenticated = false;
        this.clientId = null;
        this.resetSessionState();
        this.emitState();
        break;
      case "signaling:connected": {
        const info = payload as { client: ClientInfo };
        this.connectionState = "connected";
        this.ownPositionId = info.client.positionId ?? info.client.displayName;
        this.emitState();
        break;
      }
      case "signaling:disconnected":
        this.connectionState = "disconnected";
        this.ownPositionId = "";
        this.emitState();
        break;
      case "signaling:client-list":
        this.clients = payload as ClientInfo[];
        this.emitState();
        break;
      case "signaling:client-connected":
        this.clients = [...this.clients, payload as ClientInfo];
        this.emitState();
        break;
      case "signaling:client-disconnected": {
        const cid = String(payload);
        this.clients = this.clients.filter((c) => c.id !== cid);
        this.emitState();
        break;
      }
      case "signaling:call-invite":
      case "signaling:add-incoming-to-call-list": {
        const invite = this.normalizeInvite(payload);
        if (!this.pendingIncoming.some((c) => c.callId === invite.callId)) {
          this.pendingIncoming.push(invite);
        }
        this.inviteByCallId.set(invite.callId, invite);
        this.emitState();
        break;
      }
      case "signaling:call-end":
      case "signaling:call-reject":
      case "signaling:force-call-end":
        this.terminateCall(String(payload));
        break;
      case "signaling:ambiguous-position":
        this.ambiguous = true;
        this.emitState();
        break;
      case "signaling:outgoing-call-accepted":
        break;
      case "signaling:update-call-list":
        break;
      case "webrtc:call-connected": {
        const callId = String(payload);
        this.onWebRtcConnected(callId);
        break;
      }
      case "webrtc:call-disconnected":
        this.terminateCall(String(payload));
        break;
      case "webrtc:call-error": {
        const err = payload as { callId: string; reason: string };
        toast.error(err.reason);
        this.terminateCall(err.callId);
        break;
      }
      case "error":
        console.warn("VACS error event", payload);
        break;
      default:
        break;
    }
  }

  private normalizeInvite(payload: unknown): CallInvite {
    if (this.isCallInvite(payload)) {
      return payload;
    }
    const entry = payload as { callId: string; source: CallInvite["source"] };
    return {
      callId: entry.callId,
      source: entry.source,
      target: {},
      prio: false,
    };
  }

  private isCallInvite(value: unknown): value is CallInvite {
    return (
      typeof value === "object" &&
      value !== null &&
      "callId" in value &&
      "source" in value
    );
  }

  private onWebRtcConnected(callId: string): void {
    this.activeCallId = callId;
    this.pendingIncoming = this.pendingIncoming.filter((c) => c.callId !== callId);
    const invite = this.inviteByCallId.get(callId);
    const peerCid = invite?.source.clientId;
    this.activePeer = peerCid
      ? (this.clients.find((c) => c.id === peerCid) ?? null)
      : null;
    this.outgoingCallId = null;
    this.emitState();
  }

  private terminateCall(callId: string): void {
    this.pendingIncoming = this.pendingIncoming.filter((c) => c.callId !== callId);
    this.inviteByCallId.delete(callId);
    if (this.outgoingCallId === callId) {
      this.outgoingCallId = null;
    }
    if (this.activeCallId === callId) {
      this.activeCallId = null;
      this.activePeer = null;
    }
    this.emitState();
  }

  private async refetchSession(): Promise<void> {
    if (!this.wsConnected) {
      return;
    }
    try {
      const snapshot = (await this.invoke(
        "remote_get_session_state",
        {},
      )) as SessionStateSnapshot;
      this.applySnapshot(snapshot);
      this.emitState();
    } catch {
      // ignore
    }
  }

  private async startCall(target: string | { Position: string }): Promise<void> {
    if (!this.ownPositionId) {
      throw new Error("No VACS position");
    }
    const callId = (await this.invoke("signaling_start_call", {
      target,
      source: this.ownPositionId,
      prio: false,
    })) as string;
    this.outgoingCallId = callId;
    this.inviteByCallId.set(callId, {
      callId,
      source: { clientId: this.clientId ?? "" },
      target,
      prio: false,
    });
  }

  private cancelAllPending(): void {
    for (const [, pending] of this.pending) {
      clearTimeout(pending.timer);
      pending.reject(new Error("VACS cancelled"));
    }
    this.pending.clear();
  }
}

let singleton: VacsClient | null = null;

export function getVacsClient(): VacsClient {
  if (!singleton) {
    singleton = new VacsClient({
      onStateChange: (state) => {
        useVacsStore.getState().setState(state);
      },
    });
  }
  return singleton;
}

export function resetVacsClientForTest(): void {
  singleton?.stop();
  singleton = null;
}

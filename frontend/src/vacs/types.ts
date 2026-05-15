export interface ClientInfo {
  id: string;
  displayName: string;
  frequency: string;
  positionId?: string;
}

export interface CallSource {
  clientId: string;
  positionId?: string;
  stationId?: string;
}

export interface CallInvite {
  callId: string;
  source: CallSource;
  target: unknown;
  prio: boolean;
}

export interface CallError {
  callId: string;
  reason: string;
}

export interface SessionInfo {
  client: ClientInfo;
  profile: { type: string };
}

export interface SessionStateSnapshot {
  connectionState: "disconnected" | "connecting" | "connected" | "test";
  sessionInfo: SessionInfo | null;
  stations: unknown[];
  clients: ClientInfo[];
  clientId: string | null;
  callConfig: unknown;
  clientPageSettings: unknown;
  capabilities: unknown;
  incomingCalls: CallInvite[];
  outgoingCall: CallInvite | null;
}

export type VacsState =
  | { status: "unavailable" }
  | { status: "unauthenticated" }
  | { status: "disconnected" }
  | { status: "ambiguous" }
  | { status: "idle"; clients: ClientInfo[]; ownPositionId: string }
  | { status: "incoming"; calls: CallInvite[]; clients: ClientInfo[]; ownPositionId: string }
  | { status: "connected"; callId: string; peer: ClientInfo | null };

export type VacsActions = {
  acceptCall(callId: string): Promise<void>;
  rejectCall(callId: string): Promise<void>;
  endCall(callId: string): Promise<void>;
  dial(targetCid: string): Promise<void>;
  dialByPosition(position: string): Promise<void>;
};

export type VacsInvokeMessage = {
  type: "invoke";
  id: string;
  cmd: string;
  args: Record<string, unknown>;
};

export type VacsSubscribeMessage = {
  type: "subscribe";
  event: string;
};

export type VacsPingMessage = {
  type: "ping";
};

export type VacsClientMessage = VacsInvokeMessage | VacsSubscribeMessage | VacsPingMessage;

export type VacsResponseMessage = {
  type: "response";
  id: string;
  ok: boolean;
  data?: unknown;
  error?: {
    type: string;
    title?: string;
    detail?: string;
    isNonCritical?: boolean;
  };
};

export type VacsEventMessage = {
  type: "event";
  name: string;
  payload: unknown;
};

export type VacsServerMessage =
  | VacsResponseMessage
  | VacsEventMessage
  | { type: "pong" };

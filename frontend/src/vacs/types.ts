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

export type VacsState = { actionable?: boolean; pending?: boolean; tooltip?: string; waiting?: number } & (
  | { status: "disabled" | "reconnecting" | "synchronizing" }
  | { status: "unavailable" }
  | { status: "unauthenticated" }
  | { status: "disconnected" }
  | { status: "ambiguous" }
  | { status: "idle"; clients: ClientInfo[]; ownPositionId: string; ownClientId: string | null }
  | {
      status: "incoming";
      calls: CallInvite[];
      clients: ClientInfo[];
      ownPositionId: string;
      ownClientId: string | null;
    }
  | {
      status: "outgoing";
      callId: string;
      peer: ClientInfo;
      clients: ClientInfo[];
      ownPositionId: string;
      ownClientId: string | null;
    }
  | { status: "connected" | "connecting"; callId: string; peer: ClientInfo | null });

export type VacsRelayAction = "status" | "dial" | "answer" | "reject" | "cancel" | "end";

/** Native plugin snapshot, serialized by the backend with protobuf JSON tags. */
export interface VacsRelaySnapshot {
  status: string;
  label?: string;
  tooltip?: string;
  call_id?: string;
  pending?: boolean;
  actionable?: boolean;
  waiting?: number;
  clients?: Array<{id: string; display_name?: string; frequency?: string; position_id?: string}>;
  own_client_id?: string;
  own_position_id?: string;
  sequence: number;
}

export interface VacsRelayResult {
  request_id: string;
  state?: VacsRelaySnapshot;
  error?: string;
}

export type VacsActions = {
  acceptCall(callId: string): Promise<void>;
  rejectCall(callId: string): Promise<void>;
  endCall(callId: string): Promise<void>;
  dialClient(client: ClientInfo): Promise<void>;
};

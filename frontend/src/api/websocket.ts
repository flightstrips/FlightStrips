import {
  ActionType,
  EventType,
  type ActionRejectedEvent,
  type FrontendAircraftDisconnectEvent,
  type FrontendAssignedSquawkEvent,
  type FrontendBayEvent, type FrontendBroadcastEvent, type FrontendBulkBayEvent, type FrontendCdmDataBatchEvent, type FrontendCdmDataEvent, type FrontendCdmWaitEvent,
  type FrontendClearedAltitudeEvent,
  type FrontendCommunicationTypeEvent,
  type FrontendControllerOfflineEvent,
  type FrontendControllerOnlineEvent,
  type FrontendControllerUpdateEvent,
  type FrontendDisconnectEvent,
  type FrontendGoAroundEvent,
  type FrontendInitialEvent, type FrontendLayoutUpdateEvent, type FrontendOwnersUpdateEvent, type FrontendCoordinationForceAssumeResultEvent,
  type FrontendPdcStateUpdateEvent,
  type FrontendReleasePointEvent,
  type FrontendMarkedEvent,
  type FrontendCoordinationTransferBroadcastEvent,
  type FrontendCoordinationAssumeBroadcastEvent,
  type FrontendCoordinationRejectBroadcastEvent,
  type FrontendCoordinationFreeBroadcastEvent,
  type FrontendCoordinationTagRequestBroadcastEvent,
  type FrontendRequestedAltitudeEvent,
  type FrontendRunwayConfigurationEvent,
  type FrontendSendEvent,
  type FrontendSetHeadingEvent,
  type FrontendSquawkEvent,
  type FrontendStandEvent,
  type FrontendStandStatusSnapshotEvent,
  type FrontendStandAssignmentUpdateEvent,
  type FrontendStandAssignmentRemovedEvent,
  type FrontendStandBlockUpdateEvent,
  type FrontendStripUpdateEvent,
  type FrontendTacticalStripCreatedEvent,
  type FrontendTacticalStripDeletedEvent,
  type FrontendTacticalStripUpdatedEvent,
  type FrontendTacticalStripMovedEvent,
  type FrontendMessageReceivedEvent,
  type FrontendAtisUpdateEvent,
  type WebSocketEvent,
  type AvailableSidsEvent,
} from "./models";
import type {AMANCommandRejectedEvent, AMANCoordinationStateEvent, AMANStateEvent} from "./aman";
import {create, fromBinary, toBinary} from "@bufbuild/protobuf";
import {FrontendFrameSchema} from "./generated/cluster/v1/wire_pb";
import {CommandOutcome_Status} from "./generated/cluster/v1/storage_pb";
import {encodeAction} from "./commands";
import {FrontendProjection} from "./projection";


type EventMap = {
  [EventType.FrontendInitial]: FrontendInitialEvent;
  [EventType.FrontendGoAround]: FrontendGoAroundEvent;
  [EventType.FrontendStripUpdate]: FrontendStripUpdateEvent;
  [EventType.FrontendControllerOnline]: FrontendControllerOnlineEvent;
  [EventType.FrontendControllerUpdate]: FrontendControllerUpdateEvent;
  [EventType.FrontendControllerOffline]: FrontendControllerOfflineEvent;
  [EventType.FrontendAssignedSquawk]: FrontendAssignedSquawkEvent;
  [EventType.FrontendSquawk]: FrontendSquawkEvent;
  [EventType.FrontendRequestedAltitude]: FrontendRequestedAltitudeEvent;
  [EventType.FrontendClearedAltitude]: FrontendClearedAltitudeEvent;
  [EventType.FrontendBay]: FrontendBayEvent;
  [EventType.FrontendBulkBay]: FrontendBulkBayEvent;
  [EventType.FrontendDisconnect]: FrontendDisconnectEvent;
  [EventType.FrontendAircraftDisconnect]: FrontendAircraftDisconnectEvent;
  [EventType.FrontendStand]: FrontendStandEvent;
  [EventType.FrontendSetHeading]: FrontendSetHeadingEvent;
  [EventType.FrontendCommunicationType]: FrontendCommunicationTypeEvent;
  [EventType.FrontendOwnersUpdate]: FrontendOwnersUpdateEvent;
  [EventType.FrontendCoordinationForceAssumeResult]: FrontendCoordinationForceAssumeResultEvent;
  [EventType.FrontendLayoutUpdate]: FrontendLayoutUpdateEvent;
  [EventType.FrontendBroadcast]: FrontendBroadcastEvent;
  [EventType.FrontendCdmData]: FrontendCdmDataEvent;
  [EventType.FrontendCdmDataBatch]: FrontendCdmDataBatchEvent;
  [EventType.FrontendCdmWait]: FrontendCdmWaitEvent;
  [EventType.FrontendReleasePoint]: FrontendReleasePointEvent;
  [EventType.FrontendMarked]: FrontendMarkedEvent;
  [EventType.FrontendPdcStateChange]: FrontendPdcStateUpdateEvent;
  [EventType.FrontendCoordinationTransferBroadcast]: FrontendCoordinationTransferBroadcastEvent;
  [EventType.FrontendCoordinationAssumeBroadcast]: FrontendCoordinationAssumeBroadcastEvent;
  [EventType.FrontendCoordinationRejectBroadcast]: FrontendCoordinationRejectBroadcastEvent;
  [EventType.FrontendCoordinationFreeBroadcast]: FrontendCoordinationFreeBroadcastEvent;
  [EventType.FrontendCoordinationTagRequestBroadcast]: FrontendCoordinationTagRequestBroadcastEvent;
  [EventType.FrontendRunWayConfiguration]: FrontendRunwayConfigurationEvent;
  [EventType.FrontendTacticalStripCreated]: FrontendTacticalStripCreatedEvent;
  [EventType.FrontendTacticalStripDeleted]: FrontendTacticalStripDeletedEvent;
  [EventType.FrontendTacticalStripUpdated]: FrontendTacticalStripUpdatedEvent;
  [EventType.FrontendTacticalStripMoved]: FrontendTacticalStripMovedEvent;
  [EventType.FrontendMessageReceived]: FrontendMessageReceivedEvent;
  [EventType.FrontendAtisUpdate]: FrontendAtisUpdateEvent;
  [EventType.FrontendActionRejected]: ActionRejectedEvent;
  [EventType.FrontendAvailableSids]: AvailableSidsEvent;
  [EventType.FrontendStandStatusSnapshot]: FrontendStandStatusSnapshotEvent;
  [EventType.FrontendStandAssignmentUpdate]: FrontendStandAssignmentUpdateEvent;
  [EventType.FrontendStandAssignmentRemoved]: FrontendStandAssignmentRemovedEvent;
  [EventType.FrontendStandBlockUpdate]: FrontendStandBlockUpdateEvent;
  [EventType.FrontendAMANState]: AMANStateEvent;
  [EventType.FrontendAMANCommandRejected]: AMANCommandRejectedEvent;
  [EventType.FrontendAMANCoordinationState]: AMANCoordinationStateEvent;
};

type WebSocketClientDelegate = {
  onConnected?: () => void;
  onDisconnected?: () => void;
};

export class WebSocketClient {
  private socket: WebSocket | null = null;
  private eventHandlers: Map<EventType, Array<(data: unknown) => void>> = new Map();
  private readonly url: string;
  private token: string | null = null;
  private readOnly = false;
  private reconnectAttempts = 0;
  private projection = new FrontendProjection(event => this.dispatch(event));
  private pending = new Map<string, {event: FrontendSendEvent; frame: unknown}>();
  private forceAssumeWait = new Map<string, {legacyId: string; callsign: string}>();
  private presenceTimer: ReturnType<typeof setInterval> | null = null;
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  private manuallyClosed = false;
  private delegate?: WebSocketClientDelegate;

  constructor(url: string, delegate?: WebSocketClientDelegate) {
    this.url = url;
    this.delegate = delegate;
  }

  setToken(token: string): void {
    const tokenChanged = this.token !== token;
    this.token = token;
    if (tokenChanged && this.isConnected()) {
      this.reconnect();
    }
  }

  setReadOnly(readOnly: boolean): void {
    this.readOnly = readOnly;
  }

  connect(): Promise<void> {
    return new Promise((resolve,) => {
      this.manuallyClosed = false;
      const socket = new WebSocket(this.url, "flightstrips.frontend.pb.v2");
      socket.binaryType = "arraybuffer";
      this.socket = socket;

      socket.onopen = () => {
        if (socket.protocol !== "flightstrips.frontend.pb.v2") {
          socket.close(1008, "unsupported frontend protocol");
          return;
        }
        console.log('WebSocket connection established');
        this.reconnectAttempts = 0;
        if (this.token) {
          this.sendAuthenticationEvent();
          if (this.pending.size) this.write({case: "statusQuery", value: {requestIds: [...this.pending.keys()].slice(0, 100)}});
        }
        if (this.presenceTimer) clearInterval(this.presenceTimer);
        this.presenceTimer = setInterval(() => this.projection.expirePresence(), 1000);
        if (this.delegate?.onConnected) {
          this.delegate.onConnected();
        }
        resolve();
      };

      socket.onerror = (error) => {
        console.error('WebSocket error:', error);
      };

      socket.onclose = (event) => {
        // Ignore close events from a socket that has already been replaced (e.g. during reconnect)
        if (this.socket !== socket) return;
        console.log('WebSocket connection closed:', event.code, event.reason);
        if (this.presenceTimer) { clearInterval(this.presenceTimer); this.presenceTimer = null; }
        if (this.delegate?.onDisconnected) {
          this.delegate.onDisconnected();
        }
        if (!this.manuallyClosed) {
          this.retryConnection();
        }
      };

      socket.onmessage = (event) => {
        try {
          if (!(event.data instanceof ArrayBuffer)) throw new Error("binary frame required");
          const frame = fromBinary(FrontendFrameSchema, new Uint8Array(event.data));
          if (frame.protocolRevision !== 2) throw new Error("unsupported frontend revision");
          switch (frame.frame.case) {
            case "initial": this.projection.initial(frame.frame.value); break;
            case "delta": this.projection.delta(frame.frame.value); break;
            case "observation": this.projection.observation(frame.frame.value); break;
            case "actionResult": {
              const result = frame.frame.value;
              const pending = this.pending.get(result.requestId);
              if (result.status === CommandOutcome_Status.FAILED && pending) {
                if (pending.event.type.startsWith("aman.")) {
                  this.dispatch({type: EventType.FrontendAMANCommandRejected, version: 1,
                    data: {command_id: result.requestId, code: result.reasonCode,
                      message: result.detail || result.reasonCode,
                      current_revision: Number(result.aggregateRevision), retryable: false}});
                } else {
                  this.dispatch({type: EventType.FrontendActionRejected, action: pending.event.type,
                    reason: result.detail || result.reasonCode, code: result.reasonCode,
                    request_id: pending.event.type === ActionType.FrontendCoordinationForceAssumeRequest
                      ? pending.event.request_id : result.requestId});
                }
                this.forceAssumeWait.delete(result.requestId);
              }
              if (result.status !== CommandOutcome_Status.ACCEPTED) this.pending.delete(result.requestId);
              break;
            }
            case "statusMissing": {
              const pending = this.pending.get(frame.frame.value.requestId);
              if (pending) this.write(pending.frame);
              break;
            }
            case "error": throw new Error(frame.frame.value.detail || "frontend protocol error");
            case "heartbeat": break;
            default: throw new Error("unexpected server frame");
          }
        } catch (error) {
          console.error('Invalid frontend frame:', error);
          socket.close(1002, "invalid frontend frame");
        }
      };
    });
  }

  reconnect(): void {
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
    this.connect().catch(() => {
      // Connection errors are handled by socket.onerror; retryConnection kicks in via onclose
    });
  }

  private retryConnection() {
    this.reconnectAttempts += 1;
    const delay = Math.min(1000 * 2 ** this.reconnectAttempts, 30000); // Exponential backoff, max 30s

    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
    }

    this.reconnectTimeout = setTimeout(() => {
      console.log(`Reconnecting WebSocket (attempt ${this.reconnectAttempts})...`);
      this.connect().catch(() => {
        // error is logged in connect()
        // retry logic continues in onclose
      });
    }, delay);
  }

  disconnect(): void {
    this.manuallyClosed = true;
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
    if (this.presenceTimer) { clearInterval(this.presenceTimer); this.presenceTimer = null; }
  }

  on<T extends EventType>(eventType: T, handler: (data: EventMap[T]) => void): void {
    if (!this.eventHandlers.has(eventType)) {
      this.eventHandlers.set(eventType, []);
    }
    this.eventHandlers.get(eventType)!.push(handler as never);
  }

  send(event: FrontendSendEvent): void {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      console.error('WebSocket is not connected');
      return;
    }
    if (this.readOnly && event.type !== ActionType.FrontendToken) {
      console.warn('WebSocket client is read-only');
      return;
    }

    try {
      if (event.type === ActionType.FrontendToken) { this.reconnect(); return; }
      const encoded = encodeAction(event, this.projection.entityRevisions, this.projection);
      const frame = {case: "command", value: {requestId: encoded.requestId, action: encoded.action,
        expectedEntityRevision: encoded.expectedEntityRevision}};
      this.pending.set(encoded.requestId, {event, frame});
      if (event.type === ActionType.FrontendCoordinationForceAssumeRequest && event.request_id)
        this.forceAssumeWait.set(encoded.requestId, {legacyId: event.request_id, callsign: event.callsign});
      this.write(frame);
    } catch (error) {
      console.error('Error sending frontend command:', error);
      this.dispatch({type: EventType.FrontendActionRejected, action: event.type, reason: String(error)});
    }
  }

  private dispatch(data: WebSocketEvent): void {
    this.eventHandlers.get(data.type as EventType)?.forEach(handler => handler(data));
    if (data.type === EventType.FrontendStripUpdate) {
      for (const [id, pending] of this.forceAssumeWait) {
        if (pending.callsign !== data.callsign || data.owner !== this.projection.myPosition) continue;
        this.forceAssumeWait.delete(id);
        this.dispatch({type: EventType.FrontendCoordinationForceAssumeResult,
          callsign: data.callsign, request_id: pending.legacyId,
          owner: data.owner, next_owners: data.next_controllers});
      }
    }
  }

  private write(frame: unknown): void {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) throw new Error("WebSocket is not connected");
    this.socket.send(toBinary(FrontendFrameSchema, create(FrontendFrameSchema, {protocolRevision: 2, frame} as never)));
  }

  private sendAuthenticationEvent(): void {
    if (this.token) {
      this.write({case: "authenticate", value: {bearerToken: this.token, airport: "", sessionName: ""}});
    }
  }

  isConnected(): boolean {
    return this.socket !== null && this.socket.readyState === WebSocket.OPEN;
  }
}

export function createWebSocketClient(
  url: string,
  delegate?: WebSocketClientDelegate
): WebSocketClient {
  return new WebSocketClient(url, delegate);
}

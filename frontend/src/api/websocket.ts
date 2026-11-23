import {
  type FrontendAircraftSelectionEvent,
  ActionType,
  EventType,
  type ActionRejectedEvent,
  type FrontendActionReconciliationEvent,
  type FrontendAircraftDisconnectEvent,
  type FrontendAssignedSquawkEvent,
  type FrontendBayEvent, type FrontendBroadcastEvent, type FrontendBulkBayEvent, type FrontendCdmDataBatchEvent, type FrontendCdmDataEvent, type FrontendCdmWaitEvent, type FrontendCdmDeicePlatformUpdateEvent,
  type FrontendClearedAltitudeEvent,
  type FrontendCommunicationTypeEvent,
  type FrontendControllerOfflineEvent,
  type FrontendControllerOnlineEvent,
  type FrontendControllerUpdateEvent,
  type FrontendDisconnectEvent,
  type FrontendGoAroundEvent,
  type FrontendInitialEvent, type FrontendLayoutUpdateEvent, type FrontendOwnersUpdateEvent, type FrontendCoordinationForceAssumeResultEvent,
  type FrontendMasterInfoEvent,
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
import {boardContext, sameBoard, type BoardContext, type OwnMembership} from "./sessions";
import { FRONTEND_VERSION } from "@/lib/app-version";
import {ConnectionRecovery, type ConnectionPhase} from "./connection-recovery";
export type {ConnectionPhase} from "./connection-recovery";
import type {InboundDeparturesEvent} from "./inbound-departures";
import type {AMANArrivalSequenceRebuiltEvent, AMANCommandRejectedEvent, AMANCoordinationStateEvent, AMANFMPAuthorityEvent, AMANStateEvent} from "./aman";


type EventMap = {
  [EventType.FrontendVacsResult]: import("@/vacs/types").VacsRelayResult;
  [EventType.FrontendAircraftSelection]: FrontendAircraftSelectionEvent;
 [EventType.FrontendConnectedFlightResults]:import("./strip-board").ConnectedFlightSearchResult;
  [EventType.FrontendMasterInfo]: FrontendMasterInfoEvent;
  [EventType.FrontendFrequencyMessageResult]: import("@/lib/text-pilot").FrequencyMessageResult;
  [EventType.FrontendInboundDepartures]: InboundDeparturesEvent;
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
  [EventType.FrontendCdmDeicePlatformUpdate]: FrontendCdmDeicePlatformUpdateEvent;
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
  [EventType.FrontendActionReconciliation]: FrontendActionReconciliationEvent;
  [EventType.FrontendAvailableSids]: AvailableSidsEvent;
  [EventType.FrontendStandStatusSnapshot]: FrontendStandStatusSnapshotEvent;
  [EventType.FrontendStandAssignmentUpdate]: FrontendStandAssignmentUpdateEvent;
  [EventType.FrontendStandAssignmentRemoved]: FrontendStandAssignmentRemovedEvent;
  [EventType.FrontendStandBlockUpdate]: FrontendStandBlockUpdateEvent;
  [EventType.FrontendAMANState]: AMANStateEvent;
  [EventType.FrontendAMANCommandRejected]: AMANCommandRejectedEvent;
 [EventType.FrontendAMANArrivalSequenceRebuilt]: AMANArrivalSequenceRebuiltEvent;
  [EventType.FrontendAMANCoordinationState]: AMANCoordinationStateEvent;
  [EventType.FrontendAMANFMPAuthority]: AMANFMPAuthorityEvent;
};

type WebSocketClientDelegate = {
  onConnected?: () => void;
  onDisconnected?: () => void;
  onStateChanged?: (phase: ConnectionPhase) => void;
};

export class WebSocketClient {
  private socket: WebSocket | null = null;
  private eventHandlers: Map<EventType, Array<(data: unknown) => void>> = new Map();
  private readonly url: string;
  private token: string | null = null;
  private readOnly = false;
  private board?: BoardContext;
  private disposed = false;
  private invalidationHandlers = new Set<() => void>();
  private reconnectAttempts = 0;
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  private manuallyClosed = false;
  private delegate?: WebSocketClientDelegate;
  private pendingConnection: {promise: Promise<void>; resolve: () => void; reject: (error: Error) => void} | null = null;
  private recovery: ConnectionRecovery | null = null;
  private previousFailure = "";

  constructor(url: string, delegate?: WebSocketClientDelegate, private readonly selection?: OwnMembership) {
    this.url = url;
    this.delegate = delegate;
  }

  setToken(token: string): void {
    if (this.disposed) return;
    const tokenChanged = this.token !== token;
    this.token = token;
    if (tokenChanged && this.isConnected()) {
      this.sendAuthenticationEvent();
    }
  }

  setReadOnly(readOnly: boolean): void {
    this.readOnly = readOnly;
  }

  hasBoardSelection(): boolean { return this.selection !== undefined; }
  onInvalidated(handler: () => void): () => void {
    this.invalidationHandlers.add(handler);
    return () => this.invalidationHandlers.delete(handler);
  }
  private invalidate(): void {
    this.readOnly = true;
    this.invalidationHandlers.forEach(handler => handler());
  }
  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.disconnect();
    this.eventHandlers.clear();
    this.invalidationHandlers.clear();
  }
  connect(): Promise<void> {
    if (this.disposed) return Promise.reject(new Error("Board subscription is disposed"));
    if (this.pendingConnection) return this.pendingConnection.promise;
    if (this.isConnected()) return Promise.resolve();
    // Token refresh and a retry timer may request the same connection. There
    // must be only one socket and subscription for this board at a time.
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    let resolve!: () => void;
    let reject!: (error: Error) => void;
    const promise = new Promise<void>((done, fail) => {resolve = done; reject = fail;});
    this.pendingConnection = {promise, resolve, reject};
    try {
      this.manuallyClosed = false;
      const socket = new WebSocket(this.url);
      this.socket = socket;
      this.recovery = new ConnectionRecovery({
        send: message => {if (this.socket === socket) socket.send(JSON.stringify(message));},
        fail: reason => {if (this.socket === socket) this.loseConnection(reason);},
        phase: phase => this.delegate?.onStateChanged?.(phase),
        previousFailure: this.previousFailure, attempt: this.reconnectAttempts,
      });
      this.board = this.selection ? boardContext(this.selection, crypto.randomUUID()) : undefined;
      if (this.selection) this.readOnly = true; // Wait for the authorized snapshot.

      socket.onopen = () => {
        if (this.socket !== socket) return;
        console.log('WebSocket connection established');
        this.recovery?.opened();
        if (this.token) {
          this.sendAuthenticationEvent();
        }
        if (this.delegate?.onConnected) {
          this.delegate.onConnected();
        }
        this.finishConnection();
      };

      socket.onerror = (error) => {
        if (this.socket !== socket) return;
        console.error('WebSocket error:', error);
      };

      socket.onclose = (event) => {
        // Ignore close events from a socket that has already been replaced (e.g. during reconnect)
        if (this.socket !== socket) return;
        this.finishConnection(new Error('WebSocket closed before opening'));
        console.log('WebSocket connection closed:', event.code, event.reason);
        this.loseConnection("socket_closed");
      };

      socket.onmessage = (event) => {
        if (this.socket !== socket) return;
        try {
          const data = JSON.parse(event.data) as WebSocketEvent & {board?: BoardContext};
          if ((data as {type: string}).type === "connection_health") {
            this.recovery?.receiveHealth(data as unknown as Record<string, unknown>);
            return;
          }
          if (this.selection) {
            if (!sameBoard(data.board, this.board)) return;
            if (data.type === EventType.FrontendInitial &&
                (data.session_id !== this.selection.session_id || data.airport !== this.selection.airport || data.definition?.definition_revision !== this.selection.definition_revision || data.definition.view !== this.selection.view)) return;
          }
          const handlers = this.eventHandlers.get(data.type as EventType);
          if (handlers) {
            handlers.forEach(handler => handler(data));
          }
          if (data.type === EventType.FrontendInitial) {this.reconnectAttempts = 0; this.recovery?.ready();}
          else if (data.type === EventType.FrontendDisconnect) this.recovery?.waitingForPlugin();
        } catch (error) {
          console.error('Error parsing WebSocket message:', error);
        }
      };
    } catch (error) {
      // Setup failures may happen before onclose is installed.
      const failed = this.socket;
      this.socket = null;
      this.recovery?.stop();
      failed?.close();
      this.finishConnection(error instanceof Error ? error : new Error(String(error)));
      this.previousFailure = "setup_failed";
      this.retryConnection();
    }
    return promise;
  }

  private finishConnection(error?: Error): void {
    const pending = this.pendingConnection;
    this.pendingConnection = null;
    if (error) pending?.reject(error);
    else pending?.resolve();
  }

  reconnect(): void {
    if (this.disposed) return;
    this.disconnect();
    this.connect().catch(() => {
      // Connection errors are handled by socket.onerror; retryConnection kicks in via onclose
    });
  }

  private loseConnection(reason: string): void {
    const previous = this.socket;
    this.socket = null;
    this.recovery?.stop();
    this.previousFailure = reason;
    this.finishConnection(new Error(reason));
    this.invalidate();
    this.delegate?.onDisconnected?.();
    this.delegate?.onStateChanged?.("disconnected");
    previous?.close(4000, reason);
    if (!this.manuallyClosed && !this.disposed) this.retryConnection();
  }

  requestSnapshot(): void {
    if (this.disposed || !this.isConnected()) return;
    this.invalidate();
    if (!this.recovery?.requestSnapshot()) this.reconnect();
  }

  private retryConnection() {
    this.reconnectAttempts += 1;
    const ceiling = Math.min(1000 * 2 ** Math.min(this.reconnectAttempts, 5), 30000);
    const delay = Math.floor(ceiling * (0.5 + Math.random() * 0.5));
    console.info("WebSocket retry scheduled", {attempt: this.reconnectAttempts, retry_in_ms: delay, previous_failure: this.previousFailure});
    this.delegate?.onStateChanged?.("reconnecting");

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
    this.recovery?.stop();
    this.delegate?.onStateChanged?.("disconnected");
    this.finishConnection(new Error('WebSocket connection cancelled'));
    this.invalidate();
    this.delegate?.onDisconnected?.();
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    if (this.socket) {
      const previous = this.socket;
      this.socket = null;
      previous.close();
    }
  }

  on<T extends EventType>(eventType: T, handler: (data: EventMap[T]) => void): () => void {
    if (!this.eventHandlers.has(eventType)) {
      this.eventHandlers.set(eventType, []);
    }
    this.eventHandlers.get(eventType)!.push(handler as never);
    return () => {
      const handlers = this.eventHandlers.get(eventType);
      if (handlers) this.eventHandlers.set(eventType, handlers.filter(candidate => candidate !== handler));
    };
  }

  send(event: FrontendSendEvent): boolean {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      console.error('WebSocket is not connected');
      return false;
    }
    if (this.readOnly && event.type !== ActionType.FrontendToken && event.type !== ActionType.FrontendActionReconciliation) {
      console.warn('WebSocket client is read-only');
      return false;
    }

    try {
      this.socket.send(JSON.stringify({...event, ...(this.board ? {board: this.board} : {})}));
      return true;
    } catch (error) {
      console.error('Error sending WebSocket message:', error);
      return false;
    }
  }

  private sendAuthenticationEvent(): void {
    if (this.token) {
      this.send({
        type: ActionType.FrontendToken,
        token: this.token,
        version: FRONTEND_VERSION,
      });
    }
  }

  isConnected(): boolean {
    return this.socket !== null && this.socket.readyState === WebSocket.OPEN;
  }
}

export function createWebSocketClient(
  url: string,
  delegate?: WebSocketClientDelegate,
  selection?: OwnMembership,
): WebSocketClient {
  return new WebSocketClient(url, delegate, selection);
}

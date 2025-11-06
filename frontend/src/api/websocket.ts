import {
  ActionType,
  EventType,
  type FrontendAircraftDisconnectEvent,
  type FrontendAssignedSquawkEvent,
  type FrontendBayEvent,
  type FrontendClearedAltitudeEvent,
  type FrontendCommunicationTypeEvent,
  type FrontendControllerOfflineEvent,
  type FrontendControllerOnlineEvent,
  type FrontendDisconnectEvent,
  type FrontendInitialEvent, type FrontendOwnersUpdateEvent,
  type FrontendRequestedAltitudeEvent,
  type FrontendSendEvent,
  type FrontendSetHeadingEvent,
  type FrontendSquawkEvent,
  type FrontendStandEvent,
  type FrontendStripUpdateEvent,
  type WebSocketEvent
} from "./models";


type EventMap = {
  [EventType.FrontendInitial]: FrontendInitialEvent;
  [EventType.FrontendStripUpdate]: FrontendStripUpdateEvent;
  [EventType.FrontendControllerOnline]: FrontendControllerOnlineEvent;
  [EventType.FrontendControllerOffline]: FrontendControllerOfflineEvent;
  [EventType.FrontendAssignedSquawk]: FrontendAssignedSquawkEvent;
  [EventType.FrontendSquawk]: FrontendSquawkEvent;
  [EventType.FrontendRequestedAltitude]: FrontendRequestedAltitudeEvent;
  [EventType.FrontendClearedAltitude]: FrontendClearedAltitudeEvent;
  [EventType.FrontendBay]: FrontendBayEvent;
  [EventType.FrontendDisconnect]: FrontendDisconnectEvent;
  [EventType.FrontendAircraftDisconnect]: FrontendAircraftDisconnectEvent;
  [EventType.FrontendStand]: FrontendStandEvent;
  [EventType.FrontendSetHeading]: FrontendSetHeadingEvent;
  [EventType.FrontendCommunicationType]: FrontendCommunicationTypeEvent;
  [EventType.FrontendOwnersUpdate]: FrontendOwnersUpdateEvent;
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
  private reconnectAttempts = 0;
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  private manuallyClosed = false;
  private delegate?: WebSocketClientDelegate;

  constructor(url: string, delegate?: WebSocketClientDelegate) {
    this.url = url;
    this.delegate = delegate;
  }

  setToken(token: string): void {
    this.token = token;
    if (this.isConnected()) {
      this.sendAuthenticationEvent();
    }
  }

  connect(): Promise<void> {
    return new Promise((resolve,) => {
      this.manuallyClosed = false;
      this.socket = new WebSocket(this.url);

      this.socket.onopen = () => {
        console.log('WebSocket connection established');
        this.reconnectAttempts = 0;
        if (this.token) {
          this.sendAuthenticationEvent();
        }
        if (this.delegate?.onConnected) {
          this.delegate.onConnected();
        }
        resolve();
      };

      this.socket.onerror = (error) => {
        console.error('WebSocket error:', error);
      };

      this.socket.onclose = (event) => {
        console.log('WebSocket connection closed:', event.code, event.reason);
        if (this.delegate?.onDisconnected) {
          this.delegate.onDisconnected();
        }
        if (!this.manuallyClosed) {
          this.retryConnection();
        }
      };

      this.socket.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as WebSocketEvent;
          const handlers = this.eventHandlers.get(data.type);
          if (handlers) {
            handlers.forEach(handler => handler(data));
          }
        } catch (error) {
          console.error('Error parsing WebSocket message:', error);
        }
      };
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

    try {
      this.socket.send(JSON.stringify(event));
    } catch (error) {
      console.error('Error sending WebSocket message:', error);
    }
  }

  private sendAuthenticationEvent(): void {
    if (this.token) {
      this.send({
        type: ActionType.FrontendToken,
        token: this.token
      });
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
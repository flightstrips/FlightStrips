import {
  EventType, type FrontendAssignedSquawkEvent, type FrontendBayEvent, type FrontendClearedAltitudeEvent,
  type FrontendControllerOfflineEvent,
  type FrontendControllerOnlineEvent,
  type FrontendInitialEvent,
  type FrontendRequestedAltitudeEvent, type FrontendSendEvent,
  type FrontendSquawkEvent,
  type FrontendStripUpdateEvent, type WebSocketEvent
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
};

export class WebSocketClient {
  private socket: WebSocket | null = null;
  private eventHandlers: Map<EventType, Array<(data: unknown) => void>> = new Map();
  private readonly url: string;
  private token: string | null = null;

  constructor(url: string) {
    this.url = url;
  }

  setToken(token: string): void {
    this.token = token;
    // If already connected, send the authentication event with the new token
    if (this.isConnected()) {
      this.sendAuthenticationEvent();
    }
  }

  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      this.socket = new WebSocket(this.url);

      this.socket.onopen = () => {
        console.log('WebSocket connection established');
        // Send authentication event if token is available
        if (this.token) {
          this.sendAuthenticationEvent();
        }
        resolve();
      };

      this.socket.onerror = (error) => {
        console.error('WebSocket error:', error);
        reject(error);
      };

      this.socket.onclose = (event) => {
        console.log('WebSocket connection closed:', event.code, event.reason);
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

  disconnect(): void {
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
        type: 'token',
        token: this.token
      });
    }
  }

  isConnected(): boolean {
    return this.socket !== null && this.socket.readyState === WebSocket.OPEN;
  }
}

export function createWebSocketClient(url: string): WebSocketClient {
  return new WebSocketClient(url);
}

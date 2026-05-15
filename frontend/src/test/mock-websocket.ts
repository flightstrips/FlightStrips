type Listener = (event: { data: string }) => void;

export class MockWebSocket {
  static instances: MockWebSocket[] = [];
  static OPEN = 1;
  static CLOSED = 3;

  readyState = MockWebSocket.OPEN;
  readonly url: string;
  private listeners: Record<string, Listener[]> = {};
  readonly sent: string[] = [];

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  addEventListener(type: string, listener: Listener): void {
    this.listeners[type] ??= [];
    this.listeners[type].push(listener);
  }

  send(data: string): void {
    this.sent.push(data);
  }

  close(): void {
    this.readyState = MockWebSocket.CLOSED;
    this.emit("close", { data: "" });
  }

  emit(type: string, event: { data: string }): void {
    for (const listener of this.listeners[type] ?? []) {
      listener(event);
    }
  }

  simulateOpen(): void {
    this.emit("open", { data: "" });
  }

  static reset(): void {
    MockWebSocket.instances = [];
  }
}

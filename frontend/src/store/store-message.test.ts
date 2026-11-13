import { beforeEach, describe, expect, it, vi } from "vitest";
import { EventType, type FrontendMessageReceivedEvent } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { createWebSocketStore } from "./store";

describe("message dismissal", () => {
  const handlers = new Map<string, (event: unknown) => void>();
  const client = {
    on: vi.fn((type: string, handler: (event: unknown) => void) => handlers.set(type, handler)),
    send: vi.fn(),
  };
  let store: ReturnType<typeof createWebSocketStore>;

  beforeEach(() => {
    vi.clearAllMocks();
    handlers.clear();
    store = createWebSocketStore(client as unknown as WebSocketClient);
  });

  const message: FrontendMessageReceivedEvent = {
    type: EventType.FrontendMessageReceived,
    id: 123,
    sender: "EKCH_TWR",
    text: "Taxi to holding point A1.",
    is_broadcast: false,
    recipients: ["EKCH_GND"],
  };

  it("does not restore a dismissed message when its acknowledgement arrives", () => {
    handlers.get(EventType.FrontendMessageReceived)?.(message);
    store.getState().dismissMessage(message.id);

    handlers.get(EventType.FrontendMessageReceived)?.({ ...message, acknowledged: true });

    expect(store.getState().messages).toEqual([]);
  });

  it("updates an acknowledged message that has not been dismissed", () => {
    handlers.get(EventType.FrontendMessageReceived)?.(message);

    handlers.get(EventType.FrontendMessageReceived)?.({ ...message, acknowledged: true });

    expect(store.getState().messages).toEqual([{ ...message, acknowledged: true }]);
  });
});

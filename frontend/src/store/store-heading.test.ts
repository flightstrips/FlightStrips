import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand/vanilla";

import { Bay, EventType, type FrontendSetHeadingEvent, type FrontendStrip } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { createWebSocketStore, type WebSocketState } from "./store";

function createMockClient() {
  const handlers = new Map<string, (data: unknown) => void>();
  return {
    on: vi.fn((eventType: string, handler: (data: unknown) => void) => handlers.set(eventType, handler)),
    send: vi.fn(),
    emit: (eventType: string, data: unknown) => handlers.get(eventType)?.(data),
  };
}

describe("heading events", () => {
  let client: ReturnType<typeof createMockClient>;
  let store: StoreApi<WebSocketState>;

  beforeEach(() => {
    client = createMockClient();
    store = createWebSocketStore(client as unknown as WebSocketClient);
    store.setState({
      strips: [{ callsign: "SAS123", bay: Bay.Final, heading: 90, version: 4, clx_validation: { faults: [] } } as unknown as FrontendStrip],
    });
  });

  it("applies the returned strip version and CLX validation", () => {
    const event: FrontendSetHeadingEvent = {
      type: EventType.FrontendSetHeading,
      callsign: "SAS123",
      heading: 180,
      version: 5,
      clx_validation: null,
    };

    client.emit(EventType.FrontendSetHeading, event);

    expect(store.getState().strips[0]).toMatchObject({ heading: 180, version: 5 });
    expect(store.getState().strips[0].clx_validation).toBeUndefined();
  });

  it("retains validation data for an older heading event", () => {
    client.emit(EventType.FrontendSetHeading, {
      type: EventType.FrontendSetHeading,
      callsign: "SAS123",
      heading: 270,
    } satisfies FrontendSetHeadingEvent);

    expect(store.getState().strips[0].clx_validation).toEqual({ faults: [] });
  });

  it("ignores a versioned heading event older than the current strip", () => {
    client.emit(EventType.FrontendSetHeading, {
      type: EventType.FrontendSetHeading,
      callsign: "SAS123",
      heading: 270,
      version: 3,
      clx_validation: null,
    } satisfies FrontendSetHeadingEvent);

    expect(store.getState().strips[0]).toMatchObject({ heading: 90, version: 4, clx_validation: { faults: [] } });
  });
});

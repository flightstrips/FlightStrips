import { describe, expect, it, beforeEach, vi } from "vitest";
import { createWebSocketStore, type WebSocketState } from "./store";
import type { WebSocketClient } from "@/api/websocket";
import {
  EventType,
  type FrontendStandStatusSnapshotEvent,
  type FrontendStandAssignmentUpdateEvent,
  type FrontendStandBlockUpdateEvent,
  type FrontendStandAssignmentEntry,
  type FrontendStandBlockEntry,
  type FrontendInitialEvent,
} from "@/api/models";
import type { StoreApi } from "zustand/vanilla";

function createMockClient() {
  const handlers = new Map<string, Array<(data: unknown) => void>>();
  return {
    on: vi.fn((eventType: string, handler: (data: unknown) => void) => {
      if (!handlers.has(eventType)) {
        handlers.set(eventType, []);
      }
      handlers.get(eventType)!.push(handler);
    }),
    send: vi.fn(),
    _emit: (eventType: string, data: unknown) => {
      const hs = handlers.get(eventType);
      if (hs) {
        hs.forEach((h) => h(data));
      }
    },
    setReadOnly: vi.fn(),
    reconnect: vi.fn(),
  } as unknown as WebSocketClient & {
    _emit: (eventType: string, data: unknown) => void;
  };
}

describe("stand store events", () => {
  let client: ReturnType<typeof createMockClient>;
  let store: StoreApi<WebSocketState>;

  beforeEach(() => {
    client = createMockClient();
    store = createWebSocketStore(client as unknown as WebSocketClient);
  });

  describe("satEnabled", () => {
    it("is false by default", () => {
      expect(store.getState().satEnabled).toBe(false);
    });

    it("is true when initial event has stand_assignment_enabled", () => {
      const init: FrontendInitialEvent = {
        type: EventType.FrontendInitial,
        controllers: [],
        strips: [],
        tactical_strips: [],
        me: { callsign: "", position: "", identifier: "", section: "", owned_sectors: [] },
        airport: "EKCH",
        layout: "EST",
        callsign: "",
        runway_setup: { departure: [], arrival: [] },
        coordinations: [],
        messages: [],
        available_sids: [],
        initial_cfl_by_runway: {},
        transition_altitude: 5000,
        read_only: false,
        position_available: true,
        stand_assignment_enabled: true,
        stand_assignments: [],
        stand_blocks: [],
      };
      client._emit(EventType.FrontendInitial, init);
      expect(store.getState().satEnabled).toBe(true);
    });
  });

  describe("stand status snapshot", () => {
    it("populates assignments and blocks", () => {
      const assignments: FrontendStandAssignmentEntry[] = [
        { callsign: "SAS123", stand: "A18", direction: "DEPARTURE", stage: "RESERVED", source: "AUTOMATIC" },
      ];
      const blocks: FrontendStandBlockEntry[] = [
        { stand: "B4", block_type: "MANUAL", reason: "Maintenance" },
      ];
      client._emit(EventType.FrontendStandStatusSnapshot, {
        type: EventType.FrontendStandStatusSnapshot,
        assignments,
        blocks,
      } as FrontendStandStatusSnapshotEvent);

      expect(store.getState().standAssignments).toEqual(assignments);
      expect(store.getState().standBlocks).toEqual(blocks);
    });
  });

  describe("stand assignment update", () => {
    it("adds new assignment", () => {
      const entry: FrontendStandAssignmentEntry = {
        callsign: "SAS456", stand: "A20", direction: "ARRIVAL", stage: "ESTIMATED", source: "AUTOMATIC",
      };
      client._emit(EventType.FrontendStandAssignmentUpdate, {
        type: EventType.FrontendStandAssignmentUpdate,
        assignment: entry,
      } as FrontendStandAssignmentUpdateEvent);

      expect(store.getState().standAssignments).toHaveLength(1);
      expect(store.getState().standAssignments[0]).toEqual(entry);
    });

    it("updates existing assignment", () => {
      const initial: FrontendStandAssignmentEntry = {
        callsign: "SAS456", stand: "A20", direction: "ARRIVAL", stage: "ESTIMATED", source: "AUTOMATIC",
      };
      client._emit(EventType.FrontendStandAssignmentUpdate, {
        type: EventType.FrontendStandAssignmentUpdate,
        assignment: initial,
      } as FrontendStandAssignmentUpdateEvent);

      const updated: FrontendStandAssignmentEntry = {
        callsign: "SAS456", stand: "A20", direction: "ARRIVAL", stage: "CONFIRMED", source: "AUTOMATIC",
      };
      client._emit(EventType.FrontendStandAssignmentUpdate, {
        type: EventType.FrontendStandAssignmentUpdate,
        assignment: updated,
      } as FrontendStandAssignmentUpdateEvent);

      expect(store.getState().standAssignments).toHaveLength(1);
      expect(store.getState().standAssignments[0].stage).toBe("CONFIRMED");
    });
  });

  describe("stand block update", () => {
    it("preserves simultaneous blocks and removes only the addressed block", () => {
      const manual = { id: 10, stand: "B8", block_type: "MANUAL", reason: "Closed", version: 1 };
      const adjacent = { id: 11, stand: "B8", block_type: "ADJACENCY", reason: "Blocked by B7", version: 1 };
      client._emit(EventType.FrontendStandBlockUpdate, { type: EventType.FrontendStandBlockUpdate, stand: "B8", block: manual });
      client._emit(EventType.FrontendStandBlockUpdate, { type: EventType.FrontendStandBlockUpdate, stand: "B8", block: adjacent });
      expect(store.getState().standBlocks).toEqual([manual, adjacent]);
      client._emit(EventType.FrontendStandBlockUpdate, { type: EventType.FrontendStandBlockUpdate, stand: "B8", block: null, block_id: 10 });
      expect(store.getState().standBlocks).toEqual([adjacent]);
    });

    it("adds new block", () => {
      const block: FrontendStandBlockEntry = {
        stand: "B8", block_type: "MANUAL", reason: "Closed",
      };
      client._emit(EventType.FrontendStandBlockUpdate, {
        type: EventType.FrontendStandBlockUpdate,
        stand: "B8",
        block,
      } as FrontendStandBlockUpdateEvent);

      expect(store.getState().standBlocks).toHaveLength(1);
      expect(store.getState().standBlocks[0]).toEqual(block);
    });

    it("removes block when null", () => {
      const block: FrontendStandBlockEntry = {
        stand: "B8", block_type: "MANUAL", reason: "Closed",
      };
      client._emit(EventType.FrontendStandBlockUpdate, {
        type: EventType.FrontendStandBlockUpdate,
        stand: "B8",
        block,
      } as FrontendStandBlockUpdateEvent);

      client._emit(EventType.FrontendStandBlockUpdate, {
        type: EventType.FrontendStandBlockUpdate,
        stand: "B8",
        block: null,
      } as FrontendStandBlockUpdateEvent);

      expect(store.getState().standBlocks).toHaveLength(0);
    });

    it("updates existing block", () => {
      const initial: FrontendStandBlockEntry = {
        stand: "B8", block_type: "MANUAL", reason: "Closed",
      };
      client._emit(EventType.FrontendStandBlockUpdate, {
        type: EventType.FrontendStandBlockUpdate,
        stand: "B8",
        block: initial,
      } as FrontendStandBlockUpdateEvent);

      const updated: FrontendStandBlockEntry = {
        stand: "B8", block_type: "MANUAL", reason: "Maintenance",
      };
      client._emit(EventType.FrontendStandBlockUpdate, {
        type: EventType.FrontendStandBlockUpdate,
        stand: "B8",
        block: updated,
      } as FrontendStandBlockUpdateEvent);

      expect(store.getState().standBlocks).toHaveLength(1);
      expect(store.getState().standBlocks[0].reason).toBe("Maintenance");
    });
  });

  describe("reconnect snapshot", () => {
    it("rehydrates stand state from initial event", () => {
      const assignments: FrontendStandAssignmentEntry[] = [
        { callsign: "SAS123", stand: "A18", direction: "DEPARTURE", stage: "RESERVED", source: "AUTOMATIC" },
        { callsign: "NAX456", stand: "C27", direction: "ARRIVAL", stage: "CONFIRMED", source: "AUTOMATIC" },
      ];
      const blocks: FrontendStandBlockEntry[] = [
        { stand: "B4", block_type: "MANUAL", reason: "Maintenance" },
      ];

      const init: FrontendInitialEvent = {
        type: EventType.FrontendInitial,
        controllers: [],
        strips: [],
        tactical_strips: [],
        me: { callsign: "", position: "", identifier: "", section: "", owned_sectors: [] },
        airport: "EKCH",
        layout: "EST",
        callsign: "",
        runway_setup: { departure: [], arrival: [] },
        coordinations: [],
        messages: [],
        available_sids: [],
        initial_cfl_by_runway: {},
        transition_altitude: 5000,
        read_only: false,
        position_available: true,
        stand_assignment_enabled: true,
        stand_assignments: assignments,
        stand_blocks: blocks,
      };
      client._emit(EventType.FrontendInitial, init);

      expect(store.getState().standAssignments).toEqual(assignments);
      expect(store.getState().standBlocks).toEqual(blocks);
      expect(store.getState().satEnabled).toBe(true);
    });

    it("clears stand state on disconnect", () => {
      const init: FrontendInitialEvent = {
        type: EventType.FrontendInitial,
        controllers: [],
        strips: [],
        tactical_strips: [],
        me: { callsign: "", position: "", identifier: "", section: "", owned_sectors: [] },
        airport: "EKCH",
        layout: "EST",
        callsign: "",
        runway_setup: { departure: [], arrival: [] },
        coordinations: [],
        messages: [],
        available_sids: [],
        initial_cfl_by_runway: {},
        transition_altitude: 5000,
        read_only: false,
        position_available: true,
        stand_assignment_enabled: true,
        stand_assignments: [{ callsign: "SAS1", stand: "A1", direction: "DEPARTURE", stage: "RESERVED", source: "AUTOMATIC" }],
        stand_blocks: [],
      };
      client._emit(EventType.FrontendInitial, init);
      expect(store.getState().standAssignments).toHaveLength(1);

      client._emit(EventType.FrontendDisconnect, { type: EventType.FrontendDisconnect });
      expect(store.getState().standAssignments).toHaveLength(0);
      expect(store.getState().standBlocks).toHaveLength(0);
      expect(store.getState().satEnabled).toBe(false);
    });
  });

  describe("occupyStand / vacateStand", () => {
    it("sends a versioned SAT manual assignment request", () => {
      store.getState().requestManualStand("SAS123", "A25", 4);
      expect(client.send).toHaveBeenCalledWith({
        type: "stand_assignment_manual_request",
        callsign: "SAS123",
        stand: "A25",
        version: 4,
      });
    });

    it("sends stand block create action", () => {
      store.getState().occupyStand("A25");
      expect(client.send).toHaveBeenCalledWith({
        type: "stand_block_create",
        stand: "A25",
        reason: "Manual block",
      });
    });

    it("sends a versioned stand block remove action", () => {
      store.getState().vacateStand("B6", 12, 3);
      expect(client.send).toHaveBeenCalledWith({
        type: "stand_block_remove",
        stand: "B6",
        block_id: 12,
        version: 3,
      });
    });
  });
});

import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {describe, expect, it} from "vitest";

import type {AMANFlight, AMANStateEvent} from "@/api/aman";
import {
  buildAMANLanes,
  buildTimelineRange,
  layoutTimelineMarkers,
  operationalMarkerTimestamp,
  orderAMANFlights,
} from "./presentation";

const golden = JSON.parse(readFileSync(
  resolve(process.cwd(), "../backend/pkg/events/frontend/testdata/aman-state-v1.json"),
  "utf8",
)) as AMANStateEvent;

function flight(id: string, order: number | null, minutes: number): AMANFlight {
  const value = structuredClone(golden.data.flights[0]);
  value.flight_id = id;
  value.callsign = id.toUpperCase();
  value.order = order;
  value.slot = value.slot && {
    ...value.slot,
    sequence: order ?? 99,
    time: new Date(Date.UTC(2026, 6, 22, 10, minutes)).toISOString(),
  };
  return value;
}

describe("AMAN presentation model", () => {
  it("uses only backend order/sequence and preserves wire order for ties", () => {
    const flights = [flight("third", 3, 3), flight("first-a", 1, 1), flight("first-b", 1, 2), flight("missing", null, 4)];
    flights[3].slot = null;

    expect(orderAMANFlights(flights).map((value) => value.flight_id)).toEqual(["first-a", "first-b", "third", "missing"]);
  });

  it("keeps declared, discovered, and unassigned runway groups deterministic", () => {
    const state = structuredClone(golden.data);
    state.runway_groups = [{id: "NORTH"}];
    state.flights = [
      {...flight("south-2", 2, 2), runway_group_id: "SOUTH"},
      {...flight("north", 1, 1), runway_group_id: "NORTH"},
      {...flight("south-1", 1, 1), runway_group_id: "SOUTH"},
      {...flight("none", null, 3), runway_group_id: null, slot: null},
    ];

    expect(buildAMANLanes(state).map((lane) => [lane.id, lane.flights.map((value) => value.flight_id)])).toEqual([
      ["NORTH", ["north"]],
      ["SOUTH", ["south-1", "south-2"]],
      ["unassigned", ["none"]],
    ]);
  });

  it("uses slot then operational TETA for marker time and never raw TETA", () => {
    const value = flight("frozen", 1, 10);
    value.operational_teta = "2026-07-22T10:11:00.000Z";
    value.raw_teta = "2026-07-22T10:50:00.000Z";
    expect(operationalMarkerTimestamp(value)).toBe(value.slot?.time);
    value.slot = null;
    expect(operationalMarkerTimestamp(value)).toBe(value.operational_teta);
  });

  it("allocates deterministic collision tracks without changing marker timestamps", () => {
    const flights = [flight("a", 1, 10), flight("b", 2, 10), flight("c", 3, 20)];
    const range = buildTimelineRange(flights)!;
    const markers = layoutTimelineMarkers(flights, range);

    expect(markers.map((marker) => marker.track)).toEqual([0, 1, 0]);
    expect(markers.map((marker) => marker.timestamp)).toEqual(flights.map((value) => value.slot!.time));
  });

  it("builds a 200-flight replacement presentation without partial lanes", () => {
    const state = structuredClone(golden.data);
    state.runway_groups = [{id: "NORTH"}, {id: "SOUTH"}];
    state.flights = Array.from({length: 200}, (_, index) => ({
      ...flight(`flight-${index}`, index, index % 60),
      runway_group_id: index % 2 === 0 ? "NORTH" : "SOUTH",
    }));

    const lanes = buildAMANLanes(state);
    expect(lanes).toHaveLength(2);
    expect(lanes.flatMap((lane) => lane.flights)).toHaveLength(200);
    expect(new Set(lanes.flatMap((lane) => lane.flights.map((value) => value.flight_id))).size).toBe(200);
  });
});

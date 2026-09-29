import {expect, it} from "vitest";
import {create} from "@bufbuild/protobuf";
import {TimestampSchema} from "@bufbuild/protobuf/wkt";
import {EntitySnapshotSchema} from "./generated/cluster/v1/storage_pb";
import {amanState} from "./aman-projection";
import {isAMANStateEvent} from "./aman";

const when = create(TimestampSchema, {seconds: 1790683200n});

it("derives a valid AMAN view from typed airport and flight entities", () => {
  const airport = create(EntitySnapshotSchema, {key: "EKCH", revision: 1n,
    value: {value: {case: "amanAirport", value: {airport: "EKCH", revision: 7n,
      policyVersion: "v1", effectiveMode: "authoritative", authoritative: true,
      generatedAt: when, runwayGroups: [{id: "22L", selected: true}],
      activeRunwayGroupIds: ["22L"], health: {status: "ready", ready: true}}}}});
  const flight = create(EntitySnapshotSchema, {key: "SAS123", revision: 1n,
    value: {value: {case: "amanFlight", value: {callsign: "SAS123", state: "planned",
      sequenceDisposition: "active", dataStatus: "fresh", freezeReason: "none", updatedAt: when}}}});
  const event = amanState("EKCH", [airport, flight]);
  expect(event).not.toBeNull();
  expect(isAMANStateEvent(event)).toBe(true);
  expect(event?.data.flights[0].callsign).toBe("SAS123");
});

import {create, fromBinary, toBinary} from "@bufbuild/protobuf";
import {describe, it, expect} from "vitest";
import {FrontendInitialSchema, FrontendDeltaSchema} from "./generated/cluster/v1/wire_pb";
import {EntitySnapshotSchema} from "./generated/cluster/v1/storage_pb";
import {FrontendProjection} from "./projection";
import {getAMANHeaderReadModel, isAMANStateEvent, type AMANStateEvent} from "./aman";

describe("AMAN binary delivery", () => {
 it("preserves backend header, traffic, holding and health through snapshots and deltas", () => {
  const at = "2026-10-04T00:00:00.000Z";
  const timestamp = {seconds: BigInt(Date.parse(at) / 1000), nanos: 0};
  const state = create(EntitySnapshotSchema, {key: "EKCH", revision: 1n, value: {value: {case: "amanAirport", value: {
   airport: "EKCH", revision: 1n, generatedAt: timestamp, policyVersion: "v1", configuredMode: "authoritative", effectiveMode: "authoritative", authoritative: true,
   activeRunwayGroupIds: ["22L"], runwayGroups: [{id: "22L", selected: true, activeRatePerHour: 40, rateEffectiveAt: timestamp}],
   health: {status: "ready", ready: true, components: ["observation_source", "navigation", "weather", "repository", "predictor", "replay_validation"].map(component => ({component, status: "ready"}))},
   header: {activeRunwayGroups: [{id: "22L", activeRatePerHour: 40, rateEffectiveAt: at}], readiness: {status: "ready", ready: true}, trafficSummary: {status: "ready", maestroHorizonCount: 3, tmaAbove1500FeetCount: 2}},
   trafficPrediction: {generatedAt: at, rangeStart: at, rangeEnd: "2026-10-04T03:00:00.000Z", bucketMinutes: 15, sourceStatus: "fresh", status: "ready",
    buckets: Array.from({length: 12}, (_, i) => ({start: new Date(Date.parse(at) + i * 900000).toISOString(), end: new Date(Date.parse(at) + (i + 1) * 900000).toISOString(), alert: "none"}))},
   holdingInformation: [{callsign: "SAS123", holding: "TESPI", eat: at, clearedAltitude: 12000, sourceStatus: "fresh", observedAt: at}],
  }}}});
  const events: unknown[] = [];
  const projection = new FrontendProjection(event => events.push(event));
  const initial = create(FrontendInitialSchema, {sessionId: 1, airport: "EKCH", aggregateRevision: 1n, airportAggregateRevision: 1n, entities: [state]});
  projection.initial(fromBinary(FrontendInitialSchema, toBinary(FrontendInitialSchema, initial)));
  const event = events.at(-1) as AMANStateEvent;
  expect(isAMANStateEvent(event)).toBe(true);
  expect(getAMANHeaderReadModel(event.data)).toMatchObject({availability: "ready", traffic_summary: {maestro_horizon_count: 3}});
  expect(event.data.technical_health.vatsim.status).toBe("ready");
  expect(event.data.traffic_prediction?.buckets).toHaveLength(12);
  expect(event.data.holding_information?.[0]).toMatchObject({eat: at, cleared_altitude: 12000});
  const airport = state.value!.value;
  if (airport.case !== "amanAirport") throw new Error("fixture");
  airport.value.revision = 2n;
  airport.value.holdingInformation = [];
  projection.delta(create(FrontendDeltaSchema, {aggregate: {target: {case: "airport", value: {icao: "EKCH"}}}, aggregateRevision: 2n, changes: [{key: "EKCH", revision: 2n, operation: {case: "upsert", value: state.value!}}]}));
  expect(isAMANStateEvent(events.at(-1))).toBe(true);
  expect((events.at(-1) as AMANStateEvent).data.holding_information).toEqual([]);
 });
});

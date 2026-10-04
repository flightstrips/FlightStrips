import {describe, expect, it} from "vitest";
import {create} from "@bufbuild/protobuf";
import {EventType, type WebSocketEvent} from "./models";
import {FrontendDeltaSchema, FrontendInitialSchema, FrontendObservationSchema} from "./generated/cluster/v1/wire_pb";
import {EntityKind, EntitySnapshotSchema, PositionValueSchema, PresenceValueSchema} from "./generated/cluster/v1/storage_pb";
import {FrontendProjection, RevisionGapError} from "./projection";

const strip = (route: string, revision: bigint) => create(EntitySnapshotSchema, {
  key: "SAS123", revision,
  value: {value: {case: "strip", value: {id: 1n, callsign: "SAS123", revision, route,
    bay: "CLEARED", pdcState: "NONE", sequence: 1000n}}},
});
const initial = () => create(FrontendInitialSchema, {sessionId: 7, airport: "EKCH", sessionName: "TEST",
  aggregateRevision: 3n, airportAggregateRevision: 9n, streamSequence: 30n,
  entities: [strip("OLD", 1n)], me: {cid: "123456", callsign: "EKCH_TWR", position: "TWR"}});
const delta = (revision: bigint, route: string) => create(FrontendDeltaSchema, {
  aggregate: {target: {case: "session", value: {id: 7}}}, aggregateRevision: revision,
  streamSequence: revision + 27n,
  changes: [{key: "SAS123", revision: revision - 2n,
    operation: {case: "upsert", value: strip(route, revision - 2n).value!}}],
});

it("preserves UTC compact CDM clocks on snapshots and replacements", () => {
  const events: WebSocketEvent[] = [];
  const projection = new FrontendProjection(event => events.push(event));
  const snapshot = initial();
  const stamp = {$typeName: "google.protobuf.Timestamp" as const, seconds: 1790769600n, nanos: 0};
  const record = snapshot.entities[0].value!.value;
  if (record.case !== "strip") throw new Error("fixture");
  record.value.eobt = stamp;
  record.value.tobtSetBy = "EKCH_DEL";
  record.value.phase = "READY";
  snapshot.entities.push(create(EntitySnapshotSchema, {key: "SAS123", revision: 1n,
    value: {value: {case: "cdmState", value: {callsign: "SAS123", tobt: stamp, tsat: stamp, ttot: stamp, ctot: stamp}}}}));
  projection.initial(snapshot);
  expect(events[0]).toMatchObject({strips: [expect.objectContaining({eobt: "1200", tobt: "1200", tsat: "1200", ttot: "1200", ctot: "1200", tobt_set_by: "EKCH_DEL", phase: "READY"})]});
  projection.delta(create(FrontendDeltaSchema, {aggregate: {target: {case: "session", value: {id: 7}}}, aggregateRevision: 4n,
    changes: [{key: "SAS123", revision: 2n, operation: {case: "upsert", value: {value: {case: "cdmState", value: {callsign: "SAS123", tobt: stamp, tsat: stamp, ttot: stamp}}}}}]}));
  expect(events.at(-1)).toMatchObject({eobt: "1200", tobt: "1200", tsat: "1200", ttot: "1200", ctot: ""});
});

describe("typed frontend projection", () => {
  it("clears a persisted altitude when the first live observation is a tombstone", () => {
    const events: WebSocketEvent[] = [];
    const projection = new FrontendProjection(event => events.push(event));
    const snapshot = initial();
    const record = snapshot.entities[0].value!.value;
    if (record.case !== "strip") throw new Error("fixture");
    record.value.positionAltitudeFeet = 500;
    projection.initial(snapshot);
    projection.observation(create(FrontendObservationSchema, {sourceRevision: 1n,
      value: {case: "position", value: {sessionId: 7, aircraftKey: "SAS123",
        observation: {case: "tombstone", value: {}}}}}));
    expect(events.at(-1)).toEqual({type: EventType.FrontendPositionAltitude,
      callsign: "SAS123", position_altitude: null});
  });

  it("keeps actual times and status when an atomic CDM replacement is presented", () => {
    const events: WebSocketEvent[] = [];
    const projection = new FrontendProjection(event => events.push(event));
    projection.initial(initial());
    const item = strip("NEW", 2n);
    if (item.value?.value.case !== "strip") throw new Error("fixture");
    item.value.value.value.aobt = { $typeName: "google.protobuf.Timestamp", seconds: 1790769600n, nanos: 0 };
    item.value.value.value.operationalStatus = "REA";
    item.value.value.value.ctotSource = "Manual";
    projection.delta(create(FrontendDeltaSchema, {
      aggregate: {target: {case: "session", value: {id: 7}}}, aggregateRevision: 4n,
      changes: [
        {key: "SAS123", revision: 2n, operation: {case: "upsert", value: item.value}},
        {key: "SAS123", revision: 1n, operation: {case: "upsert", value: {value: {case: "cdmState", value: {callsign: "SAS123", ready: true}}}}},
      ],
    }));
    expect(events.at(-1)).toMatchObject({type: EventType.FrontendCdmData, status: "REA", ctot_source: "Manual", aobt: "1200"});
    expect(projection.entityRevisions.get("cdm.SAS123")).toBe(1n);
  });
  it("rebuilds from an initial checkpoint and applies complete replacements", () => {
    const events: WebSocketEvent[] = [];
    const projection = new FrontendProjection(event => events.push(event));
    projection.initial(initial());
    expect(events[0].type).toBe(EventType.FrontendInitial);
    expect((events[0] as Extract<WebSocketEvent, {type: EventType.FrontendInitial}>).strips[0].route).toBe("OLD");
    projection.delta(delta(4n, "NEW"));
    expect(events.at(-1)?.type).toBe(EventType.FrontendStripUpdate);
    expect((events.at(-1) as Extract<WebSocketEvent, {type: EventType.FrontendStripUpdate}>).route).toBe("NEW");
    expect(projection.entityRevisions.get("strip.SAS123")).toBe(2n);
    projection.delta(delta(4n, "IGNORED"));
    expect(events).toHaveLength(2);
  });

  it("requires a fresh snapshot after a session or airport gap", () => {
    const projection = new FrontendProjection(() => {});
    projection.initial(initial());
    expect(() => projection.delta(delta(5n, "SKIPPED"))).toThrow(RevisionGapError);
    const airportGap = create(FrontendDeltaSchema, {aggregate: {target: {case: "airport", value: {icao: "EKCH"}},}, aggregateRevision: 11n});
    expect(() => projection.delta(airportGap)).toThrow(RevisionGapError);
  });

  it("updates coordination transfers by callsign and keeps a strip on position loss", () => {
    const events: WebSocketEvent[] = [];
    const projection = new FrontendProjection(event => events.push(event));
    projection.initial(initial());
    projection.delta(create(FrontendDeltaSchema, {
      aggregate: {target: {case: "session", value: {id: 7}}}, aggregateRevision: 4n,
      changes: [{key: "14", revision: 1n, operation: {case: "upsert", value: {
        value: {case: "coordination", value: {id: 14n, callsign: "SAS123", fromCid: "123456", toCid: "789012"}},
      }}}],
    }));
    expect(events.at(-1)).toMatchObject({type: EventType.FrontendCoordinationTransferBroadcast, callsign: "SAS123"});
    expect(projection.coordinationId("SAS123")).toBe("14");
    projection.delta(create(FrontendDeltaSchema, {
      aggregate: {target: {case: "session", value: {id: 7}}}, aggregateRevision: 5n,
      changes: [{key: "14", revision: 2n, operation: {case: "delete", value: {kind: EntityKind.COORDINATION}}}],
    }));
    expect(events.at(-1)).toMatchObject({type: EventType.FrontendCoordinationFreeBroadcast, callsign: "SAS123"});
    expect(projection.entityRevisions.get("strip.SAS123")).toBe(1n);
    const beforeLoss = events.length;
    projection.observation(create(FrontendObservationSchema, {value: {case: "position", value: {
      sessionId: 7, aircraftKey: "SAS123", observation: {case: "tombstone", value: {}},
    }}}));
    expect(events).toHaveLength(beforeLoss);
    expect(projection.entityRevisions.get("strip.SAS123")).toBe(1n);
  });

  it("orders tagged observations across epochs and applies removals", () => {
    const events: WebSocketEvent[] = [];
    const projection = new FrontendProjection(event => events.push(event));
    const old = create(PositionValueSchema, {sessionId: 7, aircraftKey: "SAS123", ownerEpoch: 1n,
      observation: {case: "position", value: {altitudeFeet: 100}}});
    const fresh = create(PositionValueSchema, {sessionId: 7, aircraftKey: "SAS123", ownerEpoch: 2n,
      observation: {case: "position", value: {altitudeFeet: 200}}});
    const client = create(PresenceValueSchema, {present: {case: "client", value: {connectionId: "c1", cid: "123456",
      callsign: "EKCH_TWR", position: "TWR", sessionId: 7}}});
    const controller = create(EntitySnapshotSchema, {key: "123456", revision: 1n,
      value: {value: {case: "controller", value: {cid: "123456", callsign: "EKCH_TWR", position: "TWR"}}}});
    projection.initial(create(FrontendInitialSchema, {...initial(), entities: [strip("OLD", 1n), controller], taggedObservations: [
      create(FrontendObservationSchema, {value: {case: "position", value: old}, sourceRevision: 7n, stale: true}),
      create(FrontendObservationSchema, {value: {case: "position", value: fresh}, sourceRevision: 11n}),
      create(FrontendObservationSchema, {value: {case: "presence", value: client}, sourceRevision: 2n}),
    ]}));
    const first = events[0] as Extract<WebSocketEvent, {type: EventType.FrontendInitial}>;
    expect(first.strips[0].position_altitude).toBe(200);
    expect(first.controllers).toHaveLength(1);
    projection.observation(create(FrontendObservationSchema, {value: {case: "position", value: old}, sourceRevision: 8n}));
    expect(events).toHaveLength(1);
    projection.observation(create(FrontendObservationSchema, {value: {case: "position", value: fresh}, sourceRevision: 12n, removed: true}));
    expect(events.at(-1)).toEqual({type: EventType.FrontendPositionAltitude, callsign: "SAS123", position_altitude: null});
    projection.observation(create(FrontendObservationSchema, {value: {case: "presence", value: client}, sourceRevision: 3n, removed: true}));
    expect(events.at(-1)).toMatchObject({type: EventType.FrontendControllerOffline, callsign: "EKCH_TWR"});
  });
});

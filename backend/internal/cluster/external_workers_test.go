package cluster

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func handoffAirport(t *testing.T, store *memoryStore) {
	handoffOwner(t, store, airportRef("EKCH"))
}

func handoffOwner(t *testing.T, store *memoryStore, ref *pb.AggregateRef) {
	t.Helper()
	subject, _ := Subject(ref)
	state, err := (Writer{Store: store, NodeID: "node-a"}).load(context.Background(), subject, ref)
	if err != nil {
		t.Fatal(err)
	}
	event := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, AggregateRevision: state.Revision, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-b"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-b", Epoch: state.Owner.Epoch + 1}}}
	data, _ := proto.Marshal(event)
	if _, err := store.Publish(context.Background(), subject, state.SubjectSequence, data); err != nil {
		t.Fatal(err)
	}
	// Model an expired lease without a wall-clock sleep. The JetStream server
	// timestamp, rather than the claimant's local clock, accepts the handoff.
	store.mu.Lock()
	store.entries[len(store.entries)-1].ServerTime = state.Owner.LeaseUntil.AsTime().Add(time.Millisecond)
	store.mu.Unlock()
}

func TestGlobalEcfmpFetchKeepsOneTypedCheckpoint(t *testing.T) {
	store, objects, nav := navFixture(t)
	ctx := context.Background()
	id := uuid.NewString()
	page := &pb.ProviderPage{Provider: "ecfmp", Resource: "flow-measure/active", Parsed: &pb.ProviderPage_Ecfmp{Ecfmp: &pb.EcfmpPage{FetchedAt: timestamppb.Now(), Measures: []*pb.EcfmpMeasure{{Id: 42, Kind: "mandatory_route", StartTime: timestamppb.Now(), EndTime: timestamppb.New(time.Now().Add(time.Hour)), Routes: []string{"DCT ABC"}}}}}}
	calls := 0
	fetch := func(context.Context, *pb.ProviderCheckpoint, *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
		calls++
		return page, &pb.ProviderCheckpoint{Provider: "ecfmp", Resource: "flow-measure/active"}, nil
	}
	ref := globalRef()
	first, err := nav.FetchProviderPageFor(ctx, ExternalCallWorker{Writer: nav.Writer}, id, ref, "ecfmp", "flow-measure/active", fetch)
	if err != nil || !first || calls != 1 {
		t.Fatalf("global fetch: %v %v calls=%d", first, err, calls)
	}
	handoffOwner(t, store, ref)
	navB := NavigationWeather{Writer: Writer{Store: store, NodeID: "node-b"}, Objects: objects}
	again, err := navB.FetchProviderPageFor(ctx, ExternalCallWorker{Writer: navB.Writer}, id, ref, "ecfmp", "flow-measure/active", fetch)
	if err != nil || again || calls != 1 {
		t.Fatalf("takeover refetched ECFMP: %v %v calls=%d", again, err, calls)
	}
	checkpoint, restored, revision, err := navB.CheckpointRevisionFor(ctx, ref, "ecfmp", "flow-measure/active")
	if err != nil || checkpoint == nil || checkpoint.Sha256 == "" || revision != 1 || !proto.Equal(restored, page) {
		t.Fatalf("global typed checkpoint: %v %v revision=%d err=%v", checkpoint, restored, revision, err)
	}
}

func TestExternalWeatherOneCallAcrossReplicas(t *testing.T) {
	store, _, nav := navFixture(t)
	ctx := context.Background()
	id := uuid.NewString()
	workerA := ExternalCallWorker{Writer: nav.Writer}
	called, reserved := 0, 0
	reserve := func(context.Context) (bool, error) {
		reserved++
		return nav.ReserveQuota(ctx, id, "open-meteo", time.Now().UTC().Truncate(time.Hour), 1)
	}
	fetch := func(context.Context) (*pb.WeatherObservation, error) {
		called++
		return &pb.WeatherObservation{Metar: "EKCH 301200Z 27005KT CAVOK"}, nil
	}
	ok, err := nav.FetchWeatherFenced(ctx, workerA, id, "EKCH", "open-meteo", time.Minute, reserve, fetch)
	if err != nil || !ok || called != 1 || reserved != 1 {
		t.Fatalf("first call: %v %v calls=%d reserves=%d", ok, err, called, reserved)
	}
	handoffAirport(t, store)
	navB := NavigationWeather{Writer: Writer{Store: store, NodeID: "node-b"}}
	ok, err = navB.FetchWeatherFenced(ctx, ExternalCallWorker{Writer: navB.Writer}, id, "EKCH", "open-meteo", time.Minute, reserve, fetch)
	if err != nil || ok || called != 1 || reserved != 1 {
		t.Fatalf("takeover repeated provider call: %v %v calls=%d reserves=%d", ok, err, called, reserved)
	}
	state, err := navB.Writer.load(ctx, "fs.v1.state.airport.EKCH", airportRef("EKCH"))
	if err != nil || state.Workflows[id].Status != pb.WorkflowRecord_COMPLETED || len(state.Indexes[pb.EntityKind_WEATHER_CACHE]) != 1 {
		t.Fatalf("committed weather result: %v %v", state.Workflows[id], err)
	}
}

func TestExternalCallIntentCrashRecordsUncertainty(t *testing.T) {
	store, _, nav := navFixture(t)
	ctx := context.Background()
	id := uuid.NewString()
	stepID, _ := AmanIntentID(id, "external/vatsim")
	intent := &pb.WorkflowRecord{WorkflowId: id, Source: airportRef("EKCH"), Destination: airportRef("EKCH"), Step: "external/vatsim", DerivedCommandId: stepID, Status: pb.WorkflowRecord_PENDING}
	workerA := ExternalCallWorker{Writer: nav.Writer}
	ack, fresh := workerA.advance(ctx, intent, "intent")
	if !fresh || ack.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("intent: %v", ack)
	}
	handoffAirport(t, store)
	workerB := ExternalCallWorker{Writer: Writer{Store: store, NodeID: "node-b"}}
	if err := workerB.Resume(ctx, airportRef("EKCH")); err != nil {
		t.Fatal(err)
	}
	called := 0
	ok, err := workerB.Run(ctx, ExternalCallSpec{Source: airportRef("EKCH"), Destination: airportRef("EKCH"), WorkflowID: id, Step: "external/vatsim", Fetch: func(context.Context) (proto.Message, error) { called++; return &pb.VatsimObservation{}, nil }, Commit: func(context.Context, string, proto.Message) *pb.CommandReply { called++; return nil }})
	if err != nil || ok || called != 0 {
		t.Fatalf("uncertain call repeated: %v %v %d", ok, err, called)
	}
	state, _ := workerB.Writer.load(ctx, "fs.v1.state.airport.EKCH", airportRef("EKCH"))
	if state.Workflows[id].Status != pb.WorkflowRecord_FAILED || state.Workflows[id].ReasonCode != "CALL_UNCERTAIN" {
		t.Fatalf("uncertainty: %v", state.Workflows[id])
	}
}

func TestExternalCallResultCommitCrashRecoversWithoutRefetch(t *testing.T) {
	store, _, nav := navFixture(t)
	ctx := context.Background()
	id := uuid.NewString()
	stepID, _ := AmanIntentID(id, "external/metar")
	intent := &pb.WorkflowRecord{WorkflowId: id, Source: airportRef("EKCH"), Destination: airportRef("EKCH"), Step: "external/metar", DerivedCommandId: stepID, Status: pb.WorkflowRecord_PENDING}
	workerA := ExternalCallWorker{Writer: nav.Writer}
	if ack, _ := workerA.advance(ctx, intent, "intent"); ack.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("intent: %v", ack)
	}
	now := time.Now().UTC()
	if reply, err := nav.PutWeather(ctx, stepID, &pb.WeatherCache{Airport: "EKCH", Provider: "metar", Observation: &pb.WeatherObservation{Metar: "EKCH 301200Z CAVOK"}, FetchedAt: timestamppb.New(now), ExpiresAt: timestamppb.New(now.Add(time.Minute))}); err != nil || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("result: %v %v", reply, err)
	}
	handoffAirport(t, store)
	workerB := ExternalCallWorker{Writer: Writer{Store: store, NodeID: "node-b"}}
	if err := workerB.Resume(ctx, airportRef("EKCH")); err != nil {
		t.Fatal(err)
	}
	state, _ := workerB.Writer.load(ctx, "fs.v1.state.airport.EKCH", airportRef("EKCH"))
	if state.Workflows[id].Status != pb.WorkflowRecord_COMPLETED || state.Workflows[id].DestinationStreamSequence == nil {
		t.Fatalf("recovered result: %v", state.Workflows[id])
	}
}

func TestExternalCallFetchFailureNeverRetries(t *testing.T) {
	_, _, nav := navFixture(t)
	worker := ExternalCallWorker{Writer: nav.Writer}
	id := uuid.NewString()
	calls := 0
	spec := ExternalCallSpec{Source: airportRef("EKCH"), Destination: airportRef("EKCH"), WorkflowID: id, Step: "external/metar", Fetch: func(context.Context) (proto.Message, error) { calls++; return nil, errors.New("connection reset") }, Commit: func(context.Context, string, proto.Message) *pb.CommandReply {
		t.Fatal("no result to commit")
		return nil
	}}
	if called, err := worker.Run(context.Background(), spec); !called || err == nil {
		t.Fatalf("uncertain fetch: %v %v", called, err)
	}
	if called, err := worker.Run(context.Background(), spec); called || err != nil || calls != 1 {
		t.Fatalf("retried uncertain fetch: %v %v %d", called, err, calls)
	}
}

func TestExternalCallLostIntentAckDoesNotSend(t *testing.T) {
	store, _, nav := navFixture(t)
	store.loseAck = true
	store.failAfterAck = true
	worker := ExternalCallWorker{Writer: nav.Writer}
	id := uuid.NewString()
	calls := 0
	_, err := worker.Run(context.Background(), ExternalCallSpec{Source: airportRef("EKCH"), Destination: airportRef("EKCH"), WorkflowID: id, Step: "external/vatsim", Fetch: func(context.Context) (proto.Message, error) { calls++; return &pb.VatsimObservation{}, nil }, Commit: func(context.Context, string, proto.Message) *pb.CommandReply {
		t.Fatal("unconfirmed intent dispatched")
		return nil
	}})
	if err == nil || calls != 0 {
		t.Fatalf("lost intent acknowledgment dispatched: %v calls=%d", err, calls)
	}
	state, loadErr := worker.Writer.load(context.Background(), "fs.v1.state.airport.EKCH", airportRef("EKCH"))
	if loadErr != nil || state.Workflows[id] == nil {
		t.Fatalf("durable intent: %v %v", state, loadErr)
	}
}

func TestExternalCallSecondReplicaCannotDispatch(t *testing.T) {
	store, _, _ := navFixture(t)
	workerB := ExternalCallWorker{Writer: Writer{Store: store, NodeID: "node-b"}}
	id := uuid.NewString()
	calls := 0
	_, err := workerB.Run(context.Background(), ExternalCallSpec{Source: airportRef("EKCH"), Destination: airportRef("EKCH"), WorkflowID: id, Step: "external/vatsim", Fetch: func(context.Context) (proto.Message, error) { calls++; return &pb.VatsimObservation{}, nil }, Commit: func(context.Context, string, proto.Message) *pb.CommandReply {
		t.Fatal("nonowner dispatched")
		return nil
	}})
	if err == nil || calls != 0 {
		t.Fatalf("nonowner dispatched: %v calls=%d", err, calls)
	}
}

func TestFencedProviderPagePersistsTypedCheckpoint(t *testing.T) {
	store, objects, nav := navFixture(t)
	ctx := context.Background()
	id := uuid.NewString()
	page := &pb.ProviderPage{Provider: "airacnet", Resource: "airport/EKCH", Parsed: &pb.ProviderPage_Airac{Airac: &pb.AiracPage{Fragments: []*pb.NavData{testNav("2609", "Copenhagen")}}}}
	fetched := 0
	fetch := func(context.Context, *pb.ProviderCheckpoint, *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
		fetched++
		return page, &pb.ProviderCheckpoint{Provider: "airacnet", Resource: "airport/EKCH", Etag: "W/2609", NextPage: 2}, nil
	}
	ok, err := nav.FetchProviderPageFenced(ctx, ExternalCallWorker{Writer: nav.Writer}, id, "EKCH", "airacnet", "airport/EKCH", fetch)
	if err != nil || !ok {
		t.Fatalf("provider page: %v %v", ok, err)
	}
	checkpoint, restored, err := nav.Checkpoint(ctx, "EKCH", "airacnet", "airport/EKCH")
	if err != nil || checkpoint == nil || checkpoint.Etag != "W/2609" || !proto.Equal(restored, page) || len(objects.values) != 1 {
		t.Fatalf("typed checkpoint: %v %v %v", checkpoint, restored, err)
	}
	handoffAirport(t, store)
	navB := NavigationWeather{Writer: Writer{Store: store, NodeID: "node-b"}, Objects: objects}
	ok, err = navB.FetchProviderPageFenced(ctx, ExternalCallWorker{Writer: navB.Writer}, id, "EKCH", "airacnet", "airport/EKCH", fetch)
	if err != nil || ok || fetched != 1 {
		t.Fatalf("provider refetched on takeover: %v %v %d", ok, err, fetched)
	}
}

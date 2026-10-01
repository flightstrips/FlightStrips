package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type memoryObjects struct {
	mu      sync.Mutex
	values  map[string][]byte
	failPut bool
}

func (o *memoryObjects) GetBytes(name string) ([]byte, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	data, ok := o.values[name]
	if !ok {
		return nil, nats.ErrObjectNotFound
	}
	return append([]byte(nil), data...), nil
}
func (o *memoryObjects) PutBytes(name string, data []byte) (*nats.ObjectInfo, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.failPut {
		return nil, errors.New("object store write failed")
	}
	o.values[name] = append([]byte(nil), data...)
	return &nats.ObjectInfo{}, nil
}
func (o *memoryObjects) damage(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.values[name][0] ^= 0xff
}
func (o *memoryObjects) remove(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.values, name)
}

func navFixture(t *testing.T) (*memoryStore, *memoryObjects, NavigationWeather) {
	t.Helper()
	store := &memoryStore{}
	for _, ref := range []*pb.AggregateRef{globalRef(), airportRef("EKCH")} {
		subject, _ := Subject(ref)
		claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
		data, _ := proto.Marshal(claim)
		if _, err := store.Publish(context.Background(), subject, 0, data); err != nil {
			t.Fatal(err)
		}
	}
	objects := &memoryObjects{values: map[string][]byte{}}
	return store, objects, NavigationWeather{Writer: Writer{Store: store, NodeID: "node-a"}, Objects: objects}
}

func testNav(cycle, name string) *pb.NavData {
	now := timestamppb.New(time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC))
	data := &pb.NavData{Airport: "EKCH", Version: &pb.NavDatasetVersion{Cycle: cycle}, SchemaVersion: "v1", Provenance: &pb.NavProvenance{SourceId: "fixture", SourceRevision: cycle, ImportedAt: now}, ImportedAt: now, ValidationState: "validated", Fragment: &pb.NavData_AirportFragment{AirportFragment: &pb.NavAirportFragment{Airport: &pb.NavAirport{Icao: "EKCH", Name: name}}}}
	b, _ := (proto.MarshalOptions{Deterministic: true}).Marshal(data)
	data.Digest = digest(b)
	return data
}

func activeManifest(cycle string, refs ...*pb.NavObjectRef) *pb.NavManifest {
	m := &pb.NavManifest{Airport: "EKCH", Cycle: cycle, Objects: refs, Active: true}
	m.Digest = manifestDigest(m)
	return m
}

func TestNavigationManifestRequiresVerifiedObjectsAndSurvivesCorruption(t *testing.T) {
	ctx := context.Background()
	store, objects, adapter := navFixture(t)
	// AMAN and navigation intentionally share the canonical airport key.
	if _, err := adapter.upsert(ctx, airportRef("EKCH"), uuid.NewString(), "EKCH", &pb.EntityRecord{Value: &pb.EntityRecord_AmanAirport{AmanAirport: &pb.AmanAirport{Airport: "EKCH", Revision: 1, PolicyVersion: "v1", GeneratedAt: timestamppb.Now()}}}, nil); err != nil {
		t.Fatal(err)
	}
	first, err := adapter.PublishNav(testNav("2609", "Copenhagen"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := activeManifest("2609", first)
	if _, err := adapter.ActivateManifest(ctx, uuid.NewString(), manifest); err != nil {
		t.Fatal(err)
	}
	// A new adapter rebuilds the active digest and provenance from the log.
	recovered := NavigationWeather{Writer: Writer{Store: store}, Objects: objects}
	got, err := recovered.ActiveManifest(ctx, "EKCH")
	if err != nil || !proto.Equal(got, manifest) {
		t.Fatalf("recovered manifest=%v err=%v", got, err)
	}
	state, err := recovered.read(ctx, airportRef("EKCH"))
	if err != nil {
		t.Fatal(err)
	}
	if state.Indexes[pb.EntityKind_AMAN_AIRPORT]["EKCH"] == nil || state.Indexes[pb.EntityKind_NAV_MANIFEST]["EKCH"] == nil {
		t.Fatal("airport entities collided")
	}
	snapshot, err := state.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	fromSnapshot, err := aggregateFromSnapshot(snapshot)
	if err != nil || fromSnapshot.Indexes[pb.EntityKind_NAV_MANIFEST]["EKCH"] == nil || fromSnapshot.Indexes[pb.EntityKind_AMAN_AIRPORT]["EKCH"] == nil {
		t.Fatalf("snapshot lost airport entity: %v", err)
	}
	missing := &pb.NavObjectRef{Kind: "airport", ObjectName: "nav/" + first.Sha256, Sha256: first.Sha256}
	objects.remove(first.ObjectName)
	if _, err := adapter.ActivateManifest(ctx, uuid.NewString(), activeManifest("2610", missing)); err == nil {
		t.Fatal("missing object was committed")
	}
	if _, err := recovered.ActiveManifest(ctx, "EKCH"); err == nil {
		t.Fatal("missing active object was served")
	}
	if _, err := adapter.PublishNav(testNav("2609", "Copenhagen")); err != nil {
		t.Fatal(err)
	}
	objects.damage(first.ObjectName)
	if _, err := recovered.ActiveManifest(ctx, "EKCH"); err == nil {
		t.Fatal("corrupt active object was served")
	}
	state, err = recovered.read(ctx, airportRef("EKCH"))
	if err != nil || state.Indexes[pb.EntityKind_NAV_MANIFEST]["EKCH"].GetValue().GetNavManifest().Digest != manifest.Digest {
		t.Fatal("failed switch changed durable manifest")
	}
}

func TestNavigationConcurrentImportsOnlyCommitVerifiedBlobs(t *testing.T) {
	ctx := context.Background()
	store, objects, adapter := navFixture(t)
	refs := make([]*pb.NavObjectRef, 2)
	for i, name := range []string{"first", "second"} {
		var err error
		refs[i], err = adapter.PublishNav(testNav("2609", name))
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for _, ref := range refs {
		wg.Add(1)
		go func(ref *pb.NavObjectRef) {
			defer wg.Done()
			if _, err := adapter.ActivateManifest(ctx, uuid.NewString(), activeManifest("2609", ref)); err != nil {
				t.Error(err)
			}
		}(ref)
	}
	wg.Wait()
	recovered := NavigationWeather{Writer: Writer{Store: store}, Objects: objects}
	manifest, err := recovered.ActiveManifest(ctx, "EKCH")
	if err != nil || len(manifest.Objects) != 1 {
		t.Fatalf("concurrent manifest=%v err=%v", manifest, err)
	}
	if _, err := recovered.ReadNav(manifest.Objects[0], "EKCH"); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentImportCannotSelectMissingBlob(t *testing.T) {
	ctx := context.Background()
	_, objects, adapter := navFixture(t)
	good, err := adapter.PublishNav(testNav("2609", "good"))
	if err != nil {
		t.Fatal(err)
	}
	missing, err := adapter.PublishNav(testNav("2609", "missing"))
	if err != nil {
		t.Fatal(err)
	}
	objects.remove(missing.ObjectName)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, ref := range []*pb.NavObjectRef{good, missing} {
		wg.Add(1)
		go func(ref *pb.NavObjectRef) {
			defer wg.Done()
			_, err := adapter.ActivateManifest(ctx, uuid.NewString(), activeManifest("2609", ref))
			results <- err
		}(ref)
	}
	wg.Wait()
	close(results)
	failures := 0
	for err := range results {
		if err != nil {
			failures++
		}
	}
	if failures != 1 {
		t.Fatalf("expected one missing-object rejection, got %d", failures)
	}
	manifest, err := adapter.ActiveManifest(ctx, "EKCH")
	if err != nil || len(manifest.Objects) != 1 || manifest.Objects[0].Sha256 != good.Sha256 {
		t.Fatalf("active manifest references missing blob: %v %v", manifest, err)
	}
}

func TestProviderCheckpointAndWeatherRecoverWithoutRawBody(t *testing.T) {
	ctx := context.Background()
	store, objects, adapter := navFixture(t)
	page := &pb.ProviderPage{Provider: "airacnet", Resource: "airport/EKCH", Parsed: &pb.ProviderPage_Airac{Airac: &pb.AiracPage{Fragments: []*pb.NavData{testNav("2609", "Copenhagen")}}}}
	name, sha, err := adapter.PublishProvider(page)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := &pb.ProviderCheckpoint{Provider: "airacnet", Resource: "airport/EKCH", Etag: "W/123", NextPage: 2, ObjectName: name, Sha256: sha}
	if _, err := adapter.PutCheckpoint(ctx, "EKCH", uuid.NewString(), checkpoint); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	cache := &pb.WeatherCache{Airport: "EKCH", Provider: "openmeteo", Observation: &pb.WeatherObservation{Metar: "EKCH 290000Z"}, FetchedAt: timestamppb.New(now), ExpiresAt: timestamppb.New(now.Add(time.Hour))}
	if _, err := adapter.PutWeather(ctx, uuid.NewString(), cache); err != nil {
		t.Fatal(err)
	}
	recovered := NavigationWeather{Writer: Writer{Store: store}, Objects: objects}
	got, parsed, err := recovered.Checkpoint(ctx, "EKCH", "airacnet", "airport/EKCH")
	if err != nil || !proto.Equal(got, checkpoint) || !proto.Equal(parsed, page) {
		t.Fatalf("checkpoint=%v page=%v err=%v", got, parsed, err)
	}
	weather, err := recovered.Weather(ctx, "EKCH", "openmeteo", now)
	if err != nil || !proto.Equal(weather, cache) {
		t.Fatalf("weather=%v err=%v", weather, err)
	}
	if weather, err := recovered.Weather(ctx, "EKCH", "openmeteo", now.Add(2*time.Hour)); err != nil || weather != nil {
		t.Fatalf("expired weather=%v err=%v", weather, err)
	}
	objects.damage(name)
	if _, _, err := recovered.Checkpoint(ctx, "EKCH", "airacnet", "airport/EKCH"); err == nil {
		t.Fatal("corrupt provider page was served")
	}
}

func TestRouteCacheProvenanceAndObjectIntegrity(t *testing.T) {
	ctx := context.Background()
	store, objects, adapter := navFixture(t)
	data := testNav("2609", "unused")
	data.Fragment = &pb.NavData_RouteCandidate{RouteCandidate: &pb.NavRouteCandidate{ResolverVersion: "resolver/v1", SchemaVersion: "navdata/v1", Query: &pb.NavRouteQuery{Origin: "EKCH", Destination: "EKBI"}, Geometry: &pb.NavRouteGeometry{Coverage: "complete"}, CreatedAt: timestamppb.Now()}}
	data.Digest = ""
	b, _ := (proto.MarshalOptions{Deterministic: true}).Marshal(data)
	data.Digest = digest(b)
	ref, err := adapter.PublishNav(data)
	if err != nil {
		t.Fatal(err)
	}
	cache := &pb.NavRouteCache{RouteKey: "route-1", ObjectName: ref.ObjectName, Sha256: ref.Sha256, ResolverVersion: "resolver/v1", SchemaVersion: "navdata/v1"}
	if _, err := adapter.PutRouteCache(ctx, "EKCH", uuid.NewString(), cache); err != nil {
		t.Fatal(err)
	}
	recovered := NavigationWeather{Writer: Writer{Store: store}, Objects: objects}
	got, err := recovered.RouteCache(ctx, "EKCH", "route-1")
	if err != nil || !proto.Equal(got, cache) {
		t.Fatalf("route cache=%v err=%v", got, err)
	}
	objects.damage(ref.ObjectName)
	if _, err := recovered.RouteCache(ctx, "EKCH", "route-1"); err == nil {
		t.Fatal("corrupt route was served")
	}
}

func TestQuotaReservationRetryConcurrencyAndUncertainResponse(t *testing.T) {
	ctx := context.Background()
	store, objects, adapter := navFixture(t)
	window := time.Now().UTC().Truncate(time.Minute)
	id := uuid.NewString()
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fresh, err := adapter.ReserveQuota(ctx, id, "openmeteo", window, 2)
			if err != nil {
				t.Error(err)
			}
			results <- fresh
		}()
	}
	wg.Wait()
	close(results)
	count := 0
	for fresh := range results {
		if fresh {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("same workflow issued %d requests", count)
	}
	quota, err := adapter.Quota(ctx, "openmeteo", window)
	if err != nil || quota.Used != 1 {
		t.Fatalf("quota=%v err=%v", quota, err)
	}
	recovered := NavigationWeather{Writer: Writer{Store: store, NodeID: "node-a"}, Objects: objects}
	fresh, err := recovered.ReserveQuota(ctx, id, "openmeteo", window, 2)
	if err != nil || fresh {
		t.Fatalf("recovery retried provider: %v %v", fresh, err)
	}
	store.loseAck = true
	uncertainID := uuid.NewString()
	fresh, err = recovered.ReserveQuota(ctx, uncertainID, "openmeteo", window, 2)
	if err != nil || fresh {
		t.Fatalf("uncertain PubAck allowed request: %v %v", fresh, err)
	}
	quota, _ = recovered.Quota(ctx, "openmeteo", window)
	if quota.Used != 2 {
		t.Fatalf("uncertain reservation lost: %v", quota)
	}
	fresh, err = recovered.ReserveQuota(ctx, uncertainID, "openmeteo", window, 2)
	if err != nil || fresh {
		t.Fatalf("uncertain retry allowed request: %v %v", fresh, err)
	}
	if weather, err := recovered.Weather(ctx, "EKCH", "openmeteo", time.Now()); err != nil || weather != nil {
		t.Fatalf("fabricated weather=%v err=%v", weather, err)
	}
}

func TestWeatherFetchDoesNotRepeatAfterProviderFailure(t *testing.T) {
	ctx := context.Background()
	store, objects, adapter := navFixture(t)
	id := uuid.NewString()
	window := time.Now().UTC().Truncate(time.Hour)
	calls := 0
	fetch := func(context.Context) (*pb.WeatherObservation, error) {
		calls++
		return nil, errors.New("provider connection reset")
	}
	if _, err := adapter.FetchWeather(ctx, id, "EKCH", "openmeteo", window, 10, time.Minute, fetch); err == nil {
		t.Fatal("provider failure was ignored")
	}
	recovered := NavigationWeather{Writer: Writer{Store: store, NodeID: "node-a"}, Objects: objects}
	if fresh, err := recovered.FetchWeather(ctx, id, "EKCH", "openmeteo", window, 10, time.Minute, fetch); err != nil || fresh || calls != 1 {
		t.Fatalf("recovery reissued call: fresh=%v calls=%d err=%v", fresh, calls, err)
	}
	if cache, err := recovered.Weather(ctx, "EKCH", "openmeteo", time.Now()); err != nil || cache != nil {
		t.Fatalf("fabricated cache=%v err=%v", cache, err)
	}
}

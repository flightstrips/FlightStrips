package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
)

type positionKVTest struct {
	nats.KeyValue
	mu       sync.Mutex
	values   map[string]positionKVEntry
	next     uint64
	conflict bool
}
type positionKVEntry struct {
	nats.KeyValueEntry
	data     []byte
	revision uint64
}

func (e positionKVEntry) Value() []byte    { return e.data }
func (e positionKVEntry) Revision() uint64 { return e.revision }
func (kv *positionKVTest) Get(key string) (nats.KeyValueEntry, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	item, ok := kv.values[key]
	if !ok {
		return nil, nats.ErrKeyNotFound
	}
	return item, nil
}
func (kv *positionKVTest) Create(key string, data []byte) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if _, ok := kv.values[key]; ok {
		return 0, nats.ErrKeyExists
	}
	kv.next++
	kv.values[key] = positionKVEntry{data: append([]byte(nil), data...), revision: kv.next}
	return kv.next, nil
}
func (kv *positionKVTest) Update(key string, data []byte, expected uint64) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	old := kv.values[key]
	if kv.conflict || old.revision != expected {
		return 0, nats.ErrKeyExists
	}
	kv.next++
	kv.values[key] = positionKVEntry{data: append([]byte(nil), data...), revision: kv.next}
	return kv.next, nil
}

func TestPositionWriterOrdersDisconnectAndFencesEpoch(t *testing.T) {
	kv := &positionKVTest{values: map[string]positionKVEntry{}}
	allowed := uint64(1)
	authority := func(_ context.Context, _ int32, epoch uint64, _ string) error {
		if epoch != allowed {
			return errors.New("stale owner")
		}
		return nil
	}
	w, err := NewPositionWriter(kv, 12, 1, "conn-1", authority, 2, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	first, err := w.QueuePosition(ctx, "SAS101", &pb.AircraftPosition{Latitude: 55, Longitude: 12}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := w.QueuePosition(ctx, "SAS101", &pb.AircraftPosition{Latitude: 56, Longitude: 13}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	disconnect, err := w.QueueDisconnect(ctx, "SAS101", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.QueuePosition(ctx, "SAS101", &pb.AircraftPosition{Latitude: 57, Longitude: 14}, time.Now()); err == nil {
		t.Fatal("late position accepted after disconnect")
	}
	a, b, c := <-first, <-second, <-disconnect
	if a.Err != nil || b.Err != nil || c.Err != nil || !(a.Revision < b.Revision && b.Revision < c.Revision) {
		t.Fatalf("per-aircraft FIFO failed: %v %v %v", a, b, c)
	}
	stored, err := kv.Get("12.SAS101.1")
	if err != nil {
		t.Fatal(err)
	}
	value := &pb.PositionValue{}
	if err := pb.UnmarshalStrict(stored.Value(), value); err != nil || value.GetTombstone() == nil {
		t.Fatalf("disconnect was not final observation: %v %v", value, err)
	}
	newMaster, err := NewPositionWriter(kv, 12, 1, "conn-2", authority, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer newMaster.Close(context.Background())
	reappeared, err := newMaster.QueuePosition(ctx, "SAS101", &pb.AircraftPosition{Latitude: 57, Longitude: 14}, time.Now())
	if err != nil || (<-reappeared).Err != nil {
		t.Fatalf("new master could not restore aircraft: %v", err)
	}
	allowed = 2
	newOwner, err := NewPositionWriter(kv, 12, 2, "conn-2", authority, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer newOwner.Close(context.Background())
	fresh, err := newOwner.QueuePosition(ctx, "SAS101", &pb.AircraftPosition{Latitude: 58, Longitude: 15}, time.Now())
	if err != nil || (<-fresh).Err != nil {
		t.Fatalf("new epoch position: %v", err)
	}
	if _, err := kv.Get("12.SAS101.2"); err != nil {
		t.Fatal(err)
	}
	late, err := w.QueuePosition(ctx, "SAS102", &pb.AircraftPosition{Latitude: 55, Longitude: 12}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result := <-late; result.Err == nil {
		t.Fatal("previous owner wrote after handoff")
	}
}

func TestPositionWriterDoesNotOverwriteCASConflict(t *testing.T) {
	kv := &positionKVTest{values: map[string]positionKVEntry{}}
	w, err := NewPositionWriter(kv, 1, 1, "conn", func(context.Context, int32, uint64, string) error { return nil }, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close(context.Background())
	first, err := w.QueuePosition(context.Background(), "SAS101", &pb.AircraftPosition{Latitude: 55, Longitude: 12}, time.Now())
	if err != nil || (<-first).Err != nil {
		t.Fatalf("first write: %v", err)
	}
	kv.mu.Lock()
	kv.conflict = true
	kv.mu.Unlock()
	second, err := w.QueuePosition(context.Background(), "SAS101", &pb.AircraftPosition{Latitude: 56, Longitude: 13}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result := <-second; result.Err == nil {
		t.Fatal("CAS conflict was overwritten")
	}
}

func TestPositionDerivedCommandRechecksKVAndHoldsPositionBarrier(t *testing.T) {
	kv := &positionKVTest{values: map[string]positionKVEntry{}}
	w, err := NewPositionWriter(kv, 1, 1, "conn", func(context.Context, int32, uint64, string) error { return nil }, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close(context.Background())
	first, err := w.QueuePosition(context.Background(), "SAS101", &pb.AircraftPosition{Latitude: 55, Longitude: 12}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	firstResult := <-first
	if firstResult.Err != nil {
		t.Fatal(firstResult.Err)
	}
	var queued <-chan PositionWriteResult
	reply, err := w.ExecuteDerived(context.Background(), "SAS101", firstResult.Revision, func() (*pb.CommandReply, error) {
		var submitErr error
		queued, submitErr = w.QueuePosition(context.Background(), "SAS101", &pb.AircraftPosition{Latitude: 56, Longitude: 13}, time.Now())
		if submitErr != nil {
			return nil, submitErr
		}
		select {
		case <-queued:
			t.Fatal("position passed the derived-command barrier")
		default:
		}
		return &pb.CommandReply{Status: pb.CommandReply_COMMITTED}, nil
	})
	if err != nil || reply.GetStatus() != pb.CommandReply_COMMITTED {
		t.Fatalf("derived commit: %v %v", reply, err)
	}
	if result := <-queued; result.Err != nil {
		t.Fatal(result.Err)
	}
	called := false
	if _, err := w.ExecuteDerived(context.Background(), "SAS101", firstResult.Revision, func() (*pb.CommandReply, error) {
		called = true
		return &pb.CommandReply{Status: pb.CommandReply_COMMITTED}, nil
	}); err == nil || called {
		t.Fatal("stale source revision committed a derived transition")
	}
}

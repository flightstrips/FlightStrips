package cluster

import (
	"context"
	"sync"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type presenceKVTest struct {
	nats.KeyValue
	mu     sync.Mutex
	values map[string][]byte
}

func (kv *presenceKVTest) Put(key string, value []byte) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.values[key] = append([]byte(nil), value...)
	return 1, nil
}
func (kv *presenceKVTest) Delete(key string, _ ...nats.DeleteOpt) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	delete(kv.values, key)
	return nil
}

func TestClientPresenceLeasePublishesAndRemovesSocket(t *testing.T) {
	kv := &presenceKVTest{values: map[string][]byte{}}
	lease := ClientPresenceLease{KV: kv, Client: &pb.ClientPresence{ConnectionId: "connection-1", NodeId: "node-1", SessionId: 42,
		Cid: "123", Kind: pb.ClientPresence_EUROSCOPE, ConnectedAt: timestamppb.Now()}}
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := lease.Renew(ctx); err != nil {
		t.Fatal(err)
	}
	value := &pb.PresenceValue{}
	if err := pb.UnmarshalStrict(kv.values["client.connection-1"], value); err != nil || value.GetClient().ConnectionId != "connection-1" {
		t.Fatalf("typed presence not published: %v %v", value, err)
	}
	cancel()
	if err := lease.Run(ctx); err == nil {
		t.Fatal("cancelled lease reported success")
	}
	if _, ok := kv.values["client.connection-1"]; ok {
		t.Fatal("closed socket retained presence")
	}
}

func TestReconnectAllocatesNewPresenceGeneration(t *testing.T) {
	kv := &presenceKVTest{values: map[string][]byte{}}
	first, err := NewSocketPresenceLease(kv, "node-1", 42, "123", "EKCH_A_TWR", "TWR", false, pb.ClientPresence_EUROSCOPE)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSocketPresenceLease(kv, "node-1", 42, "123", "EKCH_A_TWR", "TWR", false, pb.ClientPresence_EUROSCOPE)
	if err != nil {
		t.Fatal(err)
	}
	if first.Client.ConnectionId == second.Client.ConnectionId {
		t.Fatal("reconnected socket reused the prior generation")
	}
	if _, err := first.Renew(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Renew(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(kv.values) != 2 {
		t.Fatal("a new generation replaced the old key")
	}
}

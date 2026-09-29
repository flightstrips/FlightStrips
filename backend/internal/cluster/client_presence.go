package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ClientPresenceLease is owned by one physical socket in the candidate
// runtime. Connection IDs are incarnation-specific and never reused.
type ClientPresenceLease struct {
	KV     nats.KeyValue
	Client *pb.ClientPresence
}

func (lease ClientPresenceLease) validate() error {
	client := lease.Client
	if lease.KV == nil || client == nil || client.ConnectionId == "" || client.NodeId == "" || client.SessionId < 1 ||
		strings.Contains(client.ConnectionId, ".") || strings.Contains(client.NodeId, ".") ||
		client.Kind == pb.ClientPresence_KIND_UNSPECIFIED || client.Cid == "" ||
		client.ConnectedAt == nil || client.ConnectedAt.CheckValid() != nil {
		return fmt.Errorf("invalid client presence lease")
	}
	return nil
}

// NewSocketPresenceLease allocates a fresh generation for each authenticated
// physical socket. Call it again after reconnect, even for the same CID.
func NewSocketPresenceLease(kv nats.KeyValue, nodeID string, sessionID int32, cid, callsign, position string, observer bool, kind pb.ClientPresence_Kind) (ClientPresenceLease, error) {
	lease := ClientPresenceLease{KV: kv, Client: &pb.ClientPresence{
		ConnectionId: uuid.NewString(), NodeId: nodeID, SessionId: sessionID,
		Cid: cid, Callsign: callsign, Position: position, Observer: observer,
		ConnectedAt: timestamppb.Now(), Kind: kind,
	}}
	return lease, lease.validate()
}

func (lease ClientPresenceLease) Renew(ctx context.Context) (uint64, error) {
	if err := lease.validate(); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	value := &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Client{Client: proto.Clone(lease.Client).(*pb.ClientPresence)}}
	data, err := proto.Marshal(value)
	if err != nil {
		return 0, err
	}
	return lease.KV.Put("client."+lease.Client.ConnectionId, data)
}

// Run publishes immediately, renews every three seconds, and expires promptly
// at socket close. A renewal failure must close the caller's physical socket;
// other backends will also expire it through the bucket's ten-second TTL.
func (lease ClientPresenceLease) Run(ctx context.Context) error {
	if err := lease.validate(); err != nil {
		return err
	}
	defer func() {
		_ = lease.KV.Delete("client." + lease.Client.ConnectionId)
	}()
	if _, err := lease.Renew(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := lease.Renew(ctx); err != nil {
				return err
			}
		}
	}
}

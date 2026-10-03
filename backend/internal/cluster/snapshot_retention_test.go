package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"testing"
)

type retentionIndex struct {
	nats.KeyValue
	entries []nats.KeyValueEntry
}

func (s retentionIndex) History(string, ...nats.WatchOpt) ([]nats.KeyValueEntry, error) {
	return s.entries, nil
}

type retentionSnapshots struct {
	nats.ObjectStore
	infos   []*nats.ObjectInfo
	deleted []string
}

func (s *retentionSnapshots) List(...nats.ListObjectsOpt) ([]*nats.ObjectInfo, error) {
	return s.infos, nil
}
func (s *retentionSnapshots) Delete(name string) error {
	s.deleted = append(s.deleted, name)
	return nil
}
func TestSnapshotPrunePreservesRecoveryPointersAndConcurrentUploads(t *testing.T) {
	entries := []nats.KeyValueEntry{}
	for _, index := range []*pb.SnapshotIndex{{ObjectName: "snapshot/global/20/retained", LastStreamSequence: 20}, {ObjectName: "snapshot/global/30/current", LastStreamSequence: 30}} {
		data, err := proto.Marshal(index)
		require.NoError(t, err)
		entries = append(entries, positionKVEntry{data: data})
	}
	objects := &retentionSnapshots{}
	for _, name := range []string{"snapshot/global/10/obsolete", "snapshot/global/20/retained", "snapshot/global/30/current", "snapshot/global/40/staged", "snapshot/airport/EKCH/10/other", "provider/vatsim/other"} {
		objects.infos = append(objects.infos, &nats.ObjectInfo{ObjectMeta: nats.ObjectMeta{Name: name}})
	}
	count, err := (SnapshotStore{Index: retentionIndex{entries: entries}, Objects: objects}).Prune(context.Background(), globalRef())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, []string{"snapshot/global/10/obsolete"}, objects.deleted)
}

package natsresources

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/nats-io/nats.go"
)

func stateConfig(names Names) *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:     names.State,
		Subjects: []string{"fs.v1.state.global", "fs.v1.state.airport.*", "fs.v1.state.session.*"},
		Storage:  nats.FileStorage, Replicas: 3, Retention: nats.LimitsPolicy,
		MaxMsgs: -1, MaxBytes: -1, MaxMsgsPerSubject: -1, MaxAge: 0,
		MaxMsgSize: 1 << 20,
		Discard:    nats.DiscardNew,
	}
}

type bucketSpec struct {
	name    string
	storage nats.StorageType
	history int64
	ttl     time.Duration
}

func buckets(names Names) []bucketSpec {
	return []bucketSpec{
		{names.Positions, nats.FileStorage, 1, 0},
		{names.Presence, nats.MemoryStorage, 1, 10 * time.Second},
		{names.SnapshotIndex, nats.FileStorage, 2, 0},
	}
}

func jetStream(nc *nats.Conn, timeout time.Duration) (nats.JetStreamContext, error) {
	return nc.JetStream(nats.MaxWait(timeout))
}

// Bootstrap only creates missing resources. Existing drift is an error, never
// an invitation to update or replace a stream with durable data in it.
func Bootstrap(ctx context.Context, nc *nats.Conn, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	js, err := jetStream(nc, c.RequestTimeout)
	if err != nil {
		return err
	}
	if err := createIfMissing(ctx, js, c.Names.State, func() error {
		_, err := js.AddStream(stateConfig(c.Names))
		return err
	}); err != nil {
		return err
	}
	for _, bucket := range buckets(c.Names) {
		b := bucket
		if err := createIfMissing(ctx, js, "KV_"+b.name, func() error {
			_, err := js.CreateKeyValue(&nats.KeyValueConfig{Bucket: b.name, Storage: b.storage, Replicas: 3, History: uint8(b.history), TTL: b.ttl})
			return err
		}); err != nil {
			return err
		}
	}
	if err := createIfMissing(ctx, js, "OBJ_"+c.Names.Objects, func() error {
		_, err := js.CreateObjectStore(&nats.ObjectStoreConfig{Bucket: c.Names.Objects, Storage: nats.FileStorage, Replicas: 3})
		return err
	}); err != nil {
		return err
	}
	return Verify(ctx, nc, c)
}

func createIfMissing(ctx context.Context, js nats.JetStreamContext, name string, create func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := js.StreamInfo(name)
	if err == nil {
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("%s: inspect: %w", name, err)
	}
	if err := create(); err != nil {
		return fmt.Errorf("%s: create: %w", name, err)
	}
	return nil
}

// Verify reads server metadata; it does not modify resources.
func Verify(ctx context.Context, nc *nats.Conn, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	js, err := jetStream(nc, c.RequestTimeout)
	if err != nil {
		return err
	}
	if _, err := js.AccountInfo(); err != nil {
		return fmt.Errorf("JetStream account: %w", err)
	}
	state := stateConfig(c.Names)
	if err := verifyStream(ctx, js, state.Name, func(got nats.StreamConfig) error {
		if !reflect.DeepEqual(got.Subjects, state.Subjects) {
			return fmt.Errorf("subjects: got %v, want %v", got.Subjects, state.Subjects)
		}
		if got.Retention != nats.LimitsPolicy || got.Storage != nats.FileStorage || got.Replicas != 3 || got.MaxAge != 0 || got.MaxMsgs != -1 || got.MaxBytes != -1 || got.MaxMsgsPerSubject != -1 || got.MaxMsgSize != 1<<20 || got.Discard != nats.DiscardNew {
			return fmt.Errorf("retention/storage/replicas/limits differ from contract")
		}
		return nil
	}); err != nil {
		return err
	}
	for _, b := range buckets(c.Names) {
		wantSubjects := []string{"$KV." + b.name + ".>"}
		if err := verifyStream(ctx, js, "KV_"+b.name, func(got nats.StreamConfig) error {
			if !reflect.DeepEqual(got.Subjects, wantSubjects) || got.Retention != nats.LimitsPolicy || got.Storage != b.storage || got.Replicas != 3 || got.MaxMsgsPerSubject != b.history || got.MaxAge != b.ttl || got.MaxMsgs != -1 || got.MaxBytes != -1 || got.Discard != nats.DiscardNew {
				return fmt.Errorf("subjects/history/TTL/retention/storage/replicas/limits differ from contract")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return verifyStream(ctx, js, "OBJ_"+c.Names.Objects, func(got nats.StreamConfig) error {
		if !reflect.DeepEqual(got.Subjects, []string{"$O." + c.Names.Objects + ".C.>", "$O." + c.Names.Objects + ".M.>"}) || got.Retention != nats.LimitsPolicy || got.Storage != nats.FileStorage || got.Replicas != 3 || got.MaxAge != 0 || got.MaxMsgs != -1 || got.MaxBytes != -1 || got.Discard != nats.DiscardNew {
			return fmt.Errorf("subjects/retention/storage/replicas/limits differ from contract")
		}
		return nil
	})
}

func verifyStream(ctx context.Context, js nats.JetStreamContext, name string, check func(nats.StreamConfig) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := js.StreamInfo(name)
	if err != nil {
		return fmt.Errorf("%s: inspect: %w", name, err)
	}
	if err := check(info.Config); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if info.Cluster == nil || info.Cluster.Leader == "" || len(info.Cluster.Replicas) != 2 {
		return fmt.Errorf("%s: expected a three-node JetStream cluster", name)
	}
	return nil
}

package cluster

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

const historyWorkingSet = 512

// historyCache is disposable local materialization, never durable authority.
// Every process rebuilds it from verified snapshots and the retained FS_STATE
// log. It preserves old command identities without keeping their protobuf
// graphs in the Go heap. Versioned keys keep earlier published states immutable.
type historyCache struct {
	db   *bolt.DB
	path string
}

func newHistoryCache() (*historyCache, error) {
	dir := os.Getenv("NATS_HISTORY_CACHE_DIR")
	if dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	cacheDir := dir
	if cacheDir == "" {
		cacheDir = os.TempDir()
	}
	// A killed process cannot run close(). Only remove our old caches after
	// acquiring their database lock; caches of live backends stay untouched.
	paths, _ := filepath.Glob(filepath.Join(cacheDir, "flightstrips-history-*.db"))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		old, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true, Timeout: time.Millisecond})
		if err != nil {
			continue
		}
		if old.Close() == nil {
			_ = os.Remove(path)
		}
	}
	file, err := os.CreateTemp(dir, "flightstrips-history-*.db")
	if err != nil {
		return nil, err
	}
	path := file.Name()
	if err = file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second, NoSync: true})
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return &historyCache{db: db, path: path}, nil
}

func (h *historyCache) close() error {
	err := h.db.Close()
	return errors.Join(err, os.Remove(h.path))
}
func (h *historyCache) check() error {
	return h.db.View(func(*bolt.Tx) error { return nil })
}
func historyBucket(ref *pb.AggregateRef, kind string) ([]byte, error) {
	subject, err := Subject(ref)
	return []byte(subject + "/" + kind), err
}
func historyKey(id string, sequence uint64) []byte {
	key := make([]byte, len(id)+9)
	copy(key, id)
	binary.BigEndian.PutUint64(key[len(id)+1:], sequence)
	return key
}
func (h *historyCache) get(ref *pb.AggregateRef, kind, id string, sequence uint64, value proto.Message) (bool, error) {
	bucket, err := historyBucket(ref, kind)
	if err != nil {
		return false, err
	}
	found := false
	err = h.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return nil
		}
		cursor := b.Cursor()
		wanted := historyKey(id, sequence)
		key, data := cursor.Seek(wanted)
		if key == nil {
			key, data = cursor.Last()
		} else if bytes.Compare(key, wanted) > 0 {
			key, data = cursor.Prev()
		}
		if len(key) != len(wanted) || !bytes.Equal(key[:len(id)+1], wanted[:len(id)+1]) {
			return nil
		}
		if err := pb.UnmarshalStrict(data, value); err != nil {
			return err
		}
		found = true
		return nil
	})
	return found, err
}

func (a *Aggregate) LookupOutcome(id string) (*pb.CommandOutcome, error) {
	if value := a.Ledger[id]; value != nil {
		return value, nil
	}
	if a.history == nil {
		return nil, nil
	}
	value := &pb.CommandOutcome{}
	found, err := a.history.get(a.Ref, "outcome", id, a.StreamSequence, value)
	if !found {
		return nil, err
	}
	return value, err
}
func (a *Aggregate) LookupWorkflow(id string) (*pb.WorkflowRecord, error) {
	if value := a.Workflows[id]; value != nil {
		return value, nil
	}
	if a.history == nil {
		return nil, nil
	}
	value := &pb.WorkflowRecord{}
	found, err := a.history.get(a.Ref, "workflow", id, a.StreamSequence, value)
	if !found {
		return nil, err
	}
	return value, err
}
func (a *Aggregate) LookupEffect(id string) (*pb.EffectRecord, error) {
	if value := a.Effects[id]; value != nil {
		return value, nil
	}
	if a.history == nil {
		return nil, nil
	}
	value := &pb.EffectRecord{}
	found, err := a.history.get(a.Ref, "effect", id, a.StreamSequence, value)
	if !found {
		return nil, err
	}
	return value, err
}

func terminalEffect(e *pb.EffectRecord) bool {
	return e.Status == pb.EffectRecord_EXECUTED || e.Status == pb.EffectRecord_FAILED || e.Status == pb.EffectRecord_EXPIRED || e.Status == pb.EffectRecord_UNKNOWN
}

func (a *Aggregate) boundHistory() error {
	if a.history == nil {
		return nil
	}
	if len(a.Ledger) <= historyWorkingSet && len(a.Workflows) <= historyWorkingSet && len(a.Effects) <= historyWorkingSet {
		return nil
	}
	// Leave the newest outcomes resident, and pin every nonterminal effect's
	// outcome. Pending workflows/effects must remain enumerable by workers.
	ids := make([]string, 0, len(a.Ledger))
	for id := range a.Ledger {
		if e := a.Effects[id]; e == nil || terminalEffect(e) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		l, r := a.Ledger[ids[i]], a.Ledger[ids[j]]
		if l.CommittedStreamSequence == r.CommittedStreamSequence {
			return ids[i] < ids[j]
		}
		return l.CommittedStreamSequence < r.CommittedStreamSequence
	})
	if len(ids) > historyWorkingSet {
		ids = ids[:len(ids)-historyWorkingSet]
	} else {
		ids = nil
	}
	workflows := []string{}
	for id, w := range a.Workflows {
		if w.Status != pb.WorkflowRecord_PENDING {
			workflows = append(workflows, id)
		}
	}
	sort.Strings(workflows)
	if len(workflows) > historyWorkingSet {
		workflows = workflows[:len(workflows)-historyWorkingSet]
	} else {
		workflows = nil
	}
	effects := []string{}
	for id, e := range a.Effects {
		if terminalEffect(e) {
			effects = append(effects, id)
		}
	}
	sort.Strings(effects)
	if len(effects) > historyWorkingSet {
		effects = effects[:len(effects)-historyWorkingSet]
	} else {
		effects = nil
	}
	if len(ids)+len(workflows)+len(effects) == 0 {
		return nil
	}
	err := a.history.db.Update(func(tx *bolt.Tx) error {
		put := func(kind, id string, value proto.Message) error {
			bucket, err := historyBucket(a.Ref, kind)
			if err != nil {
				return err
			}
			b, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
			data, err := proto.Marshal(value)
			if err != nil {
				return err
			}
			return b.Put(historyKey(id, a.StreamSequence), data)
		}
		for _, id := range ids {
			if err := put("outcome", id, a.Ledger[id]); err != nil {
				return err
			}
		}
		for _, id := range workflows {
			if err := put("workflow", id, a.Workflows[id]); err != nil {
				return err
			}
		}
		for _, id := range effects {
			if err := put("effect", id, a.Effects[id]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("history cache: %w", err)
	}
	for _, id := range ids {
		delete(a.Ledger, id)
	}
	for _, id := range workflows {
		delete(a.Workflows, id)
	}
	for _, id := range effects {
		delete(a.Effects, id)
	}
	return nil
}

// visit walks only the latest version at this state's checkpoint. Each value
// is consumed within its read transaction and must not escape without copying.
func (a *Aggregate) visitHistory(kind string, visit func(string, []byte) error) error {
	if a.history == nil {
		return nil
	}
	bucket, err := historyBucket(a.Ref, kind)
	if err != nil {
		return err
	}
	return a.history.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for key, _ := c.First(); key != nil; {
			if len(key) < 9 {
				return fmt.Errorf("invalid history index key")
			}
			id := string(key[:len(key)-9])
			wanted := historyKey(id, a.StreamSequence)
			matched, data := c.Seek(wanted)
			if matched == nil {
				matched, data = c.Last()
			} else if bytes.Compare(matched, wanted) > 0 {
				matched, data = c.Prev()
			}
			if len(matched) == len(wanted) && bytes.Equal(matched[:len(id)+1], wanted[:len(id)+1]) {
				if err := visit(id, data); err != nil {
					return err
				}
			}
			// All versions of one ID are contiguous. Seek the next ID directly.
			next := append([]byte(id), byte(1))
			key, _ = c.Seek(next)
		}
		return nil
	})
}

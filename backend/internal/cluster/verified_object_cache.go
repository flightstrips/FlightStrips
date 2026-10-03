package cluster

import (
	"container/list"
	"sync"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// VerifiedObjectCache retains only checksum-verified immutable typed objects.
// Accepted checkpoint/manifest references remain authoritative. Each replica
// owns a bounded cache and verifies objects again after restart or eviction.
// The byte budget measures serialized payload size; decoded Go heap is larger.
type VerifiedObjectCache struct {
	mu           sync.Mutex
	limit, bytes int
	entries      map[string]*list.Element
	order        *list.List
}

type verifiedObjectEntry struct {
	name  string
	value *pb.ObjectValue
	size  int
}

func NewVerifiedObjectCache(bytes int) *VerifiedObjectCache {
	return &VerifiedObjectCache{limit: bytes, entries: make(map[string]*list.Element), order: list.New()}
}

func (c *VerifiedObjectCache) get(name string) *pb.ObjectValue {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	entry := c.entries[name]
	if entry == nil {
		c.mu.Unlock()
		return nil
	}
	c.order.MoveToFront(entry)
	value := entry.Value.(verifiedObjectEntry).value
	c.mu.Unlock()
	return proto.Clone(value).(*pb.ObjectValue)
}

func (c *VerifiedObjectCache) put(name string, value *pb.ObjectValue, size int) {
	if c == nil || size > c.limit || size <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry := c.entries[name]; entry != nil {
		c.order.MoveToFront(entry)
		return
	}
	for c.bytes+size > c.limit || len(c.entries) >= 128 {
		entry := c.order.Back()
		old := entry.Value.(verifiedObjectEntry)
		delete(c.entries, old.name)
		c.bytes -= old.size
		c.order.Remove(entry)
	}
	c.entries[name] = c.order.PushFront(verifiedObjectEntry{name: name, value: proto.Clone(value).(*pb.ObjectValue), size: size})
	c.bytes += size
}

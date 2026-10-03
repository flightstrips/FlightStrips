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
	providers    map[string]acceptedProviderObject
	order        *list.List
}

type acceptedProviderObject struct {
	name     string
	revision uint64
}

type verifiedObjectEntry struct {
	name  string
	value *pb.ObjectValue
	size  int
}

func NewVerifiedObjectCache(bytes int) *VerifiedObjectCache {
	return &VerifiedObjectCache{limit: bytes, entries: make(map[string]*list.Element), order: list.New(), providers: make(map[string]acceptedProviderObject)}
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
	if value.GetProviderPage() != nil && !c.providerAcceptedLocked(name) {
		return
	}
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

// acceptProvider follows a durable checkpoint monotonically. Staged pages and
// readers holding an older checkpoint cannot reintroduce a superseded feed.
func (c *VerifiedObjectCache) acceptProvider(scope, key, name string, revision uint64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	identity := scope + "\x00" + key
	prior, exists := c.providers[identity]
	if exists && revision <= prior.revision {
		return
	}
	if name == "" {
		delete(c.providers, identity)
	} else {
		c.providers[identity] = acceptedProviderObject{name: name, revision: revision}
	}
	if prior.name != "" && prior.name != name && !c.providerAcceptedLocked(prior.name) {
		c.removeLocked(prior.name)
	}
}
func (c *VerifiedObjectCache) providerAcceptedLocked(name string) bool {
	for _, current := range c.providers {
		if current.name == name {
			return true
		}
	}
	return false
}
func (c *VerifiedObjectCache) removeLocked(name string) {
	if entry := c.entries[name]; entry != nil {
		c.bytes -= entry.Value.(verifiedObjectEntry).size
		delete(c.entries, name)
		c.order.Remove(entry)
	}
}
func (c *VerifiedObjectCache) discard(name string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeLocked(name)
}

func (c *VerifiedObjectCache) providerStats() (int, int) {
	if c == nil {
		return 0, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	count, bytes := 0, 0
	for _, entry := range c.entries {
		value := entry.Value.(verifiedObjectEntry)
		if value.value.GetProviderPage() != nil {
			count++
			bytes += value.size
		}
	}
	return count, bytes
}

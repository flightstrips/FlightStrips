package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"testing"
)

func TestVerifiedObjectCacheReadsDetachedValuesAndEvicts(t *testing.T) {
	cache := NewVerifiedObjectCache(10)
	value := &pb.ObjectValue{SchemaVersion: 1}
	cache.put("a", value, 6)
	value.SchemaVersion = 99
	got := cache.get("a")
	if got.SchemaVersion != 1 {
		t.Fatal("cache retained caller's mutable value")
	}
	got.SchemaVersion = 98
	if cache.get("a").SchemaVersion != 1 {
		t.Fatal("read mutated cached value")
	}
	cache.put("b", &pb.ObjectValue{SchemaVersion: 1}, 6)
	if cache.get("a") != nil || cache.get("b") == nil || cache.bytes > cache.limit {
		t.Fatal("cache did not evict to its byte budget")
	}
	cache.put("oversized", &pb.ObjectValue{}, 11)
	if cache.get("oversized") != nil {
		t.Fatal("oversized object cached")
	}
}

func TestNavigationCacheRejectsCorruptionAndUsesImmutableIdentity(t *testing.T) {
	_, objects, nav := navFixture(t)
	ref, err := nav.PublishNav(testNav("2610", "first"))
	if err != nil {
		t.Fatal(err)
	}
	nav.Cache = NewVerifiedObjectCache(1024 * 1024)
	objects.damage(ref.ObjectName)
	if _, err := nav.ReadNav(ref, "EKCH"); err == nil {
		t.Fatal("corrupt object accepted")
	}
	if nav.Cache.get(ref.ObjectName) != nil {
		t.Fatal("corrupt object cached")
	}
	objects.damage(ref.ObjectName)
	first, err := nav.ReadNav(ref, "EKCH")
	if err != nil {
		t.Fatal(err)
	}
	objects.remove(ref.ObjectName)
	first.Airport = "EGLL"
	got, err := nav.ReadNav(ref, "EKCH")
	if err != nil || got.Airport != "EKCH" {
		t.Fatalf("verified immutable read: %v %v", got, err)
	}
	if _, err := nav.ReadNav(ref, "EGLL"); err == nil {
		t.Fatal("cache bypassed airport identity")
	}
	other := *ref
	other.Sha256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := nav.ReadNav(&other, "EKCH"); err == nil {
		t.Fatal("cache bypassed digest identity")
	}
}

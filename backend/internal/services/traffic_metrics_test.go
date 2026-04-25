package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"testing"
	"time"
)

func TestBuildTrafficSnapshotCountsExpectedBuckets(t *testing.T) {
	now := time.Date(2026, time.April, 25, 13, 15, 0, 0, time.UTC)
	arrRecent := "1305"
	arrOld := "1250"
	depRecent := "131000"

	snapshot := buildTrafficSnapshot([]*models.Strip{
		{Bay: shared.BAY_STAND},
		{Bay: shared.BAY_CLEARED},
		{Bay: shared.BAY_TAXI},
		{Bay: shared.BAY_TAXI_TWR},
		{Bay: shared.BAY_HIDDEN},
		{Bay: shared.BAY_PUSH, CdmData: &models.CdmData{Aldt: &arrRecent, Aobt: &depRecent}},
		{Bay: shared.BAY_UNKNOWN, CdmData: &models.CdmData{Aldt: &arrOld}},
	}, now)

	if snapshot.onStand != 2 {
		t.Fatalf("expected 2 on-stand aircraft, got %d", snapshot.onStand)
	}
	if snapshot.taxiing != 3 {
		t.Fatalf("expected 3 taxiing aircraft, got %d", snapshot.taxiing)
	}
	if snapshot.arr15m != 1 {
		t.Fatalf("expected 1 recent arrival, got %d", snapshot.arr15m)
	}
	if snapshot.dep15m != 1 {
		t.Fatalf("expected 1 recent departure, got %d", snapshot.dep15m)
	}
}

func TestWithinLast15MinHandlesMidnightWrap(t *testing.T) {
	now := time.Date(2026, time.April, 25, 0, 5, 0, 0, time.UTC)

	if !withinLast15Min("2355", now) {
		t.Fatal("expected 23:55 to be within the last 15 minutes across midnight")
	}
	if withinLast15Min("0015", now) {
		t.Fatal("did not expect a future clock time to count as within the last 15 minutes")
	}
}

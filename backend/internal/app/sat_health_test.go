package app

import (
	"FlightStrips/internal/aman"
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/vatsim"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEvaluateSATHealthFeedStates(t *testing.T) {
	ready := appconfig.StandAssignmentReadiness{Enabled: true, Ready: true}
	now := time.Now().UTC()
	tests := []struct {
		name       string
		snapshot   vatsim.Snapshot
		wantStatus string
		wantReady  bool
	}{
		{name: "unavailable", snapshot: vatsim.Snapshot{}, wantStatus: "feed_unavailable"},
		{name: "valid", snapshot: vatsim.Snapshot{Timestamp: now}, wantStatus: "ready", wantReady: true},
		{name: "stale", snapshot: vatsim.Snapshot{Timestamp: now.Add(-2 * time.Minute)}, wantStatus: "feed_stale"},
		{name: "failed", snapshot: vatsim.Snapshot{Timestamp: now, LastRefreshError: errors.New("network down")}, wantStatus: "feed_failed"},
		{name: "recovered", snapshot: vatsim.Snapshot{Timestamp: now}, wantStatus: "ready", wantReady: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateSATHealth(ready, tt.snapshot, time.Minute)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReady, got.Ready)
		})
	}
}

func TestAMANVATSIMHealthReportsStaleReasonAndRestoration(t *testing.T) {
	now := time.Now().UTC()
	stale := amanVATSIMHealth(amanHealthSnapshotSource{snapshot: vatsim.Snapshot{Timestamp: now.Add(-2 * time.Minute)}}, time.Minute)
	fresh := amanVATSIMHealth(amanHealthSnapshotSource{snapshot: vatsim.Snapshot{Timestamp: now}}, time.Minute)
	if stale.Status != aman.HealthDegraded || stale.Reason != "snapshot_stale" || stale.AgeSeconds == nil {
		t.Fatalf("stale AMAN VATSIM health = %#v", stale)
	}
	if fresh.Status != aman.HealthReady || fresh.Reason != "" || fresh.AgeSeconds == nil {
		t.Fatalf("fresh AMAN VATSIM health = %#v", fresh)
	}
}

type amanHealthSnapshotSource struct{ snapshot vatsim.Snapshot }

func (s amanHealthSnapshotSource) Snapshot() vatsim.Snapshot { return s.snapshot }

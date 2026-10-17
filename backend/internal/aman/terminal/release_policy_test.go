package terminal

import (
	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestApproachReleasePolicyValidatesBeforeNavigationAcquisition(t *testing.T) {
	policy := aman.ApproachReleasePolicy{Enabled: true, TargetBuffer: 10, TaperStart: 6, MaxAdvanceSeconds: 600, LookaheadSeconds: 120}
	cfg := Configuration{ApproachRelease: &policy}
	require.NoError(t, cfg.ValidateOperationalSettings())
	for _, invalid := range []aman.ApproachReleasePolicy{
		{TargetBuffer: 6, TaperStart: 6, MaxAdvanceSeconds: 600, LookaheadSeconds: 120},
		{TargetBuffer: 10, TaperStart: 0, MaxAdvanceSeconds: 600, LookaheadSeconds: 120},
		{TargetBuffer: 10, TaperStart: 6, MaxAdvanceSeconds: -1, LookaheadSeconds: 120},
		{TargetBuffer: 10, TaperStart: 6, MaxAdvanceSeconds: 600, LookaheadSeconds: 0},
	} {
		cfg.ApproachRelease = &invalid
		require.ErrorContains(t, cfg.ValidateOperationalSettings(), "approachRelease")
	}
	require.NoError(t, (Configuration{}).ValidateOperationalSettings(), "missing policy retains legacy operation")
}

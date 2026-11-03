package pdc

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPDCSharedEffectsRetainHumanPluginBinding(t *testing.T) {
	b := &testutil.MockCommandBinding{SessionID: 7, CIDValue: "OWN"}
	ctx, err := shared.WithHumanCommand(context.Background(), b)
	require.NoError(t, err)
	ctx = context.WithoutCancel(ctx)
	hub := &testutil.MockEuroscopeHub{GetMasterCidFn: func(int32) string { t.Fatal("human work consulted master"); return "MASTER" }}
	s := &Service{euroscopeHub: hub}
	require.Equal(t, "OWN", s.resolveEuroscopeTargetCID(ctx, 7))
	s.notifyClearedFlagEuroscope(ctx, 7, "SAS123", true, "OTHER")
	require.Equal(t, []es.OutgoingMessage{es.ClearedFlagEvent{Callsign: "SAS123", Cleared: true}}, b.Sent())
	require.Empty(t, hub.ClearedFlags)
	b.Unavailable.Store(true)
	require.Empty(t, s.resolveEuroscopeTargetCID(ctx, 7))
	s.notifyClearedFlagEuroscope(ctx, 7, "SAS123", true, "MASTER")
	require.Len(t, b.Sent(), 1)
	require.Error(t, s.IssueClearance(ctx, "SAS123", "", "OWN", 7))
	_, err = s.applyMandatoryRouteReview(ctx, &models.Session{ID: 7}, &models.Strip{Callsign: "SAS123"})
	require.Error(t, err)
}

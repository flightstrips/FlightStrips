package frontend

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"
	pkg "FlightStrips/pkg/models"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestStripFieldEditUseOwnPluginAndRejectMissingFlightStaleOrDisconnectedBinding(t *testing.T) {
	seen := time.Now()
	strip := &models.Strip{ID: 12, Session: 7, Version: 3, Callsign: "RAD123", HasFP: true, EuroscopeSeenAt: &seen, BoardState: &models.StripBoardState{}}
	repo := &radisFieldStore{stripUpdateStoreFake: &stripUpdateStoreFake{getByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }, appendControllerModifiedFieldFn: func(context.Context, int32, string, string) error { return nil }}, reserve: func(_ context.Context, _ int32, id, version int32, _ *models.StripBoardState) (bool, error) {
		if id != strip.ID || version != strip.Version {
			return false, nil
		}
		strip.Version++
		return true, nil
	}}
	sender := &recordingStripUpdateEuroscopeSender{}
	s := NewFrontendStripUpdateService(repo, nil, sender, nil, nil, nil, nil, nil)
	b := &testutil.MockCommandBinding{SessionID: 7, CIDValue: "OWN"}
	ctx, err := shared.WithHumanCommand(t.Context(), b)
	require.NoError(t, err)
	r := pkg.StripRevision{StripID: 12, Version: 3, Definition: "synthetic"}
	alt, heading, squawk := int32(2300), int32(123), "2401"
	req := FrontendStripUpdateRequest{Session: 7, Cid: "OWN", Event: fe.UpdateStripDataEvent{Callsign: "RAD123", Revision: &r, Altitude: &alt, Heading: &heading, Squawk: &squawk}}
	require.NoError(t, s.UpdateStrip(ctx, req))
	require.Equal(t, []es.OutgoingMessage{es.AssignedSquawkEvent{Callsign: "RAD123", Squawk: "2401"}, es.ClearedAltitudeEvent{Callsign: "RAD123", Altitude: 2300}, es.HeadingEvent{Callsign: "RAD123", Heading: 123}}, b.Sent())
	require.Empty(t, sender.clearedAltitudes, "legacy/master send never executes")
	r.Version = 2
	require.Error(t, s.UpdateStrip(ctx, req))
	r.Version = strip.Version
	strip.HasFP = false
	require.Error(t, s.UpdateStrip(ctx, req))
	strip.HasFP = true
	strip.EuroscopeSeenAt = nil
	require.Error(t, s.UpdateStrip(ctx, req))
	strip.EuroscopeSeenAt = &seen
	b.Unavailable.Store(true)
	require.Error(t, s.UpdateStrip(ctx, req))
	require.Len(t, b.Sent(), 3)
}

type radisFieldStore struct {
	*stripUpdateStoreFake
	reserve func(context.Context, int32, int32, int32, *models.StripBoardState) (bool, error)
}

func (s *radisFieldStore) UpdateStripBoardState(ctx context.Context, session, id, version int32, state *models.StripBoardState) (bool, error) {
	return s.reserve(ctx, session, id, version, state)
}

func TestRADISConflictingFieldRequestsConsumeRevisionBeforePluginEcho(t *testing.T) {
	seen := time.Now()
	current := models.Strip{ID: 12, Session: 7, Version: 3, Callsign: "RAD123", HasFP: true, EuroscopeSeenAt: &seen, BoardState: &models.StripBoardState{}}
	store := &radisFieldStore{stripUpdateStoreFake: &stripUpdateStoreFake{getByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { copy := current; return &copy, nil }, appendControllerModifiedFieldFn: func(context.Context, int32, string, string) error { return nil }}, reserve: func(_ context.Context, _ int32, id, version int32, _ *models.StripBoardState) (bool, error) {
		if current.ID != id || current.Version != version {
			return false, nil
		}
		current.Version++
		return true, nil
	}}
	service := NewFrontendStripUpdateService(store, nil, &recordingStripUpdateEuroscopeSender{}, nil, nil, nil, nil, nil)
	one := &testutil.MockCommandBinding{SessionID: 7, CIDValue: "ONE"}
	two := &testutil.MockCommandBinding{SessionID: 7, CIDValue: "TWO"}
	oneCtx, err := shared.WithHumanCommand(t.Context(), one)
	require.NoError(t, err)
	twoCtx, err := shared.WithHumanCommand(t.Context(), two)
	require.NoError(t, err)
	revision := &pkg.StripRevision{StripID: 12, Version: 3, Definition: "synthetic"}
	squawk := "2401"
	require.NoError(t, service.UpdateStrip(oneCtx, FrontendStripUpdateRequest{Session: 7, Cid: "ONE", Event: fe.UpdateStripDataEvent{Callsign: "RAD123", Revision: revision, Squawk: &squawk}}))
	// No plugin result has arrived; the first request must still consume version 3.
	squawk = "2402"
	require.ErrorContains(t, service.UpdateStrip(twoCtx, FrontendStripUpdateRequest{Session: 7, Cid: "TWO", Event: fe.UpdateStripDataEvent{Callsign: "RAD123", Revision: revision, Squawk: &squawk}}), "stale")
	require.Len(t, one.Sent(), 1)
	require.Empty(t, two.Sent())
	require.EqualValues(t, 4, current.Version)
}

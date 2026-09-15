package euroscope

import (
	"context"
	"errors"
	"testing"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	events "FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/require"
)

func TestHoldRequiresTrackingControllerRegardlessOfMasterRole(t *testing.T) {
	for _, test := range []struct {
		name, sender, tracker     string
		master, observer, allowed bool
	}{
		{"tracking slave", "EKCH_A_APP", "EKCH_A_APP", false, false, true},
		{"tracking master", "EKCH_A_APP", "EKCH_A_APP", true, false, true},
		{"other master", "EKCH_TWR", "EKCH_A_APP", true, false, false},
		{"other slave", "EKCH_B_APP", "EKCH_A_APP", false, false, false},
		{"untracked", "EKCH_A_APP", "", true, false, false},
		{"blank identities", "", "", true, false, false},
		{"observer", "EKCH_A_APP", "EKCH_A_APP", false, true, false},
		{"normalized callsign", "ekch_a_app", " EKCH_A_APP ", false, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &holdAuthorityService{}
			hub := &Hub{stripService: service, master: make(map[int32]*Client), server: &testutil.MockServer{
				StripRepoVal: &testutil.MockStripRepository{GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
					require.Equal(t, int32(42), session)
					require.Equal(t, "SAS123", callsign)
					return &models.Strip{Session: session, Callsign: callsign, TrackingController: test.tracker}, nil
				}},
			}}
			client := &Client{hub: hub, session: 42, callsign: test.sender, observer: test.observer}
			if test.master {
				hub.master[42] = client
			}
			err := handleHold(context.Background(), client, Message{Message: mustMarshalMessage(t, &events.HoldEvent{
				Callsign: "SAS123", Hold: "OLPIB", HoldType: "enroute", HoldEat: "1422",
			})})
			if test.allowed {
				require.NoError(t, err)
				require.Equal(t, 1, service.calls)
			} else {
				var domainErr *aman.DomainError
				require.ErrorAs(t, err, &domainErr)
				require.Equal(t, aman.ErrorUnauthorized, domainErr.Class)
				require.Zero(t, service.calls)
			}
		})
	}
}

type holdAuthorityService struct {
	noOpStripService
	calls int
}

func (s *holdAuthorityService) UpdateHold(context.Context, int32, string, string, string, string) error {
	s.calls++
	return nil
}

func TestHoldReplayAfterTrackingConfirmation(t *testing.T) {
	strip := &models.Strip{Callsign: "SAS123", TrackingController: "EKCH_A_APP"}
	service := &trackingConfirmationService{strip: strip}
	hub := &Hub{stripService: service, send: make(chan internalMessage, 1), server: &testutil.MockServer{
		StripRepoVal: &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }},
	}}
	tracker := &Client{hub: hub, session: 42, callsign: "EKCH_B_APP"}
	master := &Client{hub: hub, session: 42, callsign: "EKCH_TWR"}
	// An empty hold is an authoritative cancellation, including on replay.
	hold := Message{Message: mustMarshalMessage(t, &events.HoldEvent{Callsign: "SAS123"})}
	require.Error(t, handleHold(context.Background(), tracker, hold))
	require.Zero(t, service.calls)
	change := Message{Message: mustMarshalMessage(t, &events.TrackingControllerChangedEvent{Callsign: "SAS123", TrackingController: "EKCH_B_APP"})}
	require.NoError(t, handleTrackingControllerChanged(context.Background(), master, change))
	confirmation := <-hub.send
	require.Equal(t, int32(42), confirmation.session)
	require.Nil(t, confirmation.cid, "confirmation must reach the tracking slave, not just the master")
	event := confirmation.message.(events.TrackingControllerChangedEvent)
	require.Equal(t, strip.TrackingController, event.TrackingController)
	require.NoError(t, handleHold(context.Background(), tracker, hold))
	require.Equal(t, 1, service.calls)
	service.err = errors.New("write failed")
	require.Error(t, handleTrackingControllerChanged(context.Background(), master, change))
	require.Empty(t, hub.send, "a failed ownership update must not be confirmed")
}

type trackingConfirmationService struct {
	holdAuthorityService
	strip *models.Strip
	err   error
}

func (s *trackingConfirmationService) HandleTrackingControllerChanged(_ context.Context, _ int32, _ string, tracker string) error {
	if s.err != nil {
		return s.err
	}
	s.strip.TrackingController = tracker
	return nil
}

package euroscope

import (
	"context"
	"errors"
	"testing"

	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestControlAcknowledgementCapturesAppliedElection(t *testing.T) {
	for _, elect := range []bool{false, true} {
		name := "unchanged role"
		if elect {
			name = "promoted master"
		}
		t.Run(name, func(t *testing.T) {
			hub := membershipTestHub()
			hub.server = &testutil.MockServer{
				ControllerRepoVal: &testutil.MockControllerRepository{SetPositionFn: func(context.Context, int32, string, string) (int64, error) { return 1, nil }},
				FrontendHubVal:    &testutil.MockFrontendHub{}, UpdateLayoutsFn: func(int32) error { return nil },
			}
			root := membershipTestRoot(hub, "123", "instance")
			root.transport.evidence = es.ControlEvent{Environment: "LIVE", Callsign: "EKCH_A_TWR", PrimaryFrequency: "119.500", RelevantAirport: "AAAA"}
			member := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "member")
			member.updateIdentity("119.500", "EKCH_A_TWR", false, "")
			if elect {
				other := membershipTestRoot(hub, "456", "other")
				current := membershipTestMember(hub, other, 1, "AAAA", "LIVE", "current")
				current.updateIdentity("119.555", "EKDK_B_CTR", false, "")
			}
			hub.election(1).epoch.Store(7)
			control := proto.Clone(&root.transport.evidence).(*es.ControlEvent)
			control.PrimaryFrequency = "131.040"
			payload, err := proto.Marshal(control)
			require.NoError(t, err)
			require.NoError(t, handleControlEvent(t.Context(), root, Message{Message: payload}))
			require.Same(t, member, hub.getMasterClient(1))
			epoch := uint64(7)
			if elect {
				epoch++
			}
			var ack *es.Envelope
			roles := 0
			for len(root.send) > 0 {
				data, err := (<-root.send).Marshal()
				require.NoError(t, err)
				var event es.Envelope
				require.NoError(t, proto.Unmarshal(data, &event))
				if event.GetSessionInfo() != nil {
					roles++
				}
				if event.GetControl() != nil {
					ack = &event
				}
			}
			require.NotNil(t, ack, "even an unchanged role needs an applied-control barrier")
			require.True(t, proto.Equal(control, ack.GetControl()))
			require.Equal(t, epoch, ack.Context.MasterEpoch)
			require.True(t, member.matches(ack.Context))
			if !elect {
				require.Zero(t, roles, "acknowledgement must not force a full sync")
			}
			// A retry of identical evidence must also be acknowledged.
			require.NoError(t, handleControlEvent(t.Context(), root, Message{Message: payload}))
			require.NotEmpty(t, root.send)
		})
	}
}

func TestFailedControlUpdateIsNotAcknowledged(t *testing.T) {
	hub := membershipTestHub()
	failure := errors.New("position update failed")
	hub.server = &testutil.MockServer{ControllerRepoVal: &testutil.MockControllerRepository{
		SetPositionFn: func(context.Context, int32, string, string) (int64, error) { return 0, failure },
	}}
	root := membershipTestRoot(hub, "123", "instance")
	root.transport.evidence = es.ControlEvent{Environment: "LIVE", Callsign: "EKCH_A_TWR", PrimaryFrequency: "119.500", RelevantAirport: "AAAA"}
	member := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "member")
	member.updateIdentity("119.500", "EKCH_A_TWR", false, "")
	control := proto.Clone(&root.transport.evidence).(*es.ControlEvent)
	control.PrimaryFrequency = "131.040"
	payload, err := proto.Marshal(control)
	require.NoError(t, err)
	require.ErrorIs(t, handleControlEvent(t.Context(), root, Message{Message: payload}), failure)
	require.Empty(t, root.send)
}

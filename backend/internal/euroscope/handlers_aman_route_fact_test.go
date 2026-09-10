package euroscope

import (
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestHandleAMANRouteFactDerivesTrustedConnectionFields(t *testing.T) {
	reporter := &routeFactReporter{}
	client := &Client{
		hub: &Hub{amanRouteFacts: reporter}, session: 42, airport: "EKCH", callsign: "EKCH_A_APP",
	}
	payload, err := proto.Marshal(&euroscopeEvents.AMANRouteFactEvent{Version: 1, Data: &euroscopeEvents.AMANRouteFactData{
		Callsign: "SAS123", Kind: "direct_to", DirectToFix: stringPointer("KEMAX"), ObservedAt: "2026-08-20T11:59:00Z",
	}})
	require.NoError(t, err)

	err = handleAMANRouteFact(context.Background(), client, Message{Type: euroscopeEvents.AMANRouteFact, Message: payload})
	require.NoError(t, err)
	require.Equal(t, int32(42), reporter.session)
	require.Equal(t, "EKCH", reporter.airport)
	require.Equal(t, "EKCH_A_APP", reporter.controller)
	require.Equal(t, "SAS123", reporter.callsign)
	require.Equal(t, "KEMAX", *reporter.fix)
	require.Equal(t, time.Date(2026, 8, 20, 11, 59, 0, 0, time.UTC), reporter.observedAt)
}

func TestHandleAMANRouteFactRejectsSpoofableOrExtendedContract(t *testing.T) {
	reporter := &routeFactReporter{}
	client := &Client{hub: &Hub{amanRouteFacts: reporter}, session: 42, airport: "EKCH", callsign: "EKCH_A_APP"}

	validData := &euroscopeEvents.AMANRouteFactData{Callsign: "SAS123", Kind: "direct_to", ObservedAt: "2026-08-20T11:59:00Z"}
	unknownField, err := proto.Marshal(&euroscopeEvents.AMANRouteFactEvent{Version: 1, Data: validData})
	require.NoError(t, err)
	unknownField = protowire.AppendTag(unknownField, 99, protowire.VarintType)
	unknownField = protowire.AppendVarint(unknownField, 1)

	invalidEvents := []*euroscopeEvents.AMANRouteFactEvent{
		{Version: 2, Data: validData},
		{Version: 1},
		{Version: 1, Data: &euroscopeEvents.AMANRouteFactData{Callsign: "SAS123", Kind: "other", ObservedAt: "2026-08-20T11:59:00Z"}},
	}
	for _, event := range invalidEvents {
		payload, marshalErr := proto.Marshal(event)
		require.NoError(t, marshalErr)
		require.Error(t, handleAMANRouteFact(context.Background(), client, Message{Type: euroscopeEvents.AMANRouteFact, Message: payload}))
	}
	require.Error(t, handleAMANRouteFact(context.Background(), client, Message{Type: euroscopeEvents.AMANRouteFact, Message: unknownField}))
	require.Zero(t, reporter.calls)
}

func stringPointer(value string) *string { return &value }

type routeFactReporter struct {
	calls                         int
	session                       int32
	airport, callsign, controller string
	fix                           *string
	observedAt                    time.Time
}

func (r *routeFactReporter) ReportDirectTo(_ context.Context, session int32, airport, callsign, controller string, fix *string, observedAt time.Time) error {
	r.calls++
	r.session, r.airport, r.callsign, r.controller, r.fix, r.observedAt = session, airport, callsign, controller, fix, observedAt
	return nil
}

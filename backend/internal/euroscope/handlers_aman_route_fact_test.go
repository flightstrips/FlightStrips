package euroscope

import (
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHandleAMANRouteFactDerivesTrustedConnectionFields(t *testing.T) {
	reporter := &routeFactReporter{}
	client := &Client{
		hub: &Hub{amanRouteFacts: reporter}, session: 42, airport: "EKCH", callsign: "EKCH_A_APP",
	}
	payload := []byte(`{"type":"aman.route_fact","version":1,"data":{"callsign":"SAS123","kind":"direct_to","direct_to_fix":"KEMAX","observed_at":"2026-08-20T11:59:00Z"}}`)

	err := handleAMANRouteFact(context.Background(), client, Message{Type: euroscopeEvents.AMANRouteFact, Message: payload})
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

	for _, payload := range []string{
		`{"type":"aman.route_fact","version":1,"data":{"callsign":"SAS123","kind":"direct_to","direct_to_fix":null,"observed_at":"2026-08-20T11:59:00Z","airport":"EKBI"}}`,
		`{"type":"aman.route_fact","version":1,"data":{"callsign":"SAS123","kind":"direct_to","direct_to_fix":null,"observed_at":"2026-08-20T11:59:00Z","issuer":"OTHER"}}`,
		`{"type":"aman.route_fact","version":1,"data":{"callsign":"SAS123","kind":"direct_to","direct_to_fix":null,"observed_at":"2026-08-20T11:59:00Z","flight_id":"spoofed"}}`,
		`{"type":"aman.route_fact","version":2,"data":{"callsign":"SAS123","kind":"direct_to","direct_to_fix":null,"observed_at":"2026-08-20T11:59:00Z"}}`,
	} {
		require.Error(t, handleAMANRouteFact(context.Background(), client, Message{Type: euroscopeEvents.AMANRouteFact, Message: []byte(payload)}))
	}
	require.Zero(t, reporter.calls)
}

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

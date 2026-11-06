package airportworkflow

import (
	"FlightStrips/internal/config"
	"bytes"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

func TestRADISAdmissionAndAnnotationContract(t *testing.T) {
	data, err := os.ReadFile("../config/testdata/radis-ekbi.yaml")
	require.NoError(t, err)
	defs, err := config.NewAirportRegistry(config.ProductionAirportCatalog(), []config.AirportInput{{Source: "synthetic-only", Reader: bytes.NewReader(data)}})
	require.NoError(t, err)
	definition, _ := defs.Lookup("EKBI")
	b, err := Bind(definition, RADISDescriptor())
	require.NoError(t, err)
	for _, tc := range []struct {
		origin, dest string
		lat          float64
		alt          int32
		admit        bool
		bay          string
	}{{"EKBI", "EKYT", 0, 100, true, "PASSIVE_TOP"}, {"EKYT", "EKBI", 0, 10000, true, "PASSIVE_BOTTOM"}, {"EGLL", "EHAM", 0, 0, false, "PASSIVE_BOTTOM"}, {"EKBI", "EKYT", 0, 9000, false, "PASSIVE_TOP"}, {"EKYT", "EKBI", 20, 100, false, "PASSIVE_BOTTOM"}} {
		d, err := b.Lifecycle(Context{Session: 1}, Lifecycle{Origin: tc.origin, Destination: tc.dest, Latitude: tc.lat, Altitude: tc.alt})
		require.NoError(t, err)
		require.Equal(t, tc.admit, d.Admit)
		require.Equal(t, tc.bay, *d.Bay)
	}
	c := Context{Session: 1, Source: HumanSource, Now: time.Date(2026, 1, 1, 12, 34, 56, 0, time.UTC)}
	d, err := b.Action(c, "strip.annotate", StripAnnotation{Field: "rea", Enabled: true})
	require.NoError(t, err)
	require.Equal(t, "2026-01-01T12:34:56Z", *d.Annotations.REA)
	require.Nil(t, d.GroundState)
	require.Nil(t, d.Cleared)
	for _, edit := range []StripAnnotation{{Field: "not-a-field"}, {Field: "atd", Value: "2460"}, {Field: "rnav", Value: "arbitrary"}, {Field: "qnh", Value: "fake"}} {
		_, err = b.Action(c, "strip.annotate", edit)
		require.Error(t, err)
	}
	_, err = b.Action(Context{Session: 1, Source: ObservationSource}, "strip.annotate", StripAnnotation{Field: "notes"})
	require.Error(t, err)
	_, err = b.Action(c, "strip.move", Move{Bay: "RUNWAY"})
	require.Error(t, err, "local strips have no ES context")
	c.Strip = Snapshot{Exists: true, BoardMember: true, Origin: "EKBI", Bay: "ACTIVE", EuroscopeSeen: true, HasFP: true}
	d, err = b.Action(c, "strip.move", Move{Bay: "RUNWAY"})
	require.NoError(t, err)
	require.Equal(t, "LINEUP", *d.GroundState)
	d, err = b.Observe(c, Observation{Kind: FullFlight, Origin: "EKBI", GroundState: "TAXI"})
	require.NoError(t, err)
	require.Equal(t, "ACTIVE", *d.Bay)
	require.Nil(t, d.GroundState)
}

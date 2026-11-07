package airportworkflow

import (
	"FlightStrips/internal/config"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func regionalWorkflow(t *testing.T, airport string) *Bound {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "..", "..", "..")
	definitions, err := config.LoadAirportFiles(config.ProductionAirportCatalog(), []string{"backend/config/airports/" + strings.ToLower(airport) + ".airport.yaml"}, config.AirportLoadOptions{RepositoryRoot: root})
	require.NoError(t, err)
	definition, ok := definitions.Lookup(airport)
	require.True(t, ok)
	b, err := Bind(definition, RADISDescriptor())
	require.NoError(t, err)
	return b
}

func TestRegionalRADISConfirmedBayEffectsAndLifecycle(t *testing.T) {
	for _, airport := range []string{"EKBI", "EKYT"} {
		t.Run(airport, func(t *testing.T) {
			b := regionalWorkflow(t, airport)
			geography := b.Definition().Document().Geography
			settings := b.Definition().Document().Workflow.Settings.(config.RADISSettings)
			require.False(t, b.Definition().ServiceEnabled("cdm"))
			require.Nil(t, settings.RemovalDistanceNM)
			require.Nil(t, settings.DepartureRemovalAGLFT)
			require.Equal(t, 60.0, settings.DisconnectRemovalSeconds)
			require.Equal(t, 300, settings.RecoverySeconds)
			for _, departure := range []bool{true, false} {
				origin, destination := "EGLL", airport
				if departure {
					origin, destination = airport, "EGLL"
				}
				c := Context{Session: 1, Source: HumanSource, Now: time.Date(2026, 11, 7, 12, 34, 0, 0, time.UTC), Strip: Snapshot{Exists: true, BoardMember: true, HasFP: true, EuroscopeSeen: true, Origin: origin, Destination: destination, Bay: "OTHER"}}
				for bay, pair := range map[string][2]string{"ACTIVE": {"TAXI", "TAXI"}, "RUNWAY": {"TKOFF", "LAND"}, "PASSIVE_TOP": {"", ""}, "PASSIVE_BOTTOM": {"", "PARK"}} {
					d, err := b.Action(c, "strip.move", Move{Bay: bay})
					require.NoError(t, err)
					index := 1
					if departure {
						index = 0
					}
					require.Equal(t, pair[index], *d.GroundState)
				}
				c.Strip.Bay = "RUNWAY"
				d, err := b.Action(c, "strip.move", Move{Bay: "RUNWAY"})
				require.NoError(t, err)
				require.Nil(t, d.GroundState, "same-bay ordering has no ES effect")
				d, err = b.Observe(c, Observation{Kind: Position, Latitude: 0, Longitude: 0, Altitude: 40000})
				require.NoError(t, err)
				require.Nil(t, d.Visibility, "admitted strips are not removed by distance or altitude")
				for _, altitude := range []int32{13000, 13001} {
					l, err := b.Lifecycle(Context{Session: 1}, Lifecycle{Origin: origin, Destination: destination, Latitude: geography.Latitude, Longitude: geography.Longitude, Altitude: altitude})
					require.NoError(t, err)
					require.Equal(t, !departure || altitude == 13000, l.Admit)
				}
				removed := c.Now.Add(-299 * time.Second)
				c.Strip.RemovedAt, c.Strip.RemovalReason = &removed, "disconnect"
				d, err = b.Observe(c, Observation{Kind: Position, Latitude: 0, Longitude: 0, Altitude: 40000})
				require.NoError(t, err)
				require.True(t, d.Visibility.Visible, "returning known flights do not need a second admission")
				c.Now = c.Now.Add(time.Second)
				d, err = b.Observe(c, Observation{Kind: Position})
				require.NoError(t, err)
				require.Nil(t, d.Visibility, "five-minute recovery boundary is exclusive")
				c.Strip.RemovedAt = nil
				for _, manual := range []bool{false, true} {
					c.Strip.ManualAdmission, c.Strip.Local = manual, !manual
					l, err := b.Lifecycle(c, Lifecycle{Kind: Removal, Reason: "disconnect"})
					require.NoError(t, err)
					require.Nil(t, l.Visibility, "NEW and FIND need manual deletion even after ES association")
				}
			}
			l, err := b.Lifecycle(Context{Session: 1}, Lifecycle{Origin: airport, Destination: airport, Latitude: geography.Latitude, Longitude: geography.Longitude})
			require.NoError(t, err)
			require.Equal(t, "PASSIVE_TOP", *l.Bay)
		})
	}
}

func TestRegionalRADISATDUsesAGLAndRetainsManualOverrides(t *testing.T) {
	b := regionalWorkflow(t, "EKBI")
	c := Context{Session: 1, Now: time.Date(2026, 11, 7, 12, 34, 0, 0, time.UTC), Strip: Snapshot{BoardMember: true, HasFP: true, Origin: "EGLL", Destination: "EKBI", Bay: "ACTIVE"}}
	for _, altitude := range []int32{746, 747} {
		d, err := b.Observe(c, Observation{Kind: Position, Altitude: altitude})
		require.NoError(t, err)
		if altitude == 746 {
			require.Nil(t, d.ReconciledATD)
		} else {
			require.Equal(t, "1234", *d.ReconciledATD)
		}
	}
	c.Source = HumanSource
	d, err := b.Action(c, "strip.annotate", StripAnnotation{Field: "atd", Value: ""})
	require.NoError(t, err)
	require.True(t, d.ATDManual, "manual clearing also suppresses later automatic stamps")
	c.Strip.ATDManual = true
	d, err = b.Observe(c, Observation{Kind: Position, Altitude: 1000})
	require.NoError(t, err)
	require.Nil(t, d.ReconciledATD)
	c.Strip.ATDManual = false
	c.Strip.Annotations.ATD = "1100"
	d, err = b.Observe(c, Observation{Kind: Position, Altitude: 1000})
	require.NoError(t, err)
	require.Nil(t, d.ReconciledATD, "first timestamp stays frozen")
	c.Source = HumanSource
	stamp := "1234"
	require.Error(t, b.Validate(c, Decision{ReconciledATD: &stamp}), "controller actions cannot spoof observations")
}

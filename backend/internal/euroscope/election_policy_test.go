package euroscope

import (
	"FlightStrips/internal/config"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
)

func TestSessionElectionUsesBoundAirportPreemptionPolicy(t *testing.T) {
	data, err := os.ReadFile("../../config/airports/ekch.airport.yaml")
	require.NoError(t, err)
	definitions := func(preempt bool) *config.AirportDefinition {
		text := string(data)
		if !preempt {
			text = strings.Replace(text, "preempt_higher_priority: true", "preempt_higher_priority: false", 1)
		}
		registry, err := config.NewAirportRegistry(config.ProductionAirportCatalog(), []config.AirportInput{{Source: "preemption-fixture", Reader: strings.NewReader(text)}})
		require.NoError(t, err)
		definition, ok := registry.Lookup("EKCH")
		require.True(t, ok)
		return definition
	}
	retain, preempt := definitions(false), definitions(true)
	hub := &Hub{}
	towerA, fmpA := masterTestClient(1, "EKCH_A_TWR", "tower-a"), masterTestClient(1, "EKDK_FMP", "fmp-a")
	towerB, fmpB := masterTestClient(2, "EKCH_A_TWR", "tower-b"), masterTestClient(2, "EKDK_FMP", "fmp-b")
	towerA.definition = retain
	fmpA.definition = retain
	towerB.definition = preempt
	fmpB.definition = preempt
	hub.setMasterClient(towerA)
	hub.setMasterClient(towerB)
	require.False(t, hub.promoteMasterIfPreferred(fmpA, true))
	require.True(t, hub.promoteMasterIfPreferred(fmpB, true))
	require.Same(t, towerA, hub.getMasterClient(1))
	require.Same(t, fmpB, hub.getMasterClient(2))
	require.Equal(t, uint64(1), hub.election(1).epoch.Load())
	require.Equal(t, uint64(2), hub.election(2).epoch.Load())
}

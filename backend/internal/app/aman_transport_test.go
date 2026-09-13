package app

import (
	internalEuroscope "FlightStrips/internal/euroscope"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAMANTransportKeepsEuroScopeHubForHoldingEATWhenGainLossIsDisabled(t *testing.T) {
	transport := &amanTransport{holdingEATEnabled: true}
	hub := &internalEuroscope.Hub{}
	transport.setHubs(nil, hub)

	require.Same(t, hub, transport.euroscopeHub)
}

func TestAMANTransportDoesNotInstallEuroScopeHubWhenOutputsAreDisabled(t *testing.T) {
	transport := &amanTransport{}
	transport.setHubs(nil, &internalEuroscope.Hub{})

	require.Nil(t, transport.euroscopeHub)
}

func TestAMANTransportInstallsEuroScopeHubWhenGainLossIsEnabled(t *testing.T) {
	hub := &internalEuroscope.Hub{}
	transport := &amanTransport{gainLossEnabled: true}
	transport.setHubs(nil, hub)

	require.Same(t, hub, transport.euroscopeHub)
}

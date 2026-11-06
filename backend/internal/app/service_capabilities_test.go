package app

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisabledAirportAMANRejectsCommandsAndPublicationBeforeDependencies(t *testing.T) {
	registry := syntheticServiceAirports(t) // CDM/stands/text enabled; PDC/AMAN disabled
	cfg := Config{Airports: registry, EnablePDC: true, EnableECFMP: true, AMAN: aman.RuntimeConfig{Mode: aman.ModeAuthoritative, EnabledAirports: []string{"AAAA"}}}
	filtered, err := enforceAirportCapabilities(cfg)
	require.NoError(t, err)
	require.False(t, filtered.EnablePDC)
	require.Equal(t, aman.ModeDisabled, filtered.AMAN.Mode)
	require.Empty(t, filtered.AMAN.EnabledAirports)
	sessions := &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
		t.Fatal("disabled service must reject before session or domain lookup")
		return nil, nil
	}}
	scope := amanAirportScope(cfg, sessions)
	gate := &amanCommandGate{scope: scope} // nil command/health peers prove early rejection
	auth := aman.CommandContext{SessionID: 1, Airport: "AAAA"}
	_, err = gate.MoveFlight(t.Context(), auth, aman.MoveFlightCommand{})
	require.ErrorContains(t, err, "disabled")
	_, err = gate.RebuildArrivalSequence(t.Context(), auth, aman.RebuildArrivalSequenceCommand{})
	require.ErrorContains(t, err, "disabled")
	_, err = gate.CurrentRevision(t.Context(), "AAAA")
	require.ErrorContains(t, err, "disabled")
	transport := &amanTransport{scope: scope}
	_, err = transport.CurrentAMANState(t.Context(), "AAAA")
	require.ErrorContains(t, err, "disabled")
	_, err = transport.CurrentAMANGainLoss(t.Context(), "AAAA")
	require.ErrorContains(t, err, "disabled")
	_, err = transport.CurrentAMANHoldingEAT(t.Context(), "AAAA")
	require.ErrorContains(t, err, "disabled")
	require.ErrorContains(t, transport.PublishAMANState(t.Context(), aman.AirportState{Airport: "AAAA", SessionID: 1}), "disabled")
	require.ErrorContains(t, transport.PublishAMANAuthority(t.Context(), aman.AirportState{Airport: "AAAA", SessionID: 1}), "disabled")
}

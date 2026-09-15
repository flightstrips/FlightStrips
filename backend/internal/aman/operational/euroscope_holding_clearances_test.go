package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEuroScopeHoldingClearanceObserverPublishesPolicyNeutralFacts(t *testing.T) {
	now := time.Date(2026, time.September, 11, 14, 20, 0, 0, time.UTC)
	tests := []struct {
		name            string
		origin          string
		destination     string
		wantDestination string
		holdType        string
	}{
		{name: "en-route arrival", origin: "ESSA", destination: "ekch", wantDestination: "EKCH", holdType: "enroute"},
		{name: "TSA hold", origin: "ESSA", destination: "EKCH", wantDestination: "EKCH", holdType: "tsa"},
		{name: "departure", origin: "EKCH", destination: "ESSA", wantDestination: "ESSA", holdType: "enroute"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink := &holdingClearanceSink{}
			observer, err := NewEuroScopeHoldingClearanceObserver(EuroScopeHoldingClearanceObserverDependencies{
				Sink: sink, Identities: euroScopeIdentityBinder{}, Now: func() time.Time { return now },
			})
			require.NoError(t, err)
			cid := "123456"
			altitude := int32(12000)
			strip := &models.Strip{
				Callsign: "SAS123", VatsimCID: &cid, Origin: test.origin, Destination: test.destination,
				Hold: "OLPIB", HoldType: test.holdType, HoldEat: "1422", ClearedAltitude: &altitude,
			}

			require.NoError(t, observer.ObserveHoldingClearance(context.Background(), strip))
			require.Len(t, sink.facts, 1, "eligibility remains downstream read-model policy")
			fact := sink.facts[0]
			require.Equal(t, aman.FlightID("aman-123456"), fact.FlightID)
			require.Equal(t, test.wantDestination, fact.Destination)
			require.Equal(t, aman.HoldingClearanceType(test.holdType), fact.HoldType)
			require.Equal(t, "1422", fact.HoldEAT, "the boundary must not resolve or normalize HHMM")
			require.Equal(t, altitude, *fact.ClearedAltitude)
			require.Equal(t, now, fact.ObservedAt)
		})
	}
}

type holdingClearanceSink struct{ facts []aman.HoldingClearanceFact }

func TestEuroScopeHoldingClearanceObserverBatchesFactsWithOriginalObservationTimes(t *testing.T) {
	now := time.Date(2026, 9, 14, 19, 0, 0, 0, time.UTC)
	sink := &batchHoldingClearanceSink{}
	observer, err := NewEuroScopeHoldingClearanceObserver(EuroScopeHoldingClearanceObserverDependencies{
		Sink: sink, Identities: euroScopeIdentityBinder{}, Now: func() time.Time { return now.Add(time.Minute) },
	})
	require.NoError(t, err)
	cid1, cid2 := "1234567", "2345678"
	err = observer.ObserveHoldingClearances(context.Background(), []shared.HoldingClearanceObservation{
		{Strip: &models.Strip{Callsign: "SAS123", VatsimCID: &cid1, Destination: "ekch", Hold: "OLPIB"}, ObservedAt: now},
		{Strip: &models.Strip{Callsign: "SAS456", VatsimCID: &cid2, Destination: "EKCH", Hold: "TIDVU"}, ObservedAt: now.Add(time.Second)},
		{Strip: &models.Strip{Callsign: "no-identity"}, ObservedAt: now},
	})
	require.NoError(t, err)
	require.Equal(t, 1, sink.batches)
	require.Empty(t, sink.holdingClearanceSink.facts, "the single-fact fallback must not be used")
	require.Len(t, sink.batch, 2)
	require.Equal(t, now, sink.batch[0].ObservedAt)
	require.Equal(t, now.Add(time.Second), sink.batch[1].ObservedAt)
	require.Equal(t, aman.FlightID("aman-2345678"), sink.batch[1].FlightID)
}

type batchHoldingClearanceSink struct {
	holdingClearanceSink
	batches int
	batch   []aman.HoldingClearanceFact
}

func (s *batchHoldingClearanceSink) ObserveHoldingClearances(_ context.Context, facts []aman.HoldingClearanceFact) error {
	s.batches++
	s.batch = facts
	return nil
}

func (s *holdingClearanceSink) ObserveHoldingClearance(_ context.Context, fact aman.HoldingClearanceFact) error {
	s.facts = append(s.facts, fact)
	return nil
}

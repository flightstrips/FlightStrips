package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type clearedHandoverRouteStub struct {
	target             string
	resolvedStrip      *models.Strip
	ownerAtRouteUpdate string
}

func (s *clearedHandoverRouteStub) ResolveClearedStripOwnerContext(_ context.Context, strip *models.Strip, _ int32) (string, bool, error) {
	s.resolvedStrip = strip
	return s.target, true, nil
}

func (s *clearedHandoverRouteStub) UpdateRouteForStrip(string, int32, bool) error {
	return nil
}

func (s *clearedHandoverRouteStub) UpdateRouteForStripContext(context.Context, string, int32, bool) error {
	if s.resolvedStrip != nil && s.resolvedStrip.Owner != nil {
		s.ownerAtRouteUpdate = *s.resolvedStrip.Owner
	}
	return nil
}

func (s *clearedHandoverRouteStub) ComputeNextDisplayForStripContext(context.Context, *models.Strip, int32) (*models.NextDisplay, error) {
	return &models.NextDisplay{Label: "AD", Frequency: "121.905"}, nil
}

func TestWithRouteRecalculatorWiresHandoverAndDisplayCapabilities(t *testing.T) {
	route := &clearedHandoverRouteStub{}

	service := NewStripService(&testutil.MockStripRepository{}, WithRouteRecalculator(route))

	assert.Same(t, route, service.routeRecalculator)
	assert.Same(t, route, service.routeDisplayComputer)
	assert.Same(t, route, service.clearedOwnerResolver)
}

func TestAutoAssumeForClearedStrip_UsesRadioHandoverResolverWithoutCoordination(t *testing.T) {
	const (
		session  = int32(42)
		callsign = "SAS380"
		target   = "121.905"
	)
	ownerSet := false
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			if !ownerSet {
				return &models.Strip{
					Callsign:    callsign,
					Origin:      "EKCH",
					Destination: "EKBI",
					Bay:         "CLEARED",
					Version:     7,
					NextOwners:  []string{target},
				}, nil
			}
			owner := target
			return &models.Strip{
				Callsign:   callsign,
				Owner:      &owner,
				NextOwners: []string{"121.730"},
			}, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, nextOwners, previousOwners []string) error {
			assert.Empty(t, nextOwners)
			assert.Empty(t, previousOwners)
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, owner *string, version int32) (int64, error) {
			require.NotNil(t, owner)
			assert.Equal(t, target, *owner)
			assert.Equal(t, int32(7), version)
			ownerSet = true
			return 1, nil
		},
	}
	hub := &testutil.MockFrontendHub{}
	service := NewStripService(stripRepo)
	service.SetFrontendHub(hub)
	route := &clearedHandoverRouteStub{target: target}
	service.SetRouteRecalculator(route)

	err := service.AutoAssumeForClearedStrip(context.Background(), session, callsign)

	require.NoError(t, err)
	assert.Equal(t, target, route.ownerAtRouteUpdate)
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, target, hub.OwnersUpdates[0].Owner)
	assert.Equal(t, []string{"121.730"}, hub.OwnersUpdates[0].NextOwners)
	require.NotNil(t, hub.OwnersUpdates[0].NextDisplay)
	assert.Equal(t, "AD", hub.OwnersUpdates[0].NextDisplay.Label)
	assert.Equal(t, "121.905", hub.OwnersUpdates[0].NextDisplay.Frequency)
}

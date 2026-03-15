package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveAirborneController_NilSID verifies that a strip without a SID returns nil.
func TestResolveAirborneController_NilSID(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	strip := &models.Strip{Sid: nil}

	controller, err := svc.resolveAirborneController(strip, []*models.Controller{})
	require.NoError(t, err)
	assert.Nil(t, controller, "no controller should be returned for a strip without a SID")
}

// TestResolveAirborneController_EmptySID verifies that an empty SID returns nil.
func TestResolveAirborneController_EmptySID(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	empty := ""
	strip := &models.Strip{Sid: &empty}

	controller, err := svc.resolveAirborneController(strip, []*models.Controller{})
	require.NoError(t, err)
	assert.Nil(t, controller, "no controller should be returned for a strip with an empty SID")
}

// TestResolveAirborneController_UnknownSID verifies that an unconfigured SID returns nil without error.
// config package variables are empty during unit tests, so any SID will be unknown.
func TestResolveAirborneController_UnknownSID(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	sid := "NOSUCHSID9A"
	strip := &models.Strip{Sid: &sid}

	controller, err := svc.resolveAirborneController(strip, []*models.Controller{})
	require.NoError(t, err, "unknown SID must not return an error — it falls back to nil")
	assert.Nil(t, controller, "no controller should be returned for an unknown SID")
}

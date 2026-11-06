package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStandSessionCleanupPreservesOtherAirportAndEnvironment(t *testing.T) {
	allocator := &StandAllocationService{automaticTerminalFailures: map[string]automaticTerminalFailure{"1:SAS101": {}, "2:SAS101": {}, "3:SAS101": {}}}
	departure := &DepartureLifecycleService{allocations: allocator, warnings: map[string]string{"1:SAS101": "a", "2:SAS101": "b", "3:SAS101": "c"}}
	allocator.RemoveSession(1)
	departure.RemoveSession(1)
	require.Len(t, allocator.automaticTerminalFailures, 2)
	require.Equal(t, map[string]string{"2:SAS101": "b", "3:SAS101": "c"}, departure.warnings)
	require.ErrorContains(t, allocator.validateAirport(t.Context(), 1, "AAAA"), "expired")
	require.NoError(t, allocator.validateAirport(t.Context(), 2, "BBBB"))
	require.NoError(t, allocator.validateAirport(t.Context(), 3, "AAAA"))
}

package standdiagnostics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAllocationFailureLogRetainsNewestFailures(t *testing.T) {
	t.Parallel()

	log := NewAllocationFailureLog(2)
	log.Record(AllocationFailure{Callsign: "ONE", OccurredAt: time.Unix(1, 0)})
	log.Record(AllocationFailure{Callsign: "TWO", OccurredAt: time.Unix(2, 0)})
	log.Record(AllocationFailure{Callsign: "THREE", OccurredAt: time.Unix(3, 0)})

	failures := log.List()
	require.Len(t, failures, 2)
	require.Equal(t, "THREE", failures[0].Callsign)
	require.Equal(t, uint64(3), failures[0].ID)
	require.Equal(t, "TWO", failures[1].Callsign)

	failures[0].Callsign = "CHANGED"
	require.Equal(t, "THREE", log.List()[0].Callsign)
}

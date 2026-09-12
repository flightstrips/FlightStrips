package sequence_test

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestRunwayClosuresRemoveFiniteAndIndefiniteCapacity(t *testing.T) {
	start := testTime()
	finiteEnd := start.Add(4 * time.Minute)
	policy := simplePolicy("A", start, 60)
	policy.Closures = []aman.RunwayClosure{{Start: start.Add(time.Minute), End: &finiteEnd}}

	result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
		flight("BEFORE", "A", start, "M"),
		flight("BLOCKED", "A", start.Add(time.Minute), "M"),
	}})
	require.NoError(t, err)
	require.Equal(t, start, entryFor(t, result, "BEFORE").Time)
	require.Equal(t, finiteEnd, entryFor(t, result, "BLOCKED").Time)

	policy.Closures = []aman.RunwayClosure{{Start: start.Add(time.Minute)}}
	_, err = sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
		flight("NO-CAPACITY", "A", start.Add(time.Minute), "M"),
	}})
	require.ErrorContains(t, err, "no slot grid")
}

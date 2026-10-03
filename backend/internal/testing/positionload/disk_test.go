package positionload

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDiskAlertAndReleaseBoundaries(t *testing.T) {
	for _, tc := range []struct {
		used           uint64
		alert, blocked bool
	}{{6999, false, false}, {7000, true, false}, {8499, true, false}, {8500, true, true}, {10000, true, true}} {
		gate, err := EvaluateDisk(tc.used, 10000)
		require.NoError(t, err)
		require.Equal(t, tc.alert, gate.Alert)
		require.Equal(t, tc.blocked, gate.ReleaseBlocked)
	}
	for _, tc := range [][2]uint64{{0, 0}, {101, 100}} {
		gate, err := EvaluateDisk(tc[0], tc[1])
		require.Error(t, err)
		require.True(t, gate.ReleaseBlocked)
	}
}

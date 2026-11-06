package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAMANSessionCleanupPreservesOtherSessionAuthority(t *testing.T) {
	transport := &amanTransport{lastGainLossAuthority: map[string]bool{"1/EKCH": true, "2/EKCH": true, "3/AAAA": true}, lastHoldingEAT: map[string]map[string]holdingEATProjection{"1/EKCH": {}, "2/EKCH": {}, "3/AAAA": {}}}
	transport.RemoveSession(1)
	require.Equal(t, map[string]bool{"2/EKCH": true, "3/AAAA": true}, transport.lastGainLossAuthority)
	require.Len(t, transport.lastHoldingEAT, 2)
	require.Contains(t, transport.lastHoldingEAT, "2/EKCH")
}

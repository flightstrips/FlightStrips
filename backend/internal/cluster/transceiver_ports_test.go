package cluster_test

import (
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/pdc"
)

// The PDC source port and the shared reconciliation reader use the accepted
// transceiver generation; no process-local provider cache is constructed.
var _ pdc.TransceiverLookup = (*cluster.TransceiverSource)(nil)
var _ pdc.TransceiverLookup = cluster.TransceiverGeneration{}

package cluster_test

import (
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/server"
)

// Both production source ports compile against the candidate; no legacy cache
// is constructed or started. Task20 passes this reader to both constructors.
var _ pdc.TransceiverLookup = (*cluster.TransceiverSource)(nil)
var _ server.TransceiverLookup = (*cluster.TransceiverSource)(nil)
var _ pdc.TransceiverLookup = cluster.TransceiverGeneration{}
var _ server.TransceiverLookup = cluster.TransceiverGeneration{}

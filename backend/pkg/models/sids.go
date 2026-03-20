package models

// SidInfo holds a SID name and the runway it belongs to, as reported by EuroScope.
type SidInfo struct {
	Name   string `json:"name"`
	Runway string `json:"runway"`
}

// AvailableSids is the list of SID identifiers available at the session's airport,
// sourced from the master EuroScope client on each sync.
type AvailableSids []SidInfo

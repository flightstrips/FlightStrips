package aman

import (
	"context"
	"time"
)

// ForcedFMPGrant gives one controller callsign FMP authority for an AMAN
// session while no FMP position is online.
type ForcedFMPGrant struct {
	SessionID int32
	Airport   string
	Callsign  string
	CID       string
	GrantedAt time.Time
	// ControllerID is the holder's controller record at the time of the
	// grant. Logging off deletes that record, so a stored grant whose ID no
	// longer matches belongs to an earlier logon and was revoked. Zero means
	// unknown and never survives a restart.
	ControllerID int32
}

// ForcedFMPStore persists the single forced FMP grant of each session so it
// survives a backend restart.
type ForcedFMPStore interface {
	LoadForcedFMP(ctx context.Context, sessionID int32) (ForcedFMPGrant, bool, error)
	SaveForcedFMP(ctx context.Context, grant ForcedFMPGrant) error
	ClearForcedFMP(ctx context.Context, sessionID int32) error
}

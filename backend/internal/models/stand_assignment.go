package models

import "time"

// StandAssignment is the durable, backend-authoritative SAT assignment for a
// callsign within a session. The operational stand remains strips.stand; this
// record stores SAT decision and provenance metadata alongside it.
type StandAssignment struct {
	ID             int64
	SessionID      int32
	Callsign       string
	Stand          string
	Direction      string
	Stage          string
	Source         string
	RuleID         *string
	Tier           *int32
	MatchedVariant *string
	ETA            *time.Time
	ETASource      *string
	AssignedAt     *time.Time
	ExpiresAt      *time.Time
	Manual         bool
	Acknowledged   bool
	AcknowledgedAt *time.Time
	AcknowledgedBy *string
	VatsimCID      *int64
	VatsimRevision *int64
	Version        int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// StandBlock represents a closure or occupancy that is not necessarily backed
// by a strip. A nil Callsign is valid for controller-created closures.
type StandBlock struct {
	ID        int64
	SessionID int32
	Stand     string
	BlockType string
	Source    string
	Reason    *string
	Callsign  *string
	CreatedBy *string
	ExpiresAt *time.Time
	Manual    bool
	Version   int32
	CreatedAt time.Time
	UpdatedAt time.Time
}

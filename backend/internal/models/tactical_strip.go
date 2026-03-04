package models

import "time"

// TacticalStripType represents valid types for tactical strips
type TacticalStripType = string

const (
	TacticalStripTypeMemaid   TacticalStripType = "MEMAID"
	TacticalStripTypeCrossing TacticalStripType = "CROSSING"
	TacticalStripTypeStart    TacticalStripType = "START"
	TacticalStripTypeLand     TacticalStripType = "LAND"
)

type TacticalStrip struct {
	ID          int64
	SessionID   int32
	Type        string
	Bay         string
	Label       string
	Aircraft    *string
	ProducedBy  string
	Sequence    int32
	TimerStart  *time.Time
	Confirmed   bool
	ConfirmedBy *string
	CreatedAt   time.Time
}

// TacticalStripSequence is used for recalculation broadcasts
type TacticalStripSequence struct {
	ID       int64
	Sequence int32
}

package models

import "time"

type PdcClearance struct {
	ID              int32
	Session         int32
	Callsign        string
	MessageSequence int32
	Clearance       string
	SentAt          *time.Time
	Acknowledged    bool
	AcknowledgedAt  *time.Time
}

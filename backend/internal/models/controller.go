package models

import "time"

type Controller struct {
	ID                int32
	Session           int32
	Callsign          string
	Position          string
	Observer          bool
	Cid               *string
	LastSeenEuroscope *time.Time
	LastSeenFrontend  *time.Time
	Layout            *string
}

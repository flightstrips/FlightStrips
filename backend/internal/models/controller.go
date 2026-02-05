package models

import "time"

type Controller struct {
	ID                int32
	Session           int32
	Callsign          string
	Position          string
	Cid               *string
	LastSeenEuroscope *time.Time
	LastSeenFrontend  *time.Time
	Layout            *string
}

package models

import "time"

type Coordination struct {
	ID            int32
	Session       int32
	StripID       int32
	FromPosition  string
	ToPosition    string
	CoordinatedAt *time.Time
}

package models

import "time"

// StripAnnotations are shared paper-strip data, separate from ES flight facts.
// Clock stamps are UTC RFC3339 instants; clearing a stamp stores nil.
type StripAnnotations struct {
	ReviewRequired bool    `json:"review_required"`
	Notes          string  `json:"notes"`
	Deicing        bool    `json:"deicing"`
	REA            *string `json:"rea"`
	ATA            *string `json:"ata"`
	ATD            string  `json:"atd"`
	EndurancePOB   string  `json:"endurance_pob"`
	QNH            string  `json:"qnh"`
	RNAV           string  `json:"rnav"`
	VFR            bool    `json:"vfr"`
}

// StripRevision fences reused callsigns and concurrent edits as well as changes
// to the immutable airport definition. Socket board context fences memberships.
type StripRevision struct {
	StripID    int32  `json:"strip_id"`
	Version    int32  `json:"version"`
	Definition string `json:"definition_revision"`
}

type StripBoardState struct {
	CreatedAt       *time.Time       `json:"created_at,omitempty"`
	Annotations     StripAnnotations `json:"annotations"`
	RemovedAt       *time.Time       `json:"removed_at"`
	RemovalReason   string           `json:"removal_reason"`
	Local           bool             `json:"local"`
	ManualAdmission bool             `json:"manual_admission,omitempty"`
	ATDManual       bool             `json:"atd_manual,omitempty"`
}

type LocalStripCreation struct {
	Definition        string `json:"definition_revision"`
	Callsign          string `json:"callsign"`
	Origin            string `json:"origin"`
	Destination       string `json:"destination"`
	Aircraft          string `json:"aircraft_type"`
	Route             string `json:"route"`
	Registration      string `json:"registration"`
	RequestedAltitude int32  `json:"requested_altitude"`
	Runway            string `json:"runway"`
	Notes             string `json:"notes"`
}
type ConnectedFlightCandidate struct {
	Callsign       string    `json:"callsign"`
	CID            string    `json:"cid"`
	LogonTime      time.Time `json:"logon_time"`
	SourceRevision int64     `json:"source_revision,string"`
	Origin         string    `json:"origin"`
	Destination    string    `json:"destination"`
	Aircraft       string    `json:"aircraft_type"`
}

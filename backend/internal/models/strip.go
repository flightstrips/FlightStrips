package models

import "time"

type NextDisplay struct {
	Label     string
	Frequency string
}

// VatsimStripSource contains the fields that the VATSIM reconciler is allowed
// to persist. It intentionally excludes all controller-owned operational state.
type VatsimStripSource struct {
	CID            string
	Revision       int64
	SeenAt         time.Time
	Origin         string
	Destination    string
	Alternate      string
	Route          string
	Remarks        string
	AssignedSquawk string
	AircraftType   string
	Online         bool
	Latitude       float64
	Longitude      float64
	Altitude       int32
}

// ArrivalETA records the currently accepted arrival estimate and the inputs
// that produced it. It belongs to the strip rather than a stand assignment:
// arrivals receive an ETA before SAT is allowed to reserve a stand.
type ArrivalETA struct {
	Time            time.Time `json:"time"`
	Source          string    `json:"source"`
	CalculatedAt    time.Time `json:"calculated_at"`
	EOBT            string    `json:"eobt,omitempty"`
	EnrouteDuration string    `json:"enroute_duration,omitempty"`
	DistanceNM      *float64  `json:"distance_nm,omitempty"`
	Groundspeed     *int32    `json:"groundspeed,omitempty"`
}

type Strip struct {
	ID                       int32
	Version                  int32
	Callsign                 string
	Session                  int32
	Origin                   string
	Destination              string
	Alternative              *string
	Route                    *string
	Remarks                  *string
	AssignedSquawk           *string
	Squawk                   *string
	Sid                      *string
	Star                     *string
	ClearedAltitude          *int32
	Heading                  *int32
	AircraftType             *string
	Runway                   *string
	RequestedAltitude        *int32
	Capabilities             *string
	CommunicationType        *string
	AircraftCategory         *string
	Stand                    *string
	Sequence                 *int32
	State                    *string
	Cleared                  bool
	Owner                    *string
	Bay                      string
	PositionLatitude         *float64
	PositionLongitude        *float64
	PositionAltitude         *int32
	CdmData                  *CdmData
	PdcData                  *PdcData
	NextOwners               []string
	PreviousOwners           []string
	NextDisplay              *NextDisplay
	ReleasePoint             *string
	PdcState                 string
	PdcRequestRemarks        *string
	PdcRequestedAt           *time.Time
	PdcMessageSequence       *int32
	PdcMessageSent           *time.Time
	StartReq                 bool
	Marked                   bool
	Registration             *string
	TrackingController       string
	EngineType               string
	SpokenCallsign           *string
	RunwayCleared            bool
	RunwayConfirmed          bool
	UnexpectedChangeFields   []string
	ControllerModifiedFields []string
	IsManual                 bool
	PersonsOnBoard           *int32
	FplType                  *string
	Language                 *string
	HasFP                    bool
	ValidationStatus         *ValidationStatus
	VatsimCID                *string
	VatsimRevision           *int64
	VatsimSeenAt             *time.Time
	EuroscopeSeenAt          *time.Time
	ArrivalETA               *ArrivalETA
}

// IsValidationLocked returns true when the strip has an active validation issue
// that must be acknowledged before certain mutations are permitted.
func (s *Strip) IsValidationLocked() bool {
	return s != nil && s.ValidationStatus != nil && s.ValidationStatus.Active
}

func (s *Strip) EffectiveTobt() *string {
	if s == nil || s.CdmData == nil {
		return nil
	}
	return s.CdmData.EffectiveTobt()
}

func (s *Strip) EffectiveTsat() *string {
	if s == nil || s.CdmData == nil {
		return nil
	}
	return s.CdmData.EffectiveTsat()
}

func (s *Strip) EffectiveTtot() *string {
	if s == nil || s.CdmData == nil {
		return nil
	}
	return s.CdmData.EffectiveTtot()
}

func (s *Strip) EffectiveCtot() *string {
	if s == nil || s.CdmData == nil {
		return nil
	}
	return s.CdmData.EffectiveCtot()
}

func (s *Strip) EffectiveAobt() *string {
	if s == nil || s.CdmData == nil {
		return nil
	}
	return s.CdmData.EffectiveAobt()
}

func (s *Strip) EffectiveAsat() *string {
	if s == nil || s.CdmData == nil {
		return nil
	}
	return s.CdmData.EffectiveAsat()
}

func (s *Strip) EffectiveEobt() *string {
	if s == nil || s.CdmData == nil {
		return nil
	}
	return s.CdmData.EffectiveEobt()
}

func (s *Strip) EffectiveCdmStatus() *string {
	if s == nil || s.CdmData == nil {
		return nil
	}
	return s.CdmData.EffectiveStatus()
}

func (s *Strip) EffectiveAldt() *string {
	if s == nil || s.CdmData == nil {
		return nil
	}
	return s.CdmData.EffectiveAldt()
}

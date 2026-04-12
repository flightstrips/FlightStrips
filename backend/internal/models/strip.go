package models

import "time"

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
	NextOwners               []string
	PreviousOwners           []string
	ReleasePoint             *string
	PdcState                 string
	PdcRequestRemarks        *string
	PdcRequestedAt           *time.Time
	PdcMessageSequence       *int32
	PdcMessageSent           *time.Time
	Marked                   bool
	Registration             *string
	TrackingController       string
	EngineType               string
	RunwayCleared            bool
	RunwayConfirmed          bool
	UnexpectedChangeFields   []string
	ControllerModifiedFields []string
	IsManual                 bool
	PersonsOnBoard           *int32
	FplType                  *string
	Language                 *string
	HasFP                    bool
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

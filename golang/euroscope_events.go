package main

import "encoding/json"

const (
	EuroscopeGroundStateUnknown = ""
	EuroscopeGroundStateStartup = "ST-UP"
	EuroscopeGroundStatePush    = "PUSH"
	EuroscopeGroundStateTaxi    = "TAXI"
	EuroscopeGroundStateDepart  = "DEPA"
)

type EuroscopeEvent struct {
	Type EventType
}

type EuroscopeLoginEvent struct {
	Type       EventType `json:"type"`
	Connection string    `json:"connection"`
	Airport    string    `json:"airport"`
	Position   string    `json:"position"`
	Callsign   string    `json:"callsign"`
	Range      int       `json:"range"`
}

type EuroscopeControllerOnlineEvent struct {
	Type     EventType `json:"type"`
	Position string    `json:"position"`
	Callsign string    `json:"callsign"`
}

type EuroscopeControllerOfflineEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
}

type EuroscopeStrip struct {
	Callsign          string `json:"callsign"`
	Origin            string `json:"origin"`
	Destination       string `json:"destination"`
	Alternate         string `json:"alternate"`
	Route             string `json:"route"`
	Remarks           string `json:"remarks"`
	Runway            string `json:"runway"`
	Squawk            string `json:"squawk"`
	AssignedSquawk    string `json:"assigned_squawk"`
	Sid               string `json:"sid"`
	Cleared           bool   `json:"cleared"`
	GroundState       string `json:"ground_state"`
	ClearedAltitude   int    `json:"cleared_altitude"`
	RequestedAltitude int    `json:"requested_altitude"`
	Heading           int    `json:"heading"`
	AircraftType      string `json:"aircraft_type"`
	AircraftCategory  string `json:"aircraft_category"`
	Position          struct {
		Lat      float64 `json:"lat"`
		Lon      float64 `json:"lon"`
		Altitude int     `json:"altitude"`
	} `json:"position"`
	Stand             string `json:"stand"`
	Capabilities      string `json:"capabilities"`
	CommunicationType string `json:"communication_type"`
	Eobt              string `json:"eobt"`
	Eldt              string `json:"eldt"`
}

type EuroscopeSyncEvent struct {
	Type        EventType `json:"type"`
	Controllers []struct {
		Position string `json:"position"`
		Callsign string `json:"callsign"`
	} `json:"controllers"`
	Strips []EuroscopeStrip `json:"strips"`
}

type EuroscopeAssignedSquawkEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Squawk   string    `json:"squawk"`
}

type EuroscopeSquawkEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Squawk   string    `json:"squawk"`
}

type EuroscopeClearedAltitudeEvent struct {
	Type     EventType `json:"type"`
	Altitude int       `json:"altitude"`
	Callsign string    `json:"callsign"`
}

type EuroscopeRequestedAltitudeEvent struct {
	Type     EventType `json:"type"`
	Altitude int       `json:"altitude"`
	Callsign string    `json:"callsign"`
}

type EuroscopeCommunicationTypeEvent struct {
	Callsign          string    `json:"callsign"`
	CommunicationType string    `json:"communication_type"`
	Type              EventType `json:"type"`
}

type EuroscopeGroundStateEvent struct {
	Callsign    string    `json:"callsign"`
	GroundState string    `json:"ground_state"`
	Type        EventType `json:"type"`
}

type EuroscopeClearedFlagEvent struct {
	Callsign string    `json:"callsign"`
	Cleared  bool      `json:"cleared"`
	Type     EventType `json:"type"`
}

type EuroscopeAircraftPositionUpdateEvent struct {
	Altitude int64     `json:"altitude"`
	Callsign string    `json:"callsign"`
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
	Type     EventType `json:"type"`
}

type EuroscopeHeadingEvent struct {
	Callsign string    `json:"callsign"`
	Heading  int       `json:"heading"`
	Type     EventType `json:"type"`
}

type EuroscopeAircraftDisconnectEvent struct {
	Callsign string    `json:"callsign"`
	Type     EventType `json:"type"`
}

type EuroscopeStandEvent struct {
	Callsign string    `json:"callsign"`
	Stand    string    `json:"stand"`
	Type     EventType `json:"type"`
}

type EuroscopeStripUpdateEvent struct {
	EuroscopeStrip
	Type EventType `json:"type"`
}

type EuroscopeRunwayEvent struct {
	Runways []struct {
		Arrival   bool   `json:"arrival"`
		Departure bool   `json:"departure"`
		Name      string `json:"name"`
	} `json:"runways"`
	Type EventType `json:"type"`
}

type SessionInfoRole string

const (
	SessionInfoMaster SessionInfoRole = "master"
	SessionInfoSlave  SessionInfoRole = "slave"
)

type EuroscopeSessionInfoEvent struct {
	Role SessionInfoRole `json:"role"`
}

func (e EuroscopeSessionInfoEvent) MarshalJSON() ([]byte, error) {
	type Alias EuroscopeSessionInfoEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  EuroscopeSessionInfo,
		Alias: (Alias)(e),
	})
}

type EuroscopeGenerateSquawkEvent struct {
	Callsign string `json:"callsign"`
}

func (e EuroscopeGenerateSquawkEvent) MarshalJSON() ([]byte, error) {
	type Alias EuroscopeGenerateSquawkEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  EuroscopeGenerateSquawk,
		Alias: (Alias)(e),
	})
}

type EuroscopeSendEvent interface {
	EuroscopeSessionInfoEvent | EuroscopeGenerateSquawkEvent
}

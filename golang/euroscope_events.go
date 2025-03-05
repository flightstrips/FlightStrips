package main

type EuroscopeEvent struct {
	Type EventType
}

type EuroscopeAuthenticationEvent struct {
	Type  EventType
	Token string
}

type EuroscopeLoginEvent struct {
	Type     EventType `json:"type"`
	Airport  string    `json:"airport"`
	Position string    `json:"position"`
	Callsign string    `json:"callsign"`
	Range    int       `json:"range"`
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

type EuroscopeSyncEvent struct {
	Type        EventType `json:"type"`
	Controllers []struct {
		Position string `json:"position"`
		Callsign string `json:"callsign"`
	} `json:"controllers"`
	Strips []struct {
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
			Lat      int `json:"lat"`
			Lon      int `json:"lon"`
			Altitude int `json:"altitude"`
		} `json:"position"`
		Stand             string `json:"stand"`
		Capabilities      string `json:"capabilities"`
		CommunicationType string `json:"communication_type"`
		Eobt              string `json:"eobt"`
		Eldt              string `json:"eldt"`
	} `json:"strips"`
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
	Altitude int64     `json:"altitude"`
	Callsign string    `json:"callsign"`
}

type EuroscopeRequestedAltitudeEvent struct {
	Type     EventType `json:"type"`
	Altitude int64     `json:"altitude"`
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
	Lat      int64     `json:"lat"`
	Lon      int64     `json:"lon"`
	Type     EventType `json:"type"`
}

type EuroscopeHeadingEvent struct {
	Callsign string    `json:"callsign"`
	Heading  int64     `json:"heading"`
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
	AircraftCategory string    `json:"aircraft_category"`
	AircraftType     string    `json:"aircraft_type"`
	Alternate        string    `json:"alternate"`
	Callsign         string    `json:"callsign"`
	Capabilities     string    `json:"capabilities"`
	Destination      string    `json:"destination"`
	Eldt             *string   `json:"eldt"`
	Eobt             *string   `json:"eobt"`
	Origin           string    `json:"origin"`
	Remarks          string    `json:"remarks"`
	Route            string    `json:"route"`
	Runway           string    `json:"runway"`
	Sid              string    `json:"sid"`
	Type             EventType `json:"type"`
}

type EuroscopeRunwayEvent struct {
	Runways []struct {
		Arrival   bool   `json:"arrival"`
		Departure bool   `json:"departure"`
		Name      string `json:"name"`
	} `json:"runways"`
	Type EventType `json:"type"`
}


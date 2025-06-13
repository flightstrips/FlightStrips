package main

import "encoding/json"

type RunwayConfiguration struct {
	Departure []string `json:"departure"`
	Arrival   []string `json:"arrival"`
}

type FrontendStrip struct {
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
	ClearedAltitude   int    `json:"cleared_altitude"`
	RequestedAltitude int    `json:"requested_altitude"`
	Heading           int    `json:"heading"`
	AircraftType      string `json:"aircraft_type"`
	AircraftCategory  string `json:"aircraft_category"`
	Stand             string `json:"stand"`
	Capabilities      string `json:"capabilities"`
	CommunicationType string `json:"communication_type"`
	Eobt              string `json:"eobt"`
	Eldt              string `json:"eldt"`
	Bay               string `json:"bay"`
	ReleasePoint      string `json:"release_point"`
	Version           int    `json:"version"`
	Sequence          int    `json:"sequence"`
}

type FrontendController struct {
	Callsign string `json:"callsign"`
	Position string `json:"position"`
}

type FrontendInitialEvent struct {
	Controllers []FrontendController `json:"controllers"`
	Strips      []FrontendStrip      `json:"strips"`
	Position    string               `json:"position"`
	Airport     string               `json:"airport"`
	Callsign    string               `json:"callsign"`
	RunwaySetup RunwayConfiguration  `json:"runway_setup"`
}

type FrontendStripUpdateEvent struct {
	FrontendStrip
}

func (e FrontendInitialEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendInitialEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendInitial,
		Alias: (Alias)(e),
	})
}

func (e FrontendStripUpdateEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendStripUpdateEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendStripUpdate,
		Alias: (Alias)(e),
	})
}

type FrontendControllerOnlineEvent struct {
	FrontendController
}

func (e FrontendControllerOnlineEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendControllerOnlineEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendControllerOnline, // You should add this EventType constant if not present
		Alias: (Alias)(e),
	})
}

type FrontendControllerOfflineEvent struct {
	FrontendController
}

func (e FrontendControllerOfflineEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendControllerOfflineEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendControllerOffline, // Ensure this EventType exists
		Alias: (Alias)(e),
	})
}

type FrontendAssignedSquawkEvent struct {
	Callsign string `json:"callsign"`
	Squawk   string `json:"squawk"`
}

func (e FrontendAssignedSquawkEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendAssignedSquawkEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendAssignedSquawk,
		Alias: (Alias)(e),
	})
}

type FrontendSquawkEvent struct {
	Callsign string `json:"callsign"`
	Squawk   string `json:"squawk"`
}

func (e FrontendSquawkEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendSquawkEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendSquawk,
		Alias: (Alias)(e),
	})
}

type FrontendRequestedAltitudeEvent struct {
	Callsign string `json:"callsign"`
	Altitude int    `json:"altitude"`
}

func (e FrontendRequestedAltitudeEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendRequestedAltitudeEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendRequestedAltitude,
		Alias: (Alias)(e),
	})
}

type FrontendClearedAltitudeEvent struct {
	Callsign string `json:"callsign"`
	Altitude int    `json:"altitude"`
}

func (e FrontendClearedAltitudeEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendClearedAltitudeEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendClearedAltitude,
		Alias: (Alias)(e),
	})
}

type FrontendBayEvent struct {
	Callsign string `json:"callsign"`
	Bay      string `json:"bay"`
}

func (e FrontendBayEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendBayEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendBay,
		Alias: (Alias)(e),
	})
}

type FrontendDisconnectEvent struct{}

func (e FrontendDisconnectEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendDisconnectEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type: FrontendDisconnect,
	})
}

type FrontendAircraftDisconnectEvent struct {
	Callsign string `json:"callsign"`
}

func (e FrontendAircraftDisconnectEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendAircraftDisconnectEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendAircraftDisconnect,
		Alias: (Alias)(e),
	})
}

type FrontendStandEvent struct {
	Callsign string `json:"callsign"`
	Stand    string `json:"stand"`
}

func (e FrontendStandEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendStandEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendStand,
		Alias: (Alias)(e),
	})
}

type FrontendSetHeadingEvent struct {
	Callsign string `json:"callsign"`
	Heading  int    `json:"heading"`
}

func (e FrontendSetHeadingEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendSetHeadingEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendSetHeading,
		Alias: (Alias)(e),
	})
}

type FrontendCommunicationTypeEvent struct {
	Callsign          string `json:"callsign"`
	CommunicationType string `json:"communication_type"`
}

func (e FrontendCommunicationTypeEvent) MarshalJSON() ([]byte, error) {
	type Alias FrontendCommunicationTypeEvent
	return json.Marshal(&struct {
		Type EventType `json:"type"`
		Alias
	}{
		Type:  FrontendCommunicationType,
		Alias: (Alias)(e),
	})
}

type FrontendMoveEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Bay      string    `json:"bay"`
}

type FrontendGenerateSquawkEvent struct {
	Callsign string `json:"callsign"`
}

type FrontendSendEvent interface {
	FrontendInitialEvent | FrontendStripUpdateEvent | FrontendDisconnectEvent | FrontendAircraftDisconnectEvent | FrontendStandEvent | FrontendSetHeadingEvent | FrontendCommunicationTypeEvent | FrontendAssignedSquawkEvent | FrontendSquawkEvent | FrontendRequestedAltitudeEvent | FrontendClearedAltitudeEvent | FrontendBayEvent | FrontendControllerOnlineEvent | FrontendControllerOfflineEvent
}

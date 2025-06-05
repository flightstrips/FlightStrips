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
	Cleared           bool   `json:"cleared"`
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

type FrontendSendEvent interface {
	FrontendInitialEvent | FrontendStripUpdateEvent
}

package main

type AirportConfiguration struct {
	runwayConfiguration string
	atis                string
}

type Position struct {
	Location Location
	Height   int
}

type Location struct {
	Longitude float64
	Latitude  float64
}

type Controller struct {
	Cid      string
	Airport  string
	Position string
}

package config

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/golang/geo/s2"
)

type featureCollection struct {
	Type     string    `json:"type"`
	Features []feature `json:"features"`
}

type feature struct {
	Type     string `json:"type"`
	Geometry struct {
		Type        string      `json:"type"`
		Coordinates coordinates `json:"coordinates"`
	} `json:"geometry"`
	Properties struct {
		Name string `json:"name"`
	} `json:"properties"`
}

type coordinates [][][]float64

type Region struct {
	Name   string
	Region *s2.Loop
}

func loadRegions(f io.Reader) error {
	var features featureCollection
	decoder := json.NewDecoder(f)
	err := decoder.Decode(&features)
	if err != nil {
		return err
	}

	regions = make([]Region, 0)
	for _, feature := range features.Features {
		if feature.Type != "Feature" || feature.Geometry.Type != "Polygon" {
			return errors.New("invalid feature type")
		}

		name := feature.Properties.Name
		coordinates := feature.Geometry.Coordinates[0]

		points := make([]s2.Point, len(coordinates))
		for i, c := range coordinates {
			points[i] = s2.PointFromLatLng(s2.LatLngFromDegrees(c[1], c[0]))
		}

		loop := s2.LoopFromPoints(points)
		regions = append(regions, Region{name, loop})
	}

	return nil
}

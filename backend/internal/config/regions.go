package config

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

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
		Name    string   `json:"name"`
		Runways []string `json:"runways"`
	} `json:"properties"`
}

type coordinates [][][]float64

type Region struct {
	Name    string
	Runways []string // populated for runway regions; empty for ground regions
	Region  *s2.Loop
}

var ErrUnsupportedRegion = errors.New("unsupported region")

func GetRegionForPosition(lat, lon float64) (*Region, error) {
	point := s2.PointFromLatLng(s2.LatLngFromDegrees(lat, lon))
	for _, region := range regions {
		if region.Region.ContainsPoint(point) {
			return &region, nil
		}
	}

	return nil, ErrUnsupportedRegion
}

// GetRunwayRegionForPosition checks only runway regions and returns the region (with its
// Runways list) if the position falls inside one, or nil and false otherwise.
func GetRunwayRegionForPosition(lat, lon float64) (*Region, bool) {
	point := s2.PointFromLatLng(s2.LatLngFromDegrees(lat, lon))
	for i, region := range runwayRegions {
		if region.Region.ContainsPoint(point) {
			return &runwayRegions[i], true
		}
	}
	return nil, false
}

func loadRegions(f io.Reader) error {
	var features featureCollection
	decoder := json.NewDecoder(f)
	err := decoder.Decode(&features)
	if err != nil {
		return err
	}

	regions = make([]Region, 0)
	runwayRegions = make([]Region, 0)
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
		region := Region{Name: name, Runways: feature.Properties.Runways, Region: loop}
		if strings.HasPrefix(name, "RWY_") {
			runwayRegions = append(runwayRegions, region)
		} else {
			regions = append(regions, region)
		}
	}

	return nil
}

package config

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"slices"
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
		Name                   string   `json:"name"`
		Runways                []string `json:"runways"`
		GlideslopeDegrees      float64  `json:"glideslopeDegrees"`
		MaxAboveGlideslopeFeet int64    `json:"maxAboveGlideslopeFeet"`
		ThresholdLat           float64  `json:"thresholdLat"`
		ThresholdLon           float64  `json:"thresholdLon"`
	} `json:"properties"`
}

type coordinates [][][]float64

const (
	defaultFinalApproachGlideslopeDegrees    = 3.0
	defaultFinalApproachMaxAboveGlideslopeFt = int64(500)
	feetPerNauticalMile                      = 6076.12
)

type Region struct {
	Name                 string
	Runways              []string // populated for runway regions; empty for ground regions
	Region               *s2.Loop
	ThresholdLat         float64
	ThresholdLon         float64
	GlideslopeDegrees    float64
	MaxAboveGlideslopeFt int64
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

// GetFinalApproachRegionForRunway returns the final-approach region for the given runway
// when the position falls inside one of the configured FINAL_* polygons.
func GetFinalApproachRegionForRunway(runway string, lat, lon float64) (*Region, bool) {
	point := s2.PointFromLatLng(s2.LatLngFromDegrees(lat, lon))
	normalizedRunway := strings.ToUpper(strings.TrimSpace(runway))
	if normalizedRunway == "" {
		return nil, false
	}

	for i, region := range finalApproachRegions {
		if !slices.Contains(region.Runways, normalizedRunway) {
			continue
		}
		if region.Region.ContainsPoint(point) {
			return &finalApproachRegions[i], true
		}
	}

	return nil, false
}

func (r Region) FinalApproachAltitudeCeiling(distanceNM float64, airportElevationFeet int64) int64 {
	glideslopeDegrees := r.GlideslopeDegrees
	if glideslopeDegrees <= 0 {
		glideslopeDegrees = defaultFinalApproachGlideslopeDegrees
	}

	maxAboveGlideslopeFt := r.MaxAboveGlideslopeFt
	if maxAboveGlideslopeFt <= 0 {
		maxAboveGlideslopeFt = defaultFinalApproachMaxAboveGlideslopeFt
	}

	aglFeet := math.Tan(glideslopeDegrees*math.Pi/180.0) * distanceNM * feetPerNauticalMile
	return airportElevationFeet + int64(math.Round(aglFeet)) + maxAboveGlideslopeFt
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
	finalApproachRegions = make([]Region, 0)
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
		loop.Normalize()
		region := Region{
			Name:                 name,
			Runways:              feature.Properties.Runways,
			Region:               loop,
			GlideslopeDegrees:    feature.Properties.GlideslopeDegrees,
			MaxAboveGlideslopeFt: feature.Properties.MaxAboveGlideslopeFeet,
			ThresholdLat:         feature.Properties.ThresholdLat,
			ThresholdLon:         feature.Properties.ThresholdLon,
		}
		if strings.HasPrefix(name, "FINAL_") && region.ThresholdLat == 0 && region.ThresholdLon == 0 && len(coordinates) >= 2 {
			region.ThresholdLon = (coordinates[0][0] + coordinates[1][0]) / 2
			region.ThresholdLat = (coordinates[0][1] + coordinates[1][1]) / 2
		}
		if strings.HasPrefix(name, "RWY_") {
			runwayRegions = append(runwayRegions, region)
		} else if strings.HasPrefix(name, "FINAL_") {
			finalApproachRegions = append(finalApproachRegions, region)
		} else {
			regions = append(regions, region)
		}
	}

	return nil
}

package terminal

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

const EKCHTMAUpperAltitudeFeet = 19_500

const DefaultEKCHTMAVolumePath = "config/aman/boundaries/EKCH.json"

type geoJSONFeature struct {
	Type     string `json:"type"`
	Geometry struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	} `json:"geometry"`
}

type tmaPoint struct {
	longitude float64
	latitude  float64
}

type tmaPolygon [][]tmaPoint

// TMAVolume is an immutable, validated horizontal boundary with the EKCH
// operator-approved vertical limits: surface inclusive to strictly below FL195.
type TMAVolume struct {
	polygons []tmaPolygon
}

// LoadTMAVolume parses and validates a GeoJSON MultiPolygon without retaining
// any operational observation or entry state.
func LoadTMAVolume(path string) (TMAVolume, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return TMAVolume{}, fmt.Errorf("read TMA boundary: %w", err)
	}

	var feature geoJSONFeature
	if err := json.Unmarshal(encoded, &feature); err != nil {
		return TMAVolume{}, fmt.Errorf("decode TMA boundary: %w", err)
	}
	if feature.Type != "Feature" {
		return TMAVolume{}, fmt.Errorf("TMA boundary type must be Feature")
	}
	if feature.Geometry.Type != "MultiPolygon" {
		return TMAVolume{}, fmt.Errorf("TMA geometry type must be MultiPolygon")
	}

	var coordinates [][][][]float64
	if err := json.Unmarshal(feature.Geometry.Coordinates, &coordinates); err != nil {
		return TMAVolume{}, fmt.Errorf("decode TMA MultiPolygon coordinates: %w", err)
	}
	if len(coordinates) == 0 {
		return TMAVolume{}, fmt.Errorf("TMA MultiPolygon must contain a polygon")
	}

	volume := TMAVolume{polygons: make([]tmaPolygon, len(coordinates))}
	for polygonIndex, polygon := range coordinates {
		if len(polygon) == 0 {
			return TMAVolume{}, fmt.Errorf("TMA polygon %d must contain an exterior ring", polygonIndex)
		}
		volume.polygons[polygonIndex] = make(tmaPolygon, len(polygon))
		for ringIndex, ring := range polygon {
			validated, err := validateTMARing(ring)
			if err != nil {
				return TMAVolume{}, fmt.Errorf("TMA polygon %d ring %d: %w", polygonIndex, ringIndex, err)
			}
			volume.polygons[polygonIndex][ringIndex] = validated
		}
	}
	return volume, nil
}

func validateTMARing(coordinates [][]float64) ([]tmaPoint, error) {
	if len(coordinates) < 4 {
		return nil, fmt.Errorf("must contain at least four positions")
	}
	ring := make([]tmaPoint, len(coordinates))
	for i, coordinate := range coordinates {
		if len(coordinate) < 2 {
			return nil, fmt.Errorf("position %d must contain longitude and latitude", i)
		}
		longitude, latitude := coordinate[0], coordinate[1]
		if math.IsNaN(longitude) || math.IsInf(longitude, 0) || longitude < -180 || longitude > 180 {
			return nil, fmt.Errorf("position %d longitude is invalid", i)
		}
		if math.IsNaN(latitude) || math.IsInf(latitude, 0) || latitude < -90 || latitude > 90 {
			return nil, fmt.Errorf("position %d latitude is invalid", i)
		}
		ring[i] = tmaPoint{longitude: longitude, latitude: latitude}
	}
	if ring[0] != ring[len(ring)-1] {
		return nil, fmt.Errorf("must be closed")
	}
	area := 0.0
	for i := 1; i < len(ring); i++ {
		area += ring[i-1].longitude*ring[i].latitude - ring[i].longitude*ring[i-1].latitude
	}
	if math.Abs(area) <= 1e-12 {
		return nil, fmt.Errorf("must enclose an area")
	}
	return ring, nil
}

// Contains reports whether a surveillance position is within the approved
// volume. Exterior and hole boundaries are included; hole interiors are not.
func (v TMAVolume) Contains(latitudeDeg, longitudeDeg, altitudeFeet float64) bool {
	if altitudeFeet < 0 || altitudeFeet >= EKCHTMAUpperAltitudeFeet ||
		math.IsNaN(altitudeFeet) || math.IsNaN(latitudeDeg) || math.IsNaN(longitudeDeg) ||
		math.IsInf(altitudeFeet, 0) || math.IsInf(latitudeDeg, 0) || math.IsInf(longitudeDeg, 0) ||
		latitudeDeg < -90 || latitudeDeg > 90 || longitudeDeg < -180 || longitudeDeg > 180 {
		return false
	}
	point := tmaPoint{longitude: longitudeDeg, latitude: latitudeDeg}
	for _, polygon := range v.polygons {
		if polygonContains(polygon, point) {
			return true
		}
	}
	return false
}

func polygonContains(polygon tmaPolygon, point tmaPoint) bool {
	exterior := ringContains(polygon[0], point)
	if exterior == ringOutside {
		return false
	}
	if exterior == ringBoundary {
		return true
	}
	for _, hole := range polygon[1:] {
		switch ringContains(hole, point) {
		case ringBoundary:
			return true
		case ringInside:
			return false
		}
	}
	return true
}

type ringLocation uint8

const (
	ringOutside ringLocation = iota
	ringInside
	ringBoundary
)

func ringContains(ring []tmaPoint, point tmaPoint) ringLocation {
	inside := false
	for i, previous := 0, len(ring)-1; i < len(ring); previous, i = i, i+1 {
		a, b := ring[previous], ring[i]
		cross := (point.latitude-a.latitude)*(b.longitude-a.longitude) -
			(point.longitude-a.longitude)*(b.latitude-a.latitude)
		if math.Abs(cross) <= 1e-12 &&
			point.longitude >= math.Min(a.longitude, b.longitude) && point.longitude <= math.Max(a.longitude, b.longitude) &&
			point.latitude >= math.Min(a.latitude, b.latitude) && point.latitude <= math.Max(a.latitude, b.latitude) {
			return ringBoundary
		}
		if (a.latitude > point.latitude) != (b.latitude > point.latitude) &&
			point.longitude < (b.longitude-a.longitude)*(point.latitude-a.latitude)/(b.latitude-a.latitude)+a.longitude {
			inside = !inside
		}
	}
	if inside {
		return ringInside
	}
	return ringOutside
}

package airacnet

import (
	"context"
	"encoding/json"
	"math"
	"net/url"

	"FlightStrips/internal/aman/navdata"
)

// Repair only with a unique source-supplied coordinate supported by both
// neighboring fixes. Destination-region lookup is inappropriate for an en-route
// navaid. If resolution fails, retain an explicit gap and the trustworthy suffix.
func (a *Adapter) resolveRouteDetours(ctx context.Context, result *routeData) error {
	legs := make([]navdata.ProcedureLeg, len(result.Segments))
	for i, segment := range result.Segments {
		from, to := navdata.FixID(upper(segment.From.Identifier)), navdata.FixID(upper(segment.To.Identifier))
		start, end := routePointPosition(segment.From), routePointPosition(segment.To)
		legs[i] = navdata.ProcedureLeg{FromFix: &from, ToFix: &to, FromPosition: &start, ToPosition: &end}
	}
	for _, i := range navdata.SuspectRouteDetours(legs) {
		left, right := &result.Segments[i], &result.Segments[i+1]
		if left.GapFix != "" || right.GapFix != "" {
			continue
		}
		fix := upper(left.To.Identifier)
		endpoint := "waypoints/"
		if left.To.Type == "navaid" {
			endpoint = "navaids/"
		}
		var response waypointResponse
		err := a.getJSON(ctx, endpoint+url.PathEscape(fix), nil, &response)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var candidates []waypointDTO
		if err == nil {
			if json.Unmarshal(response.Data, &candidates) != nil {
				var one waypointDTO
				if json.Unmarshal(response.Data, &one) == nil {
					candidates = []waypointDTO{one}
				}
			}
		}
		identityPosition, region := a.routeFixIdentity(ctx, *left)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		start, end := routePointPosition(left.From), routePointPosition(right.To)
		limit := math.Max(300, 2*navdata.CoordinateDistanceNM(start, end))
		positions := map[navdata.Coordinate]bool{}
		for _, candidate := range candidates {
			position := coordinate(candidate.Latitude, candidate.Longitude, candidate.Coordinates)
			if upper(candidate.Identifier) != fix || position.Validate() != nil {
				continue
			}
			// AIRAC.NET uses 0,0 for DME entries with no published location.
			// This provider-specific sentinel is not a global coordinate rule.
			if candidate.Type.Code == "DME" && position.LatitudeDeg == 0 && position.LongitudeDeg == 0 {
				continue
			}
			if identityPosition == nil && region == "" {
				continue
			}
			if region != "" && candidate.Region != region {
				continue
			}
			if identityPosition != nil && navdata.CoordinateDistanceNM(*identityPosition, position) > .1 {
				continue
			}
			if navdata.CoordinateDistanceNM(start, position) <= limit && navdata.CoordinateDistanceNM(position, end) <= limit {
				positions[position] = true
			}
		}
		if len(positions) == 1 {
			for position := range positions {
				point := routePointDTO{Identifier: fix, Latitude: position.LatitudeDeg, Longitude: position.LongitudeDeg}
				left.To, right.From = point, point
				recalculateRouteSegment(left)
				recalculateRouteSegment(right)
			}
		} else {
			left.GapFix, right.GapFix = fix, fix
		}
	}
	return nil
}

func routePointPosition(point routePointDTO) navdata.Coordinate {
	return coordinate(point.Latitude, point.Longitude, point.Coordinates)
}

func recalculateRouteSegment(segment *routeSegmentDTO) {
	a, b := routePointPosition(segment.From), routePointPosition(segment.To)
	segment.Distance = navdata.CoordinateDistanceNM(a, b)
	lat1, lat2 := a.LatitudeDeg*math.Pi/180, b.LatitudeDeg*math.Pi/180
	dlon := (b.LongitudeDeg - a.LongitudeDeg) * math.Pi / 180
	segment.Bearing = math.Mod(math.Atan2(math.Sin(dlon)*math.Cos(lat2), math.Cos(lat1)*math.Sin(lat2)-math.Sin(lat1)*math.Cos(lat2)*math.Cos(dlon))*180/math.Pi+360, 360)
}

// routeFixIdentity corroborates a homonym using its native region or the
// precise fix in the expanded airway sequence. Nearby coordinates alone are
// insufficient evidence, especially when the intended entry lacks a position.
func (a *Adapter) routeFixIdentity(ctx context.Context, inbound routeSegmentDTO) (*navdata.Coordinate, string) {
	if inbound.To.Region != "" {
		return nil, inbound.To.Region
	}
	if inbound.To.Via == "" {
		return nil, ""
	}
	var airway struct {
		Data struct {
			Segments []struct {
				FixIdentifier  string         `json:"fix_identifier"`
				FixCoordinates *coordinateDTO `json:"fix_coordinates"`
				Region         string         `json:"region"`
			} `json:"segments"`
		} `json:"data"`
	}
	if a.getJSON(ctx, "airways/"+url.PathEscape(inbound.To.Via), nil, &airway) != nil {
		return nil, ""
	}
	var positions []navdata.Coordinate
	region := ""
	matches := 0
	for i, fix := range airway.Data.Segments {
		if upper(fix.FixIdentifier) != upper(inbound.To.Identifier) || i == 0 || upper(airway.Data.Segments[i-1].FixIdentifier) != upper(inbound.From.Identifier) {
			continue
		}
		matches++
		region = fix.Region
		if fix.FixCoordinates != nil {
			position := coordinate(0, 0, *fix.FixCoordinates)
			if position.Validate() == nil {
				positions = append(positions, position)
			}
		}
	}
	if matches != 1 {
		return nil, ""
	}
	if len(positions) == 1 {
		return &positions[0], region
	}
	return nil, region
}

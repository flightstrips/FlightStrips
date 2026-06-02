package ecfmp

import (
	"strings"
	"time"

	"FlightStrips/internal/models"
)

type StripRestriction struct {
	MeasureID   int64    `json:"measure_id,omitempty"`
	Ident       string   `json:"ident,omitempty"`
	Type        string   `json:"type"`
	Reason      string   `json:"reason,omitempty"`
	Routes      []string `json:"routes,omitempty"`
	Destination string   `json:"destination,omitempty"`
	MaxLevel    *int     `json:"max_level,omitempty"`
	MinLevel    *int     `json:"min_level,omitempty"`
	ExactLevels []int    `json:"exact_levels,omitempty"`
	HasCtot     bool     `json:"has_ctot,omitempty"`
}

func MatchingRestrictions(strip *models.Strip, measures []FlowMeasure, now time.Time) []StripRestriction {
	var restrictions []StripRestriction

	for _, measure := range measures {
		if !measure.Measure.Type.IsRelevant() {
			continue
		}
		if !measure.IsActive(now) {
			continue
		}
		if !matchesFilters(strip, measure.Filters) {
			continue
		}

		restriction := StripRestriction{
			MeasureID: measure.ID,
			Ident:     measure.Ident,
			Type:      string(measure.Measure.Type),
			Reason:    measure.Reason,
		}

		switch measure.Measure.Type {
		case MeasureTypeMandatoryRoute:
			restriction.Routes = measure.Measure.MandatoryRoutes()
			if len(restriction.Routes) == 0 {
				continue
			}

		case MeasureTypeGroundStop:
			restriction.Destination = strip.Destination

		case MeasureTypeProhibit:
			for _, filter := range measure.Filters {
				switch filter.Type {
				case FilterTypeLevelAbove:
					if val := filter.LevelValue(); val != nil {
						restriction.MinLevel = val
					}
				case FilterTypeLevelBelow:
					if val := filter.LevelValue(); val != nil {
						restriction.MaxLevel = val
					}
				case FilterTypeLevel:
					restriction.ExactLevels = filter.Levels()
				}
			}
		}

		restrictions = append(restrictions, restriction)
	}

	return restrictions
}

func matchesFilters(strip *models.Strip, filters []FlowMeasureFilter) bool {
	for _, filter := range filters {
		switch filter.Type {
		case FilterTypeADEP:
			airports := filter.Airports()
			if len(airports) == 0 {
				continue
			}
			if !airportMatchesPattern(strip.Origin, airports) {
				return false
			}

		case FilterTypeADES:
			airports := filter.Airports()
			if len(airports) == 0 {
				continue
			}
			if !airportMatchesPattern(strip.Destination, airports) {
				return false
			}

		case FilterTypeLevelAbove:
			if strip.RequestedAltitude == nil {
				continue
			}
			val := filter.LevelValue()
			if val == nil {
				continue
			}
			if *strip.RequestedAltitude < int32(*val)*100 {
				return false
			}

		case FilterTypeLevelBelow:
			if strip.RequestedAltitude == nil {
				continue
			}
			val := filter.LevelValue()
			if val == nil {
				continue
			}
			if *strip.RequestedAltitude > int32(*val)*100 {
				return false
			}

		case FilterTypeLevel:
			if strip.RequestedAltitude == nil {
				return false
			}
			levels := filter.Levels()
			if len(levels) == 0 {
				continue
			}
			fl := *strip.RequestedAltitude / 100
			found := false
			for _, level := range levels {
				if fl == int32(level) {
					found = true
					break
				}
			}
			if !found {
				return false
			}

		case FilterTypeWaypoint:
			waypoints := filter.Airports()
			if len(waypoints) == 0 {
				continue
			}
			route := ""
			if strip.Route != nil {
				route = *strip.Route
			}
			if !routeContainsWaypoint(route, waypoints) {
				return false
			}
		}
	}
	return true
}

func routeContainsWaypoint(route string, waypoints []string) bool {
	upperRoute := strings.ToUpper(route)
	for _, wp := range waypoints {
		if strings.Contains(upperRoute, strings.ToUpper(wp)) {
			return true
		}
	}
	return false
}
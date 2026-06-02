package ecfmp

import (
	"encoding/json"
	"strings"
	"time"
)

type FlowMeasure struct {
	ID                                int64                `json:"id"`
	Ident                             string               `json:"ident"`
	EventID                           *int64               `json:"event_id"`
	Reason                            string               `json:"reason"`
	StartTime                         time.Time            `json:"starttime"`
	EndTime                           time.Time            `json:"endtime"`
	WithdrawnAt                       *time.Time           `json:"withdrawn_at"`
	NotifiedFlightInformationRegions  []int64              `json:"notified_flight_information_regions"`
	Measure                           FlowMeasureType      `json:"measure"`
	Filters                           []FlowMeasureFilter  `json:"filters"`
}

func (fm FlowMeasure) IsActive(now time.Time) bool {
	if fm.WithdrawnAt != nil {
		return false
	}
	return !now.Before(fm.StartTime) && now.Before(fm.EndTime)
}

type FlowMeasureType struct {
	Type  MeasureType    `json:"type"`
	Value json.RawMessage `json:"value"`
}

type MeasureType string

const (
	MeasureTypeMinimumDepartureInterval MeasureType = "minimum_departure_interval"
	MeasureTypeAverageDepartureInterval MeasureType = "average_departure_interval"
	MeasureTypePerHour                  MeasureType = "per_hour"
	MeasureTypeMilesInTrail             MeasureType = "miles_in_trail"
	MeasureTypeMaxIAS                    MeasureType = "max_ias"
	MeasureTypeMaxMach                  MeasureType = "max_mach"
	MeasureTypeIASReduction             MeasureType = "ias_reduction"
	MeasureTypeMachReduction            MeasureType = "mach_reduction"
	MeasureTypeProhibit                 MeasureType = "prohibit"
	MeasureTypeGroundStop               MeasureType = "ground_stop"
	MeasureTypeMandatoryRoute           MeasureType = "mandatory_route"
)

func (mt MeasureType) IsRelevant() bool {
	switch mt {
	case MeasureTypeProhibit, MeasureTypeGroundStop, MeasureTypeMandatoryRoute:
		return true
	default:
		return false
	}
}

func (ft *FlowMeasureType) MandatoryRoutes() []string {
	if ft.Type != MeasureTypeMandatoryRoute {
		return nil
	}
	var routes []string
	if err := json.Unmarshal(ft.Value, &routes); err != nil {
		return nil
	}
	return routes
}

func (ft *FlowMeasureType) IntervalSeconds() *int64 {
	if ft.Type != MeasureTypeMinimumDepartureInterval && ft.Type != MeasureTypeAverageDepartureInterval {
		return nil
	}
	var val int64
	if err := json.Unmarshal(ft.Value, &val); err != nil {
		return nil
	}
	return &val
}

func (ft *FlowMeasureType) PerHourValue() *int64 {
	if ft.Type != MeasureTypePerHour {
		return nil
	}
	var val int64
	if err := json.Unmarshal(ft.Value, &val); err != nil {
		return nil
	}
	return &val
}

type FlowMeasureFilter struct {
	Type  FilterType      `json:"type"`
	Value json.RawMessage `json:"value"`
}

type FilterType string

const (
	FilterTypeADEP            FilterType = "ADEP"
	FilterTypeADES            FilterType = "ADES"
	FilterTypeLevelAbove      FilterType = "level_above"
	FilterTypeLevelBelow      FilterType = "level_below"
	FilterTypeLevel           FilterType = "level"
	FilterTypeWaypoint        FilterType = "waypoint"
	FilterTypeMemberEvent     FilterType = "member_event"
	FilterTypeMemberNotEvent  FilterType = "member_not_event"
)

func (f FlowMeasureFilter) Airports() []string {
	switch f.Type {
	case FilterTypeADEP, FilterTypeADES:
		var airports []string
		if err := json.Unmarshal(f.Value, &airports); err != nil {
			return nil
		}
		return airports
	case FilterTypeWaypoint:
		var waypoints []string
		if err := json.Unmarshal(f.Value, &waypoints); err != nil {
			return nil
		}
		return waypoints
	default:
		return nil
	}
}

func (f FlowMeasureFilter) Levels() []int {
	if f.Type != FilterTypeLevel {
		return nil
	}
	var levels []int
	if err := json.Unmarshal(f.Value, &levels); err != nil {
		return nil
	}
	return levels
}

func (f FlowMeasureFilter) LevelValue() *int {
	switch f.Type {
	case FilterTypeLevelAbove, FilterTypeLevelBelow:
		var val int
		if err := json.Unmarshal(f.Value, &val); err != nil {
			return nil
		}
		return &val
	default:
		return nil
	}
}

func airportMatchesPattern(airport string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.EqualFold(airport, pattern) {
			return true
		}
		if strings.HasSuffix(pattern, "**") {
			prefix := strings.ToUpper(pattern[:len(pattern)-2])
			if strings.HasPrefix(strings.ToUpper(airport), prefix) {
				return true
			}
		}
		if strings.HasSuffix(pattern, "*") && !strings.HasSuffix(pattern, "**") {
			prefix := strings.ToUpper(pattern[:len(pattern)-1])
			if strings.HasPrefix(strings.ToUpper(airport), prefix) {
				return true
			}
		}
	}
	return false
}
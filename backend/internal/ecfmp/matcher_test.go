package ecfmp

import (
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func int32Ptr(i int32) *int32 { return &i }
func strPtr(s string) *string  { return &s }

func makeMeasure(measureType MeasureType, value json.RawMessage, filters []FlowMeasureFilter) FlowMeasure {
	return FlowMeasure{
		ID:        1,
		Ident:     "TEST01A",
		Reason:    "Testing",
		Measure:   FlowMeasureType{Type: measureType, Value: value},
		Filters:   filters,
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now().Add(1 * time.Hour),
	}
}

func TestMatchingRestrictions_MandatoryRoute(t *testing.T) {
	measures := []FlowMeasure{
		makeMeasure(
			MeasureTypeMandatoryRoute,
			json.RawMessage(`["UL612 LAKEY DCT NUGRA"]`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
				{Type: FilterTypeADES, Value: json.RawMessage(`["EHAM"]`)},
			},
		),
	}

	strip := &models.Strip{
		Origin:      "EGLL",
		Destination: "EHAM",
		Route:       strPtr("UL612 LAKEY DCT NUGRA"),
	}

	now := time.Now()
	restrictions := MatchingRestrictions(strip, measures, now)
	require.Len(t, restrictions, 1)
	assert.Equal(t, "mandatory_route", restrictions[0].Type)
	assert.Equal(t, []string{"UL612 LAKEY DCT NUGRA"}, restrictions[0].Routes)
}

func TestMatchingRestrictions_MandatoryRoute_NoMatch(t *testing.T) {
	measures := []FlowMeasure{
		makeMeasure(
			MeasureTypeMandatoryRoute,
			json.RawMessage(`["UL612 LAKEY DCT NUGRA"]`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
			},
		),
	}

	strip := &models.Strip{
		Origin:      "LFPG",
		Destination: "EHAM",
	}

	now := time.Now()
	restrictions := MatchingRestrictions(strip, measures, now)
	assert.Empty(t, restrictions)
}

func TestMatchingRestrictions_GroundStop(t *testing.T) {
	measures := []FlowMeasure{
		makeMeasure(
			MeasureTypeGroundStop,
			json.RawMessage(`null`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EKCH"]`)},
				{Type: FilterTypeADES, Value: json.RawMessage(`["EGLL"]`)},
			},
		),
	}

	strip := &models.Strip{
		Origin:      "EKCH",
		Destination: "EGLL",
	}

	now := time.Now()
	restrictions := MatchingRestrictions(strip, measures, now)
	require.Len(t, restrictions, 1)
	assert.Equal(t, "ground_stop", restrictions[0].Type)
	assert.Equal(t, "EGLL", restrictions[0].Destination)
}

func TestMatchingRestrictions_Prohibit_Levels(t *testing.T) {
	measures := []FlowMeasure{
		makeMeasure(
			MeasureTypeProhibit,
			json.RawMessage(`null`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
				{Type: FilterTypeLevelAbove, Value: json.RawMessage(`200`)},
				{Type: FilterTypeLevelBelow, Value: json.RawMessage(`350`)},
			},
		),
	}

	strip := &models.Strip{
		Origin:             "EGLL",
		Destination:        "EHAM",
		RequestedAltitude:  int32Ptr(28000),
	}

	now := time.Now()
	restrictions := MatchingRestrictions(strip, measures, now)
	require.Len(t, restrictions, 1)
	assert.Equal(t, "prohibit", restrictions[0].Type)
	require.NotNil(t, restrictions[0].MinLevel)
	assert.Equal(t, 200, *restrictions[0].MinLevel)
	require.NotNil(t, restrictions[0].MaxLevel)
	assert.Equal(t, 350, *restrictions[0].MaxLevel)
}

func TestMatchingRestrictions_Prohibit_ExactLevels(t *testing.T) {
	measures := []FlowMeasure{
		makeMeasure(
			MeasureTypeProhibit,
			json.RawMessage(`null`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
				{Type: FilterTypeLevel, Value: json.RawMessage(`[230,240]`)},
			},
		),
	}

	strip := &models.Strip{
		Origin:             "EGLL",
		Destination:        "EHAM",
		RequestedAltitude:  int32Ptr(24000),
	}

	now := time.Now()
	restrictions := MatchingRestrictions(strip, measures, now)
	require.Len(t, restrictions, 1)
	assert.Equal(t, "prohibit", restrictions[0].Type)
	assert.Equal(t, []int{230, 240}, restrictions[0].ExactLevels)
}

func TestMatchingRestrictions_SkipsIrrelevantTypes(t *testing.T) {
	measures := []FlowMeasure{
		makeMeasure(
			MeasureTypeMinimumDepartureInterval,
			json.RawMessage(`120`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
			},
		),
	}

	strip := &models.Strip{Origin: "EGLL"}
	now := time.Now()
	restrictions := MatchingRestrictions(strip, measures, now)
	assert.Empty(t, restrictions, "irrelevant measure types should be skipped")
}

func TestMatchingRestrictions_SkipsWithdrawn(t *testing.T) {
	withdrawn := time.Now().Add(-30 * time.Minute)
	measures := []FlowMeasure{
		{
			ID:     1,
			Ident:  "EGTT01A",
			Measure: FlowMeasureType{Type: MeasureTypeGroundStop, Value: json.RawMessage(`null`)},
			Filters: []FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
			},
			StartTime:   time.Now().Add(-1 * time.Hour),
			EndTime:     time.Now().Add(1 * time.Hour),
			WithdrawnAt: &withdrawn,
		},
	}

	strip := &models.Strip{Origin: "EGLL"}
	now := time.Now()
	restrictions := MatchingRestrictions(strip, measures, now)
	assert.Empty(t, restrictions, "withdrawn measures should be skipped")
}

func TestMatchingRestrictions_WildcardADES(t *testing.T) {
	measures := []FlowMeasure{
		makeMeasure(
			MeasureTypeGroundStop,
			json.RawMessage(`null`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EKCH"]`)},
				{Type: FilterTypeADES, Value: json.RawMessage(`["EH**"]`)},
			},
		),
	}

	strip := &models.Strip{
		Origin:      "EKCH",
		Destination: "EHAM",
	}

	now := time.Now()
	restrictions := MatchingRestrictions(strip, measures, now)
	require.Len(t, restrictions, 1)
	assert.Equal(t, "EHAM", restrictions[0].Destination)
}

func TestMatchingRestrictions_MultipleRestrictions(t *testing.T) {
	measures := []FlowMeasure{
		makeMeasure(
			MeasureTypeMandatoryRoute,
			json.RawMessage(`["UL612 LAKEY"]`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
			},
		),
		makeMeasure(
			MeasureTypeProhibit,
			json.RawMessage(`null`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
				{Type: FilterTypeLevelAbove, Value: json.RawMessage(`200`)},
			},
		),
	}

	strip := &models.Strip{
		Origin:             "EGLL",
		Destination:        "EHAM",
		RequestedAltitude:  int32Ptr(28000),
	}

	now := time.Now()
	restrictions := MatchingRestrictions(strip, measures, now)
	require.Len(t, restrictions, 2)
	assert.Equal(t, "mandatory_route", restrictions[0].Type)
	assert.Equal(t, "prohibit", restrictions[1].Type)
}

func TestMatchingRestrictions_WaypointFilter(t *testing.T) {
	measures := []FlowMeasure{
		makeMeasure(
			MeasureTypeMandatoryRoute,
			json.RawMessage(`["BIG UL9 CPT"]`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
				{Type: FilterTypeWaypoint, Value: json.RawMessage(`["BIG"]`)},
			},
		),
	}

	route := "BIG UL9 CPT EXMOR"
	strip := &models.Strip{
		Origin: "EGLL",
		Route:  &route,
	}

	now := time.Now()
	restrictions := MatchingRestrictions(strip, measures, now)
	require.Len(t, restrictions, 1)
}

func TestMatchingRestrictions_WaypointFilter_NoMatch(t *testing.T) {
	measures := []FlowMeasure{
		makeMeasure(
			MeasureTypeMandatoryRoute,
			json.RawMessage(`["BIG UL9 CPT"]`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
				{Type: FilterTypeWaypoint, Value: json.RawMessage(`["KOK"]`)},
			},
		),
	}

	route := "BIG UL9 CPT EXMOR"
	strip := &models.Strip{
		Origin: "EGLL",
		Route:  &route,
	}

	now := time.Now()
	restrictions := MatchingRestrictions(strip, measures, now)
	assert.Empty(t, restrictions, "should not match when waypoint not in route")
}
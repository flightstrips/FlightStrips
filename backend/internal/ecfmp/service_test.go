package ecfmp

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_InjectTestMeasures_AppliesRestrictionsImmediately(t *testing.T) {
	t.Cleanup(config.SetFeatureFlagsForTest(config.FeatureFlagsConfig{MandatoryRouteClearanceFlow: true}))

	frontendHub := &testutil.MockFrontendHub{}
	euroscopeHub := &testutil.MockEuroscopeHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}

	stripRepo.ListFn = func(ctx context.Context, session int32) ([]*models.Strip, error) {
		return []*models.Strip{
			{
				Callsign:    "ABC123",
				Origin:      "EGLL",
				Destination: "EHAM",
				Route:       strPtr("BIG UL9 CPT EXMOR"),
			},
		}, nil
	}
	stripRepo.GetCdmDataFn = func(ctx context.Context, session int32) ([]*models.CdmDataRow, error) {
		return []*models.CdmDataRow{
			{
				Callsign: "ABC123",
				Data:     &models.CdmData{},
			},
		}, nil
	}

	var persisted *models.CdmData
	stripRepo.SetCdmDataFn = func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
		persisted = data.Clone()
		return 1, nil
	}

	sessionRepo.ListFn = func(ctx context.Context) ([]*models.Session, error) {
		return []*models.Session{
			{
				ID:      1,
				Airport: "EGLL",
				Name:    "LIVE",
			},
		}, nil
	}

	service := NewService(NewClient(), stripRepo, sessionRepo, frontendHub, euroscopeHub)
	measures := []FlowMeasure{
		makeMeasure(
			MeasureTypeMandatoryRoute,
			json.RawMessage(`["BIG UL9 CPT EXMOR"]`),
			[]FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
				{Type: FilterTypeWaypoint, Value: json.RawMessage(`["BIG"]`)},
			},
		),
	}

	require.NoError(t, service.InjectTestMeasures(context.Background(), measures))
	require.NotNil(t, persisted)
	require.Len(t, persisted.EcfmpRestrictions, 1)
	assert.Equal(t, "mandatory_route", persisted.EcfmpRestrictions[0].Type)
	assert.Equal(t, []string{"BIG UL9 CPT EXMOR"}, persisted.EcfmpRestrictions[0].Routes)

	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Len(t, frontendHub.CdmUpdates[0].Event.EcfmpRestrictions, 1)

	require.Len(t, euroscopeHub.Broadcasts, 1)
	event, ok := euroscopeHub.Broadcasts[0].(euroscopeEvents.CdmUpdateEvent)
	require.True(t, ok, "expected ECFMP update broadcast to use a CdmUpdateEvent")
	assert.Contains(t, event.EcfmpRestrictionsJSON, "mandatory_route")
}

func TestService_InjectTestMeasures_DefaultsMissingTimeWindow(t *testing.T) {
	frontendHub := &testutil.MockFrontendHub{}
	euroscopeHub := &testutil.MockEuroscopeHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}

	stripRepo.ListFn = func(ctx context.Context, session int32) ([]*models.Strip, error) {
		return []*models.Strip{
			{
				Callsign:    "DEF456",
				Origin:      "EGLL",
				Destination: "EHAM",
				Route:       strPtr("BIG UL9 CPT EXMOR"),
			},
		}, nil
	}
	stripRepo.GetCdmDataFn = func(ctx context.Context, session int32) ([]*models.CdmDataRow, error) {
		return []*models.CdmDataRow{
			{
				Callsign: "DEF456",
				Data:     &models.CdmData{},
			},
		}, nil
	}

	var persisted *models.CdmData
	stripRepo.SetCdmDataFn = func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
		persisted = data.Clone()
		return 1, nil
	}

	sessionRepo.ListFn = func(ctx context.Context) ([]*models.Session, error) {
		return []*models.Session{
			{
				ID:      1,
				Airport: "EGLL",
				Name:    "LIVE",
			},
		}, nil
	}

	service := NewService(NewClient(), stripRepo, sessionRepo, frontendHub, euroscopeHub)
	measures := []FlowMeasure{
		{
			ID:     2,
			Ident:  "TEST02A",
			Reason: "Testing",
			Measure: FlowMeasureType{
				Type:  MeasureTypeMandatoryRoute,
				Value: json.RawMessage(`["BIG UL9 CPT EXMOR"]`),
			},
			Filters: []FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
				{Type: FilterTypeWaypoint, Value: json.RawMessage(`["BIG"]`)},
			},
		},
	}

	require.NoError(t, service.InjectTestMeasures(context.Background(), measures))
	require.NotNil(t, persisted)
	require.Len(t, persisted.EcfmpRestrictions, 1)
	assert.Equal(t, "mandatory_route", persisted.EcfmpRestrictions[0].Type)
}

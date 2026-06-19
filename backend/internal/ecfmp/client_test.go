package ecfmp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_FlowMeasures_FetchesActiveMeasures(t *testing.T) {
	measures := []FlowMeasure{
		{
			ID:     1,
			Ident:  "EGTT01A",
			Reason: "Runway capacity",
			Measure: FlowMeasureType{
				Type:  MeasureTypeMandatoryRoute,
				Value: json.RawMessage(`["UL612 LAKEY DCT NUGRA"]`),
			},
			Filters: []FlowMeasureFilter{
				{Type: FilterTypeADEP, Value: json.RawMessage(`["EGLL"]`)},
				{Type: FilterTypeADES, Value: json.RawMessage(`["EH**"]`)},
			},
			StartTime: time.Now().Add(-1 * time.Hour),
			EndTime:   time.Now().Add(1 * time.Hour),
		},
	}
	measureJSON, err := json.Marshal(measures)
	require.NoError(t, err)

	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		assert.Equal(t, "1", r.URL.Query().Get("active"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(measureJSON)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithCacheTTL(0))
	data, err := client.FlowMeasures(context.Background())
	require.NoError(t, err)
	require.Len(t, data, 1)
	assert.Equal(t, "EGTT01A", data[0].Ident)
	assert.Equal(t, MeasureTypeMandatoryRoute, data[0].Measure.Type)
	assert.Equal(t, "/flow-measure", capturedPath)
}

func TestClient_FlowMeasures_UsesCache(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheTTL(5*time.Minute),
	)

	_, err := client.FlowMeasures(context.Background())
	require.NoError(t, err)
	_, err = client.FlowMeasures(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load(), "expected only one API call due to cache")
}

func TestClient_FlowMeasures_ApiError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithCacheTTL(0))
	_, err := client.FlowMeasures(context.Background())
	require.Error(t, err)
}

func TestFlowMeasureType_MandatoryRoutes(t *testing.T) {
	ft := FlowMeasureType{
		Type:  MeasureTypeMandatoryRoute,
		Value: json.RawMessage(`["LOGAN","UL612 LAKEY DCT NUGRA"]`),
	}
	routes := ft.MandatoryRoutes()
	assert.Equal(t, []string{"LOGAN", "UL612 LAKEY DCT NUGRA"}, routes)
}

func TestFlowMeasureType_MandatoryRoutes_WrongType(t *testing.T) {
	ft := FlowMeasureType{
		Type:  MeasureTypeGroundStop,
		Value: json.RawMessage(`null`),
	}
	routes := ft.MandatoryRoutes()
	assert.Nil(t, routes)
}

func TestFlowMeasure_IsActive(t *testing.T) {
	now := time.Now()

	t.Run("active within time window", func(t *testing.T) {
		fm := FlowMeasure{StartTime: now.Add(-1 * time.Hour), EndTime: now.Add(1 * time.Hour)}
		assert.True(t, fm.IsActive(now))
	})

	t.Run("inactive before start time", func(t *testing.T) {
		fm := FlowMeasure{StartTime: now.Add(1 * time.Hour), EndTime: now.Add(2 * time.Hour)}
		assert.False(t, fm.IsActive(now))
	})

	t.Run("inactive after end time", func(t *testing.T) {
		fm := FlowMeasure{StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-1 * time.Hour)}
		assert.False(t, fm.IsActive(now))
	})

	t.Run("withdrawn measure is inactive", func(t *testing.T) {
		withdrawn := now.Add(-30 * time.Minute)
		fm := FlowMeasure{
			StartTime:   now.Add(-1 * time.Hour),
			EndTime:     now.Add(1 * time.Hour),
			WithdrawnAt: &withdrawn,
		}
		assert.False(t, fm.IsActive(now))
	})
}

func TestAirportMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		airport  string
		patterns []string
		want     bool
	}{
		{"exact match", "EGLL", []string{"EGLL"}, true},
		{"case insensitive", "egll", []string{"EGLL"}, true},
		{"double star wildcard", "EHAM", []string{"EH**"}, true},
		{"double star no match", "EGLL", []string{"EH**"}, false},
		{"single star wildcard", "EGLL", []string{"EG*"}, true},
		{"no match", "EGLL", []string{"LFPG"}, false},
		{"multiple patterns match first", "EGLL", []string{"LFPG", "EGLL"}, true},
		{"multiple patterns no match", "EGLL", []string{"LFPG", "EHAM"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, airportMatchesPattern(tt.airport, tt.patterns))
		})
	}
}

func TestFlowMeasureFilter_Airports(t *testing.T) {
	filter := FlowMeasureFilter{
		Type:  FilterTypeADEP,
		Value: json.RawMessage(`["EGLL","EGKK"]`),
	}
	assert.Equal(t, []string{"EGLL", "EGKK"}, filter.Airports())
}

func TestFlowMeasureFilter_LevelValue(t *testing.T) {
	filter := FlowMeasureFilter{
		Type:  FilterTypeLevelAbove,
		Value: json.RawMessage(`350`),
	}
	val := filter.LevelValue()
	require.NotNil(t, val)
	assert.Equal(t, 350, *val)
}

func TestFlowMeasureFilter_Levels(t *testing.T) {
	filter := FlowMeasureFilter{
		Type:  FilterTypeLevel,
		Value: json.RawMessage(`[230,240]`),
	}
	assert.Equal(t, []int{230, 240}, filter.Levels())
}
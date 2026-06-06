package webapi

import (
	"FlightStrips/internal/ecfmp"
	"encoding/json"
	"net/http"
	"time"
)

type WebAPI struct {
	client *ecfmp.Client
}

func NewWebAPI(client *ecfmp.Client) *WebAPI {
	return &WebAPI{client: client}
}

func (a *WebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ecfmp/measures", a.handleMeasures)
	mux.HandleFunc("/ecfmp/test/inject", a.handleInject)
	mux.HandleFunc("/ecfmp/test/clear", a.handleClear)
}

func (a *WebAPI) handleMeasures(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	measures, err := a.client.FlowMeasures(r.Context())
	if err != nil {
		http.Error(w, "failed to fetch measures: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(measures)
}

func (a *WebAPI) handleInject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var measures []ecfmp.FlowMeasure
	if err := json.NewDecoder(r.Body).Decode(&measures); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	now := time.Now()
	for i := range measures {
		if measures[i].StartTime.IsZero() {
			measures[i].StartTime = now.Add(-1 * time.Hour)
		}
		if measures[i].EndTime.IsZero() {
			measures[i].EndTime = now.Add(24 * time.Hour)
		}
	}

	a.client.SetTestMeasures(measures)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"injected": len(measures),
	})
}

func (a *WebAPI) handleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	a.client.SetTestMeasures(nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cleared": true,
	})
}
package webapi

import (
	"FlightStrips/internal/ecfmp"
	"encoding/json"
	"net/http"
)

type WebAPI struct {
	service *ecfmp.Service
}

func NewWebAPI(service *ecfmp.Service) *WebAPI {
	return &WebAPI{service: service}
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

	measures, err := a.service.FlowMeasures(r.Context())
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

	if err := a.service.InjectTestMeasures(r.Context(), measures); err != nil {
		http.Error(w, "failed to inject measures: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

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

	if err := a.service.ClearTestMeasures(r.Context()); err != nil {
		http.Error(w, "failed to clear measures: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cleared": true,
	})
}

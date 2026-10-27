package main

import (
	"log"
	"net/http"
	"path"
	"strings"
	"time"
)

func (h *amanSuite) feed(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	scenario, epoch, baseURL := h.scenario, h.epoch, h.feedURL
	at := h.clock.Now()
	h.mu.Unlock()
	switch {
	case r.URL.Path == "/status":
		writeJSON(w, map[string]any{"data": map[string]any{"v3": []string{baseURL + "/vatsim"}}})
	case r.URL.Path == "/vatsim":
		if scenario == "outage" {
			http.Error(w, "controlled shared source outage", 503)
			return
		}

		pilots := []any{}
		for i, callsign := range []string{"SAS101", "SAS202", "CCA877Q"} {
			lat, lon, speed := 57.0+float64(i)*.15, 13.3, 420
			lon = 13.4 + (lat-56.1)*(17.92-13.4)/(59.65-56.1)
			if callsign == "CCA877Q" && (scenario == "spike" || scenario == "unreliable") {
				speed = 180
			}
			pilots = append(pilots, map[string]any{"cid": 100000 + i, "callsign": callsign, "latitude": lat, "longitude": lon, "altitude": 25000, "groundspeed": speed, "logon_time": epoch.Add(-time.Hour), "last_updated": at, "flight_plan": map[string]any{"flight_rules": "I", "aircraft": "A320/M", "aircraft_short": "A320", "departure": "ESSA", "arrival": "EKCH", "deptime": epoch.Add(-30 * time.Minute).Format("1504"), "enroute_time": "0100", "altitude": "25000", "route": "DCT TESPI", "revision_id": 1}})
		}
		writeJSON(w, map[string]any{"general": map[string]any{"update_timestamp": at}, "pilots": pilots, "prefiles": []any{}})
	case r.URL.Path == "/api/v1/airac/current":
		writeJSON(w, map[string]any{"status": "success", "data": map[string]any{"cycle": "2610", "effective_date": "2026-10-01T00:00:00Z", "expiration_date": "2026-10-29T00:00:00Z"}})
	case r.URL.Path == "/api/v1/airports/EKCH":
		writeJSON(w, map[string]any{"status": "success", "data": map[string]any{"icao": "EKCH", "name": "Copenhagen E2E", "coordinates": map[string]any{"lat": 55.618, "lon": 12.656}}})
	case r.URL.Path == "/api/v1/procedures":
		writeJSON(w, map[string]any{"status": "success", "data": []any{}, "pagination": map[string]bool{"has_more": false}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/waypoints/"):
		writeJSON(w, map[string]any{"status": "success", "data": point(path.Base(r.URL.Path))})
	case r.URL.Path == "/api/v1/routes/parse":
		writeJSON(w, map[string]any{"status": "success", "data": map[string]any{"total_distance": 185, "segments": []any{map[string]any{"from": point("ESSA"), "to": point("TESPI"), "distance": 185, "bearing": 210}}, "errors": []any{}}})
	case r.URL.Path == "/v1/gfs":
		// A supported missing-weather response exercises the production fallback,
		// including its warning and authority policy; no predictor is replaced.
		http.Error(w, "controlled unavailable weather", 503)
	default:
		log.Printf("UNEXPECTED EXTERNAL REQUEST %s", r.URL)
		http.Error(w, "unmatched external fixture", 500)
	}
}

// Synthetic AIRAC.NET wire fixtures. These go through the real HTTP decoder,
// materializer, activation validation and PostgreSQL navigation cache.
var positions = map[string][2]float64{
	"ESSA": {59.65, 17.92}, "EKCH": {55.618, 12.656},
	"TESPI": {56.1, 13.4}, "ROSBI": {55.9, 13.0}, "TUDLO": {56.2, 12.7},
	"MONAK": {55.9, 11.2}, "OLPIB": {55.7, 11.9}, "TIDVU": {55.4, 13.8}, "ERNOV": {55.0, 12.2},
	"CH632": {55.938464971, 12.4659798388},
	"ERPUK": {55.45, 12.40}, "DOPEM": {55.44, 12.42}, "ABEGI": {55.76, 12.85}, "ADOVI": {55.75, 12.84}, "AGTIC": {55.682428, 12.176869}, "FEDJO": {55.837475, 12.351667}, "HOFFO": {55.570833, 13.073333}, "COPHO": {55.416558, 12.902131}, "BUDIQ": {55.435267, 12.362403}, "CATWU": {55.431833, 12.3695}, "CH12F": {55.706753, 12.415008}, "EXTAR": {55.781472, 12.910667}, "RUCCI": {55.785053, 12.903611}, "CH30F": {55.531728, 12.888461},
}

func point(id string) map[string]any {
	coordinate, ok := positions[id]
	if !ok {
		hash := 0
		for _, c := range id {
			hash += int(c)
		}
		coordinate = [2]float64{55.5 + float64(hash%71)/100, 12 + float64(hash%97)/100}
	}
	return map[string]any{"identifier": id, "coordinates": map[string]float64{"lat": coordinate[0], "lon": coordinate[1]}}
}

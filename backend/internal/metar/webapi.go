package metar

import (
	"FlightStrips/internal/shared"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

var airportCode = regexp.MustCompile(`^[A-Z]{4}$`)

// RegisterRoutes exposes cached observations only. Browser/EFB reads never
// create provider clients or initiate an upstream request.
func RegisterRoutes(mux *http.ServeMux, auth shared.AuthenticationService, poller *Poller) {
	mux.HandleFunc("/weather/atis", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") || auth == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if _, err := auth.Validate(strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		airport := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("airport")))
		if !airportCode.MatchString(airport) {
			http.Error(w, "invalid airport", http.StatusBadRequest)
			return
		}
		observation := poller.GetATIS(airport, r.URL.Query().Get("departure") == "true")
		w.Header().Set("Content-Type", "application/json")
		if observation == nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "ATIS is offline or not yet acquired"})
			return
		}
		_ = json.NewEncoder(w).Encode(observation)
	})
}

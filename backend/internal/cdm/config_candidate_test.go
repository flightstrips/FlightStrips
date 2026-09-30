package cdm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchAirportCandidateUsesProvidersWithoutMutatingLegacyCache(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rates", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"airport":"EKCH","depRwyYes":["22L"],"rates":["24"]},{"airport":"ESSA","rates":["19"]}]`))
	})
	mux.HandleFunc("/sid", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"airport":"EKCH","rwy":"22L","sid1":"SOK","sid2":"KEMAX","value":3.5}]`))
	})
	mux.HandleFunc("/taxi", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"airport":"EKCH","runway":"22L","minutes":12,"polygon":[{"lat":55.6,"lon":12.6}]}]`))
	})
	mux.HandleFunc("/etfms/restrictions", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"airport":"EKCH","rate":18,"rateLvo":14}]`))
	})
	mux.HandleFunc("/ifps/callsign", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"callsign":"SAS123","departure":"EKCH","cdmData":{"tsat":"1220","reqTobtType":"VIFF"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	store := NewCdmConfigStore(server.URL+"/rates", server.URL+"/sid", server.URL+"/taxi", time.Minute, CdmConfigDefaults{}, server.Client())
	store.SeedAirportConfig("EKCH", 20, 12, CdmDeiceConfig{Light: 4})
	store.SetCdmClient(NewClient(WithAPIKey("test"), WithBaseURL(server.URL), WithHTTPClient(server.Client())))
	row, err := NewClient(WithBaseURL(server.URL), WithHTTPClient(server.Client())).IFPSByCallsignParsed(context.Background(), "SAS123")
	if err != nil || row == nil || row.CDMData.TSAT != "1220" || row.CDMData.ReqTOBTType != "VIFF" {
		t.Fatalf("typed vIFF callsign response: %v %v", row, err)
	}
	candidate, err := store.FetchAirportCandidate(context.Background(), "EKCH")
	if err != nil {
		t.Fatal(err)
	}
	if candidate.DefaultRate != 18 || candidate.DefaultRateLvo != 14 || candidate.DeiceConfig.Light != 4 || len(candidate.Rates) != 1 || len(candidate.SidIntervals) != 1 || len(candidate.TaxiZones) != 1 {
		t.Fatalf("incomplete CDM candidate: %#v", candidate)
	}
	legacy := store.ConfigForAirport("EKCH")
	if legacy.DefaultRate != 20 || len(legacy.Rates) != 0 || len(legacy.SidIntervals) != 0 || len(legacy.TaxiZones) != 0 {
		t.Fatalf("candidate fetch mutated legacy cache: %#v", legacy)
	}
}

package vatsim

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
)

func TestCandidatePageRoundTripsFullFlightAndPreservesNewerPlan(t *testing.T) {
	var generation atomic.Int32
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			_, _ = fmt.Fprintf(w, `{"data":{"v3":[%q]}}`, base+"/data")
			return
		}
		if r.URL.Path != "/data" {
			http.NotFound(w, r)
			return
		}
		revision := 9
		if generation.Add(1) > 1 {
			revision = 8
		}
		_, _ = fmt.Fprintf(w, `{"general":{"update_timestamp":"2026-09-30T12:00:00Z"},"pilots":[{"cid":123456,"callsign":"SAS123","latitude":55.6,"longitude":12.6,"altitude":12000,"groundspeed":300,"flight_plan":{"departure":"EKCH","arrival":"EDDF","route":"DCT ABC","revision_id":%d}}],"prefiles":[]}`, revision)
	}))
	defer server.Close()
	base = server.URL
	cache := NewCache(server.URL+"/status", 0, server.Client())
	first, err := cache.CandidatePage(context.Background(), nil)
	if err != nil || len(first.Flights) != 1 || first.Flights[0].FlightPlan.Revision != 9 {
		t.Fatalf("first typed VATSIM page: %v %v", first, err)
	}
	second, err := cache.CandidatePage(context.Background(), first)
	if err != nil || second.Flights[0].FlightPlan.Revision != 9 {
		t.Fatalf("older plan replaced newer: %v %v", second, err)
	}
	snapshot, err := SnapshotFromPage(&pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: second}})
	flight, ok := snapshot.FlightByCID("123456")
	if err != nil || !ok || flight.FlightPlan.Origin != "EKCH" || flight.FlightPlan.Route != "DCT ABC" || !flight.Online() {
		t.Fatalf("restored snapshot: %v %v %v", flight, ok, err)
	}
}

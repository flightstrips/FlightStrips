package cluster

import (
	"context"
	"reflect"
	"testing"
	"time"

	"FlightStrips/internal/cdm"
)

func TestCdmConfigTypedCheckpointAndReplay(t *testing.T) {
	store, objects, nav := navFixture(t)
	ctx := context.Background()
	deadline := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	calls := 0
	fetch := func(context.Context, string) (*cdm.CdmAirportConfig, error) {
		calls++
		return &cdm.CdmAirportConfig{Airport: "EKCH", DefaultRate: 20, DefaultRateLvo: 14, DefaultTaxiMinutes: 10,
			Rates:       []cdm.CdmRate{{Airport: "EKCH", DepRwyYes: []string{"22L"}, Rates: []string{"24"}}},
			TaxiZones:   []cdm.CdmTaxiZone{{Airport: "EKCH", Runway: "22L", Minutes: 12, Polygon: []cdm.CdmTaxiPoint{{Lat: 55.6, Lon: 12.6}}}},
			DeiceConfig: cdm.CdmDeiceConfig{Light: 4, Platform: []cdm.CdmDeicePlatformConfig{{Name: "north", Time: 8}}}}, nil
	}
	worker := CdmConfigCandidateWorker{State: nav, Worker: ExternalCallWorker{Writer: nav.Writer}, Fetch: fetch, Now: func() time.Time { return deadline }}
	if sent, err := worker.Refresh(ctx, "EKCH", deadline); err != nil || !sent || calls != 1 {
		t.Fatalf("first CDM refresh: %v %v calls=%d", sent, err, calls)
	}
	page, revision, err := worker.Read(ctx, "EKCH")
	if err != nil || page == nil || revision != 1 || len(page.Rates) != 1 || len(page.TaxiZones) != 1 || len(page.Deice.Platforms) != 1 {
		t.Fatalf("typed CDM page: %v revision=%d err=%v", page, revision, err)
	}
	handoffAirport(t, store)
	otherNav := NavigationWeather{Writer: Writer{Store: store, NodeID: "node-b"}, Objects: objects}
	other := CdmConfigCandidateWorker{State: otherNav, Worker: ExternalCallWorker{Writer: otherNav.Writer}, Fetch: fetch}
	if err := other.Resume(ctx, "EKCH"); err != nil {
		t.Fatal(err)
	}
	if sent, err := other.Refresh(ctx, "EKCH", deadline); err != nil || sent || calls != 1 {
		t.Fatalf("replayed CDM fetch: %v %v calls=%d", sent, err, calls)
	}
	if _, err := typedCdmConfig(&cdm.CdmAirportConfig{Airport: "EKCH", DefaultRate: 20, DefaultRateLvo: 14, DefaultTaxiMinutes: 10, TaxiZones: []cdm.CdmTaxiZone{{Airport: "ESSA"}}}, deadline); err == nil {
		t.Fatal("foreign airport configuration accepted")
	}
}

func TestCdmConfigurationRoundTrip(t *testing.T) {
	source := &cdm.CdmAirportConfig{Airport: "EKCH", DefaultRate: 20, DefaultRateLvo: 14, DefaultTaxiMinutes: 10, LvoActive: true,
		ActiveArrivalRunways: []string{"22L"}, ActiveDepartureRunways: []string{"22R"},
		Rates:        []cdm.CdmRate{{Airport: "EKCH", ArrRwyYes: []string{"22L"}, DepRwyNo: []string{"04R"}, Rates: []string{"24"}, RatesLvo: []string{"18"}}},
		SidIntervals: []cdm.CdmSidInterval{{Airport: "EKCH", Runway: "22L", Sid1: "SOK", Sid2: "KEMAX", Value: 3.5}},
		TaxiZones:    []cdm.CdmTaxiZone{{Airport: "EKCH", Runway: "22L", Minutes: 12, Polygon: []cdm.CdmTaxiPoint{{Lat: 55.6, Lon: 12.6}}, RemoteTaxiMinutes: []int{14}}},
		Delays:       []cdm.CdmDelay{{Airport: "EKCH", Runway: "22L", Time: "1200", Type: "deice"}},
		DeiceConfig:  cdm.CdmDeiceConfig{Light: 4, Medium: 6, Heavy: 8, Super: 10, Platform: []cdm.CdmDeicePlatformConfig{{Name: "north", Time: 8}}}}
	page, err := typedCdmConfig(source, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	restored, err := OperationalCdmConfig(page)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(source, restored) {
		t.Fatalf("CDM configuration changed in typed page:\nsource=%#v\nrestored=%#v", source, restored)
	}
}

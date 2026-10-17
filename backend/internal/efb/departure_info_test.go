package efb

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDepartureInfoPrefersMatchingStrip(t *testing.T) {
	for _, matched := range []bool{true, false} {
		sid, runway, altitude := "GOLGA1F", "04R", int32(7000)
		cfg := WebAPIConfig{
			Auth: authStub{}, Network: networkTestSource(t, true, time.Now()),
			ATIS: &atisLookupStub{}, Viff: &networkTimingStub{},
			Departures: departureFrequencyStub{frequency: "124.980"},
		}
		if matched {
			cfg.Flights = &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{
				Callsign: "SAS99X", Origin: "EKCH", Destination: "EKYT",
				Sid: &sid, Runway: &runway, ClearedAltitude: &altitude,
			}}}
		} else {
			cfg.Flights = &finderStub{err: pdc.ErrWebStripNotFound}
		}
		api := NewWebAPI(cfg)
		req := httptest.NewRequest(http.MethodGet, "/efb/flight", nil)
		req.Header.Set("Authorization", "Bearer test")
		rec := httptest.NewRecorder()
		api.handleFlight(rec, req)
		var got snapshot
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
			t.Fatalf("flight lookup failed: %d %s", rec.Code, rec.Body.String())
		}
		if matched {
			if got.SID == nil || *got.SID != sid || got.Runway == nil || *got.Runway != runway || got.ClearedAltitude == nil || *got.ClearedAltitude != altitude {
				t.Fatalf("expected strip departure information: %s", rec.Body.String())
			}
		} else if got.SID == nil || *got.SID != "GOLGA" || got.Runway == nil || *got.Runway != "22R" || got.ClearedAltitude != nil {
			t.Fatalf("expected filed waypoint/ATIS fallback without invented clearance: %s", rec.Body.String())
		}
		if got.InitialClimb == nil || *got.InitialClimb != "FL70" || got.DepartureFrequency == nil || *got.DepartureFrequency != "120.255" {
			t.Fatalf("expected EKCH runway/SID rules with or without a strip: %s", rec.Body.String())
		}
		if got.CTOT == nil || *got.CTOT != "1950" {
			t.Fatal("CTOT must remain sourced from vIFF")
		}
	}
}

func TestEKCHDepartureRules(t *testing.T) {
	for _, tc := range []struct{ runway, want string }{
		{"22R", "FL70"}, {"22L", "FL70"}, {"04R", "FL70"}, {"04L", "FL70"},
		{"12", "4000ft"}, {"30", "4000ft"}, {" 22r ", "FL70"}, {"09", ""},
	} {
		got := snapshot{Phase: "DEPARTURE", Origin: "EKCH", Runway: &tc.runway}
		applyDepartureRules(&got)
		if tc.want == "" {
			if got.InitialClimb != nil {
				t.Fatalf("unknown runway %q must not invent initial climb", tc.runway)
			}
		} else if got.InitialClimb == nil || *got.InitialClimb != tc.want {
			t.Fatalf("runway %q expected %s", tc.runway, tc.want)
		}
		if got.ClearedAltitude != nil {
			t.Fatal("published initial climb must not masquerade as an ATC clearance")
		}
	}
	for _, group := range []struct {
		sids []string
		want string
	}{
		{[]string{"NEXEN", "LANGO", "KOPEX", "SALLO", "SIMEG", "KEMAX"}, "124.980"},
		{[]string{"ODDON", "GOLGA", "VEDAR"}, "120.255"},
	} {
		for _, sid := range group.sids {
			for _, suffix := range []string{"", "1A", "12B"} {
				value := sid + suffix
				got := snapshot{Phase: "DEPARTURE", Origin: "EKCH", SID: &value}
				applyDepartureRules(&got)
				if got.DepartureFrequency == nil || *got.DepartureFrequency != group.want {
					t.Fatalf("SID %s expected %s", value, group.want)
				}
			}
		}
	}
	for _, sid := range []string{"", "UNKNOWN", "GOLGAX", "GOLGA1", "GOLGA1AX"} {
		got := snapshot{Phase: "DEPARTURE", Origin: "EKCH", SID: &sid}
		applyDepartureRules(&got)
		if got.DepartureFrequency != nil {
			t.Fatalf("unknown SID %q must not invent frequency", sid)
		}
	}
	runway, sid := "22R", "GOLGA"
	for _, got := range []snapshot{
		{Phase: "ARRIVAL", Origin: "EKCH", Runway: &runway, SID: &sid},
		{Phase: "DEPARTURE", Origin: "ESSA", Runway: &runway, SID: &sid},
	} {
		applyDepartureRules(&got)
		if got.InitialClimb != nil || got.DepartureFrequency != nil {
			t.Fatal("EKCH rules must not affect arrivals or other airports")
		}
	}
}

func TestDepartureRunwayFromATIS(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"RWY IN USE 22R RUNWAY 22L CONDITION REPORT", "22R"},
		{"rwy in use\n04R", "04R"},
		{"DEPARTURE RUNWAY 22R", "22R"},
		{"DEP RWY 04R", "04R"},
		{"TAKE-OFF RUNWAY 22R", "22R"},
		{"RUNWAYS IN USE 22R AND 22L", "22R"},
		{"RUNWAY 22R CONDITION REPORT", ""},
		{"RWY IN USE 99", ""},
		{"", ""},
	} {
		got := departureRunwayFromATIS(tc.text)
		if tc.want == "" {
			if got != nil {
				t.Fatalf("%q must not declare a runway, got %s", tc.text, *got)
			}
		} else if got == nil || *got != tc.want {
			t.Fatalf("%q expected %s, got %v", tc.text, tc.want, got)
		}
	}
}

func TestFiledSIDIndicator(t *testing.T) {
	for _, tc := range []struct{ route, want string }{
		{"GOLGA T506 LAPMA L621 AAL", "GOLGA"},
		{"N0440F220 DCT GOLGA/N0430F240 T506", "GOLGA"},
		{"EKCH DCT GOLGA1F", "GOLGA1F"},
		{" dct golga ", "GOLGA"},
		{"", ""},
		{"DCT", ""},
	} {
		got := filedSIDIndicator(tc.route, "EKCH")
		if tc.want == "" {
			if got != nil {
				t.Fatalf("%q expected no indicator", tc.route)
			}
		} else if got == nil || *got != tc.want {
			t.Fatalf("%q expected %s, got %v", tc.route, tc.want, got)
		}
	}
}

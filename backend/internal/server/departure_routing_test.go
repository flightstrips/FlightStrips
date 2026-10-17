package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Use the EKCH configuration loaded by TestMain so these tests exercise sector
// allocation as well as clearance selection, route trimming and display.
func TestDepartureRoutingSequencingEligibility(t *testing.T) {
	type target struct{ owner, label, frequency string }
	ad := target{"121.730", "AD", "121.730"}
	ge := target{"121.830", "GE", "121.830"}
	gw := target{"118.580", "GW", "118.580"}
	tw := target{"119.355", "TW", "119.355"}
	sequence := target{"121.905", "SEQ PLN", "121.905"}
	delivery := target{"119.905", "DEL", "119.905"}
	carriedSequence := target{"118.105", "SEQ PLN", "121.905"}
	carriedGW := target{"118.105", "GW", "118.580"}
	carriedGE := target{"118.105", "GE", "121.830"}
	combined := target{"121.905", "AD", "121.905"}
	combinedAD := target{"121.730", "AD", "121.905"}
	tests := []struct {
		name          string
		omit          []string
		carry         []string
		add           []config.ControllerCoverage
		want          [3]target // North, East/South, West
		wantAfterAD   []string
		wantAfterShow target
	}{
		{
			name: "AA carriage cannot capture staffed departure recipients",
			want: [3]target{ad, ge, gw}, wantAfterAD: []string{"118.580", "119.355"}, wantAfterShow: gw,
		},
		{
			name:  "primed GE beats A TWR carrying GE",
			carry: []string{"121.830"},
			want:  [3]target{ad, ge, gw}, wantAfterAD: []string{"118.580", "119.355"}, wantAfterShow: gw,
		},
		{
			name:  "primed GW beats A TWR carrying GW",
			carry: []string{"118.580"},
			want:  [3]target{ad, ge, gw}, wantAfterAD: []string{"118.580", "119.355"}, wantAfterShow: gw,
		},
		{
			name:  "primed AD beats A TWR carrying AD",
			carry: []string{"121.730"},
			want:  [3]target{ad, ge, gw}, wantAfterAD: []string{"118.580", "119.355"}, wantAfterShow: gw,
		},
		{
			name: "genuine GW carriage works when GE and GW are offline",
			omit: []string{"121.830", "118.580"}, carry: []string{"118.580"},
			want: [3]target{ad, tw, carriedGW}, wantAfterAD: []string{"118.105", "119.355"}, wantAfterShow: carriedGW,
		},
		{
			name: "GE carriage cannot capture GW from primed D TWR fallback",
			omit: []string{"121.830", "118.580"}, carry: []string{"121.830"},
			want: [3]target{ad, carriedGE, tw}, wantAfterAD: []string{"119.355"}, wantAfterShow: tw,
		},
		{
			name: "stale AD strip ownership does not retain unrelated SQ fallback",
			omit: []string{"121.730", "121.830", "118.580"},
			want: [3]target{tw, tw, tw}, wantAfterAD: []string{"119.355"}, wantAfterShow: tw,
		},
		{
			name: "unavailable AD retains legitimate GW primary fallback",
			omit: []string{"121.730"},
			want: [3]target{gw, ge, gw}, wantAfterAD: []string{"118.580", "119.355"}, wantAfterShow: gw,
		},
		{
			name: "explicit DEL still sequences without carrying SQ",
			add:  []config.ControllerCoverage{{Name: "EKCH_DEL", Frequency: "119.905"}},
			want: [3]target{delivery, delivery, delivery}, wantAfterAD: []string{"118.580", "119.355"}, wantAfterShow: gw,
		},
		{
			name: "explicit SQ precedes DEL and departure stages",
			add:  []config.ControllerCoverage{{Name: "EKCH_B_GND", Frequency: "121.905"}, {Name: "EKCH_DEL", Frequency: "119.905"}},
			want: [3]target{sequence, sequence, sequence}, wantAfterAD: []string{"118.580", "119.355"}, wantAfterShow: gw,
		},
		{
			name:  "genuine SQ carriage precedes staffed departure stages",
			carry: []string{"121.905"},
			want:  [3]target{carriedSequence, carriedSequence, carriedSequence}, wantAfterAD: []string{"118.580", "119.355"}, wantAfterShow: gw,
		},
		{
			name:        "genuine DEL carriage preserves sequencing eligibility",
			carry:       []string{"119.905"},
			want:        [3]target{{"118.105", "TE", "118.105"}, {"118.105", "TE", "118.105"}, {"118.105", "TE", "118.105"}},
			wantAfterAD: []string{"118.580", "119.355"}, wantAfterShow: gw,
		},
		{
			name: "combined SQ primary carrying AD keeps AD on sequence frequency",
			omit: []string{"121.730"},
			add:  []config.ControllerCoverage{{Name: "EKCH_B_GND", Frequency: "121.905", CoveredFrequencies: []string{"121.730"}}},
			want: [3]target{combined, sequence, sequence}, wantAfterAD: []string{"121.905", "118.580", "119.355"}, wantAfterShow: combined,
		},
		{
			name: "combined AD primary carrying SQ keeps AD on sequence frequency",
			// The fixture adds this carriage to C GND rather than A TWR.
			add:         []config.ControllerCoverage{{Name: "EKCH_C_GND", Frequency: "121.730", CoveredFrequencies: []string{"121.905"}}},
			want:        [3]target{combinedAD, {"121.730", "SEQ PLN", "121.905"}, {"121.730", "SEQ PLN", "121.905"}},
			wantAfterAD: []string{"118.580", "119.355"}, wantAfterShow: gw,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coverage := []config.ControllerCoverage{
				{Name: "EKCH_A_TWR", Frequency: "118.105", CoveredFrequencies: append([]string{"121.630"}, tt.carry...)},
				{Name: "EKCH_D_TWR", Frequency: "119.355"},
				{Name: "EKCH_C_GND", Frequency: "121.730"},
				{Name: "EKCH_GE_TWR", Frequency: "121.830"},
				{Name: "EKCH_GW_TWR", Frequency: "118.580"},
			}
			coverage = slices.DeleteFunc(coverage, func(c config.ControllerCoverage) bool { return slices.Contains(tt.omit, c.Frequency) })
			for _, c := range tt.add {
				coverage = slices.DeleteFunc(coverage, func(existing config.ControllerCoverage) bool { return existing.Frequency == c.Frequency })
				coverage = append(coverage, c)
			}
			session := handoverSession()
			owners, radio := departureRoutingCoverage(session, coverage)
			ownership := buildRouteOwnership(owners)
			for i, stand := range []string{"A12", "G110", "W1"} {
				t.Run(stand, func(t *testing.T) {
					strip := &models.Strip{Origin: "EKCH", Stand: handoverStringPtr(stand), Runway: handoverStringPtr("22R")}
					route, ok := config.ComputeDepartureRoute(session.ActiveRunways.GetAllActiveRunways(), stand, "22R")
					require.True(t, ok)
					got := resolveClearedRouteTarget(route.Path, strip, session, ownership, radio)
					require.NotNil(t, got)
					assert.Equal(t, tt.want[i].owner, got.Owner)
					require.NotNil(t, got.Display)
					assert.Equal(t, &models.NextDisplay{Label: tt.want[i].label, Frequency: tt.want[i].frequency}, got.Display)
					state, update, err := computeRouteStateForStrip(strip, session, owners, radio)
					require.NoError(t, err)
					require.True(t, update)
					require.NotEmpty(t, state.NextOwners)
					assert.Equal(t, got.Owner, state.NextOwners[0])
					assert.Equal(t, got.Display, state.NextDisplay)
					strip.Owner = handoverStringPtr(got.Owner)
					state, update, err = computeRouteStateForStrip(strip, session, owners, radio)
					require.NoError(t, err)
					require.True(t, update)
					if len(state.NextOwners) > 0 {
						assert.NotEqual(t, got.Owner, state.NextOwners[0], "clearance recipient must not transfer to itself")
					}
				})
			}
			strip := &models.Strip{Origin: "EKCH", Stand: handoverStringPtr("A12"), Runway: handoverStringPtr("22R"), Owner: handoverStringPtr("121.730")}
			state, update, err := computeRouteStateForStrip(strip, session, owners, radio)
			require.NoError(t, err)
			require.True(t, update)
			assert.Equal(t, tt.wantAfterAD, state.NextOwners)
			assert.Equal(t, &models.NextDisplay{Label: tt.wantAfterShow.label, Frequency: tt.wantAfterShow.frequency}, state.NextDisplay)
		})
	}
}

func departureRoutingCoverage(session *models.Session, coverage []config.ControllerCoverage) ([]*models.SectorOwner, routeRadioState) {
	active := session.ActiveRunways.GetAllActiveRunways()
	assigned := config.GetControllerSectorsWithCoverage(coverage, active)
	owners := make([]*models.SectorOwner, 0, len(coverage))
	radio := handoverRadio(handoverCoverage())
	for _, c := range coverage {
		radio.roleByPrimary[c.Frequency] = c.Name
		radio.coverage[c.Frequency] = map[string]struct{}{c.Frequency: {}}
		for _, f := range c.CoveredFrequencies {
			radio.coverage[c.Frequency][f] = struct{}{}
		}
		identifier, _ := config.GetPositionLogicalIdentifier(active, c.Name, false)
		o := &models.SectorOwner{Position: c.Frequency, Identifier: identifier}
		for _, sector := range assigned[c.Frequency] {
			o.Sector = append(o.Sector, sector.KeyOrName())
		}
		owners = append(owners, o)
	}
	return owners, radio
}

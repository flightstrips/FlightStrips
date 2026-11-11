package cdm

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestReadyTobtUpdatePreservesAssignedTsatThroughPeriodicSequencing(t *testing.T) {
	now := time.Date(2026, 11, 9, 19, 32, 0, 0, time.UTC)
	stored := (&models.CdmData{
		Eobt: stringPtr("1908"),
		Tobt: stringPtr("1908"),
		Tsat: stringPtr("193500"),
		Ttot: stringPtr("194800"),
		Calculation: &models.CdmCalculation{
			BaseTime:    stringPtr("190800"),
			BaseSource:  stringPtr(models.CdmCalculationBaseTobt),
			TaxiMinutes: intPointerIfPositive(13),
			TaxiRunway:  stringPtr("04R"),
		},
	}).Normalize()
	strip := func() *models.Strip {
		return &models.Strip{Callsign: "EWG4TX", Origin: "EKCH", Runway: stringPtr("04R"), EuroscopeSeenAt: &now, CdmData: stored.Clone()}
	}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn:         func(context.Context, int32, string) (*models.Strip, error) { return strip(), nil },
		GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) { return stored.Clone(), nil },
		ListByOriginFn:          func(context.Context, int32, string) ([]*models.Strip, error) { return []*models.Strip{strip()}, nil },
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			stored = data.Clone()
			return 1, nil
		},
	}
	service := newTestCdmService(newTestClientWithAirportMasters(nil), repo, milestoneTestSessions(), &testutil.MockControllerRepository{})
	tobt, change := readyRequestTobt(stored, now)
	require.True(t, change)
	require.Equal(t, "1932", tobt)
	_, _, updated, previous, changed, recalculate, err := service.actionService.prepareTobtUpdate(context.Background(), 1267, "EWG4TX", tobt, "EKCH_GND", "ATC", now)
	require.NoError(t, err)
	require.Equal(t, "1908", previous)
	require.True(t, changed)
	require.False(t, recalculate, "bringing TOBT inside the existing TSAT window must not resequence the flight")
	stored = updated
	sequence := newTestSequenceService(repo, milestoneTestSessions(), NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), nil, nil)
	sequence.now = func() time.Time { return now }
	for _, clock := range []string{"1932", "1935", "1937", "1940"} {
		now, err = time.Parse("2006-01-02 1504", "2026-11-09 "+clock)
		require.NoError(t, err)
		require.NoError(t, sequence.RecalculateAirport(context.Background(), 1267, "EKCH"))
		require.Equal(t, "1932", valueOrEmpty(stored.Tobt), clock)
		require.Equal(t, "193500", valueOrEmpty(stored.Tsat), clock)
		require.Equal(t, "194800", valueOrEmpty(stored.Ttot), clock)
	}
}

func TestProtectedTsatSurvivesHigherPriorityDeparture(t *testing.T) {
	for _, at := range []string{"1932", "1938", "1940"} {
		for _, mode := range []string{"sequence", "snapshot"} {
			t.Run(at+"/"+mode, func(t *testing.T) {
				now, err := time.Parse("2006-01-02 1504", "2026-11-09 "+at)
				require.NoError(t, err)
				assigned := &models.Strip{Callsign: "EWG4TX", Origin: "EKCH", Runway: stringPtr("04R"), EuroscopeSeenAt: &now, CdmData: &models.CdmData{
					Tobt: stringPtr("1908"), Tsat: stringPtr("193500"), Ttot: stringPtr("194800"),
					TobtManuallyConfirmed: true,
				}}
				assigned.CdmData.Tobt = stringPtr("1932")
				require.False(t, applyTobtRecalculationPolicy(assigned.CdmData, "1932"))
				pendingTobt := at
				if at == "1932" {
					pendingTobt = "1931"
				}
				pending := &models.Strip{Callsign: "SASPENDING", Origin: "EKCH", Runway: stringPtr("04R"), EuroscopeSeenAt: &now, CdmData: &models.CdmData{
					Tobt: stringPtr(pendingTobt), TobtManuallyConfirmed: true,
				}}
				strips := []*models.Strip{pending, assigned}
				config := &CdmAirportConfig{Airport: "EKCH", DefaultTaxiMinutes: 13, DefaultRate: 10}
				if mode == "snapshot" {
					rows := buildSequenceSnapshotRows(strips, config, now)
					for _, row := range rows {
						if row.response.Callsign == assigned.Callsign {
							require.Equal(t, "1935", row.response.Tsat)
							return
						}
					}
					t.Fatal("protected departure disappeared from the sequence snapshot")
				}
				repo := &testutil.MockStripRepository{
					ListByOriginFn: func(context.Context, int32, string) ([]*models.Strip, error) { return strips, nil },
					SetCdmDataFn: func(_ context.Context, _ int32, callsign string, data *models.CdmData) (int64, error) {
						for _, strip := range strips {
							if strip.Callsign == callsign {
								strip.CdmData = data.Clone()
							}
						}
						return 1, nil
					},
				}
				store := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil)
				store.configs["EKCH"] = config
				sequence := newTestSequenceService(repo, milestoneTestSessions(), store, nil, nil)
				sequence.now = func() time.Time { return now }
				require.NoError(t, sequence.RecalculateAirport(context.Background(), 1267, "EKCH"))
				require.Equal(t, "193500", valueOrEmpty(assigned.CdmData.Tsat))
				require.Equal(t, "194800", valueOrEmpty(assigned.CdmData.Ttot))
				require.Equal(t, "194100", valueOrEmpty(pending.CdmData.Tsat), "new traffic must sequence around the protected assignment")
			})
		}
	}
}

func TestRecalculationUsesAssignedTsatForExpiry(t *testing.T) {
	for _, tc := range []struct {
		name, tobt, tsat, at, ctot, want string
	}{
		{"future TSAT with old TOBT", "1908", "193500", "1932", "", "193500"},
		{"TOBT expires before TSAT", "1932", "193500", "1938", "", "193500"},
		{"TSAT grace boundary", "1908", "193500", "1940", "", "193500"},
		{"expired TSAT", "1908", "193500", "1941", "", ""},
		{"no TSAT falls back to TOBT", "1908", "", "1932", "", ""},
		{"new CTOT with old TOBT", "1908", "193500", "1932", "1950", "193700"},
		{"midnight", "2330", "000200", "0006", "", "000200"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.Parse("2006-01-02 1504", "2026-11-09 "+tc.at)
			require.NoError(t, err)
			strip := &models.Strip{Callsign: "EWG4TX", Origin: "EKCH", Runway: stringPtr("04R"), EuroscopeSeenAt: &now, CdmData: &models.CdmData{
				Tobt: stringPtr(tc.tobt), Tsat: stringPointerIfPresent(tc.tsat),
				Recalculate: true, RecalculationMode: models.CdmRecalculationRequired,
			}}
			if tc.tsat != "" {
				strip.CdmData.Ttot = stringPtr(addMinutes(tc.tsat, 13))
			}
			if tc.ctot != "" {
				strip.CdmData.Ctot = stringPtr(tc.ctot)
				strip.CdmData.CtotSource = stringPtr(models.CtotSourceATFCM)
			}
			repo := &testutil.MockStripRepository{
				ListByOriginFn: func(context.Context, int32, string) ([]*models.Strip, error) { return []*models.Strip{strip}, nil },
				SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
					strip.CdmData = data.Clone()
					return 1, nil
				},
			}
			store := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil)
			store.configs["EKCH"] = &CdmAirportConfig{Airport: "EKCH", DefaultTaxiMinutes: 13, DefaultRate: 30}
			rows := buildSequenceSnapshotRows([]*models.Strip{strip}, store.configs["EKCH"], now)
			require.Len(t, rows, 1)
			require.Equal(t, truncateCDMClockValue(tc.want), rows[0].response.Tsat)
			sequence := newTestSequenceService(repo, milestoneTestSessions(), store, nil, nil)
			sequence.now = func() time.Time { return now }
			require.NoError(t, sequence.RecalculateAirport(context.Background(), 1267, "EKCH"))
			require.Equal(t, tc.want, valueOrEmpty(strip.CdmData.Tsat))
		})
	}
}

func TestApplyTobtRecalculationPolicyBoundaries(t *testing.T) {
	tests := []struct {
		name             string
		tobt             string
		wantRecalculate  bool
		wantMode         string
		wantTimesCleared bool
	}{
		{name: "six minutes earlier improves only", tobt: "0954", wantRecalculate: true, wantMode: models.CdmRecalculationImproveOnly},
		{name: "minus five is protected", tobt: "0955"},
		{name: "same time is protected", tobt: "1000"},
		{name: "three minutes after is protected", tobt: "1003"},
		{name: "plus five is protected", tobt: "1005"},
		{name: "six minutes later resequences", tobt: "1006", wantRecalculate: true, wantMode: models.CdmRecalculationRequired, wantTimesCleared: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tsat, ttot, ctot := "1000", "1010", "1020"
			ctotSource := models.CtotSourceATFCM
			data := &models.CdmData{
				Tobt:       stringPtr(tc.tobt),
				Tsat:       &tsat,
				Ttot:       &ttot,
				Ctot:       &ctot,
				CtotSource: &ctotSource,
			}

			got := applyTobtRecalculationPolicy(data, tc.tobt)
			if got != tc.wantRecalculate {
				t.Fatalf("recalculation = %v, want %v", got, tc.wantRecalculate)
			}
			if data.RecalculationMode != tc.wantMode {
				t.Fatalf("recalculation mode = %q, want %q", data.RecalculationMode, tc.wantMode)
			}
			if valueOrEmpty(data.Tobt) != tc.tobt {
				t.Fatalf("TOBT = %q, want %q", valueOrEmpty(data.Tobt), tc.tobt)
			}
			if tc.wantTimesCleared {
				if data.Tsat != nil || data.Ttot != nil || data.Ctot != nil || data.CtotSource != nil {
					t.Fatalf("expected automatic assignment to be cleared, got %#v", data)
				}
				return
			}
			if valueOrEmpty(data.Tsat) != tsat || valueOrEmpty(data.Ttot) != ttot || valueOrEmpty(data.Ctot) != ctot || valueOrEmpty(data.CtotSource) != ctotSource {
				t.Fatalf("expected assignment to be preserved, got %#v", data)
			}
		})
	}
}

func TestApplyTobtRecalculationPolicyProtectsAcrossMidnight(t *testing.T) {
	tsat, ttot, ctot := "2358", "0008", "0015"
	ctotSource := models.CtotSourceATFCM
	data := &models.CdmData{
		Tobt:       stringPtr("0002"),
		Tsat:       &tsat,
		Ttot:       &ttot,
		Ctot:       &ctot,
		CtotSource: &ctotSource,
	}

	if applyTobtRecalculationPolicy(data, "0002") {
		t.Fatal("expected four-minute midnight crossing to remain protected")
	}
	if valueOrEmpty(data.Tsat) != tsat || valueOrEmpty(data.Ttot) != ttot || valueOrEmpty(data.Ctot) != ctot {
		t.Fatalf("expected midnight assignment to be preserved, got %#v", data)
	}
}

func TestApplyTobtRecalculationPolicyPreservesManualCtotForResequence(t *testing.T) {
	tsat, ttot, ctot := "1000", "1010", "1020"
	ctotSource := models.CtotSourceManual
	data := &models.CdmData{
		Tobt:       stringPtr("1006"),
		Tsat:       &tsat,
		Ttot:       &ttot,
		Ctot:       &ctot,
		CtotSource: &ctotSource,
	}

	if !applyTobtRecalculationPolicy(data, "1006") {
		t.Fatal("expected later TOBT to require resequencing")
	}
	if data.Tsat != nil || data.Ttot != nil {
		t.Fatalf("expected old TSAT/TTOT to be cleared, got %#v", data)
	}
	if valueOrEmpty(data.Ctot) != ctot || valueOrEmpty(data.CtotSource) != ctotSource {
		t.Fatalf("expected manual CTOT to remain as a calculation constraint, got %#v", data)
	}
}

func TestApplyTobtRecalculationPolicyPreservesRequiredRecalculation(t *testing.T) {
	tests := []struct {
		name string
		tobt string
	}{
		{name: "protected TOBT keeps unrelated recalculation", tobt: "1003"},
		{name: "earlier TOBT does not downgrade recalculation", tobt: "0954"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := &models.CdmData{
				Tobt:              stringPtr(tc.tobt),
				Tsat:              stringPtr("1000"),
				Ttot:              stringPtr("1010"),
				Recalculate:       true,
				RecalculationMode: models.CdmRecalculationRequired,
			}

			if !applyTobtRecalculationPolicy(data, tc.tobt) {
				t.Fatal("expected the existing recalculation to remain scheduled")
			}
			if !data.NeedsLocalRecalculation() || data.RecalculationMode != models.CdmRecalculationRequired {
				t.Fatalf("expected required recalculation to be preserved, got %#v", data)
			}
		})
	}
}

func TestApplyTobtRecalculationPolicyCancelsSupersededImprovementInsideWindow(t *testing.T) {
	data := &models.CdmData{
		Tobt:              stringPtr("0955"),
		Tsat:              stringPtr("1000"),
		Ttot:              stringPtr("1010"),
		Recalculate:       true,
		RecalculationMode: models.CdmRecalculationImproveOnly,
	}

	if applyTobtRecalculationPolicy(data, "0955") {
		t.Fatal("expected the superseded improvement to be cancelled inside the protected window")
	}
	if data.NeedsLocalRecalculation() || data.RecalculationMode != "" {
		t.Fatalf("expected no pending improvement, got %#v", data)
	}
	if valueOrEmpty(data.Tsat) != "1000" || valueOrEmpty(data.Ttot) != "1010" {
		t.Fatalf("expected the protected assignment to remain frozen, got %#v", data)
	}
}

func TestPrepareTobtUpdateClearsManualCtotWhenDeicingMakesItUnachievable(t *testing.T) {
	ctotSource := models.CtotSourceManual
	stored := (&models.CdmData{
		Tobt:       stringPtr("1000"),
		Tsat:       stringPtr("1000"),
		Ttot:       stringPtr("1010"),
		Ctot:       stringPtr("1020"),
		CtotSource: &ctotSource,
		DeIce:      stringPtr("M"),
	}).Normalize()

	service := newTestCdmService(
		newTestClientWithAirportMasters(nil),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return &models.Strip{Callsign: "SASDEICE", Origin: "EKCH", Runway: stringPtr("22R")}, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return stored.Clone(), nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	service.SetConfigProvider(&stubConfigProvider{config: &CdmAirportConfig{
		Airport:            "EKCH",
		DefaultTaxiMinutes: 10,
		DeiceConfig: CdmDeiceConfig{
			Medium: 10,
		},
	}})

	_, _, updated, _, changed, recalculate, err := service.actionService.prepareTobtUpdate(
		context.Background(), 7, "SASDEICE", "1006", "EKCH_DEL", "ATC", time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("prepareTobtUpdate returned error: %v", err)
	}
	if !changed || !recalculate {
		t.Fatal("expected the later TOBT to require resequencing")
	}
	if updated.Ctot != nil || updated.CtotSource != nil {
		t.Fatalf("expected deicing to make the manual CTOT unachievable, got %#v", updated)
	}
}

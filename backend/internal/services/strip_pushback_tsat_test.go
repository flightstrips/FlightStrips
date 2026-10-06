package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/cdm"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushbackRejectionExplainsTimingAndVerificationFailures(t *testing.T) {
	now := time.Date(2026, 10, 5, 17, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, tsat, ctot, remoteCtot, reason, message string
		verified                                      bool
		err                                           error
	}{
		{"remote CTOT within TSAT", "1700", "", "1720", "ctot_present", "aircraft has a CTOT", true, nil},
		{"local CTOT", "1700", "1720", "", "ctot_present", "aircraft has a CTOT", true, nil},
		{"late TSAT", "1654", "", "", "tsat_outside_window", "outside the TSAT window", true, nil},
		{"missing TSAT", "", "", "", "tsat_unknown", "missing or invalid", true, nil},
		{"unconfirmed within TSAT", "1700", "", "", "viff_unconfirmed", "vIFF could not confirm", false, nil},
		{"failed export", "1700", "", "", "viff_unconfirmed", "vIFF could not confirm", false, fmt.Errorf("%w: vIFF declined update", cdm.ErrPushbackVerification)},
		{"failed local preparation", "1700", "", "", "preparation_failed", "timing could not be prepared", false, errors.New("database unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strip := &models.Strip{CdmData: &models.CdmData{Tsat: &tc.tsat, Ctot: &tc.ctot}}
			reason, message := pushbackRejectionDetails(strip, tc.remoteCtot, tc.verified, tc.err, now)
			assert.Equal(t, tc.reason, reason)
			assert.Contains(t, message, tc.message)
			assert.NotContains(t, message, "Aircraft pushed")
		})
	}
}

func TestPushbackDiagnosticPreservesInitialTimingAndExportError(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	now := time.Now().UTC()
	tobt, tsat := now.Add(-10*time.Minute).Format("1504"), now.Add(20*time.Minute).Format("1504")
	final := now.Format("1504")
	strip := &models.Strip{Callsign: "SAS779", CdmData: &models.CdmData{Tobt: &tobt, Tsat: &tsat}}
	svc := &StripService{
		stripReader: &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }},
		validationStore: &validationStoreFake{setValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			strip.ValidationStatus = status
			return nil
		}},
		publisher: &testutil.MockFrontendHub{},
		cdmService: pushbackCdmStub{prepare: func(context.Context, int32, string) (string, string, bool, error) {
			strip.CdmData.Tobt, strip.CdmData.Tsat = &final, &final
			return final, "", false, fmt.Errorf("%w: vIFF did not confirm update: false", cdm.ErrPushbackVerification)
		}},
	}
	require.ErrorContains(t, svc.validatePushbackTiming(context.Background(), 779, strip, "EKCH_GND"), "override")
	var entry map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
	assert.Equal(t, "viff_unconfirmed", entry["reason"])
	assert.Equal(t, tsat, entry["initial_tsat"])
	assert.Equal(t, tobt, entry["initial_tobt"])
	assert.Equal(t, final, entry["tsat"])
	assert.Equal(t, "early", entry["initial_window_state"])
	assert.Equal(t, "valid", entry["window_state"])
	assert.Contains(t, entry["verification_error"], "vIFF did not confirm update: false")
	assert.Contains(t, strip.ValidationStatus.Message, "vIFF could not confirm")
}

func TestPushbackOverrideLogOnlyFollowsSuccessfulAcknowledgement(t *testing.T) {
	for _, rows := range []int64{0, 1} {
		t.Run(fmt.Sprint(rows), func(t *testing.T) {
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			strip := &models.Strip{ValidationStatus: &models.ValidationStatus{IssueType: pushbackTsatValidationIssueType, OwningPosition: "EKCH_GND", ActivationKey: "current", Active: true}}
			svc := &StripValidationService{
				stripReader:     &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }},
				validationStore: &validationStoreFake{acknowledgeValidationStatusFn: func(context.Context, int32, string, string) (int64, error) { return rows, nil }},
				publisher:       &testutil.MockFrontendHub{},
			}
			require.NoError(t, svc.AcknowledgeValidationStatus(context.Background(), 779, "SAS779", "current", "EKCH_GND"))
			if rows == 0 {
				assert.Empty(t, output.String(), "stale/no-op acknowledgement must not claim an override")
			} else {
				assert.Contains(t, output.String(), "Pushback timing override acknowledged")
			}
		})
	}
}

func TestPushbackTransitionLogFollowsBayPersistenceOnlyOnce(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	fixture := newFrontendMoveFixture(&models.Strip{Callsign: "SAS779", Bay: shared.BAY_CLEARED, Origin: "EKCH"})
	require.NoError(t, fixture.svc.MoveFrontendStrip(fixture.ctx, 779, "SAS779", shared.BAY_PUSH, "123", "EKCH", "EKCH_GND", false, false))
	assert.Equal(t, shared.BAY_PUSH, fixture.strip.Bay)
	assert.Equal(t, 1, strings.Count(output.String(), "Pushback bay transition recorded"))
	assert.Contains(t, output.String(), `"source":"frontend"`)
	require.NoError(t, fixture.svc.MoveFrontendStrip(fixture.ctx, 779, "SAS779", shared.BAY_PUSH, "123", "EKCH", "EKCH_GND", false, false))
	assert.Equal(t, 1, strings.Count(output.String(), "Pushback bay transition recorded"))
}

type pushbackCdmStub struct {
	StripCdmService
	prepare  func(context.Context, int32, string) (string, string, bool, error)
	readCtot func(context.Context, int32, string) (string, error)
}

type takeoffAtotCdmStub struct {
	*spyStripCdmService
	calls int
}

func (s *takeoffAtotCdmStub) RecordTakeoffClearanceAtot(context.Context, int32, string) error {
	s.calls++
	return nil
}

func (s pushbackCdmStub) PreparePushback(ctx context.Context, session int32, callsign string) (string, string, bool, error) {
	return s.prepare(ctx, session, callsign)
}

func (s pushbackCdmStub) ReadPushbackCtot(ctx context.Context, session int32, callsign string) (string, error) {
	if s.readCtot != nil {
		return s.readCtot(ctx, session, callsign)
	}
	return "", nil
}

func TestPushbackTsatWindowIncludesBoundariesAcrossMidnight(t *testing.T) {
	now := time.Date(2026, 9, 26, 23, 58, 0, 0, time.UTC)
	for _, tc := range []struct {
		tsat    string
		allowed bool
	}{
		{"2353", true}, {"2352", false},
		{"0004", true}, {"0005", false},
		{"", false},
	} {
		assert.Equal(t, tc.allowed, pushbackTsatWithinWindow(tc.tsat, now), tc.tsat)
	}
}

func TestPushbackReadsRemoteCtotWithoutAcceptingRemoteTsat(t *testing.T) {
	now := time.Now().UTC()
	tobt, localTsat := now.Format("1504"), now.Add(2*time.Minute).Format("1504")
	strip := &models.Strip{Callsign: "SAS779", Origin: "EKCH", CdmData: &models.CdmData{Tobt: &tobt, Tsat: &localTsat}}
	remoteCtot := now.Add(30 * time.Minute).Format("1504")
	reads, preparations := 0, 0
	svc := &StripService{
		stripReader: &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }},
		validationStore: &validationStoreFake{setValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			strip.ValidationStatus = status
			return nil
		}},
		publisher: &testutil.MockFrontendHub{},
		cdmService: pushbackCdmStub{
			readCtot: func(context.Context, int32, string) (string, error) { reads++; return remoteCtot, nil },
			prepare: func(context.Context, int32, string) (string, string, bool, error) {
				preparations++
				return "", "", false, nil
			},
		},
	}
	require.ErrorContains(t, svc.validatePushbackTiming(context.Background(), 779, strip, "EKCH_GND"), "override")
	assert.Equal(t, 1, reads)
	assert.Zero(t, preparations)
	assert.Equal(t, localTsat, *strip.CdmData.Tsat)
	strip.ValidationStatus.Active = false
	require.NoError(t, svc.validatePushbackTiming(context.Background(), 779, strip, "EKCH_GND"))
	assert.Equal(t, 2, reads, "Override retry must recheck vIFF CTOT")
	remoteCtot = now.Add(31 * time.Minute).Format("1504")
	require.ErrorContains(t, svc.validatePushbackTiming(context.Background(), 779, strip, "EKCH_GND"), "override")
}

func TestPushbackUnconfirmedCtotNeedsOverride(t *testing.T) {
	now := time.Now().UTC()
	tobt, tsat := now.Format("1504"), now.Add(2*time.Minute).Format("1504")
	strip := &models.Strip{Callsign: "SAS779", Origin: "EKCH", CdmData: &models.CdmData{Tobt: &tobt, Tsat: &tsat}}
	svc := &StripService{
		stripReader: &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }},
		validationStore: &validationStoreFake{setValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			strip.ValidationStatus = status
			return nil
		}},
		publisher: &testutil.MockFrontendHub{},
		cdmService: pushbackCdmStub{readCtot: func(context.Context, int32, string) (string, error) {
			return "", errors.New("vIFF timeout")
		}},
	}
	require.ErrorContains(t, svc.validatePushbackTiming(context.Background(), 779, strip, "EKCH_GND"), "override")
	strip.ValidationStatus.Active = false
	require.NoError(t, svc.validatePushbackTiming(context.Background(), 779, strip, "EKCH_GND"))
}

func TestPushbackOverrideContextChangesWithWindowPhase(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	tobt, tsat := "1150", "1220"
	strip := &models.Strip{CdmData: &models.CdmData{Tobt: &tobt, Tsat: &tsat}}
	before := pushbackTimingContext(strip, "", true, now)
	after := pushbackTimingContext(strip, "", true, now.Add(15*time.Minute))
	assert.NotEqual(t, before, after)
}

func TestPushbackTimeoutRequiresOverrideForSameTimingState(t *testing.T) {
	now := time.Now().UTC()
	tobt := now.Format("1504")
	tsat := now.Add(20 * time.Minute).Format("1504")
	owner := "EKCH_GND"
	strip := &models.Strip{Callsign: "SAS779", Origin: "EKCH", Owner: &owner, CdmData: &models.CdmData{Tobt: &tobt, Tsat: &tsat}}
	prepareCalls := 0
	svc := &StripService{
		stripReader: &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }},
		validationStore: &validationStoreFake{
			setValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
				strip.ValidationStatus = status
				return nil
			},
			clearValidationStatusFn: func(context.Context, int32, string) error { strip.ValidationStatus = nil; return nil },
		},
		publisher: &testutil.MockFrontendHub{},
		cdmService: pushbackCdmStub{prepare: func(context.Context, int32, string) (string, string, bool, error) {
			prepareCalls++
			return tsat, "", false, nil
		}},
	}
	err := svc.validatePushbackTiming(context.Background(), 779, strip, owner)
	require.ErrorContains(t, err, "override")
	require.NotNil(t, strip.ValidationStatus)
	assert.True(t, strip.ValidationStatus.Active)
	assert.Equal(t, 1, prepareCalls)

	strip.ValidationStatus.Active = false
	require.NoError(t, svc.validatePushbackTiming(context.Background(), 779, strip, owner))
	assert.Equal(t, 1, prepareCalls)

	changedTsat := now.Add(21 * time.Minute).Format("1504")
	strip.CdmData.Tsat = &changedTsat
	require.ErrorContains(t, svc.validatePushbackTiming(context.Background(), 779, strip, owner), "override")
	assert.Equal(t, 2, prepareCalls)
}

func TestPushbackRecalculationRequiresOverrideWhenViffReturnsCtot(t *testing.T) {
	now := time.Now().UTC()
	tobt := now.Format("1504")
	tsat := now.Add(20 * time.Minute).Format("1504")
	ctot := now.Add(30 * time.Minute).Format("1504")
	owner := "EKCH_GND"
	strip := &models.Strip{Callsign: "SAS780", Origin: "EKCH", Owner: &owner, CdmData: &models.CdmData{Tobt: &tobt, Tsat: &tsat}}
	prepared := 0
	svc := &StripService{
		stripReader: &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }},
		validationStore: &validationStoreFake{
			setValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
				strip.ValidationStatus = status
				return nil
			},
			clearValidationStatusFn: func(context.Context, int32, string) error { strip.ValidationStatus = nil; return nil },
		},
		publisher: &testutil.MockFrontendHub{},
		cdmService: pushbackCdmStub{prepare: func(context.Context, int32, string) (string, string, bool, error) {
			prepared++
			recalculated := now.Format("1504")
			strip.CdmData.Tsat = &recalculated
			strip.CdmData.Ctot = &ctot
			return recalculated, ctot, true, nil
		}},
	}
	require.ErrorContains(t, svc.validatePushbackTiming(context.Background(), 780, strip, owner), "override")
	require.NotNil(t, strip.ValidationStatus)
	assert.Equal(t, 1, prepared)
	strip.ValidationStatus.Active = false
	require.NoError(t, svc.validatePushbackTiming(context.Background(), 780, strip, owner))
	assert.Equal(t, 1, prepared)
}

func TestDirectPushbackEntryPointsValidateBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		name string
		move func(*StripService) error
	}{
		{"ground state move", func(s *StripService) error {
			return s.UpdateGroundStateForMove(context.Background(), 779, "SAS779", shared.BAY_PUSH, "1234567", "EKCH")
		}},
		{"bay move", func(s *StripService) error {
			return s.MoveToBay(context.Background(), 779, "SAS779", shared.BAY_PUSH, true)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			tobt, tsat := now.Format("1504"), now.Add(20*time.Minute).Format("1504")
			strip := &models.Strip{Callsign: "SAS779", Origin: "EKCH", Bay: shared.BAY_CLEARED,
				CdmData: &models.CdmData{Tobt: &tobt, Tsat: &tsat}}
			prepareCalls := 0
			svc := &StripService{
				stripReader: &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }},
				validationStore: &validationStoreFake{setValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
					strip.ValidationStatus = status
					return nil
				}},
				publisher: &testutil.MockFrontendHub{},
				cdmService: pushbackCdmStub{prepare: func(context.Context, int32, string) (string, string, bool, error) {
					prepareCalls++
					return tsat, "", false, nil
				}},
			}
			require.ErrorContains(t, tc.move(svc), "override")
			assert.Equal(t, shared.BAY_CLEARED, strip.Bay)
			assert.Equal(t, 1, prepareCalls)
		})
	}
}

func TestEuroscopePushbackAcceptsStateOutsideTsatWindow(t *testing.T) {
	for _, fullSync := range []bool{false, true} {
		name := "ground state event"
		if fullSync {
			name = "full strip update"
		}
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
			t.Cleanup(func() { slog.SetDefault(previousLogger) })
			now := time.Now().UTC()
			tobt, tsat := now.Format("1504"), now.Add(20*time.Minute).Format("1504")
			state := euroscope.GroundStateStartup
			strip := &models.Strip{Callsign: "SAS779", Origin: "EKCH", Destination: "ENGM",
				Cleared: true, Bay: shared.BAY_CLEARED, State: &state,
				CdmData: &models.CdmData{Tobt: &tobt, Tsat: &tsat}}
			writes := 0
			repo := &testutil.MockStripRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil },
				UpdateFn: func(_ context.Context, updated *models.Strip) (int64, error) {
					writes++
					strip = updated
					return 1, nil
				},
				UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, state *string, _ string, _ *int32) (int64, error) {
					writes++
					strip.State = state
					return 1, nil
				},
				UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
					strip.Bay = bay
					return 1, nil
				},
			}
			svc, _, _ := newSyncTestFixture(t, strip, repo)
			hub := &testutil.MockEuroscopeHub{}
			svc.esCommander = hub
			svc.validationStore = &validationStoreFake{setValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
				t.Fatal("EuroScope push must not activate a blocking TSAT warning")
				return nil
			}}
			cdm := &spyStripCdmService{}
			svc.cdmService = pushbackCdmStub{
				StripCdmService: cdm,
				prepare: func(context.Context, int32, string) (string, string, bool, error) {
					t.Fatal("EuroScope push must not require TSAT preparation")
					return "", "", false, nil
				},
				readCtot: func(context.Context, int32, string) (string, error) {
					t.Fatal("EuroScope push must not wait for remote CTOT validation")
					return "", nil
				},
			}
			ctx := context.Background()
			var err error
			if fullSync {
				err = svc.syncEuroscopeStrip(ctx, 779, "1234567", euroscope.Strip{
					Callsign: strip.Callsign, Origin: strip.Origin, Destination: strip.Destination,
					Cleared: true, GroundState: euroscope.GroundStatePush,
				}, "EKCH")
			} else {
				err = svc.UpdateGroundState(ctx, 779, strip.Callsign, euroscope.GroundStatePush, "EKCH")
			}
			require.NoError(t, err)
			assert.Positive(t, writes)
			assert.Equal(t, shared.BAY_PUSH, strip.Bay)
			assert.Equal(t, euroscope.GroundStatePush, *strip.State)
			assert.Nil(t, strip.ValidationStatus)
			assert.Empty(t, hub.GroundStates)
			assert.True(t, cdm.called)
			assert.Equal(t, euroscope.GroundStatePush, cdm.groundState)
			assert.Equal(t, 1, strings.Count(output.String(), "Pushback bay transition recorded"))
			source := "euroscope"
			if fullSync {
				source = "euroscope_sync"
			}
			assert.Contains(t, output.String(), `"source":"`+source+`"`)
			assert.NotContains(t, output.String(), `"source":"frontend"`)
			assert.Contains(t, output.String(), `"initial_tsat":"`+tsat+`"`)
			assert.Contains(t, output.String(), `"window_state":"early"`)
			if fullSync {
				err = svc.syncEuroscopeStrip(ctx, 779, "1234567", euroscope.Strip{Callsign: strip.Callsign, Origin: strip.Origin, Destination: strip.Destination, Cleared: true, GroundState: euroscope.GroundStatePush}, "EKCH")
			} else {
				err = svc.UpdateGroundState(ctx, 779, strip.Callsign, euroscope.GroundStatePush, "EKCH")
			}
			require.NoError(t, err)
			assert.Equal(t, 1, strings.Count(output.String(), "Pushback bay transition recorded"), "repeated observation must not claim another transition")
		})
	}
}

func TestDirectBayPushbackRecordsAobt(t *testing.T) {
	tobt, tsat := "1200", "1202"
	strip := &models.Strip{Callsign: "SAS779", Origin: "EKCH", Bay: shared.BAY_CLEARED,
		CdmData: &models.CdmData{Tobt: &tobt, Tsat: &tsat}}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn:       func(context.Context, int32, string) (*models.Strip, error) { return strip, nil },
		GetMaxSequenceInBayFn: func(context.Context, int32, string) (int32, error) { return 0, nil },
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			strip.Bay = bay
			return 1, nil
		},
	}
	cdm := &spyStripCdmService{}
	svc := NewStripService(repo)
	svc.SetCdmService(cdm)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})
	require.NoError(t, svc.MoveToBay(context.Background(), 779, "SAS779", shared.BAY_PUSH, false))
	assert.Equal(t, euroscope.GroundStatePush, cdm.groundState)
}

func TestFullEuroscopeSyncPushbackRecordsAobt(t *testing.T) {
	current := &models.Strip{Callsign: "SAS780", Origin: "EKCH", Destination: "ENGM", Bay: shared.BAY_CLEARED}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return current, nil },
		UpdateFn: func(_ context.Context, updated *models.Strip) (int64, error) {
			current = updated
			return 1, nil
		},
	}
	svc, _, _ := newSyncTestFixture(t, current, repo)
	cdm := &spyStripCdmService{}
	svc.SetCdmService(cdm)
	incoming := euroscope.Strip{Callsign: "SAS780", Origin: "EKCH", Destination: "ENGM", Cleared: true,
		GroundState: euroscope.GroundStatePush}
	require.NoError(t, svc.syncEuroscopeStrip(context.Background(), 780, "", incoming, "EKCH"))
	assert.Equal(t, shared.BAY_PUSH, current.Bay)
	assert.Equal(t, euroscope.GroundStatePush, cdm.groundState)
}

func TestFullEuroscopeSyncDoesNotRecordAtotOnAirborneObservation(t *testing.T) {
	depart := euroscope.GroundStateDepart
	current := &models.Strip{Callsign: "SAS781", Origin: "EKCH", Destination: "ENGM", Bay: shared.BAY_DEPART, State: &depart}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return current, nil },
		UpdateFn: func(_ context.Context, updated *models.Strip) (int64, error) {
			current = updated
			return 1, nil
		},
	}
	svc, _, _ := newSyncTestFixture(t, current, repo)
	cdm := &takeoffAtotCdmStub{spyStripCdmService: &spyStripCdmService{}}
	svc.cdmService = cdm
	incoming := euroscope.Strip{
		Callsign: "SAS781", Origin: "EKCH", Destination: "ENGM", Cleared: true,
		Position: &euroscope.Position{Lat: 55.62, Lon: 12.65, Altitude: 10000},
	}
	require.NoError(t, svc.syncEuroscopeStrip(context.Background(), 781, "", incoming, "EKCH"))
	assert.Equal(t, shared.BAY_AIRBORNE, current.Bay)
	require.NoError(t, svc.syncEuroscopeStrip(context.Background(), 781, "", incoming, "EKCH"))
	assert.Zero(t, cdm.calls)
}

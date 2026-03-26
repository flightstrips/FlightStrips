package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"testing"
	"time"
)

func TestSequenceService_RecalculateAirportPersistsAndBroadcasts(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	firstTobt := addMinutes(timeToClock(now), 10)
	secondTobt := addMinutes(firstTobt, 1)
	firstTtot := addMinutes(firstTobt, 10)
	secondTtot := addMinutes(firstTtot, 3)
	secondTsat := subtractMinutes(secondTtot, 10)

	first := &models.Strip{
		Callsign: "SAS123",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		Sid:      testStringPtr("MIKLA1A"),
		CdmData: models.NewLegacyCdmData(
			testStringPtr(firstTobt),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}
	second := &models.Strip{
		Callsign: "SAS456",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		Sid:      testStringPtr("NEXEN1A"),
		CdmData: &models.CdmData{
			Tobt:    testStringPtr(secondTobt),
			EcfmpID: testStringPtr("REGUL"),
		},
	}

	var persisted []*models.CdmData
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{first, second}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted = append(persisted, data.Clone())
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{
				ID:      id,
				Airport: "EKCH",
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{"04L"},
					ArrivalRunways:   []string{"22L"},
				},
			}, nil
		},
	}
	frontendHub := &testutil.MockFrontendHub{}
	euroscopeHub := &testutil.MockEuroscopeHub{}
	configStore := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil)

	service := NewSequenceService(stripRepo, sessionRepo, configStore, frontendHub, euroscopeHub)

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}

	if len(persisted) != 2 {
		t.Fatalf("expected 2 persisted updates, got %d", len(persisted))
	}

	if got := valueOrEmpty(persisted[0].Tsat); got != firstTobt {
		t.Fatalf("expected first TSAT %q, got %q", firstTobt, got)
	}
	if got := valueOrEmpty(persisted[0].Ttot); got != firstTtot {
		t.Fatalf("expected first TTOT %q, got %q", firstTtot, got)
	}
	if got := valueOrEmpty(persisted[1].Tsat); got != secondTsat {
		t.Fatalf("expected second TSAT %q, got %q", secondTsat, got)
	}
	if got := valueOrEmpty(persisted[1].Ttot); got != secondTtot {
		t.Fatalf("expected second TTOT %q, got %q", secondTtot, got)
	}

	if len(frontendHub.CdmUpdates) != 2 {
		t.Fatalf("expected 2 frontend CDM updates, got %d", len(frontendHub.CdmUpdates))
	}

	if len(euroscopeHub.Broadcasts) != 2 {
		t.Fatalf("expected 2 EuroScope broadcasts, got %d", len(euroscopeHub.Broadcasts))
	}

	event, ok := euroscopeHub.Broadcasts[1].(euroscopeEvents.CdmUpdateEvent)
	if !ok {
		t.Fatalf("expected second broadcast to be CdmUpdateEvent, got %T", euroscopeHub.Broadcasts[1])
	}
	if event.Tsat != truncateToHHMM(secondTsat) || event.Ttot != truncateToHHMM(secondTtot) {
		t.Fatalf("unexpected broadcast event timings: %#v", event)
	}
	if event.EcfmpID != "REGUL" {
		t.Fatalf("unexpected broadcast event metadata: %#v", event)
	}
}

func testStringPtr(value string) *string {
	return &value
}

func TestSequenceService_RecalculateAirport_PreservesExternalCtot(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tobt := addMinutes(timeToClock(now), 10)
	ctot := addMinutes(tobt, 35)
	expectedTtot := toHHMMSS(ctot)
	expectedTsat := subtractMinutes(expectedTtot, 10)

	strip := &models.Strip{
		Callsign: "SAS777",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr(tobt),
			Ctot: testStringPtr(ctot),
		},
	}

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{strip}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.Ctot); got != ctot {
		t.Fatalf("expected CTOT to remain %q, got %q", ctot, got)
	}
	if got := valueOrEmpty(persisted.Tsat); got != expectedTsat {
		t.Fatalf("expected recalculated TSAT %q, got %q", expectedTsat, got)
	}
	if got := valueOrEmpty(persisted.Ttot); got != expectedTtot {
		t.Fatalf("expected recalculated TTOT %q, got %q", expectedTtot, got)
	}
}

func TestSequenceService_RecalculateAirport_UsesRequestedTobtWhenNoTobtExists(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	reqTobt := addMinutes(timeToClock(now), 15)
	expectedTtot := addMinutes(reqTobt, 10)

	strip := &models.Strip{
		Callsign: "SAS778",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			ReqTobt: testStringPtr(reqTobt),
			Eobt:    testStringPtr(addMinutes(reqTobt, -25)),
		},
	}

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{strip}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.Tsat); got != reqTobt {
		t.Fatalf("expected TSAT %q, got %q", reqTobt, got)
	}
	if got := valueOrEmpty(persisted.Ttot); got != expectedTtot {
		t.Fatalf("expected TTOT %q, got %q", expectedTtot, got)
	}
}

func TestSequenceService_RecalculateAirport_SkipsArrivalsAndStripsWithoutBaseTime(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	validTobt := addMinutes(timeToClock(now), 10)
	arrivalTobt := addMinutes(validTobt, 5)

	valid := &models.Strip{
		Callsign: "SAS779",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: models.NewLegacyCdmData(
			testStringPtr(validTobt),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}
	arrival := &models.Strip{
		Callsign: "SAS780",
		Origin:   "EKCH",
		State:    testStringPtr("ARR"),
		Runway:   testStringPtr("04L"),
		CdmData: models.NewLegacyCdmData(
			testStringPtr(arrivalTobt),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}
	empty := &models.Strip{
		Callsign: "SAS781",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData:  &models.CdmData{},
	}
	zeroBase := &models.Strip{
		Callsign: "SAS782",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: models.NewLegacyCdmData(
			testStringPtr("0000"),
			nil,
			nil,
			nil,
			nil,
			nil,
			testStringPtr("0000"),
			nil,
		),
	}

	var persistedCallsigns []string
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{arrival, empty, zeroBase, valid}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persistedCallsigns = append(persistedCallsigns, callsign)
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}
	if len(persistedCallsigns) != 1 {
		t.Fatalf("expected exactly 1 strip to be recalculated, got %d", len(persistedCallsigns))
	}
	if persistedCallsigns[0] != "SAS779" {
		t.Fatalf("expected only valid strip to persist, got %v", persistedCallsigns)
	}
}

func TestSequenceService_RecalculateAirport_SkipsAircraftWithAsatAndPreservesExistingTsat(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	activeTobt := addMinutes(timeToClock(now), 10)

	locked := &models.Strip{
		Callsign: "SAS783",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr("1000"),
			Tsat: testStringPtr("1010"),
			Ttot: testStringPtr("1020"),
			Asat: testStringPtr("1005"),
		},
	}
	active := &models.Strip{
		Callsign: "SAS784",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: models.NewLegacyCdmData(
			testStringPtr(activeTobt),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}

	var persistedCallsigns []string
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{locked, active}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persistedCallsigns = append(persistedCallsigns, callsign)
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}
	if len(persistedCallsigns) != 1 || persistedCallsigns[0] != "SAS784" {
		t.Fatalf("expected only non-ASAT strip to be recalculated, got %v", persistedCallsigns)
	}
}

func TestSequenceService_RecalculateAirport_SkipsAircraftWithAobtAndPreservesExistingTsat(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	activeTobt := addMinutes(timeToClock(now), 10)

	// Aircraft is moving — AOBT set but no ASAT (edge case, but must still lock)
	locked := &models.Strip{
		Callsign: "SAS795",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr("1000"),
			Tsat: testStringPtr("1010"),
			Ttot: testStringPtr("1020"),
			Aobt: testStringPtr("1012"),
		},
	}
	active := &models.Strip{
		Callsign: "SAS796",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData:  &models.CdmData{Tobt: testStringPtr(activeTobt)},
	}

	var persistedCallsigns []string
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{locked, active}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persistedCallsigns = append(persistedCallsigns, callsign)
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}
	if len(persistedCallsigns) != 1 || persistedCallsigns[0] != "SAS796" {
		t.Fatalf("expected only non-AOBT strip to be recalculated, got %v", persistedCallsigns)
	}
}

func TestSequenceService_RecalculateAirport_ExpiredTsatDoesNotInvalidateStartedStrip(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	expiredTsat := addMinutes(timeToClock(now), -10)
	freshTobt := addMinutes(timeToClock(now), 15)
	freshTtot := addMinutes(freshTobt, 10)

	// Aircraft started with ASAT, TSAT already past — must NOT be marked invalid
	started := &models.Strip{
		Callsign: "SAS797",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr(expiredTsat),
			Tsat: testStringPtr(expiredTsat),
			Ttot: testStringPtr(addMinutes(expiredTsat, 10)),
			Asat: testStringPtr(addMinutes(expiredTsat, 2)),
		},
	}
	fresh := &models.Strip{
		Callsign: "SAS798",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData:  &models.CdmData{Tobt: testStringPtr(freshTobt)},
	}

	persisted := map[string]*models.CdmData{}
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{started, fresh}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted[callsign] = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}
	if _, ok := persisted["SAS797"]; ok {
		t.Fatal("expected started strip (ASAT set) to remain untouched even with expired TSAT")
	}
	assertPersistedCdmTimes(t, persisted, "SAS798", freshTobt, freshTtot)
}

func TestSequenceService_RecalculateAirport_KeepsExistingLocalCalcTimesLocked(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	lockedTobt := addMinutes(timeToClock(now), 10)
	lockedTtot := addMinutes(lockedTobt, 10)
	activeTobt := addMinutes(lockedTobt, 2)

	locked := &models.Strip{
		Callsign: "SAS785",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr(lockedTobt),
			Tsat: testStringPtr(lockedTobt),
			Ttot: testStringPtr(lockedTtot),
		},
	}
	active := &models.Strip{
		Callsign: "SAS786",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: models.NewLegacyCdmData(
			testStringPtr(activeTobt),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}

	persisted := map[string]*models.CdmData{}
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{locked, active}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted[callsign] = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}

	if _, ok := persisted["SAS785"]; ok {
		t.Fatalf("expected locked local-calc strip to remain untouched, got persisted update")
	}
	if persisted["SAS786"] == nil {
		t.Fatal("expected mutable strip to be recalculated")
	}
	if got := valueOrEmpty(persisted["SAS786"].Ttot); got <= lockedTtot {
		t.Fatalf("expected recalculated TTOT to remain behind locked slot, got %q", got)
	}
}

func TestSequenceService_RecalculateAirport_ClearsExpiredLocalCalcTimes(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	expiredTobt := addMinutes(timeToClock(now), -6)
	freshTobt := addMinutes(timeToClock(now), 15)
	freshTtot := addMinutes(freshTobt, 10)

	expired := &models.Strip{
		Callsign: "SAS788",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr(expiredTobt),
			Tsat: testStringPtr(expiredTobt),
			Ttot: testStringPtr(addMinutes(expiredTobt, 10)),
		},
	}
	active := &models.Strip{
		Callsign: "SAS789",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: models.NewLegacyCdmData(
			testStringPtr(freshTobt),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}

	persisted := map[string]*models.CdmData{}
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{expired, active}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted[callsign] = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}

	if got := valueOrEmpty(persisted["SAS788"].Tsat); got != "" {
		t.Fatalf("expected expired TSAT to be cleared, got %q", got)
	}
	if got := valueOrEmpty(persisted["SAS788"].Ttot); got != "" {
		t.Fatalf("expected expired TTOT to be cleared, got %q", got)
	}
	if got := valueOrEmpty(persisted["SAS788"].Phase); got != "I" {
		t.Fatalf("expected expired strip to be marked invalid (phase=I), got %q", got)
	}
	if got := valueOrEmpty(persisted["SAS788"].Tobt); got != expiredTobt {
		t.Fatalf("expected TOBT to be preserved, got %q", got)
	}
	assertPersistedCdmTimes(t, persisted, "SAS789", freshTobt, freshTtot)
}

func TestSequenceService_RecalculateAirport_AlreadyInvalidStripIsNotRepersisted(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	expiredTobt := addMinutes(timeToClock(now), -6)
	freshTobt := addMinutes(timeToClock(now), 15)
	freshTtot := addMinutes(freshTobt, 10)
	phase := "I"

	// Strip already in invalid state — no TSAT/TTOT, phase already set
	invalid := &models.Strip{
		Callsign: "SAS792",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt:  testStringPtr(expiredTobt),
			Phase: &phase,
		},
	}
	fresh := &models.Strip{
		Callsign: "SAS793",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData:  &models.CdmData{Tobt: testStringPtr(freshTobt)},
	}

	persisted := map[string]*models.CdmData{}
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{invalid, fresh}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted[callsign] = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}
	if _, ok := persisted["SAS792"]; ok {
		t.Fatal("expected already-invalid strip to not be re-persisted")
	}
	assertPersistedCdmTimes(t, persisted, "SAS793", freshTobt, freshTtot)
}

func TestSequenceService_RecalculateAirport_InvalidStripIsRescheduledAfterNewTobt(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	oldTobt := addMinutes(timeToClock(now), -6)
	newTobt := addMinutes(timeToClock(now), 20)
	expectedTtot := addMinutes(newTobt, 10)
	phase := "I"

	strip := &models.Strip{
		Callsign: "SAS794",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt:        testStringPtr(newTobt),
			Phase:       &phase,
			Recalculate: true, // controller set new TOBT → recalc triggered
		},
	}
	_ = oldTobt

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{strip}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected strip to be recalculated")
	}
	if got := valueOrEmpty(persisted.Phase); got != "" {
		t.Fatalf("expected phase to be cleared after recalculation, got %q", got)
	}
	if got := valueOrEmpty(persisted.Tsat); got != newTobt {
		t.Fatalf("expected TSAT %q, got %q", newTobt, got)
	}
	if got := valueOrEmpty(persisted.Ttot); got != expectedTtot {
		t.Fatalf("expected TTOT %q, got %q", expectedTtot, got)
	}
}

func TestSequenceService_RecalculateAirport_RepairsPersistedDuplicateTtot(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	firstTobt := addMinutes(timeToClock(now), 10)
	duplicateTtot := addMinutes(firstTobt, 10)
	secondTobt := addMinutes(firstTobt, 1)

	locked := &models.Strip{
		Callsign: "SAS790",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr(firstTobt),
			Tsat: testStringPtr(firstTobt),
			Ttot: testStringPtr(duplicateTtot),
		},
	}
	duplicate := &models.Strip{
		Callsign: "SAS791",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr(secondTobt),
			Tsat: testStringPtr(firstTobt),
			Ttot: testStringPtr(duplicateTtot),
		},
	}

	persisted := map[string]*models.CdmData{}
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{locked, duplicate}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted[callsign] = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}

	if _, ok := persisted["SAS790"]; ok {
		t.Fatalf("expected first slot to stay locked, got persisted update")
	}
	if persisted["SAS791"] == nil {
		t.Fatal("expected duplicate TTOT strip to be recalculated")
	}
	if got := valueOrEmpty(persisted["SAS791"].Ttot); got == duplicateTtot {
		t.Fatalf("expected duplicate TTOT to be repaired, got %q", got)
	}
}

func TestSequenceService_RecalculateAirport_RecalculatesDirtyLocalCalcFlight(t *testing.T) {
	t.Parallel()

	dirty := &models.Strip{
		Callsign: "SAS787",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt:        testStringPtr("1000"),
			Tsat:        testStringPtr("1000"),
			Ttot:        testStringPtr("1010"),
			Recalculate: true,
		},
	}

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{dirty}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected dirty local-calc strip to be persisted")
	}
	if persisted.NeedsLocalRecalculation() {
		t.Fatalf("expected recalc marker to be cleared after persistence, got %#v", persisted)
	}
}

func TestSequenceService_RecalculateAirport_MixedConstraintScenario(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	baseClock := timeToClock(now)

	firstTobt := addMinutes(baseClock, 10)
	firstCtot := addMinutes(baseClock, 45)
	firstCtotFloor := toHHMMSS(firstCtot[:4])

	secondTobt := subtractMinutes(firstCtotFloor, 11)
	secondExpectedTtot := addMinutes(firstCtotFloor, 3)
	secondExpectedTsat := subtractMinutes(secondExpectedTtot, 10)

	thirdTobt := subtractMinutes(firstCtotFloor, 10)
	thirdExpectedTtot := addMinutes(firstCtotFloor, 6)
	thirdExpectedTsat := subtractMinutes(thirdExpectedTtot, 10)

	independentTobt := subtractMinutes(firstCtotFloor, 10)
	independentExpectedTtot := firstCtotFloor
	independentExpectedTsat := subtractMinutes(independentExpectedTtot, 10)

	first := &models.Strip{
		Callsign: "SAS901",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		Sid:      testStringPtr("MIKLA1A"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr(firstTobt),
			Ctot: testStringPtr(firstCtot[:4]),
		},
	}
	second := &models.Strip{
		Callsign: "SAS902",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		Sid:      testStringPtr("NEXEN1A"),
		CdmData: models.NewLegacyCdmData(
			testStringPtr(secondTobt),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}
	third := &models.Strip{
		Callsign: "SAS903",
		Origin:   "EKCH",
		Runway:   testStringPtr("22R"),
		Sid:      testStringPtr("BETUD1A"),
		CdmData: models.NewLegacyCdmData(
			testStringPtr(thirdTobt),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}
	independent := &models.Strip{
		Callsign: "SAS904",
		Origin:   "EKCH",
		Runway:   testStringPtr("30"),
		Sid:      testStringPtr("LUGAS1A"),
		CdmData: models.NewLegacyCdmData(
			testStringPtr(independentTobt),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}

	persisted := map[string]*models.CdmData{}
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{first, second, third, independent}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted[callsign] = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{
				ID:      id,
				Airport: "EKCH",
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{"04L", "22R", "30"},
					ArrivalRunways:   []string{"22L"},
				},
			}, nil
		},
	}
	configStore := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil)
	configStore.configs["EKCH"] = &CdmAirportConfig{
		Airport:            "EKCH",
		DefaultRate:        20,
		DefaultTaxiMinutes: 10,
		Rates: []CdmRate{
			{
				Airport:      "EKCH",
				DepRwyYes:    []string{"04L"},
				DependentRwy: []string{"22R"},
				Rates:        []string{"20"},
			},
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, configStore, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}

	assertPersistedCdmTimes(t, persisted, "SAS901", subtractMinutes(firstCtotFloor, 10), firstCtotFloor)
	assertPersistedCdmTimes(t, persisted, "SAS902", secondExpectedTsat, secondExpectedTtot)
	assertPersistedCdmTimes(t, persisted, "SAS903", thirdExpectedTsat, thirdExpectedTtot)
	assertPersistedCdmTimes(t, persisted, "SAS904", independentExpectedTsat, independentExpectedTtot)
}

func TestSequenceService_RecalculateAirport_DoesNotClearValidTsatWhenSharedTobtJustExpires(t *testing.T) {
	t.Parallel()

	// TOBT is just past the 5-minute expiry threshold (5.5 min ago).
	// Strip A has the natural TSAT (= TOBT), so its TSAT is also expired (5.5 min ago).
	// Strip B was pushed by 2.5 min due to sequencing, so its TSAT is only 3 min past — still valid.
	// The bug: the shared expired TOBT used to force-recalculate B, which then cleared B's TSAT
	// via shouldInvalidateStaleTobt. Only A's TSAT should be cleared.
	now := time.Now().UTC()
	expiredTobt := addMinutes(timeToClock(now), -5.5)
	pushedTsat := addMinutes(expiredTobt, 2.5)
	pushedTtot := addMinutes(pushedTsat, 10)

	a := &models.Strip{
		Callsign: "SAS801",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr(expiredTobt),
			Tsat: testStringPtr(expiredTobt),
			Ttot: testStringPtr(addMinutes(expiredTobt, 10)),
		},
	}
	b := &models.Strip{
		Callsign: "SAS802",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr(expiredTobt),
			Tsat: testStringPtr(pushedTsat),
			Ttot: testStringPtr(pushedTtot),
		},
	}

	persisted := map[string]*models.CdmData{}
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{a, b}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted[callsign] = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}

	if got := valueOrEmpty(persisted["SAS801"].Tsat); got != "" {
		t.Fatalf("SAS801: expected expired TSAT to be cleared, got %q", got)
	}
	if _, ok := persisted["SAS802"]; ok {
		t.Fatalf("SAS802: TSAT %q is still within window and should not have been cleared", pushedTsat)
	}
}

func TestSequenceService_RecalculateAirport_ClearsAllTsatsWhenBothExpiredWithSameTobt(t *testing.T) {
	t.Parallel()

	// When both strips have expired TSATs (both > 5 min past), both should be cleared —
	// this is the pre-existing correct behaviour that must remain intact.
	now := time.Now().UTC()
	expiredTobt := addMinutes(timeToClock(now), -7)

	a := &models.Strip{
		Callsign: "SAS803",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr(expiredTobt),
			Tsat: testStringPtr(expiredTobt),
			Ttot: testStringPtr(addMinutes(expiredTobt, 10)),
		},
	}
	b := &models.Strip{
		Callsign: "SAS804",
		Origin:   "EKCH",
		Runway:   testStringPtr("04L"),
		CdmData: &models.CdmData{
			Tobt: testStringPtr(expiredTobt),
			Tsat: testStringPtr(addMinutes(expiredTobt, 1)),
			Ttot: testStringPtr(addMinutes(expiredTobt, 11)),
		},
	}

	persisted := map[string]*models.CdmData{}
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{a, b}, nil
		},
		SetCdmDataFn: func(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
			persisted[callsign] = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	service := NewSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	if err := service.RecalculateAirport(context.Background(), 7, "EKCH"); err != nil {
		t.Fatalf("RecalculateAirport returned error: %v", err)
	}

	if got := valueOrEmpty(persisted["SAS803"].Tsat); got != "" {
		t.Fatalf("SAS803: expected expired TSAT to be cleared, got %q", got)
	}
	if got := valueOrEmpty(persisted["SAS804"].Tsat); got != "" {
		t.Fatalf("SAS804: expected expired TSAT to be cleared, got %q", got)
	}
}

func assertPersistedCdmTimes(t *testing.T, persisted map[string]*models.CdmData, callsign, expectedTsat, expectedTtot string) {
	t.Helper()

	data := persisted[callsign]
	if data == nil {
		t.Fatalf("expected persisted data for %s", callsign)
	}
	if got := valueOrEmpty(data.Tsat); got != expectedTsat {
		t.Fatalf("%s: expected TSAT %q, got %q", callsign, expectedTsat, got)
	}
	if got := valueOrEmpty(data.Ttot); got != expectedTtot {
		t.Fatalf("%s: expected TTOT %q, got %q", callsign, expectedTtot, got)
	}
}

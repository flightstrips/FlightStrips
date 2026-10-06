package cdm

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestRemoveSessionCancelsWorkAndClearsOnlyItsCaches(t *testing.T) {
	s := newTestCdmService(NewClient(), &testutil.MockStripRepository{}, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	ctx, cancel := s.sessionContext(context.Background(), 7)
	defer cancel()
	other, otherCancel := s.sessionContext(context.Background(), 8)
	defer otherCancel()
	s.sessionUsesViff.Store(int32(7), true)
	s.atotPushInFlight.Store("7:SAS1", struct{}{})
	s.aobtPushInFlight.Store("8:SAS2", struct{}{})
	s.RemoveSession(7)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("running work was not cancelled")
	}
	if other.Err() != nil {
		t.Fatal("another session was cancelled")
	}
	if _, ok := s.sessionUsesViff.Load(int32(7)); ok {
		t.Fatal("vIFF session cache retained")
	}
	if _, ok := s.atotPushInFlight.Load("7:SAS1"); ok {
		t.Fatal("ATOT pending cache retained")
	}
	if _, ok := s.aobtPushInFlight.Load("8:SAS2"); !ok {
		t.Fatal("another session cache was cleared")
	}
	stale, staleCancel := s.sessionContext(context.Background(), 7)
	defer staleCancel()
	if stale.Err() == nil || s.canRunLocalRecalculation(7) || s.usesViffSession(7) {
		t.Fatal("stale scheduling accepted a deleted session")
	}
}

func TestSessionDisappearedPreservesDatabaseFailures(t *testing.T) {
	s := newTestCdmService(NewClient(), &testutil.MockStripRepository{}, &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) { return nil, pgx.ErrNoRows }}, &testutil.MockControllerRepository{})
	if !s.sessionDisappeared(context.Background(), 7, pgx.ErrNoRows) {
		t.Fatal("missing session not recognised")
	}
	s.RemoveSession(7)
	if s.sessionDisappeared(context.Background(), 7, errors.New("database unavailable")) {
		t.Fatal("real database error suppressed")
	}
}

func TestPushTobtMissingStripDoesNotCallViff(t *testing.T) {
	s := newTestCdmService(NewClient(WithAPIKey("test"), WithHTTPClient(newFailingHTTPClient())), &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return nil, pgx.ErrNoRows }}, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	markSessionLive(s, 7)
	if err := s.PushTobt(context.Background(), 7, "SAS1", "1200"); err != nil {
		t.Fatal(err)
	}
}

func TestPushTobtPreservesStripLookupFailure(t *testing.T) {
	failure := errors.New("database unavailable")
	s := newTestCdmService(NewClient(WithAPIKey("test"), WithHTTPClient(newFailingHTTPClient())), &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return nil, failure }}, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	markSessionLive(s, 7)
	if err := s.PushTobt(context.Background(), 7, "SAS1", "1200"); !errors.Is(err, failure) {
		t.Fatalf("database error lost: %v", err)
	}
}

func TestSyncSessionsContinuesAfterMissingOrFailedSession(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "database failure", true: "deleted session"}[missing], func(t *testing.T) {
			var seen atomic.Int32
			failure := errors.New("database unavailable")
			if missing {
				failure = pgx.ErrNoRows
			}
			s := newTestCdmService(NewClient(WithAPIKey("test"), WithHTTPClient(newFailingHTTPClient())), &testutil.MockStripRepository{GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) { return nil, failure }}, &testutil.MockSessionRepository{
				ListFn: func(context.Context) ([]*models.Session, error) {
					return []*models.Session{{ID: 7, Name: "LIVE", Airport: "EKCH"}, {ID: 8, Name: "SWEATBOX", Airport: "EKCH"}}, nil
				},
				GetByIDFn: func(context.Context, int32) (*models.Session, error) { return nil, pgx.ErrNoRows },
			}, &testutil.MockControllerRepository{})
			err := s.syncSessions(context.Background())
			if missing && err != nil {
				t.Fatal(err)
			}
			if !missing && !errors.Is(err, failure) {
				t.Fatalf("database error lost: %v", err)
			}
			if _, ok := s.sessionUsesViff.Load(int32(8)); ok {
				seen.Add(1)
			}
			if seen.Load() != 1 {
				t.Fatal("remaining session skipped")
			}
		})
	}
}

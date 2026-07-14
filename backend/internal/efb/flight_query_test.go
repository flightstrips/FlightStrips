package efb

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"context"
	"database/sql"
	"testing"
)

func TestFlightQueryFindsFlightWithoutPDCService(t *testing.T) {
	sessions := &testutil.MockSessionRepository{GetByNamesFn: func(context.Context, string) ([]*models.Session, error) {
		return []*models.Session{{ID: 7, Name: "LIVE", Airport: "EKCH"}}, nil
	}}
	strips := &testutil.MockStripRepository{GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		if session != 7 || callsign != "SAS123" {
			t.Fatalf("unexpected lookup: session=%d callsign=%s", session, callsign)
		}
		return &models.Strip{Callsign: callsign}, nil
	}}

	match, err := NewFlightQuery(sessions, strips, true).FindWebStripByCallsign(context.Background(), " sas123 ")

	if err != nil {
		t.Fatal(err)
	}
	if match.SessionID != 7 || match.Strip.Callsign != "SAS123" {
		t.Fatalf("unexpected match: %+v", match)
	}
}

func TestFlightQueryPrefersUniqueLiveMatchInDevelopment(t *testing.T) {
	sessions := &testutil.MockSessionRepository{ListFn: func(context.Context) ([]*models.Session, error) {
		return []*models.Session{{ID: 7, Name: "LIVE"}, {ID: 8, Name: "TRAINING"}}, nil
	}}
	strips := &testutil.MockStripRepository{GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		if callsign != "SAS123" {
			t.Fatalf("unexpected callsign: %s", callsign)
		}
		if session == 7 || session == 8 {
			return &models.Strip{Callsign: callsign}, nil
		}
		return nil, sql.ErrNoRows
	}}

	match, err := NewFlightQuery(sessions, strips, false).FindWebStripByCallsign(context.Background(), "SAS123")

	if err != nil {
		t.Fatal(err)
	}
	if match.SessionID != 7 {
		t.Fatalf("expected LIVE session 7, got %d", match.SessionID)
	}
}

package postgres

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/arrivalinfo"
	"FlightStrips/internal/pdc/testdata"

	"github.com/stretchr/testify/require"
)

func TestArrivalTelexStoreReservesArrivalInfoOncePerEpisode(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	var session int32
	_, err := pool.Exec(ctx, `INSERT INTO airports (name) VALUES ('EKCH') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sessions (name, airport) VALUES ('LIVE', 'EKCH') RETURNING id`).Scan(&session))
	store := NewArrivalTelexStore(pool)
	sentAt := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

	info := arrivalinfo.SentMessage{Session: session, Callsign: "SAS123", Episode: "42", Kind: arrivalinfo.KindArrivalInfo,
		Text: "EKCH ARR INFO: DELAY 5-10MIN.", SentAt: sentAt}
	id, inserted, err := store.Reserve(ctx, info)
	require.NoError(t, err)
	require.True(t, inserted)

	_, inserted, err = store.Reserve(ctx, info)
	require.NoError(t, err)
	require.False(t, inserted, "a second arrival info for the same episode must not be recorded")

	update := arrivalinfo.SentMessage{Session: session, Callsign: "SAS123", Episode: "42", Kind: arrivalinfo.KindStandUpdate,
		Text: "EKCH ARR INFO: NEW STAND D2.", Stand: "D2", SentAt: sentAt.Add(time.Minute)}
	for range 2 {
		_, inserted, err = store.Reserve(ctx, update)
		require.NoError(t, err)
		require.True(t, inserted)
	}

	other := info
	other.Episode = "SAS123/1795089600000000000"
	_, inserted, err = store.Reserve(ctx, other)
	require.NoError(t, err)
	require.True(t, inserted, "a new arrival episode gets its own arrival info")

	messages, err := store.ListEpisode(ctx, session, info.Episode)
	require.NoError(t, err)
	require.Len(t, messages, 3)
	require.Equal(t, arrivalinfo.KindArrivalInfo, messages[0].Kind)
	require.Equal(t, "", messages[0].Stand)
	require.Equal(t, "D2", messages[2].Stand)
	require.Equal(t, sentAt, messages[0].SentAt)

	require.NoError(t, store.Release(ctx, id))
	_, inserted, err = store.Reserve(ctx, info)
	require.NoError(t, err)
	require.True(t, inserted, "a released reservation can be retried")
}

func TestAMANArrivalEpisodeBackfillPreservesLegacyTelexHistory(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		stripReplaced bool
	}{{name: "replaced strip after takeoff refresh and stable transition", stripReplaced: true}, {name: "same strip after takeoff refresh and stable transition"}} {
		t.Run(scenario.name, func(t *testing.T) {
			testAMANArrivalEpisodeBackfillPreservesLegacyTelexHistory(t, scenario.stripReplaced)
		})
	}
}

func testAMANArrivalEpisodeBackfillPreservesLegacyTelexHistory(t *testing.T, stripReplaced bool) {
	t.Helper()
	pool, queries := testdata.SetupTestDB(t)
	session := testdata.SeedTestSession(t, queries)
	ctx := aman.WithSession(t.Context(), session)
	repo := NewAMANRepository(pool)
	state := amanState(1, "unused", "SAS123")
	state.SessionID = session
	takeoff := state.GeneratedAt.Add(-time.Minute)
	entered := state.GeneratedAt.Add(-5 * time.Minute)
	state.Flights[0].Lifecycle = &aman.LifecycleState{
		EnteredAt: entered, Reason: aman.LifecycleReasonStableHorizon, LastEventAt: entered,
		LastEventID: "stable", LastEventFingerprint: "test",
	}
	state.Flights[0].LatestObservation = &aman.FlightObservation{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", TakeoffDetected: &takeoff,
		ReconciledAt: state.GeneratedAt, SourceStatus: aman.DataFresh,
	}
	state.DisconnectedArrivals = map[aman.Callsign]aman.DisconnectedArrival{
		"SAS124": {RemovedAt: state.GeneratedAt, GoAroundCommandID: "report"},
		"SAS125": {RemovedAt: state.GeneratedAt},
		"SAS126": {RemovedAt: state.GeneratedAt, EpisodeID: "SAS126/current"},
	}
	_, err := repo.Commit(ctx, aman.StateCommit{State: state})
	require.NoError(t, err)
	var currentStrip string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO strips (session, callsign, origin, destination, version)
VALUES ($1, 'SAS123', 'ESSA', 'EKCH', 1) RETURNING id::text`, session).Scan(&currentStrip))
	legacyEpisode := currentStrip
	if stripReplaced {
		legacyEpisode = "42"
		require.NotEqual(t, legacyEpisode, currentStrip, "the original strip is gone")
	}
	store := NewArrivalTelexStore(pool)
	for _, message := range []arrivalinfo.SentMessage{
		{Session: session, Callsign: "SAS123", Episode: "17", Kind: arrivalinfo.KindArrivalInfo, Text: "old", SentAt: state.GeneratedAt.Add(-time.Hour)},
		{Session: session, Callsign: "sas123", Episode: legacyEpisode, Kind: arrivalinfo.KindArrivalInfo, Text: "recent", SentAt: state.GeneratedAt.Add(-10 * time.Minute)},
		{Session: session, Callsign: "SAS123", Episode: legacyEpisode, Kind: arrivalinfo.KindStandUpdate, Text: "new stand", Stand: "D2", SentAt: state.GeneratedAt.Add(-2 * time.Minute)},
		{Session: session, Callsign: "SAS124", Episode: "43", Kind: arrivalinfo.KindArrivalInfo, Text: "disconnected", SentAt: state.GeneratedAt},
	} {
		_, inserted, err := store.Reserve(ctx, message)
		require.NoError(t, err)
		require.True(t, inserted)
	}
	_, err = pool.Exec(ctx, readArrivalEpisodeBackfill(t))
	require.NoError(t, err)

	loaded, err := repo.LoadAirportState(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, legacyEpisode, loaded.Flights[0].TelexEpisodeID())
	require.Equal(t, "legacy/SAS124", loaded.DisconnectedArrivals["SAS124"].EpisodeID)
	require.Equal(t, "report", loaded.DisconnectedArrivals["SAS124"].GoAroundCommandID)
	require.Equal(t, "legacy/SAS125", loaded.DisconnectedArrivals["SAS125"].EpisodeID)
	require.Equal(t, "SAS126/current", loaded.DisconnectedArrivals["SAS126"].EpisodeID)
	messages, err := store.ListEpisode(ctx, session, loaded.Flights[0].TelexEpisodeID())
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "recent", messages[0].Text)
	require.Equal(t, "D2", messages[1].Stand)
}

func TestAMANArrivalEpisodeBackfillRejectsPreviousLegHistory(t *testing.T) {
	for _, reuseStrip := range []bool{false, true} {
		name := "replacement strip"
		if reuseStrip {
			name = "reused strip"
		}
		t.Run(name, func(t *testing.T) {
			pool, queries := testdata.SetupTestDB(t)
			session := testdata.SeedTestSession(t, queries)
			ctx := aman.WithSession(t.Context(), session)
			repo := NewAMANRepository(pool)
			state := amanState(1, "unused", "SAS123")
			state.SessionID = session
			takeoff := state.GeneratedAt.Add(-10 * time.Minute)
			grounded := takeoff.Add(-time.Minute)
			state.Flights[0].LastGroundedAt = &grounded
			state.Flights[0].LatestObservation = &aman.FlightObservation{
				Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", TakeoffDetected: &takeoff,
				ReconciledAt: state.GeneratedAt, SourceStatus: aman.DataFresh,
			}
			_, err := repo.Commit(ctx, aman.StateCommit{State: state})
			require.NoError(t, err)
			var currentStrip string
			require.NoError(t, pool.QueryRow(ctx, `INSERT INTO strips (session, callsign, origin, destination, version)
VALUES ($1, 'SAS123', 'ESSA', 'EKCH', 1) RETURNING id::text`, session).Scan(&currentStrip))
			oldEpisode := "42"
			if reuseStrip {
				oldEpisode = currentStrip
			}
			store := NewArrivalTelexStore(pool)
			previous := arrivalinfo.SentMessage{
				Session: session, Callsign: "SAS123", Episode: oldEpisode, Kind: arrivalinfo.KindArrivalInfo,
				Text: "old arrival", SentAt: takeoff.Add(-time.Hour),
			}
			_, inserted, err := store.Reserve(ctx, previous)
			require.NoError(t, err)
			require.True(t, inserted)
			previous.Kind, previous.SentAt = arrivalinfo.KindStandUpdate, state.GeneratedAt
			_, _, err = store.Reserve(ctx, previous)
			require.NoError(t, err)

			_, err = pool.Exec(ctx, readArrivalEpisodeBackfill(t))
			require.NoError(t, err)
			loaded, err := repo.LoadAirportState(ctx, "EKCH")
			require.NoError(t, err)
			episodeID := loaded.Flights[0].TelexEpisodeID()
			require.Equal(t, "legacy/SAS123", episodeID)
			messages, err := store.ListEpisode(ctx, session, episodeID)
			require.NoError(t, err)
			require.Empty(t, messages)
		})
	}
}

func TestAMANArrivalEpisodeBackfillRecoversAuditedRemoval(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		state  aman.FlightState
		linked bool
		want   bool
	}{
		{"identified airborne leg", aman.StateStable, true, true},
		{"legacy removal sharing the previous leg's strip", "", false, false},
		{"subsequent leg on the same strip", aman.StateStable, false, false},
		{"landed before removal", aman.StateLanded, true, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			testAMANArrivalEpisodeBackfillAuditedRemoval(t, testCase.state, testCase.linked, testCase.want)
		})
	}
}

func TestAMANArrivalEpisodeBackfillPreservesManuallyResumedArrivalHistory(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		removalReason  aman.LifecycleReason
		newAirborneLeg bool
		wantEpisode    string
	}{
		{"same leg resumed into airborne", aman.LifecycleReasonManualRemoval, false, "42"},
		{"later airborne leg after an old resume", aman.LifecycleReasonManualRemoval, true, "legacy/SAS123"},
		{"resume without a matching manual removal", aman.LifecycleReasonDiverted, false, "legacy/SAS123"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			pool, queries := testdata.SetupTestDB(t)
			session := testdata.SeedTestSession(t, queries)
			ctx := aman.WithSession(t.Context(), session)
			repo := NewAMANRepository(pool)
			state := amanState(1, "unused", "SAS123")
			state.SessionID = session
			_, err := repo.Commit(ctx, aman.StateCommit{State: state})
			require.NoError(t, err)

			store := NewArrivalTelexStore(pool)
			message := arrivalinfo.SentMessage{Session: session, Callsign: "SAS123", Episode: "42", Kind: arrivalinfo.KindArrivalInfo,
				Text: "already sent", SentAt: state.GeneratedAt}
			_, inserted, err := store.Reserve(ctx, message)
			require.NoError(t, err)
			require.True(t, inserted)
			stand := message
			stand.Kind, stand.Stand, stand.SentAt = arrivalinfo.KindStandUpdate, "D2", message.SentAt.Add(time.Minute)
			_, _, err = store.Reserve(ctx, stand)
			require.NoError(t, err)

			commitDisposition := func(action string, at time.Time, before, after aman.FlightState, beforeReason, afterReason aman.LifecycleReason) {
				t.Helper()
				payload, err := json.Marshal(map[string]any{
					"action": action, "callsign": "SAS123", "changed": true, "received_at": at,
					"before_state": before, "after_state": after, "before_removal_reason": beforeReason, "after_removal_reason": afterReason,
				})
				require.NoError(t, err)
				_, err = repo.Commit(ctx, aman.StateCommit{
					ExpectedRevision: state.Revision - 1, State: state,
					AuditRecords: []aman.AuditRecord{{Airport: "EKCH", Revision: state.Revision,
						Category: "aman." + action, Payload: payload, RecordedAt: at}},
				})
				require.NoError(t, err)
			}
			removedAt := state.GeneratedAt.Add(2 * time.Minute)
			state.Revision, state.GeneratedAt = 2, removedAt
			flight := &state.Flights[0]
			flight.State, flight.UpdatedAt = aman.StateRemoved, removedAt
			flight.Slot, flight.Order, flight.ActiveRouteFact = nil, nil, nil
			flight.Lifecycle = &aman.LifecycleState{EnteredAt: removedAt, Reason: scenario.removalReason,
				LastEventAt: removedAt, LastEventID: "remove", LastEventFingerprint: "test"}
			commitDisposition("remove_flight", removedAt, aman.StateStable, aman.StateRemoved, "", scenario.removalReason)

			resumedAt := removedAt.Add(time.Minute)
			state.Revision, state.GeneratedAt = 3, resumedAt
			flight.State, flight.UpdatedAt = aman.StateAirborne, resumedAt
			flight.Lifecycle = &aman.LifecycleState{EnteredAt: resumedAt, Reason: aman.LifecycleReasonAirborneDetected,
				LastEventAt: resumedAt, LastEventID: "airborne", LastEventFingerprint: "test"}
			commitDisposition("resume_flight", resumedAt, aman.StateRemoved, aman.StateAirborne, aman.LifecycleReasonManualRemoval, "")

			// A subsequent reconciliation can update the snapshot time without
			// changing the entry time of the resumed airborne lifecycle.
			state.Revision, state.GeneratedAt = 4, resumedAt.Add(time.Minute)
			flight.UpdatedAt = state.GeneratedAt
			if scenario.newAirborneLeg {
				flight.Lifecycle.EnteredAt, flight.Lifecycle.LastEventAt = state.GeneratedAt, state.GeneratedAt
			}
			_, err = repo.Commit(ctx, aman.StateCommit{ExpectedRevision: 3, State: state})
			require.NoError(t, err)
			_, err = pool.Exec(ctx, readArrivalEpisodeBackfill(t))
			require.NoError(t, err)
			loaded, err := repo.LoadAirportState(ctx, "EKCH")
			require.NoError(t, err)
			require.Equal(t, scenario.wantEpisode, loaded.Flights[0].TelexEpisodeID())
			if scenario.wantEpisode == "42" {
				messages, err := store.ListEpisode(ctx, session, loaded.Flights[0].TelexEpisodeID())
				require.NoError(t, err)
				require.Len(t, messages, 2)
				require.Equal(t, "D2", messages[1].Stand)
			}
		})
	}
}

func testAMANArrivalEpisodeBackfillAuditedRemoval(t *testing.T, removedState aman.FlightState, linked, recoverHistory bool) {
	t.Helper()
	pool, queries := testdata.SetupTestDB(t)
	session := testdata.SeedTestSession(t, queries)
	ctx := aman.WithSession(t.Context(), session)
	repo := NewAMANRepository(pool)
	state := amanState(1, "unused", "SAS123")
	state.SessionID = session
	_, err := repo.Commit(ctx, aman.StateCommit{State: state})
	require.NoError(t, err)

	var stripID string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO strips (session, callsign, origin, destination, version)
VALUES ($1, 'SAS123', 'ESSA', 'EKCH', 1) RETURNING id::text`, session).Scan(&stripID))
	message := arrivalinfo.SentMessage{
		Session: session, Callsign: "SAS123", Episode: stripID, Kind: arrivalinfo.KindArrivalInfo,
		Text: "already sent", SentAt: state.GeneratedAt,
	}
	store := NewArrivalTelexStore(pool)
	_, inserted, err := store.Reserve(ctx, message)
	require.NoError(t, err)
	require.True(t, inserted)

	missingSince := state.GeneratedAt.Add(time.Minute)
	removedAt := missingSince.Add(5 * time.Minute)
	prefiledDeparture := missingSince.Add(-2 * time.Hour)
	state.Revision, state.GeneratedAt = 2, removedAt
	flight := &state.Flights[0]
	flight.State, flight.DataStatus, flight.UpdatedAt = aman.StateRemoved, aman.DataDisconnected, removedAt
	releasedSlot := flight.Slot
	flight.Slot, flight.Order, flight.ActiveRouteFact = nil, nil, nil
	flight.Lifecycle = &aman.LifecycleState{
		EnteredAt: removedAt, Reason: aman.LifecycleReasonSourceDisappearance, LastEventAt: removedAt,
		LastEventID: "remove", LastEventFingerprint: "test", Absence: &aman.AbsenceState{MissingSince: missingSince},
	}
	facts := map[string]any{
		"callsign": flight.Callsign, "missing_since": missingSince, "removed_at": removedAt, "released_slot": releasedSlot,
	}
	if removedState != "" {
		facts["flight_state"], facts["origin"] = removedState, "ESSA"
		facts["arrival_episode_id"] = "SAS123/successor"
		facts["consumed_prefile_departure_at"] = prefiledDeparture
		if linked {
			facts["arrival_episode_id"] = stripID
		}
	}
	payload, err := json.Marshal(facts)
	require.NoError(t, err)
	_, err = repo.Commit(ctx, aman.StateCommit{
		ExpectedRevision: 1, State: state,
		AuditRecords: []aman.AuditRecord{{Airport: "EKCH", Revision: 2, Category: "aman.disconnected_flight_removed",
			Payload: payload, RecordedAt: removedAt}},
	})
	require.NoError(t, err)
	before, err := repo.LoadAirportState(ctx, "EKCH")
	require.NoError(t, err)
	require.Empty(t, before.Flights)
	require.Empty(t, before.DisconnectedArrivals)

	_, err = pool.Exec(ctx, readArrivalEpisodeBackfill(t))
	require.NoError(t, err)
	loaded, err := repo.LoadAirportState(ctx, "EKCH")
	require.NoError(t, err)
	require.Empty(t, loaded.Flights)
	if recoverHistory {
		require.Equal(t, aman.DisconnectedArrival{EpisodeID: stripID, Origin: "ESSA", MissingSince: missingSince, RemovedAt: removedAt,
			ConsumedPrefileDepartureAt: &prefiledDeparture},
			loaded.DisconnectedArrivals["SAS123"])
		_, inserted, err = store.Reserve(ctx, message)
		require.NoError(t, err)
		require.False(t, inserted)
	} else {
		require.Empty(t, loaded.DisconnectedArrivals)
	}
}

func readArrivalEpisodeBackfill(t *testing.T) string {
	t.Helper()
	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "0065-backfill-aman-arrival-episode-ids.sql"))
	require.NoError(t, err)
	return string(migration)
}

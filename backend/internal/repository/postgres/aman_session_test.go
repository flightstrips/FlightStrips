package postgres

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/coordinationrequest"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAMANSessionQueuesRestartIsolationAndCleanup(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	first := testdata.SeedTestSession(t, queries)
	var second int32
	err := pool.QueryRow(context.Background(), `INSERT INTO sessions(name,airport) VALUES('SECOND','EKCH') RETURNING id`).Scan(&second)
	require.NoError(t, err)
	contexts := []context.Context{aman.WithSession(context.Background(), first), aman.WithSession(context.Background(), second)}
	states := make([]aman.AirportState, 2)
	for i, ctx := range contexts {
		state := amanState(1, "unused", "SAS123")
		state.SessionID = aman.SessionID(ctx)
		enabled := i == 0
		state.HoldingEATWritebackEnabled = &enabled
		state.RunwayGroups[0].ActiveRatePerHour = uint32(20 + i*20)
		state.Flights[0].Slot.Time = state.Flights[0].Slot.Time.Add(time.Duration(i) * time.Hour)
		state.Flights[0].HoldingClearance = &aman.HoldingClearance{Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute, HoldEAT: []string{"1210", "1310"}[i], ObservedAt: state.GeneratedAt}
		states[i] = state
		outcome := &aman.CommandOutcome{CommandID: "same-command", Airport: "EKCH", Revision: 1, Payload: []byte(`{"accepted":true}`), RecordedAt: state.GeneratedAt}
		_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{State: state, CommandOutcome: outcome})
		require.NoError(t, err)
		// A controller in each session may use the same callsign and command ID.
		request, err := coordinationrequest.New("same-coordination-command", "EKCH", "SAS123", "EKCH_APP", "123", "EKCH_FMP", coordinationrequest.KindSpeed, coordinationrequest.Payload{Speed: &coordinationrequest.SpeedPayload{Requested: "220"}}, state.GeneratedAt)
		if err == nil {
			_, err = coordinationrequest.NewRepository(pool).Submit(ctx, request, 0)
		}
		require.NoError(t, err)
	}
	for i, ctx := range contexts {
		restored, err := NewAMANRepository(pool).LoadAirportState(ctx, "EKCH")
		require.NoError(t, err)
		require.Equal(t, states[i], restored, "restart retains only this session's queue and EAT")
		flight, err := NewAMANRepository(pool).FindActiveFactFlight(ctx, "EKCH", "SAS123")
		require.NoError(t, err)
		require.Equal(t, "SAS123", flight)
		replay, err := coordinationrequest.NewRepository(pool).ReplayAirport(ctx, "EKCH")
		require.NoError(t, err)
		require.Len(t, replay, 1)
	}
	wrong := states[0]
	_, err = NewAMANRepository(pool).Commit(contexts[1], aman.StateCommit{State: wrong})
	requireDomainErrorClass(t, err, aman.ErrorInvalidArgument)
	_, err = pool.Exec(context.Background(), `DELETE FROM sessions WHERE id=$1`, first)
	require.NoError(t, err)
	_, err = NewAMANRepository(pool).LoadAirportState(contexts[0], "EKCH")
	requireDomainErrorClass(t, err, aman.ErrorNotFound)
	replay, err := coordinationrequest.NewRepository(pool).ReplayAirport(contexts[0], "EKCH")
	require.NoError(t, err)
	require.Empty(t, replay)
	_, err = NewAMANRepository(pool).LoadCommandOutcome(contexts[0], "same-command")
	requireDomainErrorClass(t, err, aman.ErrorNotFound)
	remaining, err := NewAMANRepository(pool).LoadAirportState(contexts[1], "EKCH")
	require.NoError(t, err)
	require.Equal(t, states[1], remaining)
	var replacement int32
	err = pool.QueryRow(context.Background(), `INSERT INTO sessions(name,airport) VALUES('REPLACEMENT','EKCH') RETURNING id`).Scan(&replacement)
	require.NoError(t, err)
	_, err = NewAMANRepository(pool).LoadAirportState(aman.WithSession(context.Background(), replacement), "EKCH")
	requireDomainErrorClass(t, err, aman.ErrorNotFound)
}

func TestAMANSessionRemovalPersistsOnlyWithinOwningSession(t *testing.T) {
	for _, reason := range []aman.LifecycleReason{aman.LifecycleReasonManualRemoval, aman.LifecycleReasonDiverted, aman.LifecycleReasonSourceDisappearance} {
		t.Run(string(reason), func(t *testing.T) {
			pool, queries := testdata.SetupTestDB(t)
			session := testdata.SeedTestSession(t, queries)
			ctx := aman.WithSession(t.Context(), session)
			state := amanState(1, "unused", "SAS123")
			state.SessionID = session
			flight := &state.Flights[0]
			flight.State = aman.StateRemoved
			flight.Slot, flight.Order, flight.ManualOrder, flight.QueueOffers = nil, nil, nil, nil
			flight.FreezeReason, flight.FrozenAt, flight.FrozenOperationalTETA, flight.FrozenSlot = aman.FreezeNone, nil, nil, nil
			flight.Lifecycle = &aman.LifecycleState{EnteredAt: state.GeneratedAt, Reason: reason, LastEventAt: state.GeneratedAt, LastEventID: "remove", LastEventFingerprint: "test"}
			_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{State: state})
			require.NoError(t, err)
			loaded, err := NewAMANRepository(pool).LoadAirportState(ctx, "EKCH")
			require.NoError(t, err)
			if reason == aman.LifecycleReasonSourceDisappearance {
				require.Empty(t, loaded.Flights)
			} else {
				require.Equal(t, state.Flights, loaded.Flights)
			}
			var replacement int32
			require.NoError(t, pool.QueryRow(t.Context(), `INSERT INTO sessions(name,airport) VALUES('NEXT','EKCH') RETURNING id`).Scan(&replacement))
			_, err = NewAMANRepository(pool).LoadAirportState(aman.WithSession(t.Context(), replacement), "EKCH")
			requireDomainErrorClass(t, err, aman.ErrorNotFound)
		})
	}
}

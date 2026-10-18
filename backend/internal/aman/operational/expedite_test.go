package operational

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestExpediteFreesSlotAndCascadesFollowersDurably(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("north")
	source := dispositionFlight("EXPEDITE", group, now.Add(3*time.Minute), now.Add(3*time.Minute), 1)
	first := dispositionFlight("FIRST", group, now.Add(3*time.Minute), now.Add(6*time.Minute), 2)
	second := dispositionFlight("SECOND", group, now.Add(3*time.Minute), now.Add(9*time.Minute), 3)
	protected := dispositionFlight("PROTECTED", group, now.Add(12*time.Minute), now.Add(12*time.Minute), 4)
	protected.FreezeReason = aman.FreezeManual
	protected.FrozenAt, protected.FrozenOperationalTETA = timePointer(now), timePointer(protected.Prediction.OperationalTETA)
	protected.FrozenSlot = retargetSlot(protected.Slot, group)
	repository := dispositionRepository(now, group, source, first, second, protected)
	repository.state.RunwayGroups[0].Gaps = nil
	repository.state.RunwayGroups[0].SameSTARSpacing = nil
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKDK_FMP", ReceivedAt: now}
	command := aman.ExpediteFlightCommand{Metadata: aman.CommandMetadata{CommandID: "expedite-1", ExpectedRevision: 5}, Callsign: source.Callsign}
	accepted, err := dispositionActions(t, repository, now).ExpediteFlight(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, accepted.Changed)
	f := repository.state.Flights[0]
	require.True(t, f.Expedited)
	require.Equal(t, aman.SequenceDispositionDesequenced, f.SequenceDisposition)
	require.Equal(t, source.State, f.State, "expedite must not invent a landing or removal")
	require.Equal(t, source.Slot.Time, f.Slot.Time, "former slot remains history only")
	require.Equal(t, source.Slot.Time, repository.state.Flights[1].Slot.Time)
	require.Equal(t, first.Slot.Time, repository.state.Flights[2].Slot.Time)
	require.Equal(t, protected.Slot.Time, repository.state.Flights[3].Slot.Time)
	require.Len(t, repository.commits[0].AuditRecords, 3)
	require.Equal(t, "aman.expedite_flight", repository.commits[0].AuditRecords[0].Category)
	var audit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[0].AuditRecords[0].Payload, &audit))
	require.Equal(t, true, audit["after_expedited"])
	require.Equal(t, auth.Actor, audit["actor"])

	restarted := dispositionActions(t, repository, now.Add(time.Second))
	retry, err := restarted.ExpediteFlight(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.Len(t, repository.commits, 1)
	_, err = restarted.ResumeFlight(context.Background(), auth, aman.ResumeFlightCommand{Metadata: aman.CommandMetadata{CommandID: "resume-stale", ExpectedRevision: 5}, Callsign: source.Callsign})
	requireDomainErrorClass(t, err, aman.ErrorRevisionConflict)
	resumed, err := restarted.ResumeFlight(context.Background(), auth, aman.ResumeFlightCommand{Metadata: aman.CommandMetadata{CommandID: "resume-expedite", ExpectedRevision: 6}, Callsign: source.Callsign})
	require.NoError(t, err)
	require.True(t, resumed.Changed)
	require.False(t, repository.state.Flights[0].Expedited)
	require.True(t, repository.state.Flights[0].SequenceDisposition.Participates())
	require.NotEqual(t, source.Slot.Time, repository.state.Flights[0].Slot.Time, "resume uses a new legal opportunity")
}

func TestExpediteRejectsUnsequencedGroundAndAdvisoryTraffic(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("north")
	for name, change := range map[string]func(*aman.AMANFlight){
		"planned":     func(f *aman.AMANFlight) { f.State = aman.StatePlanned },
		"landed":      func(f *aman.AMANFlight) { f.State = aman.StateLanded },
		"removed":     func(f *aman.AMANFlight) { f.State = aman.StateRemoved },
		"desequenced": func(f *aman.AMANFlight) { f.SequenceDisposition = aman.SequenceDispositionDesequenced },
		"advisory":    func(f *aman.AMANFlight) { f.Prediction.ModelVersion = "aman-airborne-v1" },
		"no slot":     func(f *aman.AMANFlight) { f.Slot = nil },
	} {
		t.Run(name, func(t *testing.T) {
			f := dispositionFlight("NOEXP", group, now.Add(3*time.Minute), now.Add(3*time.Minute), 1)
			change(&f)
			repository := dispositionRepository(now, group, f)
			_, err := dispositionActions(t, repository, now).ExpediteFlight(context.Background(), aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKDK_FMP", ReceivedAt: now}, aman.ExpediteFlightCommand{Metadata: aman.CommandMetadata{CommandID: "invalid", ExpectedRevision: 5}, Callsign: f.Callsign})
			requireDomainErrorClass(t, err, aman.ErrorInvalidTransition)
			require.Empty(t, repository.commits)
			require.False(t, repository.state.Flights[0].Expedited)
		})
	}
}

func TestExpediteCountsHeldTrafficOnceAndSuppressesAutomaticEAT(t *testing.T) {
	for _, due := range []bool{false, true} {
		t.Run(map[bool]string{false: "future offer", true: "due commitment"}[due], func(t *testing.T) {
			svc, state, now := approachFixture(t)
			state.Flights = state.Flights[:1]
			svc.refreshHoldingPlans(&state)
			if !due {
				state.Flights[0].ApproachRelease.ReleaseAt = now.Add(time.Minute)
			}
			originalHolding := state.Flights[0].HoldingClearance
			mutation, err := svc.ExpediteFlight(aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKDK_FMP", ReceivedAt: now}, aman.ExpediteFlightCommand{Callsign: state.Flights[0].Callsign})
			require.NoError(t, err)
			changed, err := mutation(state)
			require.NoError(t, err)
			state = changed.State
			require.True(t, state.Flights[0].Expedited)
			require.Equal(t, originalHolding, state.Flights[0].HoldingClearance, "FMP action must not issue a clearance")
			require.Nil(t, state.Flights[0].Prediction.HoldingPlan)
			require.Equal(t, 1, state.ReleaseBuffer.Projected())
			if due {
				require.NotNil(t, state.Flights[0].ApproachRelease)
			} else {
				require.Nil(t, state.Flights[0].ApproachRelease)
			}

			encoded, err := json.Marshal(state)
			require.NoError(t, err)
			var restored aman.AirportState
			require.NoError(t, json.Unmarshal(encoded, &restored))
			// Neither restart nor an elapsed former slot frees the admitted workload.
			svc.deps.Now = func() time.Time { return now.Add(time.Hour) }
			svc.refreshHoldingPlans(&restored)
			require.Equal(t, 1, restored.ReleaseBuffer.Projected())
			require.Nil(t, restored.Flights[0].Prediction.HoldingPlan)
			require.True(t, restored.Flights[0].Expedited)
			restored.Flights[0].State = aman.StateLanded
			svc.refreshHoldingPlans(&restored)
			require.Zero(t, restored.ReleaseBuffer.Projected())
		})
	}
}

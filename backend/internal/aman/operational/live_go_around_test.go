package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestLiveGoAroundDetectionCreatesOnePendingConfirmationWithoutMutatingFlight(t *testing.T) {
	base := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	service := liveGoAroundTestService(t, base)
	flight := operationalFlight("flight-1", "ARRIVAL-22", "MONAK", "M", base.Add(3*time.Minute))
	flight.State = aman.StateStable
	flight.Slot = &aman.Slot{Time: base.Add(3 * time.Minute), RunwayGroupID: "ARRIVAL-22", Sequence: 1, Revision: 4, Reason: string(sequence.ReasonRateWTC)}
	originalSlot := *flight.Slot

	for index, sample := range []struct {
		latitude float64
		altitude int
	}{{-0.030, 1200}, {-0.020, 1100}, {-0.010, 1250}, {-0.005, 1400}} {
		at := base.Add(time.Duration(index+1) * time.Second)
		observation := liveObservation(at, uint64(index+1), sample.latitude, sample.altitude)
		require.NoError(t, service.detectLiveGoAround(&flight, observation, "ARRIVAL-22", false, false, at))
	}

	require.NotNil(t, flight.GoAroundConfirmation)
	require.Equal(t, aman.GoAroundConfirmationPending, flight.GoAroundConfirmation.Status)
	require.Equal(t, "climb", flight.GoAroundConfirmation.Reason)
	require.Len(t, flight.GoAroundConfirmation.EvidenceTimes, 2)
	require.Equal(t, aman.StateStable, flight.State)
	require.Equal(t, originalSlot, *flight.Slot)

	continuedEpisode := liveObservation(base.Add(5*time.Second), 5, -0.004, 1600)
	require.NoError(t, service.detectLiveGoAround(&flight, continuedEpisode, "ARRIVAL-22", false, false, base.Add(5*time.Second)))
	require.Equal(t, "flight-1/go-around/1", flight.GoAroundConfirmation.EpisodeID)
	require.Equal(t, uint64(1), flight.GoAroundDetection.Episode)
}

func TestReconcilePersistsAndAuditsPendingGoAroundAcrossRestart(t *testing.T) {
	base := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	clock := base
	repository := &memoryRepository{}
	service := liveGoAroundService(t, repository, func() time.Time { return clock })
	for index, sample := range []struct {
		latitude float64
		altitude int
	}{{-0.030, 1200}, {-0.020, 1100}, {-0.010, 1250}, {-0.005, 1400}} {
		clock = base.Add(time.Duration(index+1) * time.Second)
		observation := liveObservation(clock, uint64(index+1), sample.latitude, sample.altitude)
		takeoff := base.Add(-time.Hour)
		observation.TakeoffDetected = &takeoff
		require.NoError(t, service.Observe(context.Background(), observation))
		service.Reconcile(context.Background())
	}

	require.Equal(t, aman.GoAroundConfirmationPending, repository.state.Flights[0].GoAroundConfirmation.Status)
	require.Equal(t, aman.StateAirborne, repository.state.Flights[0].State)
	lastCommit := repository.commits[len(repository.commits)-1]
	require.Len(t, lastCommit.AuditRecords, 1)
	require.Equal(t, "aman.go_around_confirmation_pending", lastCommit.AuditRecords[0].Category)

	restarted := liveGoAroundService(t, repository, func() time.Time { return clock.Add(time.Second) })
	require.NoError(t, restarted.Observe(context.Background(), liveObservation(clock, 4, -0.005, 1400)))
	restarted.Reconcile(context.Background())
	require.Equal(t, "flight-1/go-around/1", repository.state.Flights[0].GoAroundConfirmation.EpisodeID)
	require.Equal(t, uint64(1), repository.state.Flights[0].GoAroundDetection.Episode)
}

func TestGoAroundPendingDecisionRejectsWithoutMutationAndConfirmsExactlyOnce(t *testing.T) {
	base := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	service := liveGoAroundTestService(t, base)
	state := service.initialState("EKCH", base)
	state.Revision = 4
	flight := operationalFlight("flight-1", "ARRIVAL-22", "MONAK", "M", base.Add(3*time.Minute))
	flight.State = aman.StateStable
	flight.Slot = &aman.Slot{Time: base.Add(3 * time.Minute), RunwayGroupID: "ARRIVAL-22", Sequence: 1, Revision: 4, Reason: string(sequence.ReasonRateWTC)}
	flight.GoAroundConfirmation = pendingConfirmation(base)
	state.Flights = []aman.AMANFlight{flight}
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: base.Add(time.Minute)}

	reject, err := service.RejectGoAround(auth, aman.RejectGoAroundCommand{Metadata: aman.CommandMetadata{CommandID: "reject-1", ExpectedRevision: 4}, FlightID: flight.ID, EpisodeID: "flight-1/go-around/1"})
	require.NoError(t, err)
	rejected, err := reject(state)
	require.NoError(t, err)
	require.Equal(t, aman.StateStable, rejected.State.Flights[0].State)
	require.Equal(t, flight.Slot, rejected.State.Flights[0].Slot)
	require.Equal(t, aman.GoAroundConfirmationRejected, rejected.State.Flights[0].GoAroundConfirmation.Status)
	require.Equal(t, aman.SequenceRevision(5), *rejected.State.Flights[0].GoAroundConfirmation.ResultingRevision)
	require.Nil(t, rejected.QueueOffers, "rejection must not trigger unrelated queue projection")

	state.Flights[0].GoAroundConfirmation = pendingConfirmation(base)
	confirm, err := service.ConfirmGoAround(auth, aman.ConfirmGoAroundCommand{Metadata: aman.CommandMetadata{CommandID: "confirm-1", ExpectedRevision: 4}, FlightID: flight.ID, EpisodeID: "flight-1/go-around/1"})
	require.NoError(t, err)
	confirmed, err := confirm(state)
	require.NoError(t, err)
	require.Equal(t, aman.StateGoAround, confirmed.State.Flights[0].State)
	require.Equal(t, base.Add(10*time.Minute), confirmed.State.Flights[0].Prediction.OperationalTETA)
	require.Equal(t, aman.GoAroundConfirmationConfirmed, confirmed.State.Flights[0].GoAroundConfirmation.Status)
	_, _, err = pendingGoAround(confirmed.State, flight.ID, "flight-1/go-around/1")
	require.Error(t, err)
	reportAgain, err := service.ReportGoAround(auth, aman.ReportGoAroundCommand{Metadata: aman.CommandMetadata{CommandID: "report-again", ExpectedRevision: 4}, FlightID: flight.ID, DetectedAt: base})
	require.NoError(t, err)
	_, err = reportAgain(confirmed.State)
	require.ErrorContains(t, err, "already confirmed")

	resetFlight := confirmed.State.Flights[0]
	outsideFinal := liveObservation(base.Add(2*time.Minute), 10, 0.02, 1800)
	require.NoError(t, service.detectLiveGoAround(&resetFlight, outsideFinal, "ARRIVAL-22", false, false, outsideFinal.ReconciledAt))
	require.False(t, resetFlight.GoAroundDetection.AwaitingReset)
	require.Nil(t, resetFlight.GoAroundConfirmation)
	resetFlight.State = aman.StateStable
	confirmed.State.Flights[0] = resetFlight
	reportNext, err := service.ReportGoAround(auth, aman.ReportGoAroundCommand{Metadata: aman.CommandMetadata{CommandID: "report-next", ExpectedRevision: 4}, FlightID: flight.ID, DetectedAt: base.Add(2 * time.Minute)})
	require.NoError(t, err)
	secondEpisode, err := reportNext(confirmed.State)
	require.NoError(t, err)
	require.Equal(t, aman.StateGoAround, secondEpisode.State.Flights[0].State)
}

func TestReconcileInvalidatesPendingGoAroundWhenEvidenceBecomesStaleOrLands(t *testing.T) {
	base := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	service := liveGoAroundTestService(t, base)
	state := service.initialState("EKCH", base)
	flight := operationalFlight("flight-1", "ARRIVAL-22", "MONAK", "M", base.Add(3*time.Minute))
	flight.State = aman.StateStable
	flight.GoAroundConfirmation = pendingConfirmation(base)
	flight.GoAroundDetection = &aman.GoAroundDetectionState{PolicyVersion: "aman-go-around-v1/test-v1", Episode: 1, LastEmittedEpisode: 1, AwaitingReset: true}

	t.Run("stale source", func(t *testing.T) {
		observation := liveObservation(base.Add(time.Minute), 5, -0.004, 1500)
		observation.SourceStatus = aman.DataStale
		updated, err := service.reconcileFlight(context.Background(), state, flight, observation, observation.ReconciledAt)
		require.NoError(t, err)
		require.Nil(t, updated.GoAroundConfirmation)
		require.False(t, updated.GoAroundDetection.AwaitingReset)
	})

	t.Run("landing", func(t *testing.T) {
		observation := liveObservation(base.Add(time.Minute), 5, -0.001, 20)
		groundspeed := 4.0
		observation.Surveillance.GroundspeedKnots = &groundspeed
		takeoff := base.Add(-time.Hour)
		observation.TakeoffDetected = &takeoff
		updated, err := service.reconcileFlight(context.Background(), state, flight, observation, observation.ReconciledAt)
		require.NoError(t, err)
		require.Equal(t, aman.StateLanded, updated.State)
		require.Nil(t, updated.GoAroundConfirmation)
		require.False(t, updated.GoAroundDetection.AwaitingReset)
	})
}

func liveGoAroundTestService(t *testing.T, now time.Time) *Service {
	t.Helper()
	return liveGoAroundService(t, &memoryRepository{}, func() time.Time { return now })
}

func liveGoAroundService(t *testing.T, repository *memoryRepository, now func() time.Time) *Service {
	t.Helper()
	service, err := New(Dependencies{
		Repository: repository, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{}, Publisher: &recordingPublisher{},
		Terminal: terminal.Configuration{
			Airport: "EKCH", ConfigVersion: "test-v1",
			RunwayGroups: []terminal.RunwayGroup{{
				ID: "ARRIVAL-22",
				FinalApproaches: []terminal.FinalApproachDefinition{{
					Runway: "22L", CourseTrueDeg: 0,
					Threshold: terminal.ThresholdDefinition{Position: terminal.CoordinateDefinition{LatitudeDeg: 0, LongitudeDeg: 0}},
				}},
			}},
		},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: now,
	})
	require.NoError(t, err)
	return service
}

func liveObservation(at time.Time, sequence uint64, latitude float64, altitude int) aman.FlightObservation {
	groundspeed, track := 160.0, 0.0
	return aman.FlightObservation{FlightID: "flight-1", VATSIMCID: "123", Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", ReconciledAt: at, SourceStatus: aman.DataFresh, Surveillance: &aman.SurveillanceFact{LatitudeDegrees: latitude, LongitudeDegrees: 0, AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed, TrackTrueDegrees: &track, Sequence: &sequence, ObservedAt: &at}}
}

func pendingConfirmation(at time.Time) *aman.GoAroundConfirmation {
	return &aman.GoAroundConfirmation{EpisodeID: "flight-1/go-around/1", Reason: "climb", DetectedAt: at, EvidenceTimes: []time.Time{at.Add(-time.Second), at}, Status: aman.GoAroundConfirmationPending}
}

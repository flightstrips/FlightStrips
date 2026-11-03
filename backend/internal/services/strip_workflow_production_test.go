package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"
	"bytes"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// These settings, handler and operating values are fictional test fixtures.
// They are never registered in either production catalog.
type fictionalPaperSettings struct {
	InitialBay      string            `yaml:"initial_bay"`
	RunwayBay       string            `yaml:"runway_bay"`
	BayGroundStates map[string]string `yaml:"bay_ground_states"`
	GroundStateBays map[string]string `yaml:"ground_state_bays"`
}
type fictionalPaperHandler struct{ settings fictionalPaperSettings }
type paperRelease struct{}

func (h fictionalPaperHandler) Observe(c airportworkflow.Context, o airportworkflow.Observation) (airportworkflow.Decision, error) {
	bay := h.settings.InitialBay
	if c.Strip.Exists {
		bay = c.Strip.Bay
	}
	return airportworkflow.Decision{Bay: &bay}, nil
}
func (fictionalPaperHandler) Lifecycle(_ airportworkflow.Context, event airportworkflow.Lifecycle) (airportworkflow.LifecycleDecision, error) {
	return airportworkflow.LifecycleDecision{Admit: true, Remove: event.Kind == airportworkflow.Removal}, nil
}
func (fictionalPaperHandler) Coordinate(airportworkflow.Context, airportworkflow.Coordination) (airportworkflow.CoordinationDecision, error) {
	return airportworkflow.CoordinationDecision{}, nil
}

func productionWorkflowFixtures(t *testing.T, binds *atomic.Int32) *airportworkflow.Registry {
	t.Helper()
	metadata := config.HandlerDescriptor{ID: "example-paper/v1", View: "example-paper", Layouts: []string{"example-paper"}, DecodeSettings: config.TypedAirportSettings[fictionalPaperSettings], Validate: func(d *config.AirportDocument) error {
		if d.Ownership.Strips != "none" || d.Ownership.Sectors != "none" {
			return fmt.Errorf("fictional paper requires no ownership")
		}
		return nil
	}}
	catalog, err := config.NewAirportCatalog(config.EKCHDescriptor(), metadata)
	require.NoError(t, err)
	ekch, err := os.ReadFile("config/airports/ekch.airport.yaml")
	require.NoError(t, err)
	paper, err := os.ReadFile("../.github/specs/multi-airport-support/examples/zzzz.airport.yaml")
	require.NoError(t, err)
	definitions, err := config.NewAirportRegistry(catalog, []config.AirportInput{{Source: "EKCH", Reader: bytes.NewReader(ekch)}, {Source: "fictional-only", Reader: bytes.NewReader(paper)}})
	require.NoError(t, err)
	compiled := airportworkflow.Descriptor{ID: "example-paper/v1", Bind: func(d *config.AirportDefinition) (airportworkflow.Handler, []airportworkflow.ActionDescriptor, error) {
		binds.Add(1)
		h := fictionalPaperHandler{d.Document().Workflow.Settings.(fictionalPaperSettings)}
		return h, []airportworkflow.ActionDescriptor{
			airportworkflow.Action("strip.move", func(_ airportworkflow.Context, request airportworkflow.Move) (airportworkflow.Decision, error) {
				state := h.settings.BayGroundStates[request.Bay]
				return airportworkflow.Decision{Bay: &request.Bay, GroundState: &state}, nil
			}),
			airportworkflow.Action("paper.release", func(c airportworkflow.Context, _ paperRelease) (airportworkflow.Decision, error) {
				if c.Strip.Bay != h.settings.RunwayBay {
					return airportworkflow.Decision{}, fmt.Errorf("paper release requires runway placement")
				}
				bay, state := "DONE", es.GroundStateParked
				return airportworkflow.Decision{Bay: &bay, GroundState: &state}, nil
			}),
		}, nil
	}}
	runtime, err := airportworkflow.NewRegistry(definitions, airportworkflow.EKCHDescriptor(), compiled)
	require.NoError(t, err)
	return runtime
}

type productionWorkflowPublisher struct {
	shared.StripEventPublisher
	mu           sync.Mutex
	disconnected []int32
}

func (p *productionWorkflowPublisher) SendStripUpdate(int32, string)             {}
func (p *productionWorkflowPublisher) Broadcast(int32, fe.OutgoingMessage)       {}
func (p *productionWorkflowPublisher) SendBayEvent(int32, string, string, int32) {}
func (p *productionWorkflowPublisher) SendOwnersUpdate(int32, string, string, []string, []string, *models.NextDisplay) {
}
func (p *productionWorkflowPublisher) SendAircraftDisconnect(session int32, _ string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.disconnected = append(p.disconnected, session)
}

type blockingWorkflowBinding struct {
	*testutil.MockCommandBinding
	started chan struct{}
	release chan struct{}
}

func (b *blockingWorkflowBinding) Same(other shared.CommandBinding) bool { return other == b }
func (b *blockingWorkflowBinding) Send(event es.OutgoingMessage) error {
	close(b.started)
	<-b.release
	return b.MockCommandBinding.Send(event)
}

func TestProductionWorkflowPathsAndFictionalExtensionRemainSessionScoped(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := t.Context()
	for _, airport := range []string{"EKCH", "ZZZZ", "ZZZX"} {
		require.NoError(t, queries.InsertAirport(ctx, airport))
	}
	createSession := func(airport, environment string) int32 {
		id, err := queries.InsertSession(ctx, database.InsertSessionParams{Airport: airport, Name: environment})
		require.NoError(t, err)
		return id
	}
	ekch := createSession("EKCH", "LIVE")
	live := createSession("ZZZZ", "LIVE")
	training := createSession("ZZZZ", "SWEATBOX")
	playback := createSession("ZZZZ", "PLAYBACK")
	unknown := createSession("ZZZX", "LIVE")
	var binds atomic.Int32
	registry := productionWorkflowFixtures(t, &binds)
	repo := postgres.NewStripRepository(pool)
	sessions := postgres.NewSessionRepository(pool)
	publisher := &productionWorkflowPublisher{}
	s := NewStripService(repo, WithAirportWorkflows(registry), WithSessionReader(sessions), WithStripEventPublisher(publisher), WithControllerReader(postgres.NewControllerRepository(pool)), WithSectorOwnerRepository(postgres.NewSectorOwnerRepository(pool)), WithCoordinationStore(postgres.NewCoordinationRepository(pool)), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
	observeFull := func(session int32, airport string, event es.Strip) error {
		model, err := sessions.GetByID(ctx, session)
		if err != nil {
			return err
		}
		existing, err := repo.List(ctx, session)
		if err != nil {
			return err
		}
		indexed := map[string]*models.Strip{}
		for _, strip := range existing {
			indexed[strip.Callsign] = strip
		}
		return s.SyncStrip(shared.WithSyncState(ctx, &shared.SyncState{Session: model, ExistingStrips: indexed}), session, "MASTER", event, airport)
	}
	event := es.Strip{Callsign: "SAME123", Origin: "ZZZZ", Destination: "EGLL", AssignedSquawk: "2401", Squawk: "2401", GroundState: es.GroundStateTaxi, Cleared: true, ClearedAltitude: 3000}
	for _, session := range []int32{live, training, playback} {
		require.NoError(t, observeFull(session, "ZZZZ", event))
	}
	ekchEvent := event
	ekchEvent.Origin = "EKCH"
	ekchEvent.Position = &es.Position{Lat: 55.6181, Lon: 12.656, Altitude: 17}
	require.NoError(t, observeFull(ekch, "EKCH", ekchEvent))
	readBay := func(session int32) string {
		strip, err := repo.GetByCallsign(ctx, session, event.Callsign)
		require.NoError(t, err)
		return strip.Bay
	}
	require.Equal(t, shared.BAY_TAXI_LWR, readBay(ekch), "production full sync retains solo-tower placement")
	for _, session := range []int32{live, training, playback} {
		require.Equal(t, "HOLD", readBay(session))
	}
	require.EqualValues(t, 1, binds.Load(), "three sessions share one startup-bound fictional implementation")
	for _, session := range []int32{live, training} {
		binding := &testutil.MockCommandBinding{SessionID: session, CIDValue: fmt.Sprint(session)}
		actionCtx, err := shared.WithHumanCommand(ctx, binding)
		require.NoError(t, err)
		require.NoError(t, s.MoveFrontendStrip(actionCtx, session, event.Callsign, "RUNWAY", binding.CID(), "ZZZZ", "TWR", false, false))
		require.Equal(t, []es.OutgoingMessage{es.GroundStateEvent{Callsign: event.Callsign, GroundState: es.GroundStateLineup}}, binding.Sent())
		require.NoError(t, observeFull(session, "ZZZZ", event))
		require.NoError(t, s.UpdateGroundState(ctx, session, event.Callsign, es.GroundStatePush, "ZZZZ"))
		require.NoError(t, s.UpdateClearedFlag(ctx, session, event.Callsign, false))
		require.NoError(t, s.UpdateAircraftPosition(ctx, session, event.Callsign, 0.01, 0.01, 100, "ZZZZ"))
		require.Equal(t, "RUNWAY", readBay(session), "full and incremental observations preserve this handler's manual placement")
	}
	// A blocked outbound effect in LIVE must not serialize the training session.
	blocked := &blockingWorkflowBinding{MockCommandBinding: &testutil.MockCommandBinding{SessionID: live, CIDValue: "LIVE-OWN"}, started: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(blocked.release) }) }
	defer unblock()
	liveCtx, err := shared.WithHumanCommand(ctx, blocked)
	require.NoError(t, err)
	liveResult := make(chan error, 1)
	go func() {
		liveResult <- s.ExecuteWorkflowAction(liveCtx, live, event.Callsign, blocked.CID(), "ZZZZ", "TWR", "paper.release", paperRelease{})
	}()
	select {
	case <-blocked.started:
	case <-time.After(3 * time.Second):
		t.Fatal("live custom action did not reach outbound path")
	}
	other := &testutil.MockCommandBinding{SessionID: training, CIDValue: "TRAINING-OWN"}
	otherCtx, err := shared.WithHumanCommand(ctx, other)
	require.NoError(t, err)
	otherResult := make(chan error, 1)
	go func() {
		otherResult <- s.ExecuteWorkflowAction(otherCtx, training, event.Callsign, other.CID(), "ZZZZ", "TWR", "paper.release", paperRelease{})
	}()
	select {
	case err := <-otherResult:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		unblock()
		t.Fatal("another session was blocked by LIVE outbound work")
	}
	unblock()
	require.NoError(t, <-liveResult)
	require.Equal(t, "DONE", readBay(live))
	require.Equal(t, "DONE", readBay(training))
	require.Equal(t, "HOLD", readBay(playback))
	require.Equal(t, shared.BAY_TAXI_LWR, readBay(ekch))
	require.Error(t, s.ExecuteWorkflowAction(ctx, ekch, event.Callsign, "EKCH-OWN", "EKCH", "TWR", "paper.release", paperRelease{}))
	require.Error(t, observeFull(unknown, "ZZZX", event))
	_, err = repo.GetByCallsign(ctx, unknown, event.Callsign)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	require.NoError(t, s.DeleteStrip(ctx, live, event.Callsign))
	_, err = repo.GetByCallsign(ctx, live, event.Callsign)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	require.Equal(t, "DONE", readBay(training))
	require.Equal(t, []int32{live}, publisher.disconnected)
	// Exercise production EKCH progression against the same real stores.
	require.NoError(t, s.UpdateGroundState(ctx, ekch, event.Callsign, es.GroundStateDepart, "EKCH"))
	require.Equal(t, shared.BAY_DEPART, readBay(ekch))
	require.NoError(t, observeFull(ekch, "EKCH", ekchEvent))
	require.NoError(t, s.UpdateGroundState(ctx, ekch, event.Callsign, es.GroundStateTaxi, "EKCH"))
	require.Equal(t, shared.BAY_DEPART, readBay(ekch), "stale full and incremental TAXI must not undo departure")
	require.NoError(t, s.UpdateAircraftPosition(ctx, ekch, event.Callsign, 55.6181, 12.656, 1500, "EKCH"))
	require.Equal(t, shared.BAY_AIRBORNE, readBay(ekch))
	require.NoError(t, s.DeleteStrip(ctx, ekch, event.Callsign))
	_, err = repo.GetByCallsign(ctx, ekch, event.Callsign)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	require.Equal(t, "DONE", readBay(training))
	paperData, err := os.ReadFile("../.github/specs/multi-airport-support/examples/zzzz.airport.yaml")
	require.NoError(t, err)
	production, err := config.NewAirportRegistry(config.ProductionAirportCatalog(), []config.AirportInput{{Source: "fictional", Reader: bytes.NewReader(paperData)}})
	require.ErrorContains(t, err, "no compiled descriptor")
	require.Nil(t, production)
}

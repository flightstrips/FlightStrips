package frontend

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/internal/vatsim"
	fe "FlightStrips/pkg/events/frontend"
	pkg "FlightStrips/pkg/models"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
	"time"
)

// A second coded handler uses the identical transport and persistence path. Its
// view, initial bay, supported annotation subset and recovery rule differ.
type paperProofSettings struct {
	InitialBay      string            `yaml:"initial_bay"`
	RunwayBay       string            `yaml:"runway_bay"`
	BayGroundStates map[string]string `yaml:"bay_ground_states"`
	GroundStateBays map[string]string `yaml:"ground_state_bays"`
}
type paperProofHandler struct{}

func (paperProofHandler) Observe(airportworkflow.Context, airportworkflow.Observation) (airportworkflow.Decision, error) {
	return airportworkflow.Decision{}, nil
}
func (paperProofHandler) Lifecycle(airportworkflow.Context, airportworkflow.Lifecycle) (airportworkflow.LifecycleDecision, error) {
	return airportworkflow.LifecycleDecision{Visibility: &airportworkflow.StripVisibility{Visible: true}}, nil
}
func (paperProofHandler) Coordinate(airportworkflow.Context, airportworkflow.Coordination) (airportworkflow.CoordinationDecision, error) {
	return airportworkflow.CoordinationDecision{}, nil
}

func TestSharedStripBoardTransportUsesSecondRegisteredWorkflow(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	data, err := os.ReadFile("../../../.github/specs/multi-airport-support/examples/zzzz.airport.yaml")
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), "strips: none", "strips: route", 1))
	catalog, err := config.NewAirportCatalog(config.HandlerDescriptor{ID: "example-paper/v1", View: "example-paper", Layouts: []string{"example-paper"}, Validate: func(*config.AirportDocument) error { return nil }, DecodeSettings: config.TypedAirportSettings[paperProofSettings]})
	require.NoError(t, err)
	defs, err := config.NewAirportRegistry(catalog, []config.AirportInput{{Source: "second-coded-test-only", Reader: bytes.NewReader(data)}})
	require.NoError(t, err)
	descriptor := airportworkflow.Descriptor{ID: "example-paper/v1", BoardState: func(*config.AirportDefinition) airportworkflow.BoardStatePolicy {
		return airportworkflow.BoardStatePolicy{Revisioned: true, RecoverySeconds: 5}
	}, Bind: func(d *config.AirportDefinition) (airportworkflow.Handler, []airportworkflow.ActionDescriptor, error) {
		initial := d.Document().Workflow.Settings.(paperProofSettings).InitialBay
		return paperProofHandler{}, []airportworkflow.ActionDescriptor{
			airportworkflow.Action("strip.create-local", func(_ airportworkflow.Context, r pkg.LocalStripCreation) (airportworkflow.Decision, error) {
				return airportworkflow.Decision{Admission: &airportworkflow.StripAdmission{Bay: initial, Annotations: pkg.StripAnnotations{}}}, nil
			}),
			airportworkflow.Action("strip.annotate", func(c airportworkflow.Context, r airportworkflow.StripAnnotation) (airportworkflow.Decision, error) {
				if r.Field != "review_required" {
					return airportworkflow.Decision{}, fmt.Errorf("only review mark supported")
				}
				a := c.Strip.Annotations
				a.ReviewRequired = r.Enabled
				return airportworkflow.Decision{Annotations: &a}, nil
			}),
			airportworkflow.Action("flight.find-connected", func(_ airportworkflow.Context, r airportworkflow.ConnectedFlightSearch) (airportworkflow.Decision, error) {
				return airportworkflow.Decision{ConnectedFlights: &airportworkflow.ConnectedFlightLookup{Airport: d.Airport(), Query: r.Query}}, nil
			}),
			airportworkflow.Action("strip.admit-connected", func(airportworkflow.Context, pkg.ConnectedFlightCandidate) (airportworkflow.Decision, error) {
				return airportworkflow.Decision{Admission: &airportworkflow.StripAdmission{Bay: initial}}, nil
			}),
			airportworkflow.Action("strip.restore", func(c airportworkflow.Context, _ airportworkflow.StripRestoration) (airportworkflow.Decision, error) {
				if c.Strip.RemovedAt == nil || !c.Now.Before(c.Strip.RemovedAt.Add(5*time.Second)) {
					return airportworkflow.Decision{}, fmt.Errorf("paper recovery expired")
				}
				return airportworkflow.Decision{Visibility: &airportworkflow.StripVisibility{Visible: true}}, nil
			}),
		}, nil
	}}
	workflows, err := airportworkflow.NewRegistry(defs, descriptor)
	require.NoError(t, err)
	bound, _ := workflows.Lookup("ZZZZ")
	require.Equal(t, "example-paper", bound.Definition().ViewID())
	require.NotContains(t, bound.ActionCatalog(), "strip.remove")
	require.NoError(t, q.InsertAirport(ctx, "ZZZZ"))
	session, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: "ZZZZ", Name: "LIVE"})
	require.NoError(t, err)
	repo := postgres.NewStripRepository(pool)
	pub := &testutil.MockFrontendHub{}
	service := services.NewStripService(repo, services.WithAirportWorkflows(workflows), services.WithSessionReader(postgres.NewSessionRepository(pool)), services.WithStripEventPublisher(pub), services.WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
	source := vatsim.NewSyntheticSource()
	now := time.Now().UTC()
	service.SetConnectedFlightSource(source, time.Minute, func() time.Time { return now })
	source.Upsert(vatsim.Flight{Callsign: "PAPER2", CID: "42", State: vatsim.FlightStateOnline, LogonTime: now, FlightPlan: vatsim.FlightPlan{Origin: "ZZZZ", Destination: "EHAM", Revision: 1}})
	now = source.Snapshot().Timestamp.Add(time.Second)
	binding := &testutil.MockCommandBinding{SessionID: session, CIDValue: "OWN"}
	own, err := shared.WithHumanCommand(ctx, binding)
	require.NoError(t, err)
	client := startQueuedTestClient(&Client{session: session, airport: "ZZZZ", position: "PAPER_TWR", user: validFrontendUser("OWN"), hub: &Hub{boardActions: bindStripBoardOperations(service)}})
	defer client.Close()
	message := func(v any) Message {
		data, err := json.Marshal(v)
		require.NoError(t, err)
		return Message{Message: data}
	}
	require.NoError(t, handleLocalStripCreation(own, client, message(pkg.LocalStripCreation{Definition: bound.Definition().Revision(), Callsign: "PAPER1", Origin: "ZZZZ", Destination: "ZZZZ", Notes: "initial"})))
	read := func() pkg.StripRevision {
		v, err := repo.GetByCallsign(ctx, session, "PAPER1")
		require.NoError(t, err)
		require.Equal(t, "HOLD", v.Bay)
		return pkg.StripRevision{StripID: v.ID, Version: v.Version, Definition: bound.Definition().Revision()}
	}
	original := read()
	owner := "OTHER_TWR"
	_, err = q.SetStripOwner(ctx, database.SetStripOwnerParams{Owner: &owner, Callsign: "PAPER1", Session: session, Version: original.Version})
	require.NoError(t, err)
	wrongOwner := fe.StripAnnotationEvent{Callsign: "PAPER1", StripRevision: read(), Field: "review_required", Enabled: true}
	require.ErrorContains(t, handleStripAnnotation(own, client, message(wrongOwner)), "owned by another controller")
	require.ErrorContains(t, handleStripRemoval(own, client, message(fe.StripVisibilityEvent{Callsign: "PAPER1", StripRevision: read()})), "owned by another controller")
	require.ErrorContains(t, handleStripRestoration(own, client, message(fe.StripVisibilityEvent{Callsign: "PAPER1", StripRevision: read()})), "owned by another controller")
	owner = "PAPER_TWR"
	_, err = q.SetStripOwner(ctx, database.SetStripOwnerParams{Owner: &owner, Callsign: "PAPER1", Session: session, Version: read().Version})
	require.NoError(t, err)
	original = read()
	edit := fe.StripAnnotationEvent{Callsign: "PAPER1", StripRevision: original, Field: "review_required", Enabled: true}
	require.NoError(t, handleStripAnnotation(own, client, message(edit)))
	require.Error(t, handleStripAnnotation(own, client, message(edit)), "same CAS applies under another view")
	current, err := repo.GetByCallsign(ctx, session, "PAPER1")
	require.NoError(t, err)
	require.True(t, current.BoardState.Annotations.ReviewRequired)
	edit.StripRevision = read()
	edit.Field = "deicing"
	edit.Enabled = true
	require.Error(t, handleStripAnnotation(own, client, message(edit)))
	require.ErrorContains(t, handleStripRemoval(own, client, message(fe.StripVisibilityEvent{Callsign: "PAPER1", StripRevision: read()})), "unsupported action")
	require.NoError(t, handleConnectedFlightSearch(own, client, message(fe.ConnectedFlightSearchRequest{RequestID: "query", Definition: bound.Definition().Revision()})))
	result := (<-client.send).(fe.ConnectedFlightSearchResult)
	require.Len(t, result.Flights, 1)
	require.NoError(t, handleConnectedFlightAdmission(own, client, message(fe.ConnectedFlightAdmissionRequest{Definition: bound.Definition().Revision(), Flight: result.Flights[0]})))
	admitted, err := repo.GetByCallsign(ctx, session, "PAPER2")
	require.NoError(t, err)
	require.Equal(t, "HOLD", admitted.Bay)
	removed := now.Add(-6 * time.Second)
	state := *current.BoardState
	state.RemovedAt = &removed
	state.RemovalReason = "manual"
	ok, err := repo.UpdateStripBoardState(ctx, session, current.ID, current.Version, &state)
	require.NoError(t, err)
	require.True(t, ok)
	require.ErrorContains(t, handleStripRestoration(own, client, message(fe.StripVisibilityEvent{Callsign: "PAPER1", StripRevision: read()})), "paper recovery expired")
	current, err = repo.GetByCallsign(ctx, session, "PAPER1")
	require.NoError(t, err)
	removed = now.Add(-time.Second)
	state.RemovedAt = &removed
	ok, err = repo.UpdateStripBoardState(ctx, session, current.ID, current.Version, &state)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, handleStripRestoration(own, client, message(fe.StripVisibilityEvent{Callsign: "PAPER1", StripRevision: read()})))
	restored, err := repo.GetByCallsign(ctx, session, "PAPER1")
	require.NoError(t, err)
	require.Nil(t, restored.BoardState.RemovedAt)
	require.Equal(t, original.StripID, restored.ID)
	// The common lifecycle executor honors Visible:true from this other handler.
	state = *restored.BoardState
	state.RemovedAt = &removed
	ok, err = repo.UpdateStripBoardState(ctx, session, restored.ID, restored.Version, &state)
	require.NoError(t, err)
	require.True(t, ok)
	handled, err := service.HandleRetainedAircraftDisconnect(ctx, session, "PAPER1")
	require.NoError(t, err)
	require.True(t, handled)
	visible, err := repo.GetByCallsign(ctx, session, "PAPER1")
	require.NoError(t, err)
	require.Nil(t, visible.BoardState.RemovedAt)
	require.GreaterOrEqual(t, len(pub.StripUpdates), 4)
	require.Empty(t, binding.Sent(), "paper data emits no ES effects")
}

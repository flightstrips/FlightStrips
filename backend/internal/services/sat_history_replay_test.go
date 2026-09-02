package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/standdiagnostics"
	"FlightStrips/internal/vatsim"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	satHistoryDirectoryEnvironment  = "SAT_VATSIM_HISTORY_DIR"
	satHistoryICAOEnvironment       = "SAT_ICAO_AIRCRAFT_JSON"
	satHistoryReportOnlyEnvironment = "SAT_HISTORY_REPORT_ONLY"
)

type satHistoryAssignmentState struct {
	stage         string
	stand         string
	source        string
	observedStand string
}

type satHistoryAssignmentEvent struct {
	at      time.Time
	state   satHistoryAssignmentState
	removed bool
}

type satHistoryNotifier struct{}

func (satHistoryNotifier) SendStripUpdate(int32, string) {}

type satHistoryProblemKey struct {
	callsign string
	outcome  string
	stage    string
}

type satHistoryProblemSummary struct {
	standdiagnostics.AllocationFailure
	count   int
	firstAt time.Time
	lastAt  time.Time
	stages  map[string]int
}

// TestSATHistoricalEKCHStability replays complete, saved VATSIM feed
// generations through the production reconciler and SAT lifecycle services.
// It is opt-in because the multi-gigabyte history archive is intentionally not
// committed to the repository. Point SAT_VATSIM_HISTORY_DIR at a directory of
// vatsim-data-*.json files to run it locally.
func TestSATHistoricalEKCHStability(t *testing.T) {
	historyDirectory := strings.TrimSpace(os.Getenv(satHistoryDirectoryEnvironment))
	if historyDirectory == "" {
		t.Skipf("set %s to a VATSIM snapshot directory", satHistoryDirectoryEnvironment)
	}

	files, err := filepath.Glob(filepath.Join(historyDirectory, "vatsim-data-*.json"))
	require.NoError(t, err)
	sort.Strings(files)
	require.NotEmpty(t, files, "no vatsim-data-*.json snapshots found in %s", historyDirectory)

	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	configDirectory := filepath.Join("config", "ekch")
	stands, err := sat.LoadStandCapabilityFile(filepath.Join(configDirectory, "GRpluginStands.txt"))
	require.NoError(t, err)
	policy, err := sat.LoadAirlineAssignmentFile(filepath.Join(configDirectory, "airline_assignment.json"), stands)
	require.NoError(t, err)
	aircraft, err := sat.LoadAircraftReferenceFile(filepath.Join(configDirectory, "GRpluginAircraftInfo.txt"))
	require.NoError(t, err)
	icaoAircraftPath := satHistoryICAOAircraftPath(t)
	engines, err := sat.LoadAircraftEngineReferenceFile(icaoAircraftPath, aircraft)
	require.NoError(t, err)
	borders := sat.NewAirportCountryRegistry()
	failureLog := standdiagnostics.NewAllocationFailureLog(100_000)

	assignments := postgres.NewStandAssignmentRepository(pool)
	strips := postgres.NewStripRepository(pool)
	sessions := postgres.NewSessionRepository(pool)
	clock := &fakeClock{now: time.Date(2026, 8, 3, 16, 30, 0, 0, time.UTC)}
	allocations, err := NewStandAllocationService(pool, strips, assignments, stands, policy,
		WithStandAllocationRandom(func() float64 { return 0 }),
		WithStandAllocationClock(clock.current),
		WithStandAllocationFailureLog(failureLog),
	)
	require.NoError(t, err)
	departures, err := NewDepartureLifecycleService(
		allocations, assignments, strips, sessions, stands, aircraft, engines, borders,
		WithDepartureLifecycleClock(clock.current),
	)
	require.NoError(t, err)
	arrivals, err := NewArrivalLifecycleService(
		allocations, assignments, strips, sessions, stands, aircraft, engines, borders,
		WithArrivalLifecycleClock(clock.current),
	)
	require.NoError(t, err)

	session := testdata.SeedTestSessionNamedWithSectors(t, queries, "SAT historical EKCH stability", nil)
	source := vatsim.NewSnapshotReplaySource()
	reconciler, err := vatsim.NewReconciler(vatsim.ReconcilerDependencies{
		Cache: source, Sessions: sessions, Strips: strips, Assignments: assignments,
		DepartureLifecycle: departures, ArrivalLifecycle: arrivals, Notifier: satHistoryNotifier{},
	}, time.Second,
		vatsim.WithAirportCoordinates(55.6181, 12.6560),
		vatsim.WithClock(clock.current),
	)
	require.NoError(t, err)

	confirmedBaselines := make(map[string]string)
	prior := make(map[string]satHistoryAssignmentState)
	assignmentHistory := make(map[string][]satHistoryAssignmentEvent)
	confirmedFlights := make(map[string]struct{})
	relevantFlightSamples := 0
	maximumActiveAssignments := 0
	idempotencyChecks := 0
	physicalConfirmedMoves := 0
	assignedFlights := make(map[string]struct{})
	reportOnly, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv(satHistoryReportOnlyEnvironment)))
	instabilityCount := 0

	// The historical run deliberately exercises many expected allocation
	// shortages. Keep verbose output focused on the aggregated problem report.
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	for _, path := range files {
		file, openErr := os.Open(path)
		require.NoError(t, openErr, "open VATSIM snapshot %s", path)
		info, statErr := file.Stat()
		require.NoError(t, statErr, "stat VATSIM snapshot %s", path)
		loadErr := source.LoadFiltered(file, info.ModTime(), func(_ time.Time, flight vatsim.Flight) bool {
			return strings.EqualFold(flight.FlightPlan.Origin, "EKCH") || strings.EqualFold(flight.FlightPlan.Destination, "EKCH")
		})
		require.NoError(t, file.Close())
		require.NoError(t, loadErr, "load VATSIM snapshot %s", path)

		snapshot := source.Snapshot()
		clock.set(snapshot.Timestamp)
		relevantFlightSamples += len(snapshot.Flights())
		require.NoError(t, reconciler.ReconcileSession(ctx, session), "first reconciliation of %s", filepath.Base(path))

		first := satHistoryAssignments(t, ctx, assignments, session)
		for callsign := range first {
			assignedFlights[callsign] = struct{}{}
		}
		if len(first) > maximumActiveAssignments {
			maximumActiveAssignments = len(first)
		}
		checkSATConfirmedHistory(t, filepath.Base(path), first, prior, confirmedBaselines, confirmedFlights, &physicalConfirmedMoves)

		// The same VATSIM generation is deliberately reconciled twice. No stage
		// or stand may change merely because the poll was repeated.
		require.NoError(t, reconciler.ReconcileSession(ctx, session), "repeated reconciliation of %s", filepath.Base(path))
		second := satHistoryAssignments(t, ctx, assignments, session)
		for callsign, before := range first {
			after, ok := second[callsign]
			if !ok {
				t.Fatalf("identical snapshot instability: callsign=%s snapshot=%s assignment was deleted (before stage=%s stand=%s source=%s)",
					callsign, filepath.Base(path), before.stage, before.stand, before.source)
			}
			if before.stage != after.stage || before.stand != after.stand {
				availabilityReason := satHistoryAvailabilityReason(t, ctx, source, strips, assignments, arrivals, allocations, session, callsign, before.stand, after.stage)
				message := fmt.Sprintf("identical snapshot instability: callsign=%s snapshot=%s before stage=%s stand=%s source=%s; after stage=%s stand=%s source=%s observed=%s; first occupants=%s; second occupants=%s; assignment changes=%s",
					callsign, filepath.Base(path), before.stage, before.stand, before.source,
					after.stage, after.stand, after.source, after.observedStand,
					satHistoryStandOccupants(first, before.stand, after.stand),
					satHistoryStandOccupants(second, before.stand, after.stand),
					satHistoryAssignmentChanges(first, second)+"; former stand availability="+availabilityReason)
				if !reportOnly {
					t.Fatal(message)
				}
				instabilityCount++
				if instabilityCount <= 20 {
					t.Log(message)
				}
			}
			idempotencyChecks++
		}
		recordSATAssignmentHistory(assignmentHistory, prior, second, snapshot.Timestamp)
		prior = second
	}

	require.Greater(t, relevantFlightSamples, 0, "history contained no EKCH traffic")
	require.Greater(t, maximumActiveAssignments, 0, "history produced no SAT assignments")
	require.Greater(t, idempotencyChecks, 0, "history produced no repeated assignment checks")
	require.NotEmpty(t, confirmedFlights, "history produced no CONFIRMED arrivals")
	problems := failureLog.List()
	require.NotEmpty(t, assignedFlights, "problem aircraft prevented every flight from receiving an assignment")
	t.Logf("replayed %d snapshots with ICAO reference %s, %d relevant flight samples, %d idempotency checks, max %d active assignments, %d confirmed arrivals, %d authoritative physical confirmed moves",
		len(files), icaoAircraftPath, relevantFlightSamples, idempotencyChecks, maximumActiveAssignments, len(confirmedFlights), physicalConfirmedMoves)
	if reportOnly {
		t.Logf("report-only replay observed %d identical-snapshot assignment changes", instabilityCount)
	}
	logSATProblemReport(t, problems, len(assignedFlights), assignmentHistory)
}

func recordSATAssignmentHistory(history map[string][]satHistoryAssignmentEvent, before, after map[string]satHistoryAssignmentState, at time.Time) {
	for callsign, current := range after {
		previous, existed := before[callsign]
		if existed && previous == current {
			continue
		}
		history[callsign] = append(history[callsign], satHistoryAssignmentEvent{at: at, state: current})
	}
	for callsign, previous := range before {
		if _, retained := after[callsign]; retained {
			continue
		}
		history[callsign] = append(history[callsign], satHistoryAssignmentEvent{at: at, state: previous, removed: true})
	}
}

func satHistoryAvailabilityReason(
	t *testing.T,
	ctx context.Context,
	source *vatsim.SnapshotReplaySource,
	strips interface {
		GetByCallsign(context.Context, int32, string) (*models.Strip, error)
	},
	assignments interface {
		GetAssignment(context.Context, int32, string) (*models.StandAssignment, error)
		ListAssignments(context.Context, int32) ([]*models.StandAssignment, error)
	},
	arrivals *ArrivalLifecycleService,
	allocations *StandAllocationService,
	session int32,
	callsign, stand, stage string,
) string {
	t.Helper()
	strip, err := strips.GetByCallsign(ctx, session, callsign)
	require.NoError(t, err)
	assignment, err := assignments.GetAssignment(ctx, session, callsign)
	require.NoError(t, err)
	flight, _ := source.Snapshot().FlightByCallsign(callsign)
	request := arrivals.buildRequest(session, strip, vatsim.ArrivalFlightInfo{
		Callsign: flight.Callsign, CID: flight.CID, Online: flight.Online(), Revision: flight.FlightPlan.Revision,
		Origin: flight.FlightPlan.Origin, Destination: flight.FlightPlan.Destination, AircraftType: flight.FlightPlan.AircraftShort,
	}, stage, assignment.ETA, assignment.ExpiresAt)
	available, err := allocations.AvailableStands(ctx, request)
	require.NoError(t, err)
	for _, candidate := range available {
		if strings.EqualFold(candidate.Stand, stand) {
			if candidate.Available {
				return "available"
			}
			current, listErr := assignments.ListAssignments(ctx, session)
			require.NoError(t, listErr)
			blockers := make([]string, 0)
			for _, other := range current {
				if other == nil || strings.EqualFold(other.Callsign, callsign) ||
					!strings.Contains(candidate.Reason, other.Stand) {
					continue
				}
				blockers = append(blockers, fmt.Sprintf("%s:%s:%s:%s:eta=%v:expires=%v",
					other.Callsign, other.Direction, other.Stage, other.Stand, other.ETA, other.ExpiresAt))
			}
			sort.Strings(blockers)
			return candidate.Reason + " [" + strings.Join(blockers, ",") + "]"
		}
	}
	return "not compatible"
}

func satHistoryStandOccupants(assignments map[string]satHistoryAssignmentState, stands ...string) string {
	wanted := make(map[string]struct{}, len(stands))
	for _, stand := range stands {
		wanted[strings.ToUpper(strings.TrimSpace(stand))] = struct{}{}
	}
	occupants := make([]string, 0)
	for callsign, assignment := range assignments {
		if _, ok := wanted[strings.ToUpper(strings.TrimSpace(assignment.stand))]; !ok {
			continue
		}
		occupants = append(occupants, callsign+":"+assignment.stage+":"+assignment.stand+":"+assignment.source)
	}
	sort.Strings(occupants)
	return strings.Join(occupants, ",")
}

func satHistoryAssignmentChanges(before, after map[string]satHistoryAssignmentState) string {
	changes := make([]string, 0)
	for callsign, previous := range before {
		current, ok := after[callsign]
		if !ok {
			changes = append(changes, callsign+":deleted:"+previous.stage+":"+previous.stand+":"+previous.source)
			continue
		}
		if previous != current {
			changes = append(changes, callsign+":"+previous.stage+":"+previous.stand+":"+previous.source+"->"+
				current.stage+":"+current.stand+":"+current.source+":"+current.observedStand)
		}
	}
	for callsign, current := range after {
		if _, ok := before[callsign]; !ok {
			changes = append(changes, callsign+":created:"+current.stage+":"+current.stand+":"+current.source+":"+current.observedStand)
		}
	}
	sort.Strings(changes)
	if len(changes) > 40 {
		changes = append(changes[:40], fmt.Sprintf("...%d more", len(changes)-40))
	}
	return strings.Join(changes, ",")
}

func satHistoryICAOAircraftPath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		strings.TrimSpace(os.Getenv(satHistoryICAOEnvironment)),
		strings.TrimSpace(os.Getenv("GRPLUGIN_ICAO_AIRCRAFT_JSON")),
		filepath.Join("config", "data", "ICAO_Aircraft.json"),
	}
	if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
		candidates = append(candidates, filepath.Join(appData, "EuroScope", "EKDK", "Plugins", "GRplugin", "ICAO_Aircraft.json"))
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Size() > 10_000 {
			return candidate
		}
	}
	require.FailNow(t, "full ICAO aircraft reference is required",
		"set %s to the installed GRPlugin ICAO_Aircraft.json; the small repository test fixture would produce false problem-aircraft results", satHistoryICAOEnvironment)
	return ""
}

func logSATProblemReport(t *testing.T, problems []standdiagnostics.AllocationFailure, assignedFlights int, assignmentHistory map[string][]satHistoryAssignmentEvent) {
	t.Helper()
	summariesByKey := make(map[satHistoryProblemKey]*satHistoryProblemSummary)
	problemCallsigns := make(map[string]struct{})
	problemAircraft := make(map[string]struct{})
	outcomes := make(map[string]int)
	for _, problem := range problems {
		key := satHistoryProblemKey{callsign: problem.Callsign, outcome: problem.Outcome, stage: problem.Stage}
		summary := summariesByKey[key]
		if summary == nil {
			summary = &satHistoryProblemSummary{AllocationFailure: problem, firstAt: problem.OccurredAt, lastAt: problem.OccurredAt, stages: make(map[string]int)}
			summariesByKey[key] = summary
		}
		summary.count++
		if problem.OccurredAt.Before(summary.firstAt) {
			summary.firstAt = problem.OccurredAt
		}
		if problem.OccurredAt.After(summary.lastAt) {
			summary.lastAt = problem.OccurredAt
			summary.AllocationFailure = problem
		}
		summary.stages[problem.Stage]++
		problemCallsigns[problem.Callsign] = struct{}{}
		if aircraft := strings.TrimSpace(problem.AircraftType); aircraft != "" {
			problemAircraft[aircraft] = struct{}{}
		}
		outcomes[problem.Outcome]++
	}

	summaries := make([]satHistoryProblemSummary, 0, len(summariesByKey))
	for _, summary := range summariesByKey {
		summaries = append(summaries, *summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].count != summaries[j].count {
			return summaries[i].count > summaries[j].count
		}
		if summaries[i].Outcome != summaries[j].Outcome {
			return summaries[i].Outcome < summaries[j].Outcome
		}
		return summaries[i].Callsign < summaries[j].Callsign
	})

	outcomeNames := make([]string, 0, len(outcomes))
	for outcome := range outcomes {
		outcomeNames = append(outcomeNames, outcome)
	}
	sort.Strings(outcomeNames)
	formattedOutcomes := make([]string, 0, len(outcomeNames))
	for _, outcome := range outcomeNames {
		formattedOutcomes = append(formattedOutcomes, outcome+"="+strconv.Itoa(outcomes[outcome]))
	}
	t.Logf("assignment problem report: %d attempts across %d callsigns and %d aircraft types; %d callsigns received assignments; outcomes: %s",
		len(problems), len(problemCallsigns), len(problemAircraft), assignedFlights, strings.Join(formattedOutcomes, ", "))
	for index, summary := range summaries {
		if index == 30 {
			t.Logf("... %d additional callsign/outcome combinations omitted", len(summaries)-index)
			break
		}
		stageNames := make([]string, 0, len(summary.stages))
		for stage, count := range summary.stages {
			stageNames = append(stageNames, stage+"="+strconv.Itoa(count))
		}
		sort.Strings(stageNames)
		t.Logf("problem callsign=%s aircraft=%s direction=%s stages=%s outcome=%s attempts=%d first=%s last=%s eta=%v eta_source=%s expires=%v tobt=%v tsat=%v departure_ready=%t reason=%q",
			summary.Callsign, summary.AircraftType, summary.Direction, strings.Join(stageNames, ","), summary.Outcome, summary.count,
			summary.firstAt.Format(time.RFC3339), summary.lastAt.Format(time.RFC3339), summary.ETA, summary.ETASource,
			summary.ExpiresAt, summary.DepartureTOBT, summary.DepartureTSAT, summary.DepartureReady, summary.Reason)
	}

	problemNames := make([]string, 0, len(problemCallsigns))
	for callsign := range problemCallsigns {
		problemNames = append(problemNames, callsign)
	}
	sort.Strings(problemNames)
	for _, callsign := range problemNames {
		events := assignmentHistory[callsign]
		parts := make([]string, 0, len(events))
		for index, event := range events {
			if index == 20 {
				parts = append(parts, fmt.Sprintf("...%d more", len(events)-index))
				break
			}
			description := fmt.Sprintf("%s=%s/%s/%s", event.at.Format(time.RFC3339), event.state.stage, event.state.stand, event.state.source)
			if event.state.observedStand != "" {
				description += "/observed=" + event.state.observedStand
			}
			if event.removed {
				description += "/removed"
			}
			parts = append(parts, description)
		}
		t.Logf("assignment timeline callsign=%s events=%s", callsign, strings.Join(parts, "; "))
	}
}

func satHistoryAssignments(t *testing.T, ctx context.Context, assignments interface {
	ListAssignments(context.Context, int32) ([]*models.StandAssignment, error)
}, session int32) map[string]satHistoryAssignmentState {
	t.Helper()
	current, err := assignments.ListAssignments(ctx, session)
	require.NoError(t, err)
	result := make(map[string]satHistoryAssignmentState, len(current))
	for _, assignment := range current {
		if assignment == nil {
			continue
		}
		observedStand := ""
		if assignment.ObservedStand != nil {
			observedStand = strings.TrimSpace(*assignment.ObservedStand)
		}
		result[assignment.Callsign] = satHistoryAssignmentState{
			stage: assignment.Stage, stand: assignment.Stand, source: assignment.Source, observedStand: observedStand,
		}
	}
	return result
}

func checkSATConfirmedHistory(
	t *testing.T,
	snapshot string,
	current, prior map[string]satHistoryAssignmentState,
	confirmedBaselines map[string]string,
	confirmedFlights map[string]struct{},
	physicalMoves *int,
) {
	t.Helper()
	// Stability applies to one uninterrupted CONFIRMED lifecycle. A callsign may
	// disappear and later be reused, or an assignment may be released and
	// recreated after leaving the stage; neither should inherit a stale stand
	// baseline from the earlier lifecycle.
	for callsign := range confirmedBaselines {
		assignment, active := current[callsign]
		if !active || assignment.stage != StageConfirmed {
			delete(confirmedBaselines, callsign)
		}
	}
	for callsign, assignment := range current {
		if assignment.stage != StageConfirmed {
			continue
		}
		confirmedFlights[callsign] = struct{}{}
		baseline, seen := confirmedBaselines[callsign]
		if !seen {
			confirmedBaselines[callsign] = assignment.stand
			continue
		}
		if assignment.stand == baseline {
			continue
		}

		previous, existed := prior[callsign]
		physicalTakeover := existed && previous.stand != assignment.stand &&
			assignment.observedStand != "" && strings.EqualFold(assignment.observedStand, assignment.stand)
		require.True(t, physicalTakeover,
			"CONFIRMED stand churn for %s in %s: baseline %s, previous %s, current %s, source %s, observed %s",
			callsign, snapshot, baseline, previous.stand, assignment.stand, assignment.source, assignment.observedStand)
		confirmedBaselines[callsign] = assignment.stand
		(*physicalMoves)++
	}
}

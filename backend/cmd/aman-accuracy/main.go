// Command aman-accuracy benchmarks operational AMAN TETAs against saved
// VATSIM feed history. It uses the same operational AMAN services as runtime.
package main

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/lifecycle"
	"FlightStrips/internal/aman/materializer"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/operational"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/navigation"
	"FlightStrips/internal/vatsim"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	input := flag.String("input", `C:\vatsim-data`, "VATSIM v3 history directory")
	dsn := flag.String("dsn", "postgresql://fs:fs_password@localhost:5432/fsdb?sslmode=disable", "navigation-cache database DSN")
	terminalPath := flag.String("terminal", navigation.DefaultTerminalGeometryPath, "EKCH terminal configuration")
	cacheOnly := flag.Bool("cache-only", false, "use already materialized navigation routes only; do not call AIRAC.NET")
	runwayGroup := flag.String("runway-group", "ARRIVAL-04L", "arrival runway group used for the replay")
	rate := flag.Uint("rate", 40, "arrival rate used for the replay")
	minimumUpdateInterval := flag.Duration("minimum-update-interval", 0, "minimum interval between AMAN recalculations (zero uses every snapshot)")
	exportPath := flag.String("export", "", "optional path for replay-map JSON")
	flag.Parse()
	if *rate == 0 || *rate > uint(^uint32(0)) {
		fail("rate must be in [1,%d]", ^uint32(0))
	}
	if *minimumUpdateInterval < 0 {
		fail("minimum-update-interval must be non-negative")
	}
	files, err := filepath.Glob(filepath.Join(*input, "*.json"))
	if err != nil || len(files) == 0 {
		fail("find history files: %v", err)
	}
	sort.Strings(files)
	pool, err := pgxpool.New(context.Background(), *dsn)
	if err != nil {
		fail("connect to navigation cache: %v", err)
	}
	defer pool.Close()
	navigationSource, err := navigation.Assemble(navigation.Config{Source: navigation.SourceAIRACNet, TerminalGeometryPath: *terminalPath}, pool)
	if err != nil {
		fail("assemble AIRAC.NET navigation source: %v", err)
	}
	if navigationSource == nil {
		fail("AIRAC.NET navigation source is disabled")
	}
	if !*cacheOnly {
		if err := navigationSource.Materializer.Refresh(context.Background(), materializer.Request{Airport: "EKCH"}); err != nil {
			fail("refresh AIRAC.NET navigation cache: %v", err)
		}
	}
	clock := time.Time{}
	repo := &memoryRepository{}
	var routes routeMaterializer = &memoizingMaterializer{source: navigationSource, results: map[string]materializedRoute{}}
	label := fmt.Sprintf("AIRAC.NET routes, surveillance wind fallback, %d/h", *rate)
	if *cacheOnly {
		routes = &memoizingMaterializer{source: cacheOnlyMaterializer{geometry: navigationSource.Geometry}, results: map[string]materializedRoute{}}
		label = fmt.Sprintf("AIRAC.NET route cache, surveillance wind fallback, %d/h", *rate)
	}
	if *minimumUpdateInterval > 0 {
		label += fmt.Sprintf(", AMAN recalculated every %s", *minimumUpdateInterval)
	}
	service, err := operational.New(operational.Dependencies{Repository: repo, Materializer: routes, Geometry: navigationSource.Geometry, Wind: unavailableWind{}, Publisher: discardPublisher{}, Terminal: navigationSource.Terminal, Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return clock }})
	if err != nil {
		fail("create operational AMAN service: %v", err)
	}
	goAroundDetector, err := lifecycle.NewGoAroundDetector(replayGoAroundConfig())
	if err != nil {
		fail("create go-around detector: %v", err)
	}
	goAroundCorridor, err := replayGoAroundCorridor(navigationSource.Terminal, aman.RunwayGroupID(*runwayGroup))
	if err != nil {
		fail("configure go-around detector: %v", err)
	}
	landings, err := discoverLandings(files)
	if err != nil {
		fail("discover historical landings: %v", err)
	}
	source := vatsim.NewSnapshotReplaySource()
	fullTrackSource := vatsim.NewSnapshotReplaySource()
	binder := &binder{ids: map[string]aman.FlightID{}}
	worker, err := vatsim.NewObservationWorker(vatsim.ObservationWorkerDependencies{Cache: source, Identities: binder, Sink: service, EnabledAirports: []string{"EKCH"}, StaleAfter: time.Minute, Now: func() time.Time { return clock }})
	if err != nil {
		fail("create AMAN observation worker: %v", err)
	}
	airborne := map[string]bool{}
	points := map[string][]forecastPoint{}
	tracks := map[string][]trackPoint{}
	replayTracks := map[string][]trackPoint{}
	goAroundStates := map[string]aman.GoAroundDetectionState{}
	goAroundDetections := map[string][]replayGoAroundDetection{}
	callsigns := map[string]string{}
	filedRoutes := map[string]string{}
	availability := map[string]availabilityDiagnostic{}
	publishableAtFiveMinutes := map[string]bool{}
	horizons := []time.Duration{5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 20 * time.Minute, 30 * time.Minute}
	samples := make(map[time.Duration][]accuracySample, len(horizons))
	replayConfigured := false
	lastUpdate := time.Time{}
	for _, path := range files {
		file, openErr := os.Open(path)
		if openErr != nil {
			fail("open %q: %v", path, openErr)
		}
		info, _ := file.Stat()
		if info != nil {
			clock = info.ModTime().UTC()
		}
		if err := fullTrackSource.LoadFiltered(file, clock, func(snapshotAt time.Time, flight vatsim.Flight) bool {
			landingAt, ok := landings[flight.CID]
			return ok && !snapshotAt.After(landingAt)
		}); err != nil {
			_ = file.Close()
			fail("load full track %q: %v", path, err)
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			_ = file.Close()
			fail("rewind %q: %v", path, err)
		}
		if err := source.LoadFiltered(file, clock, func(snapshotAt time.Time, flight vatsim.Flight) bool {
			landingAt, ok := landings[flight.CID]
			return ok && !snapshotAt.After(landingAt)
		}); err != nil {
			_ = file.Close()
			fail("load %q: %v", path, err)
		}
		_ = file.Close()
		if at := source.Snapshot().Timestamp; !at.IsZero() {
			clock = at
		}
		fullTrackAt := fullTrackSource.Snapshot().Timestamp
		if fullTrackAt.IsZero() {
			fullTrackAt = clock
		}
		for _, flight := range fullTrackSource.Snapshot().Flights() {
			if flight.FlightPlan.Destination != "EKCH" || !flight.Online() {
				continue
			}
			replayTracks[flight.CID] = appendTrackPoint(replayTracks[flight.CID], trackPoint{at: fullTrackAt, latitude: flight.Latitude, longitude: flight.Longitude, altitudeFeet: flight.Altitude, groundspeedKnots: float64(flight.Groundspeed)})
		}
		for _, flight := range source.Snapshot().Flights() {
			if flight.FlightPlan.Destination != "EKCH" || !flight.Online() {
				continue
			}
			tracks[flight.CID] = appendTrackPoint(tracks[flight.CID], trackPoint{at: clock, latitude: flight.Latitude, longitude: flight.Longitude, altitudeFeet: flight.Altitude, groundspeedKnots: float64(flight.Groundspeed)})
			callsigns[flight.CID] = flight.Callsign
			filedRoutes[flight.CID] = flight.FlightPlan.Route
			wasAirborne := airborne[flight.CID]
			isAirborne := flight.Altitude >= 1000 || flight.Groundspeed > 80
			landed := wasAirborne && flight.Altitude < 1000 && flight.Groundspeed <= 50 && distanceNM(flight.Latitude, flight.Longitude, 55.62, 12.65) <= 5
			if landed {
				for _, horizon := range horizons {
					if point, ok := latestForecastBefore(points[flight.CID], clock.Add(-horizon)); ok {
						samples[horizon] = append(samples[horizon], accuracySample{
							operationalError: point.operationalTETA.Sub(clock).Seconds(), rawError: point.rawTETA.Sub(clock).Seconds(),
							rawTETA:   point.rawTETA,
							slotError: slotError(point.slotTime, clock), scheduledDelay: slotDelay(point.slotTime, point.rawTETA),
							frozen: point.freezeReason == aman.FreezeSuperstable, freezeReason: point.freezeReason, age: clock.Add(-horizon).Sub(point.generatedAt),
							distanceNM: point.distanceNM, observedGroundspeed: point.observedGroundspeed, phase: point.phase, generatedAt: point.generatedAt, landedAt: clock,
							star: point.star, inTMA: point.inTMA, track: summarizeTrack(tracks[flight.CID], point.generatedAt, clock, point.route),
							holdingDuration: point.holdingDuration, postHoldingTransit: point.postHoldingTransit,
							holdingFixETA: point.holdingFixETA, holdingFixPassage: firstObservedFixPassage(tracks[flight.CID], point.generatedAt, clock, point.holdingFix), holdingFixLastPassage: lastObservedFixPassage(tracks[flight.CID], point.generatedAt, clock, point.holdingFix),
						})
					}
				}
				airborne[flight.CID] = false
			} else if isAirborne {
				airborne[flight.CID] = true
			}
		}
		if !lastUpdate.IsZero() && clock.Sub(lastUpdate) < *minimumUpdateInterval {
			continue
		}
		if err := worker.Publish(context.Background()); err != nil {
			fail("publish %q: %v", path, err)
		}
		lastUpdate = clock
		service.Reconcile(context.Background())
		if !replayConfigured {
			if err := configureReplayRunway(repo, aman.RunwayGroupID(*runwayGroup), uint32(*rate), clock); err != nil {
				fail("configure replay runway: %v", err)
			}
			// Recalculate the first snapshot after applying the requested runway.
			// Without this, the initial state uses the terminal configuration's
			// first group (04L) before the replay rate is applied.
			service.Reconcile(context.Background())
			replayConfigured = true
		}
		if err := updateReplayGoAroundDetections(goAroundDetector, goAroundCorridor, aman.RunwayGroupID(*runwayGroup), repo.state, clock, goAroundStates, goAroundDetections); err != nil {
			fail("replay go-around detector: %v", err)
		}
		for _, flight := range repo.state.Flights {
			if landingAt, candidate := landings[flight.VATSIMCID]; candidate && !clock.After(landingAt.Add(-5*time.Minute)) {
				availability[flight.VATSIMCID] = availabilityDiagnosticFromFlight(flight)
				publishableAtFiveMinutes[flight.VATSIMCID] = publishableAtFiveMinutes[flight.VATSIMCID] || (flight.Prediction != nil && flight.Prediction.Publishable)
			}
			if flight.Prediction != nil && flight.Prediction.Publishable && flight.State != aman.StateLanded && flight.State != aman.StateRemoved {
				points[flight.VATSIMCID] = appendForecast(points[flight.VATSIMCID], forecastPointFromFlight(flight))
			}
		}
	}
	if len(samples[5*time.Minute]) == 0 {
		fail("no landing candidates had a publishable AMAN prediction from the current navigation cache")
	}
	printLandingRate(landings)
	printUnavailableCandidates(landings, publishableAtFiveMinutes, availability)
	printMetrics(label, len(landings), samples, horizons)
	if *exportPath != "" {
		if err := writeReplayExport(*exportPath, *runwayGroup, goAroundCorridor, operational.DefaultGoAroundDelay, landings, callsigns, filedRoutes, points, replayTracks, goAroundDetections); err != nil {
			fail("write replay export: %v", err)
		}
	}
}

func cloneSlotTime(slot *aman.Slot) *time.Time {
	if slot == nil {
		return nil
	}
	value := slot.Time
	return &value
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func slotDelay(slot *time.Time, raw time.Time) time.Duration {
	if slot == nil || !slot.After(raw) {
		return 0
	}
	return slot.Sub(raw)
}

func slotError(slot *time.Time, landedAt time.Time) *float64 {
	if slot == nil {
		return nil
	}
	value := slot.Sub(landedAt).Seconds()
	return &value
}

func configureReplayRunway(repo *memoryRepository, group aman.RunwayGroupID, rate uint32, effectiveAt time.Time) error {
	if !repo.has {
		return fmt.Errorf("airport state is not initialized")
	}
	found := false
	for i := range repo.state.RunwayGroups {
		policy := &repo.state.RunwayGroups[i]
		policy.Selected = policy.ID == group
		if policy.ID != group {
			// The replay has one known active arrival runway.  Removing the
			// bootstrap selection avoids an equal-time tie selecting 04L.
			policy.SelectionSchedule = nil
			continue
		}
		found = true
		policy.ActiveRatePerHour = rate
		policy.RateEffectiveAt = &effectiveAt
		policy.RateSchedule = []aman.RunwayGroupRatePoint{{EffectiveAt: effectiveAt, ArrivalsPerHour: rate}}
		policy.SelectionSchedule = []aman.RunwayGroupSelectionPoint{{EffectiveAt: effectiveAt}}
	}
	if !found {
		return fmt.Errorf("runway group %q is not configured", group)
	}
	for i := range repo.state.Flights {
		flight := &repo.state.Flights[i]
		selected := group
		flight.SelectedRunwayGroup = &selected
		flight.SelectedFeeder = nil
		flight.SelectedHolding = nil
		flight.HoldingStack = nil
		flight.ActiveRouteKey = nil
		flight.ActiveRouteDatasetID = nil
		flight.RouteProgress = nil
		flight.Prediction = nil
		flight.Slot = nil
		flight.Order = nil
		flight.ManualOrder = nil
		// The just-created bootstrap prediction was for the wrong runway, so
		// its freeze must not survive into the requested-runway replay.
		flight.FreezeReason = aman.FreezeNone
		flight.FrozenAt = nil
		flight.FrozenOperationalTETA = nil
		flight.FrozenSlot = nil
	}
	return nil
}

type forecastPoint struct {
	generatedAt         time.Time
	rawTETA             time.Time
	operationalTETA     time.Time
	slotTime            *time.Time
	freezeReason        aman.FreezeReason
	distanceNM          float64
	observedGroundspeed float64
	phase               string
	star                string
	inTMA               bool
	route               []routeSegment
	modelSegment        *modelSegment
	holdingDuration     time.Duration
	postHoldingTransit  time.Duration
	holdingFixETA       *time.Time
	holdingFix          *holdingFixPoint
}

func forecastPointFromFlight(flight aman.AMANFlight) forecastPoint {
	prediction := flight.Prediction
	point := forecastPoint{generatedAt: prediction.GeneratedAt, rawTETA: prediction.RawTETA, operationalTETA: prediction.OperationalTETA, slotTime: cloneSlotTime(flight.Slot), freezeReason: flight.FreezeReason, inTMA: flight.FreezeReason == aman.FreezeTMA}
	if prediction.DistanceToGoNM != nil {
		point.distanceNM = *prediction.DistanceToGoNM
	}
	if flight.LatestObservation != nil && flight.LatestObservation.Surveillance != nil && flight.LatestObservation.Surveillance.GroundspeedKnots != nil {
		point.observedGroundspeed = *flight.LatestObservation.Surveillance.GroundspeedKnots
	}
	if prediction.Calculation != nil && len(prediction.Calculation.Segments) > 0 {
		point.phase = prediction.Calculation.Segments[0].PhaseID
		point.modelSegment = modelSegmentFromCalculation(prediction.Calculation.Segments[0])
	}
	if prediction.Calculation != nil {
		point.route = routeFromCalculation(prediction.Calculation.Legs)
	}
	point.holdingFixETA = cloneTime(prediction.HoldingFixETA)
	point.holdingFix = holdingFixFromCalculation(prediction)
	if flight.SelectedFeeder != nil {
		point.star = *flight.SelectedFeeder
	}
	if prediction.HoldingPlan != nil {
		point.holdingDuration = prediction.HoldingPlan.ExpectedHoldingDuration
		point.postHoldingTransit = prediction.HoldingPlan.PostHoldingTransit
	}
	return point
}

// holdingFixFromCalculation derives the coordinate of the fix represented by
// HoldingFixETA. HoldingFixETA is accumulated over the same remaining-leg
// durations retained in the inspectable prediction calculation.
func holdingFixFromCalculation(prediction *aman.Prediction) *holdingFixPoint {
	if prediction == nil || prediction.HoldingFixETA == nil || prediction.Calculation == nil {
		return nil
	}
	remaining := prediction.HoldingFixETA.Sub(prediction.GeneratedAt)
	if remaining <= 0 {
		return nil
	}
	elapsed := time.Duration(0)
	for _, leg := range prediction.Calculation.Legs {
		elapsed += leg.Duration
		if elapsed >= remaining {
			return &holdingFixPoint{id: leg.To, latitude: leg.EndLatitude, longitude: leg.EndLongitude}
		}
	}
	return nil
}

// firstObservedFixPassage finds the first replay observation at the holding
// fix after a prediction is issued. The saved VATSIM feed is sampled roughly
// every 15 seconds, so this is a passage observation rather than an exact
// geometrically interpolated crossing.
func firstObservedFixPassage(points []trackPoint, start, end time.Time, fix *holdingFixPoint) *time.Time {
	if fix == nil || !start.Before(end) {
		return nil
	}
	const passageRadiusNM = 1.5
	for _, point := range points {
		if point.at.Before(start) || point.at.After(end) || distanceNM(point.latitude, point.longitude, fix.latitude, fix.longitude) > passageRadiusNM {
			continue
		}
		at := point.at
		return &at
	}
	return nil
}

func lastObservedFixPassage(points []trackPoint, start, end time.Time, fix *holdingFixPoint) *time.Time {
	if fix == nil || !start.Before(end) {
		return nil
	}
	const passageRadiusNM = 1.5
	var result *time.Time
	for _, point := range points {
		if point.at.Before(start) || point.at.After(end) || distanceNM(point.latitude, point.longitude, fix.latitude, fix.longitude) > passageRadiusNM {
			continue
		}
		at := point.at
		result = &at
	}
	return result
}

type accuracySample struct {
	operationalError      float64
	rawError              float64
	rawTETA               time.Time
	slotError             *float64
	scheduledDelay        time.Duration
	frozen                bool
	freezeReason          aman.FreezeReason
	age                   time.Duration
	distanceNM            float64
	observedGroundspeed   float64
	phase                 string
	star                  string
	inTMA                 bool
	track                 trackSummary
	generatedAt           time.Time
	landedAt              time.Time
	holdingDuration       time.Duration
	postHoldingTransit    time.Duration
	holdingFixETA         *time.Time
	holdingFixPassage     *time.Time
	holdingFixLastPassage *time.Time
}

type holdingFixPoint struct {
	id                  string
	latitude, longitude float64
}

// availabilityDiagnostic captures the most recent AMAN status before the
// five-minute measurement horizon. It makes a replay's missing predictions
// actionable without letting the later landed state obscure their cause.
type availabilityDiagnostic struct {
	callsign string
	state    aman.FlightState
	status   aman.DataStatus
	reason   string
}

func availabilityDiagnosticFromFlight(flight aman.AMANFlight) availabilityDiagnostic {
	result := availabilityDiagnostic{callsign: flight.CurrentCallsign, state: flight.State, status: flight.DataStatus, reason: "no_prediction"}
	if flight.Prediction != nil && flight.Prediction.DegradationReason != nil {
		result.reason = *flight.Prediction.DegradationReason
	}
	return result
}

type routeSegment struct {
	id                                                    string
	fromLatitude, fromLongitude, toLatitude, toLongitude  float64
	distanceNM, durationSeconds, expectedGroundspeedKnots float64
}

type modelSegment struct {
	phaseID, phaseName                                                                               string
	preTOD                                                                                           bool
	startAltitudeFeet, endAltitudeFeet, altitudeFeet                                                 float64
	indicatedAirspeedKnots, tailwindKnots                                                            *float64
	noWindGroundspeedKnots, groundspeedKnots, currentNoWindGroundspeedKnots, currentGroundspeedKnots float64
	distanceNM, durationSec                                                                          float64
}

type trackPoint struct {
	at                  time.Time
	latitude, longitude float64
	altitudeFeet        int
	groundspeedKnots    float64
}

type replayGoAroundDetection struct {
	at                         time.Time
	latitude, longitude        float64
	reason                     lifecycle.GoAroundReason
	supportingObservationTimes []time.Time
}

type replayMapExport struct {
	RunwayGroup          string            `json:"runway_group"`
	GoAroundDelaySeconds int64             `json:"go_around_delay_seconds"`
	Flights              []replayMapFlight `json:"flights"`
}

type replayMapFlight struct {
	CID        string              `json:"cid"`
	Callsign   string              `json:"callsign"`
	FiledRoute string              `json:"filed_route"`
	STAR       string              `json:"star"`
	LandedAt   time.Time           `json:"landed_at"`
	Events     []replayMapEvent    `json:"events"`
	Snapshots  []replayMapSnapshot `json:"snapshots"`
	Track      []replayMapPoint    `json:"track"`
}

type replayMapEvent struct {
	Type                       string      `json:"type"`
	At                         time.Time   `json:"at"`
	Latitude                   float64     `json:"latitude"`
	Longitude                  float64     `json:"longitude"`
	Sequence                   int         `json:"sequence,omitempty"`
	Reason                     string      `json:"reason,omitempty"`
	SupportingObservationTimes []time.Time `json:"supporting_observation_times,omitempty"`
}

type replayMapSnapshot struct {
	At              time.Time         `json:"at"`
	RawTETA         time.Time         `json:"raw_teta"`
	OperationalTETA time.Time         `json:"operational_teta"`
	FreezeReason    aman.FreezeReason `json:"freeze_reason"`
	STAR            string            `json:"star"`
	Route           []replayMapLeg    `json:"route"`
	ModelSegment    *replayMapSegment `json:"model_segment,omitempty"`
}

type replayMapSegment struct {
	PhaseID                       string   `json:"phase_id"`
	PhaseName                     string   `json:"phase_name"`
	PreTOD                        bool     `json:"pre_tod"`
	StartAltitudeFeet             float64  `json:"start_altitude_feet"`
	EndAltitudeFeet               float64  `json:"end_altitude_feet"`
	AltitudeFeet                  float64  `json:"altitude_feet"`
	IndicatedAirspeedKnots        *float64 `json:"indicated_airspeed_knots,omitempty"`
	NoWindGroundspeedKnots        float64  `json:"no_wind_groundspeed_knots"`
	GroundspeedKnots              float64  `json:"groundspeed_knots"`
	CurrentNoWindGroundspeedKnots float64  `json:"current_no_wind_groundspeed_knots"`
	CurrentGroundspeedKnots       float64  `json:"current_groundspeed_knots"`
	TailwindKnots                 *float64 `json:"tailwind_knots,omitempty"`
	DistanceNM                    float64  `json:"distance_nm"`
	DurationSeconds               float64  `json:"duration_seconds"`
}

type replayMapLeg struct {
	ID                       string     `json:"id"`
	From                     [2]float64 `json:"from"`
	To                       [2]float64 `json:"to"`
	DistanceNM               float64    `json:"distance_nm"`
	DurationSeconds          float64    `json:"duration_seconds"`
	ExpectedGroundspeedKnots float64    `json:"expected_groundspeed_knots"`
}

type replayMapPoint struct {
	At               time.Time `json:"at"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	AltitudeFeet     int       `json:"altitude_feet"`
	GroundspeedKnots float64   `json:"groundspeed_knots"`
}

const replayGoAroundPolicyVersion = "aman-go-around-v1"

func replayGoAroundConfig() lifecycle.GoAroundConfig {
	return lifecycle.GoAroundConfig{
		EvidenceLimit: 16, ArmSamples: 2, ConfirmSamples: 2, ArmBelowAltitudeFeet: 2000,
		InboundToleranceDegrees: 20, MinimumClimbFeet: 100, TrackAwayDegrees: 60,
		RunwayExitAfterThresholdNM: 0.1, LandingAltitudeToleranceFeet: 200, MinimumAirborneGroundspeedKnots: 80,
	}
}

func replayGoAroundCorridor(config terminal.Configuration, selected aman.RunwayGroupID) (lifecycle.FinalPathCorridor, error) {
	for _, group := range config.RunwayGroups {
		if group.ID != selected {
			continue
		}
		if len(group.FinalApproaches) != 1 {
			return lifecycle.FinalPathCorridor{}, fmt.Errorf("runway group %q must have exactly one final approach", selected)
		}
		final := group.FinalApproaches[0]
		elevation := 0
		if final.Threshold.ElevationFt != nil {
			elevation = *final.Threshold.ElevationFt
		}
		return lifecycle.FinalPathCorridor{
			ID:                     string(config.Airport) + "-" + string(final.Runway),
			ThresholdLatitude:      final.Threshold.Position.LatitudeDeg,
			ThresholdLongitude:     final.Threshold.Position.LongitudeDeg,
			ThresholdElevationFeet: elevation,
			InboundCourseDegrees:   final.CourseTrueDeg,
			LengthNM:               6,
			HalfWidthNM:            0.75,
		}, nil
	}
	return lifecycle.FinalPathCorridor{}, fmt.Errorf("runway group %q is not configured", selected)
}

func updateReplayGoAroundDetections(
	detector lifecycle.GoAroundDetector,
	corridor lifecycle.FinalPathCorridor,
	selectedGroup aman.RunwayGroupID,
	airport aman.AirportState,
	now time.Time,
	states map[string]aman.GoAroundDetectionState,
	detections map[string][]replayGoAroundDetection,
) error {
	for _, flight := range airport.Flights {
		if flight.LatestObservation == nil || flight.LatestObservation.Surveillance == nil ||
			flight.SelectedRunwayGroup == nil || *flight.SelectedRunwayGroup != selectedGroup {
			continue
		}
		detectionAt := now
		if observedAt := flight.LatestObservation.Surveillance.ObservedAt; observedAt != nil && observedAt.After(detectionAt) {
			detectionAt = *observedAt
		}
		result, err := detector.Detect(lifecycle.GoAroundInput{
			FlightID: flight.ID, Observation: *flight.LatestObservation, Corridor: corridor,
			Previous: states[flight.VATSIMCID], PolicyVersion: replayGoAroundPolicyVersion,
			Now: detectionAt, InScope: true, LandingConfirmed: flight.State == aman.StateLanded,
		})
		if err != nil {
			return fmt.Errorf("%s: %w", flight.CurrentCallsign, err)
		}
		states[flight.VATSIMCID] = result.State
		if result.Confirmed == nil {
			continue
		}
		surveillance := flight.LatestObservation.Surveillance
		detections[flight.VATSIMCID] = append(detections[flight.VATSIMCID], replayGoAroundDetection{
			at: result.Confirmed.ConfirmedAt, latitude: surveillance.LatitudeDegrees, longitude: surveillance.LongitudeDegrees,
			reason: result.Confirmed.Reason, supportingObservationTimes: append([]time.Time(nil), result.Confirmed.SupportingObservationTimes...),
		})
	}
	return nil
}

func replayEvents(track []trackPoint, corridor lifecycle.FinalPathCorridor, detections []replayGoAroundDetection, landedAt time.Time) []replayMapEvent {
	events := thresholdCrossingEvents(track, corridor)
	for _, detection := range detections {
		events = append(events, replayMapEvent{
			Type: "go_around_detected", At: detection.at, Latitude: detection.latitude, Longitude: detection.longitude,
			Reason: string(detection.reason), SupportingObservationTimes: append([]time.Time(nil), detection.supportingObservationTimes...),
		})
	}
	if point, ok := latestTrackPointAtOrBefore(track, landedAt); ok {
		events = append(events, replayMapEvent{Type: "landing_proxy", At: landedAt, Latitude: point.latitude, Longitude: point.longitude})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].At.Before(events[j].At) })
	return events
}

func thresholdCrossingEvents(track []trackPoint, corridor lifecycle.FinalPathCorridor) []replayMapEvent {
	result := []replayMapEvent{}
	for index := 1; index < len(track); index++ {
		previous, current := track[index-1], track[index]
		previousAlong, previousLateral := corridorRelativeNM(corridor, previous.latitude, previous.longitude)
		currentAlong, currentLateral := corridorRelativeNM(corridor, current.latitude, current.longitude)
		if previousAlong >= 0 || currentAlong < 0 ||
			math.Abs(previousLateral) > corridor.HalfWidthNM || math.Abs(currentLateral) > corridor.HalfWidthNM ||
			current.altitudeFeet > replayGoAroundConfig().ArmBelowAltitudeFeet || current.groundspeedKnots < replayGoAroundConfig().MinimumAirborneGroundspeedKnots {
			continue
		}
		denominator := currentAlong - previousAlong
		if denominator <= 0 {
			continue
		}
		ratio := math.Max(0, math.Min(1, -previousAlong/denominator))
		at := previous.at.Add(time.Duration(float64(current.at.Sub(previous.at)) * ratio))
		result = append(result, replayMapEvent{
			Type: "threshold_crossing", At: at,
			Latitude:  previous.latitude + (current.latitude-previous.latitude)*ratio,
			Longitude: previous.longitude + (current.longitude-previous.longitude)*ratio,
			Sequence:  len(result) + 1,
		})
	}
	return result
}

func corridorRelativeNM(corridor lifecycle.FinalPathCorridor, latitude, longitude float64) (float64, float64) {
	north := (latitude - corridor.ThresholdLatitude) * 60
	east := (longitude - corridor.ThresholdLongitude) * 60 * math.Cos(corridor.ThresholdLatitude*math.Pi/180)
	course := corridor.InboundCourseDegrees * math.Pi / 180
	inboundEast, inboundNorth := math.Sin(course), math.Cos(course)
	return east*inboundEast + north*inboundNorth, east*inboundNorth - north*inboundEast
}

func latestTrackPointAtOrBefore(track []trackPoint, at time.Time) (trackPoint, bool) {
	for index := len(track) - 1; index >= 0; index-- {
		if !track[index].at.After(at) {
			return track[index], true
		}
	}
	return trackPoint{}, false
}

func writeReplayExport(path, runwayGroup string, corridor lifecycle.FinalPathCorridor, goAroundDelay time.Duration, landings map[string]time.Time, callsigns, filedRoutes map[string]string, points map[string][]forecastPoint, tracks map[string][]trackPoint, detections map[string][]replayGoAroundDetection) error {
	result := replayMapExport{
		RunwayGroup: runwayGroup, GoAroundDelaySeconds: int64(goAroundDelay / time.Second),
		Flights: make([]replayMapFlight, 0, len(landings)),
	}
	for cid, landedAt := range landings {
		if len(points[cid]) == 0 || len(tracks[cid]) < 2 {
			continue
		}
		flight := replayMapFlight{
			CID: cid, Callsign: callsigns[cid], FiledRoute: filedRoutes[cid], STAR: replaySTAR(points[cid]), LandedAt: landedAt,
			Events:    replayEvents(tracks[cid], corridor, detections[cid], landedAt),
			Snapshots: make([]replayMapSnapshot, 0, len(points[cid])), Track: make([]replayMapPoint, 0, len(tracks[cid])),
		}
		for _, point := range points[cid] {
			snapshot := replayMapSnapshot{At: point.generatedAt, RawTETA: point.rawTETA, OperationalTETA: point.operationalTETA, FreezeReason: point.freezeReason, STAR: point.star, Route: make([]replayMapLeg, 0, len(point.route))}
			if point.modelSegment != nil {
				snapshot.ModelSegment = &replayMapSegment{
					PhaseID: point.modelSegment.phaseID, PhaseName: point.modelSegment.phaseName, PreTOD: point.modelSegment.preTOD,
					StartAltitudeFeet: point.modelSegment.startAltitudeFeet, EndAltitudeFeet: point.modelSegment.endAltitudeFeet, AltitudeFeet: point.modelSegment.altitudeFeet,
					IndicatedAirspeedKnots: cloneFloat(point.modelSegment.indicatedAirspeedKnots),
					NoWindGroundspeedKnots: point.modelSegment.noWindGroundspeedKnots, GroundspeedKnots: point.modelSegment.groundspeedKnots,
					CurrentNoWindGroundspeedKnots: point.modelSegment.currentNoWindGroundspeedKnots, CurrentGroundspeedKnots: point.modelSegment.currentGroundspeedKnots,
					TailwindKnots: cloneFloat(point.modelSegment.tailwindKnots), DistanceNM: point.modelSegment.distanceNM, DurationSeconds: point.modelSegment.durationSec,
				}
			}
			for _, leg := range point.route {
				snapshot.Route = append(snapshot.Route, replayMapLeg{
					ID: leg.id, From: [2]float64{leg.fromLatitude, leg.fromLongitude}, To: [2]float64{leg.toLatitude, leg.toLongitude},
					DistanceNM: leg.distanceNM, DurationSeconds: leg.durationSeconds, ExpectedGroundspeedKnots: leg.expectedGroundspeedKnots,
				})
			}
			flight.Snapshots = append(flight.Snapshots, snapshot)
		}
		for _, point := range tracks[cid] {
			flight.Track = append(flight.Track, replayMapPoint{
				At: point.at, Latitude: point.latitude, Longitude: point.longitude,
				AltitudeFeet: point.altitudeFeet, GroundspeedKnots: point.groundspeedKnots,
			})
		}
		result.Flights = append(result.Flights, flight)
	}
	sort.Slice(result.Flights, func(i, j int) bool { return result.Flights[i].LandedAt.Before(result.Flights[j].LandedAt) })
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func replaySTAR(points []forecastPoint) string {
	for index := len(points) - 1; index >= 0; index-- {
		if points[index].star != "" {
			return points[index].star
		}
	}
	return ""
}

type trackSummary struct {
	complete               bool
	flownDistanceNM        float64
	meanCrossTrackNM       float64
	maxCrossTrackNM        float64
	offRouteSamples, total int
}

func routeFromCalculation(legs []aman.PredictionLeg) []routeSegment {
	result := make([]routeSegment, 0, len(legs))
	for _, leg := range legs {
		durationSeconds := leg.Duration.Seconds()
		expectedGroundspeedKnots := 0.0
		if durationSeconds > 0 {
			expectedGroundspeedKnots = leg.DistanceNM / leg.Duration.Hours()
		}
		result = append(result, routeSegment{
			id: leg.ID, fromLatitude: leg.StartLatitude, fromLongitude: leg.StartLongitude, toLatitude: leg.EndLatitude, toLongitude: leg.EndLongitude,
			distanceNM: leg.DistanceNM, durationSeconds: durationSeconds, expectedGroundspeedKnots: expectedGroundspeedKnots,
		})
	}
	return result
}

func modelSegmentFromCalculation(segment aman.PredictionSegment) *modelSegment {
	currentNoWindGroundspeed, currentGroundspeed := segment.NoWindGroundspeedKnots, segment.GroundspeedKnots
	if segment.IndicatedAirspeedKnots != nil {
		currentNoWindGroundspeed = *segment.IndicatedAirspeedKnots / math.Sqrt(replayDensityRatio(segment.StartAltitudeFeet))
		currentGroundspeed = currentNoWindGroundspeed
		if segment.TailwindKnots != nil {
			currentGroundspeed += *segment.TailwindKnots
		}
	}
	return &modelSegment{
		phaseID: segment.PhaseID, phaseName: segment.PhaseName, preTOD: segment.PreTOD,
		startAltitudeFeet: segment.StartAltitudeFeet, endAltitudeFeet: segment.EndAltitudeFeet, altitudeFeet: segment.AltitudeFeet,
		indicatedAirspeedKnots: cloneFloat(segment.IndicatedAirspeedKnots),
		noWindGroundspeedKnots: segment.NoWindGroundspeedKnots, groundspeedKnots: segment.GroundspeedKnots,
		currentNoWindGroundspeedKnots: currentNoWindGroundspeed, currentGroundspeedKnots: currentGroundspeed,
		tailwindKnots: cloneFloat(segment.TailwindKnots), distanceNM: segment.DistanceNM, durationSec: segment.Duration.Seconds(),
	}
}

func replayDensityRatio(altitudeFeet float64) float64 {
	altitudeFeet = max(0, altitudeFeet)
	if altitudeFeet <= 36089 {
		return math.Pow(1-6.87535e-6*altitudeFeet, 4.2561)
	}
	return 0.2971 * math.Exp(-(altitudeFeet-36089)/20806.7)
}

func appendTrackPoint(points []trackPoint, next trackPoint) []trackPoint {
	if len(points) > 0 && points[len(points)-1].at.Equal(next.at) {
		points[len(points)-1] = next
		return points
	}
	return append(points, next)
}

func summarizeTrack(points []trackPoint, start, end time.Time, route []routeSegment) trackSummary {
	if len(route) == 0 || !start.Before(end) {
		return trackSummary{}
	}
	selected := make([]trackPoint, 0, len(points))
	for _, point := range points {
		if !point.at.Before(start) && !point.at.After(end) {
			selected = append(selected, point)
		}
	}
	if len(selected) < 2 || selected[0].at.Sub(start) > 30*time.Second || end.Sub(selected[len(selected)-1].at) > 30*time.Second {
		return trackSummary{}
	}
	result := trackSummary{complete: true}
	for index, point := range selected {
		cross := distanceToRouteNM(point, route)
		result.meanCrossTrackNM += cross
		result.maxCrossTrackNM = max(result.maxCrossTrackNM, cross)
		result.total++
		if cross > 2 {
			result.offRouteSamples++
		}
		if index == 0 {
			continue
		}
		previous := selected[index-1]
		interval := point.at.Sub(previous.at)
		step := distanceNM(previous.latitude, previous.longitude, point.latitude, point.longitude)
		if interval <= 0 || interval > 90*time.Second || step > .5+700*interval.Hours() {
			result.complete = false
			continue
		}
		result.flownDistanceNM += step
	}
	result.meanCrossTrackNM /= float64(result.total)
	return result
}

func distanceToRouteNM(point trackPoint, route []routeSegment) float64 {
	nearest := math.Inf(1)
	for _, segment := range route {
		nearest = math.Min(nearest, distanceToSegmentNM(point.latitude, point.longitude, segment))
	}
	return nearest
}

func distanceToSegmentNM(latitude, longitude float64, segment routeSegment) float64 {
	cosine := math.Cos(latitude * math.Pi / 180)
	px, py := longitude*60*cosine, latitude*60
	ax, ay := segment.fromLongitude*60*cosine, segment.fromLatitude*60
	bx, by := segment.toLongitude*60*cosine, segment.toLatitude*60
	dx, dy := bx-ax, by-ay
	denominator := dx*dx + dy*dy
	if denominator == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	ratio := math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/denominator))
	return math.Hypot(px-(ax+ratio*dx), py-(ay+ratio*dy))
}

func appendForecast(points []forecastPoint, next forecastPoint) []forecastPoint {
	if len(points) > 0 && points[len(points)-1].generatedAt.Equal(next.generatedAt) {
		points[len(points)-1] = next
		return points
	}
	return append(points, next)
}

func latestForecastBefore(points []forecastPoint, cutoff time.Time) (forecastPoint, bool) {
	for i := len(points) - 1; i >= 0; i-- {
		if !points[i].generatedAt.After(cutoff) {
			return points[i], true
		}
	}
	return forecastPoint{}, false
}

func discoverLandings(files []string) (map[string]time.Time, error) {
	source := vatsim.NewSnapshotReplaySource()
	airborne, result := map[string]bool{}, map[string]time.Time{}
	for _, path := range files {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %q: %w", path, err)
		}
		info, _ := file.Stat()
		receivedAt := time.Time{}
		if info != nil {
			receivedAt = info.ModTime().UTC()
		}
		err = source.Load(file, receivedAt)
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("load %q: %w", path, err)
		}
		at := source.Snapshot().Timestamp
		for _, flight := range source.Snapshot().Flights() {
			if flight.FlightPlan.Destination != "EKCH" || !flight.Online() {
				continue
			}
			wasAirborne := airborne[flight.CID]
			isAirborne := flight.Altitude >= 1000 || flight.Groundspeed > 80
			if wasAirborne && flight.Altitude < 1000 && flight.Groundspeed <= 50 && distanceNM(flight.Latitude, flight.Longitude, 55.62, 12.65) <= 5 {
				if _, exists := result[flight.CID]; !exists {
					result[flight.CID] = at
				}
				airborne[flight.CID] = false
			} else if isAirborne {
				airborne[flight.CID] = true
			}
		}
	}
	return result, nil
}

type unavailableWind struct{}

type routeMaterializer interface {
	MaterializeRoute(context.Context, navdata.RouteQuery, string) (navdata.RouteKey, error)
}
type materializedRoute struct {
	key navdata.RouteKey
	err error
}
type memoizingMaterializer struct {
	source  routeMaterializer
	results map[string]materializedRoute
}

// cacheOnlyMaterializer validates that a route is already present in the
// canonical cache. It deliberately cannot perform an import or network call.
type cacheOnlyMaterializer struct {
	geometry interface {
		Route(context.Context, navdata.RouteKey) (navdata.RouteGeometry, error)
	}
}

func (m cacheOnlyMaterializer) MaterializeRoute(ctx context.Context, query navdata.RouteQuery, resolver string) (navdata.RouteKey, error) {
	key, err := (navdata.RouteCandidate{Query: query, ResolverVersion: resolver, SchemaVersion: navdata.CanonicalSchemaVersion}).PersistenceKey()
	if err != nil {
		return "", err
	}
	if _, err := m.geometry.Route(ctx, key); err != nil {
		return "", err
	}
	return key, nil
}

func (m *memoizingMaterializer) MaterializeRoute(ctx context.Context, query navdata.RouteQuery, resolver string) (navdata.RouteKey, error) {
	cacheKey, err := query.Key()
	if err != nil {
		return "", err
	}
	key := string(cacheKey)
	if value, ok := m.results[key]; ok {
		return value.key, value.err
	}
	resolved, err := m.source.MaterializeRoute(ctx, query, resolver)
	m.results[key] = materializedRoute{key: resolved, err: err}
	return resolved, err
}

func (unavailableWind) WindProfile(context.Context, predictor.WindProfileRequest) (predictor.WindProfile, error) {
	return predictor.WindProfile{}, fmt.Errorf("historical wind profile unavailable")
}

type memoryRepository struct {
	state aman.AirportState
	has   bool
}

func (r *memoryRepository) LoadAirportState(context.Context, string) (aman.AirportState, error) {
	if !r.has {
		return aman.AirportState{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "missing"}
	}
	return r.state, nil
}
func (r *memoryRepository) Commit(_ context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	if err := commit.Validate(); err != nil {
		return aman.CommitResult{}, err
	}
	r.state, r.has = commit.State, true
	return aman.CommitResult{State: commit.State}, nil
}

type discardPublisher struct{}

func (discardPublisher) PublishAMANState(context.Context, aman.AirportState) error { return nil }

type binder struct {
	next int
	ids  map[string]aman.FlightID
}

func (b *binder) BindVATSIMFlight(_ context.Context, identity aman.VATSIMFlightIdentity) (aman.FlightID, error) {
	if err := identity.Validate(); err != nil {
		return "", err
	}
	if id, ok := b.ids[identity.VATSIMCID]; ok {
		return id, nil
	}
	b.next++
	id := aman.FlightID("history-" + strconv.Itoa(b.next))
	b.ids[identity.VATSIMCID] = id
	return id, nil
}
func distanceNM(a, b, c, d float64) float64 {
	return math.Hypot((a-c)*60, (b-d)*60*math.Cos((a+c)*math.Pi/360))
}
func printMetrics(label string, candidates int, samples map[time.Duration][]accuracySample, horizons []time.Duration) {
	fmt.Printf("AMAN accuracy benchmark (%s)\n  landing truth candidates: %d\n", label, candidates)
	for _, horizon := range horizons {
		values := samples[horizon]
		if len(values) == 0 {
			fmt.Printf("  %2dm before landing: no publishable predictions\n", int(horizon.Minutes()))
			continue
		}
		printMetricsAtHorizon(horizon, candidates, values)
	}
	printDiagnostics(samples, horizons)
}

func printLandingRate(landings map[string]time.Time) {
	times := make([]time.Time, 0, len(landings))
	for _, at := range landings {
		times = append(times, at)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	if len(times) == 0 {
		return
	}
	peak := func(window time.Duration) int {
		best, left := 0, 0
		for right, at := range times {
			for times[left].Before(at.Add(-window)) {
				left++
			}
			best = max(best, right-left+1)
		}
		return best
	}
	span := times[len(times)-1].Sub(times[0])
	average := 0.0
	if span > 0 {
		average = float64(len(times)-1) / span.Hours()
	}
	fmt.Printf("Observed landing rate (VATSIM touchdown proxy)\n  period: %s to %s (%d landings, %.1f/hour across full period)\n  peak rolling 60 minutes: %d/hour\n  peak rolling 30 minutes: %d/hour equivalent\n", times[0].Format(time.RFC3339), times[len(times)-1].Format(time.RFC3339), len(times), average, peak(time.Hour), 2*peak(30*time.Minute))
}

func printUnavailableCandidates(landings map[string]time.Time, publishable map[string]bool, diagnostics map[string]availabilityDiagnostic) {
	type candidate struct {
		cid        string
		landedAt   time.Time
		diagnostic availabilityDiagnostic
	}
	missing := make([]candidate, 0)
	for cid, landedAt := range landings {
		if publishable[cid] {
			continue
		}
		diagnostic, ok := diagnostics[cid]
		if !ok {
			diagnostic.reason = "not_observed_before_five_minute_horizon"
		}
		missing = append(missing, candidate{cid: cid, landedAt: landedAt, diagnostic: diagnostic})
	}
	if len(missing) == 0 {
		return
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].landedAt.Before(missing[j].landedAt) })
	fmt.Printf("AMAN predictions unavailable at 5-minute horizon (%d):\n", len(missing))
	for _, value := range missing {
		fmt.Printf("  %s (%s), landed %s: state %s, source %s, reason %s\n", value.diagnostic.callsign, value.cid, value.landedAt.Format(time.RFC3339), value.diagnostic.state, value.diagnostic.status, value.diagnostic.reason)
	}
}

func printMetricsAtHorizon(horizon time.Duration, candidates int, values []accuracySample) {
	operational, raw := make([]float64, len(values)), make([]float64, len(values))
	slot := make([]float64, 0, len(values))
	frozen, ageTotal, delayedFiveMinutes := 0, time.Duration(0), 0
	maxDelay := time.Duration(0)
	for i, value := range values {
		operational[i], raw[i] = value.operationalError, value.rawError
		if value.slotError != nil {
			slot = append(slot, *value.slotError)
		}
		if value.frozen {
			frozen++
		}
		if value.scheduledDelay >= 5*time.Minute {
			delayedFiveMinutes++
		}
		maxDelay = max(maxDelay, value.scheduledDelay)
		ageTotal += value.age
	}
	metrics := metricSummary(operational)
	rawMetrics := metricSummary(raw)
	slotSummary := "no slot"
	if len(slot) > 0 {
		slotMetrics := metricSummary(slot)
		slotSummary = fmt.Sprintf("slot bias %+.0fs / MAE %.0fs (%d)", slotMetrics.bias, slotMetrics.mae, len(slot))
	}
	fmt.Printf("  %2dm before landing: %3d predictions (availability %.1f%%), operational bias %+.0fs / MAE %.0fs, raw bias %+.0fs / MAE %.0fs, %s, frozen %.0f%%, slot delay >=5m %d, max slot delay %.0fs, average age %.0fs, P90 operational AE %.0fs\n", int(horizon.Minutes()), len(values), 100*float64(len(values))/float64(candidates), metrics.bias, metrics.mae, rawMetrics.bias, rawMetrics.mae, slotSummary, 100*float64(frozen)/float64(len(values)), delayedFiveMinutes, maxDelay.Seconds(), ageTotal.Seconds()/float64(len(values)), metrics.p90)
}

func printDiagnostics(samples map[time.Duration][]accuracySample, horizons []time.Duration) {
	fmt.Println("Physical phase and holding diagnostics")
	for _, horizon := range horizons {
		byPhase := map[string][]accuracySample{}
		for _, sample := range samples[horizon] {
			phase := sample.phase
			if phase == "" {
				phase = "unclassified"
			}
			byPhase[phase] = append(byPhase[phase], sample)
		}
		phases := make([]string, 0, len(byPhase))
		for phase := range byPhase {
			phases = append(phases, phase)
		}
		sort.Strings(phases)
		for _, phase := range phases {
			values := byPhase[phase]
			raw, distances, realizedSpeeds, observedSpeeds := make([]float64, len(values)), make([]float64, len(values)), make([]float64, len(values)), make([]float64, len(values))
			for i, value := range values {
				raw[i], distances[i] = value.rawError, value.distanceNM
				observedSpeeds[i] = value.observedGroundspeed
				actualDuration := value.landedAt.Sub(value.generatedAt)
				if actualDuration > 0 {
					realizedSpeeds[i] = value.distanceNM / actualDuration.Hours()
				}
			}
			metrics := metricSummary(raw)
			fmt.Printf("  %2dm %s: n=%d, raw bias %+.0fs, mean DTG %.1fnm, observed GS %.0fkt, implied route GS %.0fkt\n", int(horizon.Minutes()), phase, len(values), metrics.bias, mean(distances), mean(observedSpeeds), mean(realizedSpeeds))
		}
		printHoldingDiagnostics(horizon, samples[horizon])
		printHoldingFixDiagnostics(horizon, samples[horizon])
		printHoldingExecutionDiagnostics(horizon, samples[horizon])
		printTETAControlDiagnostics(horizon, samples[horizon])
		printSTARDiagnostics(horizon, samples[horizon])
		printTMATrackDiagnostics(horizon, samples[horizon])
	}
}

func printHoldingDiagnostics(horizon time.Duration, values []accuracySample) {
	holding, expected, observedResidual := make([]float64, 0, len(values)), make([]float64, 0, len(values)), make([]float64, 0, len(values))
	for _, value := range values {
		if value.holdingDuration <= 0 || value.postHoldingTransit <= 0 {
			continue
		}
		holding = append(holding, value.holdingDuration.Seconds())
		expected = append(expected, value.scheduledDelay.Seconds())
		observedResidual = append(observedResidual, value.landedAt.Sub(value.generatedAt).Seconds()-value.postHoldingTransit.Seconds())
	}
	if len(holding) == 0 {
		fmt.Printf("  %2dm holding plan: none\n", int(horizon.Minutes()))
		return
	}
	fmt.Printf("  %2dm holding plan: n=%d, expected hold %.0fs, slot delay %.0fs, remaining time less post-hold transit %.0fs\n", int(horizon.Minutes()), len(holding), mean(holding), mean(expected), mean(observedResidual))
}

func printHoldingFixDiagnostics(horizon time.Duration, values []accuracySample) {
	errors, predicted, observed := []float64{}, []float64{}, []float64{}
	bySTAR := map[string][]accuracySample{}
	for _, value := range values {
		if value.holdingFixETA == nil || value.holdingFixPassage == nil {
			continue
		}
		errors = append(errors, value.holdingFixETA.Sub(*value.holdingFixPassage).Seconds())
		predicted = append(predicted, value.holdingFixETA.Sub(value.generatedAt).Seconds())
		observed = append(observed, value.holdingFixPassage.Sub(value.generatedAt).Seconds())
		star := value.star
		if star == "" {
			star = "unclassified"
		}
		bySTAR[star] = append(bySTAR[star], value)
	}
	if len(errors) == 0 {
		fmt.Printf("  %2dm holding-fix ETA: no observed passages\n", int(horizon.Minutes()))
		return
	}
	metrics := metricSummary(errors)
	fmt.Printf("  %2dm holding-fix ETA: n=%d, bias %+.0fs / MAE %.0fs, predicted passage %.0fs, observed passage %.0fs\n", int(horizon.Minutes()), len(errors), metrics.bias, metrics.mae, mean(predicted), mean(observed))
	stars := make([]string, 0, len(bySTAR))
	for star := range bySTAR {
		stars = append(stars, star)
	}
	sort.Strings(stars)
	for _, star := range stars {
		values := bySTAR[star]
		entryErrors, transitErrors, predictedTransit, observedTransit := []float64{}, []float64{}, []float64{}, []float64{}
		for _, value := range values {
			entryErrors = append(entryErrors, value.holdingFixETA.Sub(*value.holdingFixPassage).Seconds())
			modelled := value.rawTETA.Sub(*value.holdingFixETA).Seconds()
			actual := value.landedAt.Sub(*value.holdingFixPassage).Seconds()
			transitErrors = append(transitErrors, modelled-actual)
			predictedTransit = append(predictedTransit, modelled)
			observedTransit = append(observedTransit, actual)
		}
		entry := metricSummary(entryErrors)
		transit := metricSummary(transitErrors)
		fmt.Printf("  %2dm holding-fix %s: n=%d, entry bias %+.0fs / MAE %.0fs, post-fix transit bias %+.0fs / MAE %.0fs, modelled %.0fs, observed %.0fs\n", int(horizon.Minutes()), star, len(values), entry.bias, entry.mae, transit.bias, transit.mae, mean(predictedTransit), mean(observedTransit))
	}
}

func printHoldingExecutionDiagnostics(horizon time.Duration, values []accuracySample) {
	bySTAR := map[string][]accuracySample{}
	for _, value := range values {
		if value.holdingDuration <= 0 || value.postHoldingTransit <= 0 || value.holdingFixPassage == nil || value.holdingFixLastPassage == nil {
			continue
		}
		star := value.star
		if star == "" {
			star = "unclassified"
		}
		bySTAR[star] = append(bySTAR[star], value)
	}
	if len(bySTAR) == 0 {
		fmt.Printf("  %2dm holding execution: no complete observed entries and exits\n", int(horizon.Minutes()))
		return
	}
	stars := make([]string, 0, len(bySTAR))
	for star := range bySTAR {
		stars = append(stars, star)
	}
	sort.Strings(stars)
	for _, star := range stars {
		values := bySTAR[star]
		holdErrors, transitErrors, expectedHold, observedHold, modelledTransit, observedTransit := []float64{}, []float64{}, []float64{}, []float64{}, []float64{}, []float64{}
		for _, value := range values {
			expected := value.holdingDuration.Seconds()
			actualHold := value.holdingFixLastPassage.Sub(*value.holdingFixPassage).Seconds()
			modelled := value.postHoldingTransit.Seconds()
			actualTransit := value.landedAt.Sub(*value.holdingFixLastPassage).Seconds()
			holdErrors = append(holdErrors, expected-actualHold)
			transitErrors = append(transitErrors, modelled-actualTransit)
			expectedHold = append(expectedHold, expected)
			observedHold = append(observedHold, actualHold)
			modelledTransit = append(modelledTransit, modelled)
			observedTransit = append(observedTransit, actualTransit)
		}
		hold := metricSummary(holdErrors)
		transit := metricSummary(transitErrors)
		fmt.Printf("  %2dm holding execution %s: n=%d, hold bias %+.0fs / MAE %.0fs, scheduled %.0fs, observed %.0fs; post-hold transit bias %+.0fs / MAE %.0fs, modelled %.0fs, observed %.0fs\n", int(horizon.Minutes()), star, len(values), hold.bias, hold.mae, mean(expectedHold), mean(observedHold), transit.bias, transit.mae, mean(modelledTransit), mean(observedTransit))
	}
}

func printTETAControlDiagnostics(horizon time.Duration, values []accuracySample) {
	byReason := map[aman.FreezeReason][]accuracySample{}
	for _, value := range values {
		byReason[value.freezeReason] = append(byReason[value.freezeReason], value)
	}
	reasons := make([]string, 0, len(byReason))
	for reason := range byReason {
		reasons = append(reasons, string(reason))
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		values := byReason[aman.FreezeReason(reason)]
		raw, operational := make([]float64, len(values)), make([]float64, len(values))
		for i, value := range values {
			raw[i], operational[i] = value.rawError, value.operationalError
		}
		rawMetrics, operationalMetrics := metricSummary(raw), metricSummary(operational)
		fmt.Printf("  %2dm TETA control %s: n=%d, raw bias %+.0fs / MAE %.0fs, operational bias %+.0fs / MAE %.0fs\n", int(horizon.Minutes()), reason, len(values), rawMetrics.bias, rawMetrics.mae, operationalMetrics.bias, operationalMetrics.mae)
	}
}

func printSTARDiagnostics(horizon time.Duration, values []accuracySample) {
	bySTAR := map[string][]accuracySample{}
	for _, value := range values {
		star := value.star
		if star == "" {
			star = "unclassified"
		}
		bySTAR[star] = append(bySTAR[star], value)
	}
	stars := make([]string, 0, len(bySTAR))
	for star := range bySTAR {
		stars = append(stars, star)
	}
	sort.Strings(stars)
	for _, star := range stars {
		values := bySTAR[star]
		raw, predicted, observed := make([]float64, len(values)), make([]float64, len(values)), make([]float64, len(values))
		for i, value := range values {
			raw[i] = value.rawError
			observed[i] = value.landedAt.Sub(value.generatedAt).Seconds()
			predicted[i] = observed[i] + value.rawError
		}
		metrics := metricSummary(raw)
		fmt.Printf("  %2dm STAR %s: n=%d, raw bias %+.0fs / MAE %.0fs, predicted remaining %.0fs, observed remaining %.0fs\n", int(horizon.Minutes()), star, len(values), metrics.bias, metrics.mae, mean(predicted), mean(observed))
	}
}

func printTMATrackDiagnostics(horizon time.Duration, values []accuracySample) {
	modelDistance, flownDistance, meanCrossTrack, maxCrossTrack, offRoute, total := []float64{}, []float64{}, []float64{}, []float64{}, 0, 0
	for _, value := range values {
		if !value.inTMA || !value.track.complete {
			continue
		}
		modelDistance = append(modelDistance, value.distanceNM)
		flownDistance = append(flownDistance, value.track.flownDistanceNM)
		meanCrossTrack = append(meanCrossTrack, value.track.meanCrossTrackNM)
		maxCrossTrack = append(maxCrossTrack, value.track.maxCrossTrackNM)
		offRoute += value.track.offRouteSamples
		total += value.track.total
	}
	if len(modelDistance) == 0 {
		fmt.Printf("  %2dm TMA track: no complete TMA samples\n", int(horizon.Minutes()))
		return
	}
	fmt.Printf("  %2dm TMA track: n=%d, published remaining %.1fnm, flown %.1fnm, detour %+.1fnm, mean cross-track %.1fnm, max cross-track %.1fnm, off-route (>2nm) %.0f%%\n", int(horizon.Minutes()), len(modelDistance), mean(modelDistance), mean(flownDistance), mean(flownDistance)-mean(modelDistance), mean(meanCrossTrack), mean(maxCrossTrack), 100*float64(offRoute)/float64(total))
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

type metricSet struct{ bias, mae, p90 float64 }

func metricSummary(values []float64) metricSet {
	abs := make([]float64, len(values))
	sum := 0.0
	for i, v := range values {
		sum += v
		abs[i] = math.Abs(v)
	}
	sort.Float64s(abs)
	p := func(q float64) float64 { return abs[max(0, int(math.Ceil(q*float64(len(abs))))-1)] }
	return metricSet{bias: sum / float64(len(values)), mae: sumAbs(abs) / float64(len(abs)), p90: p(.9)}
}
func sumAbs(values []float64) float64 {
	var s float64
	for _, v := range values {
		s += v
	}
	return s
}
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "AMAN accuracy benchmark failed: "+format+"\n", args...)
	os.Exit(1)
}

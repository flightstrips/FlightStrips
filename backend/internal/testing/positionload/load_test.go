package positionload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/app"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/navigation"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testing/e2e"
	front "FlightStrips/internal/testing/frontend"
	"FlightStrips/internal/testing/replay"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type sample struct {
	At           time.Time
	MS           float64
	Queries      int
	PoolMS       float64
	QueueMS      float64
	ProcessingMS float64
}
type capture struct {
	mu                sync.Mutex
	samples           []sample
	queries           map[trace.TraceID]int
	pools             map[trace.TraceID]float64
	completed         int
	errors            int
	operationalErrors int
	events            map[string]int
}

func (c *capture) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range spans {
		id := s.SpanContext().TraceID()
		if s.Name() == "pool.acquire" {
			c.pools[id] += float64(s.EndTime().Sub(s.StartTime())) / float64(time.Millisecond)
		}
		if len(s.Name()) >= 5 && s.Name()[:5] == "query" {
			c.queries[id]++
		}
		if s.Name() == "aircraft_position_update" {
			c.completed++
			if s.Status().Code == codes.Error {
				c.errors++
			}
			sample := sample{At: s.StartTime(), MS: float64(s.EndTime().Sub(s.StartTime())) / float64(time.Millisecond), Queries: c.queries[id], PoolMS: c.pools[id]}
			for _, a := range s.Attributes() {
				switch string(a.Key) {
				case "message.queue_ms":
					sample.QueueMS = a.Value.AsFloat64()
				case "message.processing_ms":
					sample.ProcessingMS = a.Value.AsFloat64()
				}
			}
			c.samples = append(c.samples, sample)
		}
		if !s.Parent().IsValid() && s.Name() != "aircraft_position_update" && s.Status().Code == codes.Error {
			c.operationalErrors++
		}
		if !s.Parent().IsValid() {
			delete(c.queries, id)
			delete(c.pools, id)
		}
	}
	return nil
}
func (*capture) Shutdown(context.Context) error { return nil }
func (c *capture) count() int                   { c.mu.Lock(); defer c.mu.Unlock(); return c.completed }
func duration(name string, fallback time.Duration) time.Duration {
	if raw := os.Getenv(name); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			panic(err)
		}
		return d
	}
	return fallback
}
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	return values[int(math.Ceil(float64(len(values))*p))-1]
}

// TestPositionLoad uses a fixed-rate sender and the real authenticated WebSocket,
// application services and PostgreSQL. It is opt-in because the full gate takes
// 17 minutes per traffic mix. No production endpoints or credentials are used.
func TestPositionLoad(t *testing.T) {
	if os.Getenv("POSITION_LOAD_TEST") != "true" {
		t.Skip("set POSITION_LOAD_TEST=true for the isolated PostgreSQL load gate")
	}
	require.NoError(t, os.Chdir("../../.."))
	defer os.Chdir("internal/testing/positionload")
	t.Setenv("TEST_MODE", "true")
	require.NoError(t, config.InitConfig())
	logger := slog.Default()
	if os.Getenv("POSITION_LOAD_DEBUG") != "true" {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	defer slog.SetDefault(logger)
	collector := &capture{queries: make(map[trace.TraceID]int), pools: make(map[trace.TraceID]float64)}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(collector))
	old := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(old)
	defer provider.Shutdown(context.Background())
	server, err := e2e.StartTestServerWithConfig(func(c *app.Config) {
		c.EnablePostgresTracing = true
		c.Navigation = navigation.Config{Source: navigation.SourceAIRACNet}
		c.EnableStandAssignment = true
		c.EnableTestTools = true
		c.StandAssignmentAircraftJSON = "config/test/ICAO_Aircraft.json"
		c.AMAN = aman.RuntimeConfig{Mode: aman.ModeShadow, EnabledAirports: []string{"EKCH"}}
	})
	require.NoError(t, err)
	defer testdata.ShutdownTestDB()
	defer server.Stop()
	require.True(t, config.GetStandAssignmentReadiness().Ready, "stand lifecycle must actually be enabled")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, server.Queries.InsertAirport(ctx, "EKCH"))
	cfg := replay.DefaultConfig()
	cfg.SessionFile = "synthetic-position-load"
	cfg.ServerURL = server.GetWebSocketURL()
	client, err := replay.NewClient(cfg)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer client.Close()
	client.ReadMessages(ctx, func(int, []byte) {})
	require.NoError(t, client.SendProtobuf(&es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Position: "121.630", Callsign: "EKCH_A_GND", Range: 200}, es.Login))
	var session database.Session
	require.Eventually(t, func() bool {
		session, err = server.Queries.GetSession(ctx, database.GetSessionParams{Airport: "EKCH", Name: "LIVE"})
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)
	// Complete the real operational login/sync before seeding aircraft.
	require.NoError(t, client.SendProtobuf(&es.SyncEvent{Controllers: []*es.Controller{{Callsign: "EKCH_A_GND", Position: "121.630"}}}, es.Sync))
	controllers := postgres.NewControllerRepository(server.DBPool)
	require.Eventually(t, func() bool {
		controller, err := controllers.GetByCid(ctx, "TEST_CID")
		return err == nil && controller.Session == session.ID
	}, 5*time.Second, 10*time.Millisecond)
	repo := postgres.NewStripRepository(server.DBPool)
	stands := config.GetStandCapabilities().Stands("EKCH")
	require.Greater(t, len(stands), 20)
	arrivalPercent := 50
	if raw := os.Getenv("POSITION_LOAD_ARRIVAL_PERCENT"); raw != "" {
		arrivalPercent, err = strconv.Atoi(raw)
		require.NoError(t, err)
		require.True(t, arrivalPercent >= 0 && arrivalPercent <= 100)
	}
	arrivals := 200 * arrivalPercent / 100
	for i := 0; i < 200; i++ {
		cid, aircraft, route, runway, state, squawk := strconv.Itoa(800000+i), "B738", "NEXEN", "22L", "TAXI", "1000"
		origin, destination, bay := "EKCH", "ESSA", "TAXI"
		lat, lon, alt := 55.63, 12.65, int32(20)
		if i < arrivals {
			origin, destination, bay = "ESSA", "EKCH", "ARR_HIDDEN"
			lat, lon, alt = 55.85, 12.95, 5000
		}
		stand := stands[i%len(stands)].Name
		if i >= arrivals && i%10 == 0 {
			lat, lon = stands[i%len(stands)].Latitude, stands[i%len(stands)].Longitude
			state = ""
			bay = "NOT_CLEARED"
		}
		strip := &models.Strip{Callsign: fmt.Sprintf("SAS%03d", i), Session: session.ID, Origin: origin, Destination: destination, Bay: bay, AircraftType: &aircraft, Route: &route, Runway: &runway, State: &state, AssignedSquawk: &squawk, Stand: &stand, VatsimCID: &cid, PositionLatitude: &lat, PositionLongitude: &lon, PositionAltitude: &alt, Cleared: true, Version: 1}
		if i < arrivals {
			payload, _ := json.Marshal(map[string]any{"session_id": session.ID, "preset": "arrival", "callsign": strip.Callsign, "aircraft_type": aircraft, "origin": origin, "destination": destination, "route": route, "initial_state": "online", "altitude": 5000, "groundspeed": 220})
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+server.ServerAddr+"/api/test/sat/scenarios", bytes.NewReader(payload))
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer __TEST_TOKEN__")
			req.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			require.NoError(t, err)
			require.Equal(t, http.StatusCreated, response.StatusCode, string(body))
			existing, err := repo.GetByCallsign(ctx, session.ID, strip.Callsign)
			require.NoError(t, err)
			strip.Version = existing.Version
			strip.VatsimCID = existing.VatsimCID
			_, err = repo.Update(ctx, strip)
			require.NoError(t, err)
		} else {
			require.NoError(t, repo.Create(ctx, strip))
			if i%10 == 0 {
				now := time.Now().UTC()
				require.NoError(t, postgres.NewStandAssignmentRepository(server.DBPool).CreateAssignment(ctx, &models.StandAssignment{SessionID: session.ID, Callsign: strip.Callsign, Stand: stand, Direction: string(sat.AssignmentDirectionDeparture), Stage: services.StageReserved, Source: "AUTOMATIC", AssignedAt: &now, Version: 1}))
			}
		}
	}
	frontend := front.NewClient(front.Config{ServerURL: "ws://" + server.ServerAddr + "/frontEndEvents", Token: "__TEST_TOKEN__"})
	require.NoError(t, frontend.Connect(ctx))
	defer frontend.Close()
	lastAssignments := map[string]string{}
	frontend.ReadMessages(ctx, func(_ int, raw []byte) {
		var event struct {
			Type        string `json:"type"`
			Assignments []struct {
				Callsign string `json:"callsign"`
				Stage    string `json:"stage"`
			} `json:"assignments"`
			Bay        string `json:"bay"`
			Aldt       string `json:"aldt"`
			Assignment struct {
				Stage string `json:"stage"`
			} `json:"assignment"`
		}
		if json.Unmarshal(raw, &event) != nil {
			return
		}
		collector.mu.Lock()
		defer collector.mu.Unlock()
		if collector.events == nil {
			collector.events = map[string]int{}
		}
		collector.events[event.Type]++
		if event.Type == "stand_status_snapshot" {
			next := map[string]string{}
			for _, a := range event.Assignments {
				next[a.Callsign] = a.Stage
				collector.events["stand:"+a.Stage]++
			}
			for callsign := range lastAssignments {
				if _, ok := next[callsign]; !ok {
					collector.events["stand_assignment_removed"]++
				}
			}
			lastAssignments = next
		}
		if event.Bay != "" {
			collector.events["bay:"+event.Bay]++
		}
		if event.Aldt != "" {
			collector.events["landing"]++
		}
		if event.Assignment.Stage != "" {
			collector.events["stand:"+event.Assignment.Stage]++
		}
	})
	require.Eventually(t, func() bool { collector.mu.Lock(); defer collector.mu.Unlock(); return collector.events["initial"] > 0 }, 5*time.Second, 10*time.Millisecond, "frontend must join the synced session")
	warmup, measurement := duration("POSITION_LOAD_WARMUP", 2*time.Minute), duration("POSITION_LOAD_DURATION", 15*time.Minute)
	start := time.Now()
	measureStart := start.Add(warmup)
	measureEnd := measureStart.Add(measurement)
	sent, maxBacklog := 0, 0
	var senderLag []float64
	// Five interleaved operational messages/sec are sent from the same writer.
	nextControl := start
	nextFrontend := start
	for due := start; due.Before(measureEnd); due = due.Add(10 * time.Millisecond) {
		if delay := time.Until(due); delay > 0 {
			time.Sleep(delay)
		}
		i := sent % 200
		cycle := sent / 200
		lat, lon, alt := 55.63+float64(i%20)*.0001, 12.65+float64(cycle%50)*.00002, int32(20)
		if i < arrivals {
			lat, lon, alt = 55.85-float64(cycle%100)*.0003, 12.95-float64(cycle%100)*.0003, 5000-int32(cycle%100)*20
		}
		if i < arrivals && i%10 == 0 {
			phase := cycle - (i/10)*25
			switch {
			case phase < 10:
				lat, lon, alt = 55.67, 12.73, 1500
			case phase < 20:
				lat, lon, alt = 55.626, 12.6685, 150
			case phase < 25:
				lat, lon, alt = 55.624, 12.6654, 20
			case phase < 30:
				lat, lon, alt = 55.63, 12.65, 20
			default:
				lat, lon, alt = stands[i%len(stands)].Latitude, stands[i%len(stands)].Longitude, 20
			}
		}
		if i >= arrivals && i%10 == 0 {
			phase := cycle - ((i-arrivals)/10)*25
			if phase < 15 {
				lat, lon = stands[i%len(stands)].Latitude, stands[i%len(stands)].Longitude
			} else if phase < 25 {
				lat, lon = 55.63, 12.65
			} else {
				lat, lon, alt = 55.65+float64(cycle)*.0001, 12.68, 3000
			}
			if phase == 15 {
				require.NoError(t, client.SendProtobuf(&es.GroundStateEvent{Callsign: fmt.Sprintf("SAS%03d", i), GroundState: "PUSH"}, es.GroundState))
			}
			if phase == 25 {
				require.NoError(t, client.SendProtobuf(&es.GroundStateEvent{Callsign: fmt.Sprintf("SAS%03d", i), GroundState: es.GroundStateDepart}, es.GroundState))
			}
		}
		require.NoError(t, client.SendProtobuf(&es.AircraftPositionUpdateEvent{Callsign: fmt.Sprintf("SAS%03d", i), Lat: lat, Lon: lon, Altitude: int64(alt)}, es.PositionUpdate))
		sent++
		if !due.Before(nextControl) {
			require.NoError(t, client.SendProtobuf(&es.HeadingEvent{Callsign: fmt.Sprintf("SAS%03d", sent%200), Heading: int32(cycle % 360)}, es.SetHeading))
			nextControl = nextControl.Add(200 * time.Millisecond)
		}
		if !due.Before(nextFrontend) {
			require.NoError(t, frontend.SendRawMessage(map[string]any{"type": "marked", "callsign": fmt.Sprintf("SAS%03d", sent%200), "marked": cycle%2 == 0}))
			nextFrontend = nextFrontend.Add(time.Second)
		}
		if !due.Before(measureStart) {
			senderLag = append(senderLag, float64(time.Since(due))/float64(time.Millisecond))
			maxBacklog = max(maxBacklog, sent-collector.count())
		}
	}
	require.Eventually(t, func() bool { return collector.count() == sent }, 5*time.Second, time.Millisecond, "every sent report must complete")
	burstStart := time.Now()
	for n := 0; n < 2000; n++ {
		due := burstStart.Add(time.Duration(n) * 5 * time.Millisecond)
		if wait := time.Until(due); wait > 0 {
			time.Sleep(wait)
		}
		i := n % 200
		require.NoError(t, client.SendProtobuf(&es.AircraftPositionUpdateEvent{Callsign: fmt.Sprintf("SAS%03d", i), Lat: 55.7 + float64(n%50)*.00001, Lon: 12.8, Altitude: 4000}, es.PositionUpdate))
		sent++
	}
	drainStart := time.Now()
	burstTarget := sent
	var burstDrain time.Duration
	for n := 0; n < 200; n++ {
		due := drainStart.Add(time.Duration(n) * 10 * time.Millisecond)
		if wait := time.Until(due); wait > 0 {
			time.Sleep(wait)
		}
		if burstDrain == 0 && collector.count() >= burstTarget {
			burstDrain = time.Since(drainStart)
		}
		require.NoError(t, client.SendProtobuf(&es.AircraftPositionUpdateEvent{Callsign: fmt.Sprintf("SAS%03d", n), Lat: 55.7 + float64(n%50)*.00001, Lon: 12.8, Altitude: 4000}, es.PositionUpdate))
		sent++
	}
	require.NotZero(t, burstDrain, "burst backlog must drain while continuing at 100/sec")
	require.Eventually(t, func() bool { return collector.count() == sent }, 2*time.Second, time.Millisecond)
	collector.mu.Lock()
	var times, queries, pools, queueTimes, processingTimes []float64
	for _, s := range collector.samples {
		if !s.At.Before(measureStart) && s.At.Before(measureEnd) {
			times = append(times, s.MS)
			queries = append(queries, float64(s.Queries))
			pools = append(pools, s.PoolMS)
			queueTimes = append(queueTimes, s.QueueMS)
			processingTimes = append(processingTimes, s.ProcessingMS)
		}
	}
	failures := collector.errors
	operationalFailures := collector.operationalErrors
	observedEvents := make(map[string]int, len(collector.events))
	for k, v := range collector.events {
		observedEvents[k] = v
	}
	collector.mu.Unlock()
	average := func(v []float64) float64 {
		sum := 0.
		for _, x := range v {
			sum += x
		}
		if len(v) == 0 {
			return 0
		}
		return sum / float64(len(v))
	}
	var postgresVersion string
	require.NoError(t, server.DBPool.QueryRow(ctx, "SELECT version()").Scan(&postgresVersion))
	report := map[string]any{"platform": runtime.GOOS + "/" + runtime.GOARCH, "cpus": runtime.NumCPU(), "go": runtime.Version(), "postgres": postgresVersion, "topology": loadTopology(), "pool_max_connections": server.DBPool.Config().MaxConns, "release": os.Getenv("POSITION_LOAD_REVISION"), "position_workers": os.Getenv("POSITION_WORKERS_PER_CLIENT"), "arrival_percent": arrivalPercent, "warmup": warmup.String(), "duration": measurement.String(), "sent": sent, "completed": collector.count(), "errors": failures, "operational_errors": operationalFailures, "observed_events": observedEvents, "samples": len(times), "p95_ms": percentile(times, .95), "p99_ms": percentile(times, .99), "average_ms": average(times), "average_db_operations": average(queries), "average_pool_wait_ms": average(pools), "p99_queue_ms": percentile(queueTimes, .99), "p99_processing_ms": percentile(processingTimes, .99), "max_outstanding": maxBacklog, "burst_drain_ms": float64(burstDrain) / float64(time.Millisecond), "sender_p99_lag_ms": percentile(senderLag, .99)}
	raw, err := json.MarshalIndent(report, "", "  ")
	require.NoError(t, err)
	t.Log(string(raw))
	if path := os.Getenv("POSITION_LOAD_REPORT"); path != "" {
		require.NoError(t, os.WriteFile(path, raw, 0600))
	}
	require.Zero(t, failures)
	require.Zero(t, operationalFailures, "interleaved operational messages and frontend actions must succeed")
	if warmup+measurement >= 2*time.Minute {
		if arrivals > 0 {
			require.Positive(t, observedEvents["landing"], "touchdown must publish ALDT")
			require.Positive(t, observedEvents["bay:"+shared.BAY_TWY_ARR], "runway vacation must be observed")
		}
		if arrivals < 200 {
			require.Positive(t, observedEvents["bay:"+shared.BAY_AIRBORNE], "departure airborne transition must be observed")
			require.Positive(t, observedEvents["stand:"+services.StageDepartureBlock], "stand reservation must activate")
			require.Positive(t, observedEvents["stand_assignment_removed"], "push must release a stand assignment")
		}
	}
	require.NotEmpty(t, times)
	if os.Getenv("POSITION_LOAD_ENFORCE") != "false" {
		require.LessOrEqual(t, percentile(times, .95), 20.)
		require.LessOrEqual(t, percentile(times, .99), 50.)
	}
}

func loadTopology() string {
	if value := os.Getenv("POSITION_LOAD_TOPOLOGY"); value != "" {
		return value
	}
	return "isolated local Docker PostgreSQL; Docker-managed storage; Windows loopback; CPU and storage not production-equivalent"
}

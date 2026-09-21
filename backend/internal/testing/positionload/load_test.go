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
	"strings"
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
	End          time.Time
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
	batches           []batchSample
	queryDurations    map[trace.TraceID][]batchSample
	positionQueries   []batchSample
	operations        []batchSample
}

type batchSample struct {
	At       time.Time
	Kind     string
	Size, MS float64
	Queries  int
}

func (c *capture) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range spans {
		if strings.HasPrefix(s.Name(), "position.batch.") {
			entry := batchSample{At: s.StartTime(), Kind: s.Name(), MS: float64(s.EndTime().Sub(s.StartTime())) / float64(time.Millisecond)}
			for _, a := range s.Attributes() {
				if string(a.Key) == "position.batch.size" {
					entry.Size = float64(a.Value.AsInt64())
				}
			}
			c.batches = append(c.batches, entry)
		}
		id := s.SpanContext().TraceID()
		if s.Name() == "pool.acquire" {
			c.pools[id] += float64(s.EndTime().Sub(s.StartTime())) / float64(time.Millisecond)
		}
		if len(s.Name()) >= 5 && s.Name()[:5] == "query" {
			c.queries[id]++
			kind := "other"
			for _, a := range s.Attributes() {
				if string(a.Key) != "db.statement" && string(a.Key) != "db.query.text" {
					continue
				}
				sql := a.Value.AsString()
				switch {
				case strings.Contains(sql, "position batch AMAN identity"):
					kind = "batch_aman_identity"
				case strings.Contains(sql, "GetActiveAMANVATSIMObservationIdentity"):
					kind = "aman_identity"
				case strings.Contains(sql, "position batch snapshot"):
					kind = "batch_snapshot"
				case strings.Contains(sql, "position batch persistence"):
					kind = "batch_update"
				}
			}
			if c.queryDurations == nil {
				c.queryDurations = make(map[trace.TraceID][]batchSample)
			}
			c.queryDurations[id] = append(c.queryDurations[id], batchSample{At: s.StartTime(), Kind: kind, MS: float64(s.EndTime().Sub(s.StartTime())) / float64(time.Millisecond)})
		}
		if s.Name() == "aircraft_position_update" {
			c.completed++
			c.positionQueries = append(c.positionQueries, c.queryDurations[id]...)
			if s.Status().Code == codes.Error {
				c.errors++
			}
			sample := sample{At: s.StartTime(), End: s.EndTime(), MS: float64(s.EndTime().Sub(s.StartTime())) / float64(time.Millisecond), Queries: c.queries[id], PoolMS: c.pools[id]}
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
			if s.Name() != "aircraft_position_update" {
				for _, a := range s.Attributes() {
					if string(a.Key) == "message.processing_ms" {
						c.operations = append(c.operations, batchSample{At: s.StartTime(), Kind: s.Name(), MS: a.Value.AsFloat64(), Queries: c.queries[id]})
						break
					}
				}
			}
			delete(c.queryDurations, id)
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
	pattern := os.Getenv("POSITION_LOAD_PATTERN")
	if pattern == "" {
		pattern = "even"
	}
	require.Contains(t, []string{"even", "second-burst"}, pattern)
	controlPlacement := os.Getenv("POSITION_LOAD_CONTROLS")
	if controlPlacement == "" {
		controlPlacement = "interleaved"
	}
	require.Contains(t, []string{"interleaved", "after-burst"}, controlPlacement)

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
	for n := 0; ; n++ {
		due := start.Add(reportOffset(n, 100, pattern))
		if !due.Before(measureEnd) {
			break
		}
		// Even traffic fills the gaps; ES batches interleave all five controls
		// inside the same one-second burst. Never wait for backend completion.
		for pattern == "even" && nextControl.Before(due) {
			if delay := time.Until(nextControl); delay > 0 {
				time.Sleep(delay)
			}
			require.NoError(t, client.SendProtobuf(&es.HeadingEvent{Callsign: fmt.Sprintf("SAS%03d", sent%200), Heading: int32((sent / 200) % 360)}, es.SetHeading))
			nextControl = nextControl.Add(200 * time.Millisecond)
		}
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

		controls := 0
		if pattern == "even" && !due.Before(nextControl) {
			controls = 1
		}
		if pattern == "second-burst" {
			controls = burstControls(n, controlPlacement)
		}
		for control := 0; control < controls; control++ {
			require.NoError(t, client.SendProtobuf(&es.HeadingEvent{Callsign: fmt.Sprintf("SAS%03d", (sent+control)%200), Heading: int32(cycle % 360)}, es.SetHeading))
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
	steadySent := sent
	// Finish the last one-second interval, including operational traffic.
	for pattern == "even" && nextControl.Before(measureEnd) {
		if delay := time.Until(nextControl); delay > 0 {
			time.Sleep(delay)
		}
		require.NoError(t, client.SendProtobuf(&es.HeadingEvent{Callsign: fmt.Sprintf("SAS%03d", sent%200), Heading: int32((sent / 200) % 360)}, es.SetHeading))
		nextControl = nextControl.Add(200 * time.Millisecond)
	}
	if delay := time.Until(measureEnd); delay > 0 {
		time.Sleep(delay)
	}
	require.Eventually(t, func() bool { return collector.count() == sent }, 5*time.Second, time.Millisecond, "every sent report must complete")
	burstStart := time.Now()
	for n := 0; n < 2000; n++ {
		due := burstStart.Add(reportOffset(n, 200, pattern))
		if wait := time.Until(due); wait > 0 {
			time.Sleep(wait)
		}
		i := n % 200
		require.NoError(t, client.SendProtobuf(&es.AircraftPositionUpdateEvent{Callsign: fmt.Sprintf("SAS%03d", i), Lat: 55.7 + float64(n%50)*.00001, Lon: 12.8, Altitude: 4000}, es.PositionUpdate))
		sent++
	}
	if wait := time.Until(burstStart.Add(10 * time.Second)); wait > 0 {
		time.Sleep(wait)
	}
	drainStart := time.Now()
	burstTarget := sent
	var burstDrain time.Duration
	for n := 0; n < 200; n++ {
		due := drainStart.Add(reportOffset(n, 100, pattern))
		for time.Now().Before(due) {
			if burstDrain == 0 && collector.count() >= burstTarget {
				burstDrain = time.Since(drainStart)
			}
			time.Sleep(min(time.Millisecond, time.Until(due)))
		}
		if burstDrain == 0 && collector.count() >= burstTarget {
			burstDrain = time.Since(drainStart)
		}
		require.NoError(t, client.SendProtobuf(&es.AircraftPositionUpdateEvent{Callsign: fmt.Sprintf("SAS%03d", n), Lat: 55.7 + float64(n%50)*.00001, Lon: 12.8, Altitude: 4000}, es.PositionUpdate))
		sent++
	}
	for time.Since(drainStart) < 2*time.Second {
		if burstDrain == 0 && collector.count() >= burstTarget {
			burstDrain = time.Since(drainStart)
		}
		time.Sleep(time.Millisecond)
	}
	require.NotZero(t, burstDrain, "burst backlog must drain while continuing at 100/sec")
	require.Eventually(t, func() bool { return collector.count() == sent }, 2*time.Second, time.Millisecond)
	collector.mu.Lock()
	// The single socket reader timestamps reports in wire order; workers may
	// finish them out of order. Match that order to the sender's fixed schedule
	// to include waiting in the socket while operational barriers block reads.
	ordered := append([]sample(nil), collector.samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].At.Before(ordered[j].At) })
	var scheduledTimes, batchTimes []float64
	for i := 0; i < steadySent; i++ {
		due := start.Add(reportOffset(i, 100, pattern))
		if !due.Before(measureStart) {
			scheduledTimes = append(scheduledTimes, float64(ordered[i].End.Sub(due))/float64(time.Millisecond))
		}
		if pattern == "second-burst" && i%100 == 99 && !due.Before(measureStart) {
			last := ordered[i].End
			for j := i - 99; j < i; j++ {
				if ordered[j].End.After(last) {
					last = ordered[j].End
				}
			}
			batchTimes = append(batchTimes, float64(last.Sub(due))/float64(time.Millisecond))
		}
	}
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
	batchSamples := append([]batchSample(nil), collector.batches...)
	positionQueries := append([]batchSample(nil), collector.positionQueries...)
	operationalSamples := append([]batchSample(nil), collector.operations...)
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
	// Diagnostic stage time sums are work, not additive end-to-end percentiles.
	for name, entries := range map[string][]batchSample{"position_sql_stages": positionQueries, "other_message_processing": operationalSamples} {
		stages := map[string]map[string]float64{}
		durations := map[string][]float64{}
		for _, entry := range entries {
			if entry.At.Before(measureStart) || !entry.At.Before(measureEnd) {
				continue
			}
			if stages[entry.Kind] == nil {
				stages[entry.Kind] = map[string]float64{}
			}
			stages[entry.Kind]["count"]++
			stages[entry.Kind]["total_ms"] += entry.MS
			if name == "other_message_processing" {
				stages[entry.Kind]["total_queries"] += float64(entry.Queries)
				durations[entry.Kind] = append(durations[entry.Kind], entry.MS)
			}
		}
		for kind, stage := range stages {
			stage["mean_ms"] = stage["total_ms"] / stage["count"]
			if name == "other_message_processing" {
				stage["mean_queries"] = stage["total_queries"] / stage["count"]
				stage["p95_ms"] = percentile(durations[kind], .95)
				stage["p99_ms"] = percentile(durations[kind], .99)
			}
		}
		report[name] = stages
	}
	report["traffic_pattern"] = pattern
	report["control_placement"] = controlPlacement
	report["position_db_batching"] = os.Getenv("POSITION_DB_BATCHING_ENABLED") != "false"
	for _, kind := range []string{"snapshot", "persist", "aman_identity"} {
		var sizes, durations []float64
		for _, entry := range batchSamples {
			if entry.Kind == "position.batch."+kind && !entry.At.Before(measureStart) && entry.At.Before(measureEnd) {
				sizes = append(sizes, entry.Size)
				durations = append(durations, entry.MS)
			}
		}
		report["batch_"+kind+"_groups"] = len(sizes)
		report["batch_"+kind+"_mean_size"] = average(sizes)
		report["batch_"+kind+"_max_size"] = percentile(sizes, 1)
		report["batch_"+kind+"_mean_ms"] = average(durations)
	}
	report["p95_scheduled_completion_ms"] = percentile(scheduledTimes, .95)
	report["p99_scheduled_completion_ms"] = percentile(scheduledTimes, .99)
	if pattern == "second-burst" {
		report["operational_pattern"] = "five controls interleaved within each one-second position batch"
		if controlPlacement == "after-burst" {
			report["operational_pattern"] = "five controls after the 100 positions in each one-second burst; lifecycle controls retain their original order"
		}
		report["p95_batch_completion_ms"] = percentile(batchTimes, .95)
		report["max_batch_completion_ms"] = percentile(batchTimes, 1)
	}
	report["average_queue_ms"] = average(queueTimes)
	report["average_processing_ms"] = average(processingTimes)
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
	if pattern == "second-burst" {
		require.Less(t, percentile(batchTimes, 1), 1000., "each measured batch must complete before the next second")
	}
	if os.Getenv("POSITION_LOAD_ENFORCE") != "false" {
		require.LessOrEqual(t, percentile(times, .95), 20.)
		require.LessOrEqual(t, percentile(times, .99), 50.)
		require.LessOrEqual(t, percentile(scheduledTimes, .95), 20., "include socket/barrier waiting from the sender deadline")
		require.LessOrEqual(t, percentile(scheduledTimes, .99), 50., "include socket/barrier waiting from the sender deadline")
	}
}

// A batch shares one deadline: serial socket writes do not deliberately space
// its reports out. Aircraft alternate between the two halves of the 200 fleet.
func reportOffset(n, rate int, pattern string) time.Duration {
	if pattern == "second-burst" {
		return time.Duration(n/rate) * time.Second
	}
	return time.Duration(n) * time.Second / time.Duration(rate)
}

func TestReportSchedule(t *testing.T) {
	for _, tc := range []struct {
		n, rate     int
		batch, even time.Duration
	}{
		{0, 100, 0, 0},
		{99, 100, 0, 990 * time.Millisecond},
		{100, 100, time.Second, time.Second},
		{199, 100, time.Second, 1990 * time.Millisecond},
		{200, 100, 2 * time.Second, 2 * time.Second},
		{199, 200, 0, 995 * time.Millisecond},
		{200, 200, time.Second, time.Second},
		{1999, 200, 9 * time.Second, 9995 * time.Millisecond},
	} {
		require.Equal(t, tc.batch, reportOffset(tc.n, tc.rate, "second-burst"))
		require.Equal(t, tc.even, reportOffset(tc.n, tc.rate, "even"))
	}
	for _, rate := range []int{100, 200} {
		seen := make(map[time.Duration]map[int]bool)
		for n := 0; n < rate*3; n++ {
			tick, aircraft := reportOffset(n, rate, "second-burst"), n%200
			if seen[tick] == nil {
				seen[tick] = make(map[int]bool)
			}
			require.False(t, seen[tick][aircraft], "an aircraft must not report twice in one ES tick")
			seen[tick][aircraft] = true
		}
		for _, aircraft := range seen {
			require.Len(t, aircraft, rate)
		}
	}
}

func loadTopology() string {
	if value := os.Getenv("POSITION_LOAD_TOPOLOGY"); value != "" {
		return value
	}
	return "isolated local Docker PostgreSQL; Docker-managed storage; Windows loopback; CPU and storage not production-equivalent"
}

// Both variants send five controls in the same second; only their wire order differs.
func burstControls(n int, placement string) int {
	if placement == "after-burst" {
		if (n+1)%100 == 0 {
			return 5
		}
		return 0
	}
	if (n+1)%20 == 0 {
		return 1
	}
	return 0
}

func TestBurstControlOrder(t *testing.T) {
	for _, placement := range []string{"interleaved", "after-burst"} {
		for second := 0; second < 3; second++ {
			total := 0
			for i := 0; i < 100; i++ {
				n := burstControls(second*100+i, placement)
				total += n
				if placement == "after-burst" && i < 99 {
					require.Zero(t, n)
				}
				if placement == "interleaved" && (i+1)%20 != 0 {
					require.Zero(t, n)
				}
			}
			require.Equal(t, 5, total)
		}
	}
}

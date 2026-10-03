package main

import (
	"FlightStrips/internal/sat"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type loadFleet struct {
	mu        sync.Mutex
	strips    []*es.Strip
	stands    []sat.Stand
	positions []*es.AircraftPositionUpdateEvent
	arrivals  int
	provider  *httptest.Server
}

func newLoadFleet(t *testing.T, arrivals int) *loadFleet {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	registry, err := sat.LoadStandCapabilityFile(filepath.Join(root, "config", "ekch", "GRpluginStands.txt"))
	require.NoError(t, err)
	f := &loadFleet{arrivals: arrivals, stands: registry.Stands("EKCH"), strips: make([]*es.Strip, 200), positions: make([]*es.AircraftPositionUpdateEvent, 200)}
	require.Greater(t, len(f.stands), 20)
	for i := range f.strips {
		origin, destination, runway, ground := "EKCH", "ESSA", "22R", "TAXI"
		if i < arrivals {
			origin, destination, runway = "ESSA", "EKCH", "22L"
		} else if i%10 == 0 {
			ground = ""
		}
		f.strips[i] = &es.Strip{Callsign: fmt.Sprintf("SAS%03d", i), Origin: origin, Destination: destination, AircraftType: "B738", Route: "SOK KEMAX", AssignedSquawk: "1001", Runway: runway, HasFp: true, Cleared: true, GroundState: ground, Stand: f.stands[i%len(f.stands)].Name}
		p, _ := f.position(i, 0)
		f.positions[i] = p
	}
	f.provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"v3": []string{f.provider.URL + "/network"}}})
			return
		}
		if r.URL.Path != "/network" {
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		pilots := make([]any, 200)
		now := time.Now().UTC()
		for i, s := range f.strips {
			p := f.positions[i]
			pilots[i] = map[string]any{"cid": 800000 + i, "callsign": s.Callsign, "latitude": p.Lat, "longitude": p.Lon, "altitude": p.Altitude, "groundspeed": 0, "logon_time": now.Add(-time.Hour).Format(time.RFC3339), "last_updated": now.Format(time.RFC3339), "flight_plan": map[string]any{"flight_rules": "I", "aircraft_short": "B738", "departure": s.Origin, "arrival": s.Destination, "deptime": now.Add(30 * time.Minute).Format("1504"), "route": s.Route, "altitude": "35000", "revision_id": 1}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"general": map[string]any{"update_timestamp": now.Format(time.RFC3339Nano)}, "pilots": pilots, "prefiles": []any{}})
	}))
	t.Cleanup(f.provider.Close)
	return f
}

// Retains the original staggered touchdown/vacation/stand and push/departure
// paths. Changing a workload to avoid lifecycle commits is not qualification.
func (f *loadFleet) position(i, cycle int) (*es.AircraftPositionUpdateEvent, string) {
	lat, lon, alt := 55.63+float64(i%20)*.0001, 12.65+float64(cycle%50)*.00002, int64(20)
	ground := ""
	if i < f.arrivals {
		lat, lon, alt = 55.85-float64(cycle%100)*.0003, 12.95-float64(cycle%100)*.0003, 5000-int64(cycle%100)*20
	}
	if i < f.arrivals && i%10 == 0 {
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
			lat, lon, alt = f.stands[i%len(f.stands)].Latitude, f.stands[i%len(f.stands)].Longitude, 20
		}
	}
	if i >= f.arrivals && i%10 == 0 {
		phase := cycle - ((i-f.arrivals)/10)*25
		if phase < 15 {
			lat, lon = f.stands[i%len(f.stands)].Latitude, f.stands[i%len(f.stands)].Longitude
		} else if phase < 25 {
			lat, lon = 55.63, 12.65
		} else {
			lat, lon, alt = 55.65+float64(cycle)*.0001, 12.68, 3000
		}
		if phase == 15 {
			ground = "PUSH"
		}
		if phase == 25 {
			ground = "DEPA"
		}
	}
	p := &es.AircraftPositionUpdateEvent{Callsign: fmt.Sprintf("SAS%03d", i), Lat: lat, Lon: lon, Altitude: alt}
	return p, ground
}

func (f *loadFleet) observe(i int, p *es.AircraftPositionUpdateEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.positions[i] = p
}

type loadLifecycle struct {
	stop, done chan struct{}
	counts     map[string]int
}

func (f *entrypointFixture) watchLoadLifecycle(ref *pb.AggregateRef) *loadLifecycle {
	l := &loadLifecycle{stop: make(chan struct{}), done: make(chan struct{}), counts: map[string]int{}}
	go func() {
		defer close(l.done)
		previous := map[string]string{}
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-l.stop:
				return
			case <-ticker.C:
			}
			s, err := f.projection.ReadEntityKinds(ref, pb.EntityKind_STRIP, pb.EntityKind_STAND_ASSIGNMENT)
			if err != nil {
				continue
			}
			current := map[string]string{}
			for _, e := range s.Indexes[pb.EntityKind_STRIP] {
				strip := e.Value.GetStrip()
				if strip.Aldt != nil {
					l.counts["landing"]++
				}
				if strip.Bay == "TWY_ARR" || strip.Bay == "AIRBORNE" {
					l.counts["bay:"+strip.Bay]++
				}
			}
			for key, e := range s.Indexes[pb.EntityKind_STAND_ASSIGNMENT] {
				stage := e.Value.GetStandAssignment().Stage
				current[key] = stage
				if stage == "DEPARTURE_BLOCK" {
					l.counts["stand:DEPARTURE_BLOCK"]++
				}
			}
			for key := range previous {
				if current[key] == "" {
					l.counts["stand_assignment_removed"]++
				}
			}
			previous = current
		}
	}()
	return l
}
func (l *loadLifecycle) finish() map[string]int { close(l.stop); <-l.done; return l.counts }

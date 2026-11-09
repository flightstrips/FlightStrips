// Package viff acquires flight route geometry from vIFF's bulk arrivals feed.
// The feed supplies geometry, not AMAN timing, terminal policy or AIRAC identity.
package viff

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
)

const (
	DefaultBaseURL  = "https://api.viffsys.com"
	RefreshInterval = 30 * time.Second
	ResolverVersion = "viff-route-v3"
)

type Config struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	References navdata.GeometrySnapshotReader
	Now        func() time.Time
}

type Adapter struct {
	baseURL    string
	apiKey     string
	client     *http.Client
	references navdata.GeometrySnapshotReader
	now        func() time.Time
	mu         sync.Mutex
	airports   map[navdata.AirportID]*airportFeed
}

type airportFeed struct {
	loading   chan struct{}
	nextFetch time.Time
	fetchedAt time.Time
	flights   []flight
	data      []byte
	err       error
}

type flight struct {
	Callsign  string  `json:"callsign"`
	Departure string  `json:"departure"`
	Arrival   string  `json:"arrival"`
	Path      []point `json:"path"`
}

// A malformed path belongs to one flight. Keep the original bulk payload for
// timing consumers and let that flight fail geometry validation independently.
func (f *flight) UnmarshalJSON(data []byte) error {
	var row struct {
		Callsign  string          `json:"callsign"`
		Departure string          `json:"departure"`
		Arrival   string          `json:"arrival"`
		Path      json.RawMessage `json:"path"`
	}
	if err := json.Unmarshal(data, &row); err != nil {
		return err
	}
	f.Callsign, f.Departure, f.Arrival = row.Callsign, row.Departure, row.Arrival
	if json.Unmarshal(row.Path, &f.Path) != nil {
		f.Path = nil
	}
	return nil
}

type point struct {
	Name string   `json:"name"`
	Lat  *float64 `json:"lat"`
	Lon  *float64 `json:"lon"`
}

func New(c Config) (*Adapter, error) {
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("vIFF base URL is invalid")
	}
	if c.References == nil {
		return nil, fmt.Errorf("vIFF requires cached terminal references")
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return &Adapter{baseURL: strings.TrimRight(c.BaseURL, "/"), apiKey: c.APIKey, client: c.HTTPClient, references: c.References, now: c.Now, airports: make(map[navdata.AirportID]*airportFeed)}, nil
}

// arrivals shares one bounded request per airport across all flights and sessions.
// Failed requests are also paced; missing flights never trigger per-flight HTTP.
func (a *Adapter) arrivals(ctx context.Context, airport navdata.AirportID) ([]flight, time.Time, []byte, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, time.Time{}, nil, err
		}
		a.mu.Lock()
		entry := a.airports[airport]
		if entry == nil {
			entry = &airportFeed{}
			a.airports[airport] = entry
		}
		if entry.loading != nil {
			done := entry.loading
			a.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, time.Time{}, nil, ctx.Err()
			case <-done:
				continue
			}
		}
		if a.now().Before(entry.nextFetch) {
			flights, at, data, err := entry.flights, entry.fetchedAt, entry.data, entry.err
			if limited, ok := err.(*feedError); ok && limited.retryAfter > 0 {
				err = &feedError{error: limited.error, retryAfter: entry.nextFetch.Sub(a.now())}
			}
			a.mu.Unlock()
			return flights, at, data, err
		}
		entry.loading = make(chan struct{})
		done := entry.loading
		a.mu.Unlock()
		flights, data, err := a.fetch(ctx, airport)
		at := a.now().UTC()
		a.mu.Lock()
		entry.flights, entry.fetchedAt, entry.err = flights, at, err
		entry.data = data
		entry.nextFetch = at.Add(RefreshInterval)
		if backoff, ok := err.(interface{ RetryDelay() time.Duration }); ok && backoff.RetryDelay() > RefreshInterval {
			entry.nextFetch = at.Add(backoff.RetryDelay())
		}
		entry.loading = nil
		close(done)
		a.mu.Unlock()
		return flights, at, data, err
	}
}

// Arrivals exposes the same cached response to CDM's existing inbound poller.
// Returning a copy keeps consumers from mutating the shared route feed.
func (a *Adapter) Arrivals(ctx context.Context, airport string) ([]byte, error) {
	_, _, data, err := a.arrivals(ctx, navdata.AirportID(upper(airport)))
	return bytes.Clone(data), err
}

func (a *Adapter) fetch(ctx context.Context, airport navdata.AirportID) ([]flight, []byte, error) {
	u := a.baseURL + "/ifps/arrAirport?" + url.Values{"airport": {string(airport)}, "path": {"true"}}.Encode()
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, problem("construct vIFF arrivals request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "FlightStrips-AMAN/1.0 (+https://github.com/flightstrips/FlightStrips)")
	if a.apiKey != "" {
		req.Header.Set("x-api-key", a.apiKey)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, problem("vIFF arrivals request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var retryAfter time.Duration
		if seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil && seconds > 0 {
			retryAfter = time.Duration(seconds) * time.Second
		} else if until, parseErr := http.ParseTime(resp.Header.Get("Retry-After")); parseErr == nil {
			retryAfter = until.Sub(a.now())
		}
		return nil, nil, &feedError{error: problem(fmt.Sprintf("vIFF arrivals returned HTTP %d", resp.StatusCode)), retryAfter: retryAfter}
	}
	const limit = 16 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(data) > limit {
		return nil, nil, problem("read vIFF arrivals response")
	}
	var flights []flight
	if json.Unmarshal(data, &flights) != nil || flights == nil {
		return nil, nil, problem("vIFF arrivals response must be an array")
	}
	return flights, data, nil
}

func (a *Adapter) Resolve(ctx context.Context, q navdata.RouteQuery) (navdata.RouteGeometry, error) {
	if err := q.Validate(); err != nil {
		return navdata.RouteGeometry{}, err
	}
	if q.Callsign == "" || q.TerminalEntry == nil {
		return navdata.RouteGeometry{}, problem("vIFF route requires callsign and selected terminal entry")
	}
	snapshot, err := a.references.ActiveGeometrySnapshot(ctx, q.Destination)
	if err != nil {
		return navdata.RouteGeometry{}, err
	}
	now := a.now().UTC()
	if !snapshot.Manifest.Version.Equal(q.Version) || now.Before(q.Version.EffectiveFrom) || !now.Before(q.Version.EffectiveUntil) {
		return navdata.RouteGeometry{}, problem("vIFF route requires current cached terminal dataset")
	}
	rows, at, _, err := a.arrivals(ctx, q.Destination)
	if err != nil {
		return navdata.RouteGeometry{}, err
	}
	var selected *flight
	for i := range rows {
		if upper(rows[i].Callsign) == string(q.Callsign) {
			if selected != nil {
				return navdata.RouteGeometry{}, problem("vIFF arrivals contains duplicate callsign")
			}
			selected = &rows[i]
		}
	}
	if selected == nil {
		return navdata.RouteGeometry{}, problem("flight is not yet present in vIFF arrivals")
	}
	if upper(selected.Departure) != string(q.Origin) || upper(selected.Arrival) != string(q.Destination) {
		return navdata.RouteGeometry{}, problem("vIFF flight origin or destination does not match observation")
	}
	return geometry(q, *selected, snapshot, at)
}

var identifier = regexp.MustCompile(`^[A-Z0-9_-]+$`)
var qualifiedIdentifier = regexp.MustCompile(`^([A-Z0-9_-]+)\([A-Z0-9_-]+\)$`)

// Parenthetical provider qualifiers are not part of waypoint identifiers.
func canonicalPointName(name string) string {
	name = upper(name)
	if match := qualifiedIdentifier.FindStringSubmatch(name); match != nil {
		return match[1]
	}
	return name
}

func geometry(q navdata.RouteQuery, f flight, snapshot navdata.ActiveGeometrySnapshot, at time.Time) (navdata.RouteGeometry, error) {
	bad := func(message string) (navdata.RouteGeometry, error) { return navdata.RouteGeometry{}, problem(message) }
	if len(f.Path) < 2 || len(f.Path) > 4096 {
		return bad("vIFF route path is missing or too large")
	}
	// vIFF's climb/descent markers are profile metadata, not route fixes.
	// Filter into a new slice so shared arrivals and CDM retain the raw feed.
	path := make([]point, 0, len(f.Path))
	for _, p := range f.Path {
		p.Name = canonicalPointName(p.Name)
		if p.Name != "T_O_D" && p.Name != "T_O_C" {
			path = append(path, p)
		}
	}
	f.Path = path
	if len(f.Path) < 2 {
		return bad("vIFF route path is missing or too large")
	}
	if upper(f.Path[0].Name) != string(q.Origin) || upper(f.Path[len(f.Path)-1].Name) != string(q.Destination) {
		return bad("vIFF route path airport endpoints do not match flight")
	}
	positions := make([]navdata.Coordinate, len(f.Path))
	entry := -1
	for i, p := range f.Path {
		if p.Lat == nil || p.Lon == nil {
			return bad("vIFF route point has no coordinates")
		}
		positions[i] = navdata.Coordinate{LatitudeDeg: *p.Lat, LongitudeDeg: *p.Lon}
		if positions[i].Validate() != nil || (*p.Lat == 0 && *p.Lon == 0) {
			return bad("vIFF route point has invalid coordinates")
		}
		if p.Name == string(*q.TerminalEntry) {
			if entry >= 0 {
				return bad("vIFF terminal entry is ambiguous")
			}
			entry = i
		}
	}
	if entry < 1 {
		return bad("vIFF path does not reach selected terminal entry")
	}
	entryKnown := false
	for _, fix := range snapshot.Fixes {
		if fix.ID == *q.TerminalEntry {
			entryKnown = true
			if navdata.CoordinateDistanceNM(fix.Position, positions[entry]) > 0.5 {
				return bad("vIFF terminal entry differs from cached terminal geometry")
			}
		}
	}
	if !entryKnown {
		return bad("selected terminal entry is absent from cached references")
	}
	// vIFF supplies the enroute path. Filed ITEM15 can differ from its resolved
	// route and must not veto otherwise usable geometry to the arrival entry.
	g := navdata.RouteGeometry{Version: q.Version, Coverage: navdata.CoverageComplete, Provenance: navdata.Provenance{SourceID: "viff", SourceRevision: ResolverVersion, ImportedAt: at, EffectiveFrom: q.Version.EffectiveFrom, EffectiveUntil: q.Version.EffectiveUntil}}
	// Stop at the STAR entry. vIFF's selected STAR/runway and closing airport
	// segment must never override or duplicate AMAN's selected terminal path.
	type node struct {
		id       navdata.FixID
		position navdata.Coordinate
	}
	nodes := make([]node, 0, entry+1)
	for i := 0; i <= entry; i++ {
		n := node{id: pointID(f.Path[i].Name, i), position: positions[i]}
		if i == entry {
			// The terminal composer needs the canonical fix at the boundary.
			n.id = *q.TerminalEntry
		}
		if len(nodes) > 0 && navdata.CoordinateDistanceNM(nodes[len(nodes)-1].position, n.position) < 0.000001 {
			// Co-located generated profile points do not create zero-length legs
			// or disconnect named fixes. Always preserve the selected entry name.
			if i == entry || strings.HasPrefix(string(nodes[len(nodes)-1].id), "VIFF-POINT-") {
				nodes[len(nodes)-1] = n
			}
			continue
		}
		nodes = append(nodes, n)
	}
	for i := 0; i+1 < len(nodes); i++ {
		from, to := nodes[i].position, nodes[i+1].position
		distance := navdata.CoordinateDistanceNM(from, to)
		fromID, toID := nodes[i].id, nodes[i+1].id
		lat1, lat2 := from.LatitudeDeg*math.Pi/180, to.LatitudeDeg*math.Pi/180
		dlon := (to.LongitudeDeg - from.LongitudeDeg) * math.Pi / 180
		course := math.Mod(math.Atan2(math.Sin(dlon)*math.Cos(lat2), math.Cos(lat1)*math.Sin(lat2)-math.Sin(lat1)*math.Cos(lat2)*math.Cos(dlon))*180/math.Pi+360, 360)
		g.Legs = append(g.Legs, navdata.ProcedureLeg{ID: fmt.Sprintf("VIFF-%04d", i+1), PathTerminator: navdata.PathTF, FromFix: &fromID, ToFix: &toID, FromPosition: &from, ToPosition: &to, DistanceNM: &distance, CourseTrueDeg: &course})
		g.TotalDistanceNM += distance
	}
	if len(g.Legs) == 0 || len(navdata.SuspectRouteDetours(g.Legs)) > 0 {
		return bad("vIFF route geometry is empty or contains an implausible detour")
	}
	var err error
	g.Digest, err = navdata.RouteGeometryDigest(q, g)
	return g, err
}

func pointID(name string, index int) navdata.FixID {
	name = upper(name)
	if identifier.MatchString(name) {
		return navdata.FixID(name)
	}
	return navdata.FixID(fmt.Sprintf("VIFF-POINT-%04d", index))
}

func upper(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
func problem(message string) error {
	return &aman.DomainError{Class: navdata.ErrorSourceUnavailable, Message: message}
}

type feedError struct {
	error
	retryAfter time.Duration
}

func (e *feedError) RetryDelay() time.Duration { return e.retryAfter }
func (e *feedError) Unwrap() error             { return e.error }

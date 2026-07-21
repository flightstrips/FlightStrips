// Package airacnet adapts the public AIRAC.NET HTTP API to the provider-neutral
// AMAN navigation contracts.  HTTP DTOs and checkpoint state intentionally stay
// in this package.
package airacnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultBaseURL = "https://airac.net/api/v1"

// Checkpoint is adapter-private HTTP state. It is deliberately not embedded in
// canonical navigation values or their digests.
type Checkpoint struct {
	ETag         string
	LastModified string
	NextPage     int
	Body         []byte
}

type CheckpointStore interface {
	Load(context.Context, string) (Checkpoint, bool, error)
	Save(context.Context, string, Checkpoint) error
}

// PostgresCheckpoints is the durable production checkpoint implementation.
// Its table and raw response body are source-adapter state, never canonical
// AMAN cache data.
type PostgresCheckpoints struct{ db checkpointDB }

type checkpointDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func NewPostgresCheckpoints(pool *pgxpool.Pool) *PostgresCheckpoints {
	return &PostgresCheckpoints{db: pool}
}

func (s *PostgresCheckpoints) Load(ctx context.Context, key string) (Checkpoint, bool, error) {
	var value Checkpoint
	err := s.db.QueryRow(ctx, `SELECT etag, last_modified, next_page, response_body FROM airacnet_http_checkpoints WHERE request_key = $1`, key).Scan(&value.ETag, &value.LastModified, &value.NextPage, &value.Body)
	if errors.Is(err, pgx.ErrNoRows) {
		return Checkpoint{}, false, nil
	}
	if err != nil {
		return Checkpoint{}, false, err
	}
	return value, true, nil
}

func (s *PostgresCheckpoints) Save(ctx context.Context, key string, value Checkpoint) error {
	_, err := s.db.Exec(ctx, `INSERT INTO airacnet_http_checkpoints (request_key, etag, last_modified, next_page, response_body, updated_at) VALUES ($1, $2, $3, $4, $5, now()) ON CONFLICT (request_key) DO UPDATE SET etag = EXCLUDED.etag, last_modified = EXCLUDED.last_modified, next_page = EXCLUDED.next_page, response_body = EXCLUDED.response_body, updated_at = EXCLUDED.updated_at`, key, value.ETag, value.LastModified, value.NextPage, value.Body)
	return err
}

type MemoryCheckpoints struct {
	mu     sync.Mutex
	values map[string]Checkpoint
}

func NewMemoryCheckpoints() *MemoryCheckpoints {
	return &MemoryCheckpoints{values: map[string]Checkpoint{}}
}
func (s *MemoryCheckpoints) Load(_ context.Context, key string) (Checkpoint, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	value.Body = slices.Clone(value.Body)
	return value, ok, nil
}
func (s *MemoryCheckpoints) Save(_ context.Context, key string, value Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	value.Body = slices.Clone(value.Body)
	s.values[key] = value
	return nil
}

type Config struct {
	BaseURL       string
	HTTPClient    *http.Client
	Timeout       time.Duration
	MaxConcurrent int
	Retries       int
	RetryBackoff  time.Duration
	UserAgent     string
	Checkpoints   CheckpointStore
	Now           func() time.Time
}

// Adapter implements the acquisition interfaces. It is never a runtime
// GeometryReader: materializers persist its output to the canonical cache.
type Adapter struct {
	baseURL      *url.URL
	client       *http.Client
	timeout      time.Duration
	retries      int
	backoff      time.Duration
	userAgent    string
	checkpoints  CheckpointStore
	now          func() time.Time
	requests     chan struct{}
	requestMaker func(context.Context, string, string, io.Reader) (*http.Request, error)
}

func New(config Config) (*Adapter, error) {
	base := config.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("AIRAC.NET base URL is invalid")
	}
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 4
	}
	if config.Retries < 0 {
		return nil, fmt.Errorf("AIRAC.NET retries cannot be negative")
	}
	if config.RetryBackoff <= 0 {
		config.RetryBackoff = 50 * time.Millisecond
	}
	if config.UserAgent == "" {
		config.UserAgent = "FlightStrips-AMAN/1.0 (+https://github.com/flightstrips/FlightStrips)"
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{}
	}
	if config.Checkpoints == nil {
		return nil, fmt.Errorf("AIRAC.NET checkpoint store is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Adapter{baseURL: parsed, client: config.HTTPClient, timeout: config.Timeout, retries: config.Retries, backoff: config.RetryBackoff, userAgent: config.UserAgent, checkpoints: config.Checkpoints, now: config.Now, requests: make(chan struct{}, config.MaxConcurrent), requestMaker: http.NewRequestWithContext}, nil
}

func (a *Adapter) LatestVersion(ctx context.Context) (navdata.DatasetVersion, error) {
	var result cycleResponse
	if err := a.getJSON(ctx, "airac/current", nil, &result); err != nil {
		return navdata.DatasetVersion{}, err
	}
	from, err := parseTime(result.Data.EffectiveDate)
	if err != nil {
		return navdata.DatasetVersion{}, invalid("AIRAC.NET cycle effective_date")
	}
	until, err := parseTime(result.Data.ExpirationDate)
	if err != nil {
		return navdata.DatasetVersion{}, invalid("AIRAC.NET cycle expiration_date")
	}
	version := navdata.DatasetVersion{Cycle: upper(result.Data.Cycle), SourceRevision: "airac.net-" + upper(result.Data.Cycle), EffectiveFrom: from, EffectiveUntil: until}
	if err := version.Validate(); err != nil {
		return navdata.DatasetVersion{}, invalid("AIRAC.NET cycle response")
	}
	return version, nil
}

func (a *Adapter) Airport(ctx context.Context, version navdata.DatasetVersion, id navdata.AirportID) (navdata.Airport, error) {
	if err := version.Validate(); err != nil {
		return navdata.Airport{}, err
	}
	if err := a.matchVersion(ctx, version); err != nil {
		return navdata.Airport{}, err
	}
	var response airportResponse
	if err := a.getJSON(ctx, "airports/"+url.PathEscape(string(id)), nil, &response); isHTTPStatus(err, http.StatusNotFound) {
		return navdata.Airport{}, notFound("AIRAC.NET airport was not found")
	} else if err != nil {
		return navdata.Airport{}, err
	}
	airport, err := a.airport(version, response.Data)
	if err != nil {
		return navdata.Airport{}, err
	}
	if airport.ID != id {
		return navdata.Airport{}, invalid("AIRAC.NET airport identifier mismatch")
	}
	return airport, nil
}

func (a *Adapter) Procedures(ctx context.Context, query navdata.ProcedureQuery) (navdata.ProcedureSet, error) {
	if err := query.Validate(); err != nil {
		return navdata.ProcedureSet{}, err
	}
	if err := a.matchVersion(ctx, query.Version); err != nil {
		return navdata.ProcedureSet{}, err
	}
	listing, err := a.listProcedures(ctx, query)
	if err != nil {
		return navdata.ProcedureSet{}, err
	}
	values := make([]navdata.Procedure, len(listing))
	partial := false
	var partialMu sync.Mutex
	if err := a.parallel(ctx, len(listing), func(ctx context.Context, index int) error {
		procedure, incomplete, err := a.procedure(ctx, query.Version, listing[index])
		if err != nil {
			return err
		}
		values[index] = procedure
		if incomplete {
			partialMu.Lock()
			partial = true
			partialMu.Unlock()
		}
		return nil
	}); err != nil {
		return navdata.ProcedureSet{}, err
	}
	if len(values) == 0 {
		partial = true
	}
	coverage := navdata.CoverageComplete
	if partial {
		coverage = navdata.CoveragePartial
	}
	result := navdata.ProcedureSet{Version: query.Version, Airport: query.Airport, Procedures: values, Coverage: coverage, Provenance: a.provenance(query.Version)}
	if err := result.Validate(); err != nil {
		return navdata.ProcedureSet{}, invalid("AIRAC.NET procedure response")
	}
	return result, nil
}

func (a *Adapter) Fixes(ctx context.Context, query navdata.FixQuery) (navdata.FixSet, error) {
	if err := query.Validate(); err != nil {
		return navdata.FixSet{}, err
	}
	if err := a.matchVersion(ctx, query.Version); err != nil {
		return navdata.FixSet{}, err
	}
	values := make([]navdata.Fix, len(query.Identifiers))
	found := make([]bool, len(query.Identifiers))
	if err := a.parallel(ctx, len(query.Identifiers), func(ctx context.Context, index int) error {
		var response waypointResponse
		err := a.getJSON(ctx, "waypoints/"+url.PathEscape(string(query.Identifiers[index])), nil, &response)
		var status *httpStatusError
		if errors.As(err, &status) && status.status == http.StatusNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		item, ok, err := firstWaypoint(response.Data)
		if err != nil {
			return incomplete("AIRAC.NET waypoint response is ambiguous")
		}
		if !ok {
			return nil
		}
		fix, err := a.fix(query.Version, item)
		if err != nil {
			return err
		}
		if fix.ID != query.Identifiers[index] {
			return nil
		}
		values[index], found[index] = fix, true
		return nil
	}); err != nil {
		return navdata.FixSet{}, err
	}
	fixes := make([]navdata.Fix, 0, len(values))
	for index, value := range values {
		if found[index] {
			fixes = append(fixes, value)
		}
	}
	coverage := navdata.CoverageComplete
	if len(fixes) != len(query.Identifiers) {
		coverage = navdata.CoveragePartial
	}
	result := navdata.FixSet{Version: query.Version, Fixes: fixes, Coverage: coverage, Provenance: a.provenance(query.Version)}
	if err := result.Validate(); err != nil {
		return navdata.FixSet{}, invalid("AIRAC.NET waypoint response")
	}
	return result, nil
}

func (a *Adapter) Resolve(ctx context.Context, query navdata.RouteQuery) (navdata.RouteGeometry, error) {
	if err := query.Validate(); err != nil {
		return navdata.RouteGeometry{}, err
	}
	if err := a.matchVersion(ctx, query.Version); err != nil {
		return navdata.RouteGeometry{}, err
	}
	// The key is deliberately calculated before AIRAC.NET's DCT/STAR workaround.
	if _, err := query.Key(); err != nil {
		return navdata.RouteGeometry{}, err
	}
	route := transformedRoute(query)
	if route == "" {
		return navdata.RouteGeometry{}, invalid("AIRAC.NET route is empty after DCT removal")
	}
	parameters := url.Values{"origin": {string(query.Origin)}, "destination": {string(query.Destination)}, "route": {route}}
	if query.Runway != nil {
		parameters.Set("arrival_runway", string(*query.Runway))
	}
	var response routeResponse
	if err := a.getJSON(ctx, "routes/parse", parameters, &response); err != nil {
		return navdata.RouteGeometry{}, err
	}
	geometry, err := a.routeGeometry(query, response.Data)
	if err != nil {
		return navdata.RouteGeometry{}, err
	}
	return geometry, nil
}

func (a *Adapter) listProcedures(ctx context.Context, query navdata.ProcedureQuery) ([]procedureListItem, error) {
	queries := []url.Values{{"airport": {string(query.Airport)}}}
	if len(query.Kinds) > 0 {
		queries = queries[:0]
		for _, kind := range query.Kinds {
			queries = append(queries, url.Values{"airport": {string(query.Airport)}, "type": {vendorKind(kind)}})
		}
	}
	if len(query.Identifiers) > 0 {
		queries = queries[:0]
		for _, identifier := range query.Identifiers {
			for _, kind := range kindsOrAll(query.Kinds) {
				queries = append(queries, url.Values{"airport": {string(query.Airport)}, "identifier": {string(identifier)}, "type": {vendorKind(kind)}})
			}
		}
	}
	result := make([]procedureListItem, 0)
	seen := map[string]struct{}{}
	for _, parameters := range queries {
		if len(query.Runways) == 0 {
			items, err := a.listProcedurePage(ctx, parameters)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if matchesListing(item, query) {
					key := item.Airport + "/" + item.Identifier + "/" + item.Type.Code
					if _, ok := seen[key]; !ok {
						seen[key] = struct{}{}
						result = append(result, item)
					}
				}
			}
			continue
		}
		for _, runway := range query.Runways {
			request := cloneValues(parameters)
			request.Set("runway", string(runway))
			items, err := a.listProcedurePage(ctx, request)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if matchesListing(item, query) {
					key := item.Airport + "/" + item.Identifier + "/" + item.Type.Code
					if _, ok := seen[key]; !ok {
						seen[key] = struct{}{}
						result = append(result, item)
					}
				}
			}
		}
	}
	return result, nil
}

func (a *Adapter) listProcedurePage(ctx context.Context, parameters url.Values) ([]procedureListItem, error) {
	page := 1
	result := []procedureListItem{}
	for {
		request := cloneValues(parameters)
		request.Set("page", strconv.Itoa(page))
		request.Set("per_page", "100")
		var response procedureListResponse
		if err := a.getJSON(ctx, "procedures", request, &response); err != nil {
			return nil, err
		}
		if err := a.saveNextPage(ctx, "procedures", request, nextPage(response.Pagination.HasMore, page)); err != nil {
			return nil, unavailable("save AIRAC.NET pagination checkpoint")
		}
		result = append(result, response.Data...)
		if !response.Pagination.HasMore {
			return result, nil
		}
		page++
		if page > 10000 {
			return nil, unavailable("AIRAC.NET procedure pagination did not terminate")
		}
	}
}

func (a *Adapter) procedure(ctx context.Context, version navdata.DatasetVersion, item procedureListItem) (navdata.Procedure, bool, error) {
	var response procedureDetailResponse
	if err := a.getJSON(ctx, path.Join("procedures", item.Airport, item.Identifier), nil, &response); err != nil {
		if isHTTPStatus(err, http.StatusNotFound) {
			return navdata.Procedure{}, false, unavailable("AIRAC.NET procedure changed during retrieval")
		}
		return navdata.Procedure{}, false, err
	}
	detail := response.Data
	if detail.Identifier == "" {
		detail.Identifier = item.Identifier
	}
	if detail.Airport == "" {
		detail.Airport = item.Airport
	}
	if detail.Type.Code == "" {
		detail.Type = item.Type
	}
	procedure := navdata.Procedure{ID: navdata.ProcedureID(upper(detail.Identifier)), Airport: navdata.AirportID(upper(detail.Airport)), Kind: canonicalKind(detail.Type.Code), Runways: canonicalRunways(item.Runway, detail.AvailableRunways), Provenance: a.provenance(version)}
	if !procedure.Kind.Valid() {
		return navdata.Procedure{}, false, invalid("AIRAC.NET procedure type")
	}
	segments := detail.Segments
	incomplete := false
	if len(segments) == 0 && len(detail.CommonRoute) > 0 {
		incomplete = true // Structured common routes omit documented path terminators.
		for _, leg := range detail.CommonRoute {
			segments = append(segments, segmentDTO{Sequence: leg.Sequence, FixIdentifier: leg.FixIdentifier})
		}
	}
	if len(segments) == 0 {
		incomplete = true
	}
	for index, segment := range segments {
		leg, incompleteLeg := procedureLeg(segment, index)
		procedure.Legs = append(procedure.Legs, leg)
		incomplete = incomplete || incompleteLeg
	}
	// AIRAC.NET's documented procedure payload has no published holding
	// definition fields. A supplied HA/HF/HM without an authoritative definition
	// stays unsupported rather than becoming an empty confirmed holding result.
	if err := procedure.Validate(); err != nil {
		return navdata.Procedure{}, false, invalid("AIRAC.NET procedure geometry")
	}
	return procedure, incomplete, nil
}

func (a *Adapter) airport(version navdata.DatasetVersion, value airportDTO) (navdata.Airport, error) {
	item := navdata.Airport{ID: navdata.AirportID(upper(value.ICAO)), Name: strings.TrimSpace(value.Name), Position: coordinate(value.Latitude, value.Longitude, value.Coordinates), Provenance: a.provenance(version)}
	if err := item.Validate(); err != nil {
		return navdata.Airport{}, invalid("AIRAC.NET airport geometry")
	}
	return item, nil
}
func (a *Adapter) fix(version navdata.DatasetVersion, value waypointDTO) (navdata.Fix, error) {
	item := navdata.Fix{ID: navdata.FixID(upper(value.Identifier)), Position: coordinate(value.Latitude, value.Longitude, value.Coordinates), Provenance: a.provenance(version)}
	if err := item.Validate(); err != nil {
		return navdata.Fix{}, invalid("AIRAC.NET waypoint geometry")
	}
	return item, nil
}
func (a *Adapter) routeGeometry(query navdata.RouteQuery, result routeData) (navdata.RouteGeometry, error) {
	geometry := navdata.RouteGeometry{Version: query.Version, TotalDistanceNM: result.TotalDistance, Coverage: navdata.CoverageComplete, Provenance: a.provenance(query.Version)}
	if query.ArrivalProcedure != nil {
		geometry.Coverage = navdata.CoveragePartial
		geometry.Unresolved = append(geometry.Unresolved, "PUBLISHED_HOLDING_DATA_UNAVAILABLE")
	}
	if len(result.Segments) == 0 {
		geometry.Coverage = navdata.CoveragePartial
		geometry.Unresolved = append(geometry.Unresolved, "route-empty")
	}
	for index, segment := range result.Segments {
		from, to := navdata.FixID(upper(segment.From.Identifier)), navdata.FixID(upper(segment.To.Identifier))
		if from == "" || to == "" {
			geometry.Coverage = navdata.CoveragePartial
			geometry.Unresolved = append(geometry.Unresolved, fmt.Sprintf("route-segment-%d", index))
			continue
		}
		course, distance := canonicalCourse(segment.Bearing), segment.Distance
		// /routes/parse supplies explicit from/to points and a true bearing for
		// each expanded segment, which maps to a track-to-fix geometric leg.
		geometry.Legs = append(geometry.Legs, navdata.ProcedureLeg{ID: fmt.Sprintf("ROUTE-%04d", index+1), PathTerminator: navdata.PathTF, FromFix: &from, ToFix: &to, CourseTrueDeg: &course, DistanceNM: &distance})
	}
	for _, problem := range result.Errors {
		if problem.Type != "" {
			geometry.Coverage = navdata.CoveragePartial
			geometry.Unresolved = append(geometry.Unresolved, upper(problem.Type))
		}
	}
	if geometry.TotalDistanceNM < 0 {
		return navdata.RouteGeometry{}, invalid("AIRAC.NET route distance")
	}
	digest, err := navdata.RouteGeometryDigest(query, geometry)
	if err != nil {
		return navdata.RouteGeometry{}, invalid("AIRAC.NET route geometry")
	}
	geometry.Digest = digest
	return geometry, nil
}

func (a *Adapter) getJSON(ctx context.Context, endpoint string, parameters url.Values, output any) error {
	requestURL := *a.baseURL
	requestURL.Path = path.Join(a.baseURL.Path, endpoint)
	requestURL.RawQuery = parameters.Encode()
	key := requestURL.Path + "?" + requestURL.RawQuery
	checkpoint, cached, err := a.checkpoints.Load(ctx, key)
	if err != nil {
		return unavailable("load AIRAC.NET checkpoint")
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case a.requests <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		requestCtx, cancel := context.WithTimeout(ctx, a.timeout)
		request, err := a.requestMaker(requestCtx, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			cancel()
			<-a.requests
			return invalid("construct AIRAC.NET request")
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", a.userAgent)
		if cached && checkpoint.ETag != "" {
			request.Header.Set("If-None-Match", checkpoint.ETag)
		}
		if cached && checkpoint.LastModified != "" {
			request.Header.Set("If-Modified-Since", checkpoint.LastModified)
		}
		response, doErr := a.client.Do(request)
		if doErr != nil {
			cancel()
			<-a.requests
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if retryableTransport(doErr) && attempt < a.retries {
				if err := wait(ctx, a.backoff, attempt); err != nil {
					return err
				}
				continue
			}
			return unavailable("AIRAC.NET request failed")
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
		response.Body.Close()
		cancel()
		<-a.requests
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if retryableTransport(readErr) && attempt < a.retries {
				if err := wait(ctx, a.backoff, attempt); err != nil {
					return err
				}
				continue
			}
			return unavailable("read AIRAC.NET response")
		}
		if response.StatusCode == http.StatusNotModified {
			if !cached || len(checkpoint.Body) == 0 {
				return unavailable("AIRAC.NET returned uncached conditional response")
			}
			return json.Unmarshal(checkpoint.Body, output)
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			if (response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500) && attempt < a.retries {
				if err := wait(ctx, a.backoff, attempt); err != nil {
					return err
				}
				continue
			}
			return &httpStatusError{status: response.StatusCode, endpoint: requestURL.Path}
		}
		var envelope struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil || envelope.Status != "success" {
			return invalid("malformed AIRAC.NET response")
		}
		if err := json.Unmarshal(body, output); err != nil {
			return invalid("malformed AIRAC.NET response")
		}
		if err := a.checkpoints.Save(ctx, key, Checkpoint{ETag: response.Header.Get("ETag"), LastModified: response.Header.Get("Last-Modified"), NextPage: checkpoint.NextPage, Body: body}); err != nil {
			return unavailable("save AIRAC.NET checkpoint")
		}
		return nil
	}
}

func (a *Adapter) saveNextPage(ctx context.Context, endpoint string, parameters url.Values, page int) error {
	requestURL := *a.baseURL
	requestURL.Path = path.Join(a.baseURL.Path, endpoint)
	requestURL.RawQuery = parameters.Encode()
	key := requestURL.Path + "?" + requestURL.RawQuery
	checkpoint, found, err := a.checkpoints.Load(ctx, key)
	if err != nil || !found {
		return err
	}
	checkpoint.NextPage = page
	return a.checkpoints.Save(ctx, key, checkpoint)
}

func (a *Adapter) matchVersion(ctx context.Context, requested navdata.DatasetVersion) error {
	actual, err := a.LatestVersion(ctx)
	if err != nil {
		return err
	}
	if !actual.Equal(requested) {
		return &aman.DomainError{Class: navdata.ErrorDatasetMismatch, Message: "AIRAC.NET dataset version does not match request"}
	}
	return nil
}
func (a *Adapter) provenance(version navdata.DatasetVersion) navdata.Provenance {
	return navdata.Provenance{SourceID: "airac.net", SourceRevision: version.SourceRevision, ImportedAt: a.now().UTC(), EffectiveFrom: version.EffectiveFrom, EffectiveUntil: version.EffectiveUntil}
}
func (a *Adapter) parallel(ctx context.Context, count int, fn func(context.Context, int) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var group sync.WaitGroup
	var once sync.Once
	var result error
	for index := 0; index < count; index++ {
		index := index
		group.Add(1)
		go func() {
			defer group.Done()
			if err := fn(ctx, index); err != nil {
				once.Do(func() { result = err; cancel() })
			}
		}()
	}
	group.Wait()
	return result
}

type cycleResponse struct {
	Data struct {
		Cycle          string `json:"cycle"`
		EffectiveDate  string `json:"effective_date"`
		ExpirationDate string `json:"expiration_date"`
	} `json:"data"`
}
type coordinateDTO struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}
type airportDTO struct {
	ICAO        string        `json:"icao"`
	Name        string        `json:"name"`
	Latitude    float64       `json:"latitude"`
	Longitude   float64       `json:"longitude"`
	Coordinates coordinateDTO `json:"coordinates"`
}
type airportResponse struct {
	Data airportDTO `json:"data"`
}
type waypointDTO struct {
	Identifier  string        `json:"identifier"`
	Latitude    float64       `json:"latitude"`
	Longitude   float64       `json:"longitude"`
	Coordinates coordinateDTO `json:"coordinates"`
}
type waypointResponse struct {
	Data json.RawMessage `json:"data"`
}
type procedureTypeDTO struct {
	Code string `json:"code"`
}
type procedureListItem struct {
	Airport    string           `json:"airport"`
	Identifier string           `json:"identifier"`
	Type       procedureTypeDTO `json:"type"`
	Runway     string           `json:"runway"`
}
type paginationDTO struct {
	HasMore bool `json:"has_more"`
}
type procedureListResponse struct {
	Data       []procedureListItem `json:"data"`
	Pagination paginationDTO       `json:"pagination"`
}
type segmentDTO struct {
	Sequence       int    `json:"sequence"`
	PathTerminator string `json:"path_terminator"`
	FixIdentifier  string `json:"fix_identifier"`
}
type procedureDetailDTO struct {
	Airport          string           `json:"airport"`
	Identifier       string           `json:"identifier"`
	Type             procedureTypeDTO `json:"type"`
	Segments         []segmentDTO     `json:"segments"`
	CommonRoute      []segmentDTO     `json:"common_route"`
	AvailableRunways []string         `json:"available_runways"`
}
type procedureDetailResponse struct {
	Data procedureDetailDTO `json:"data"`
}
type routePointDTO struct {
	Identifier string `json:"identifier"`
}
type routeSegmentDTO struct {
	From     routePointDTO `json:"from"`
	To       routePointDTO `json:"to"`
	Distance float64       `json:"distance"`
	Bearing  float64       `json:"bearing"`
}
type routeErrorDTO struct {
	Type string `json:"type"`
}
type routeData struct {
	TotalDistance float64           `json:"total_distance"`
	Segments      []routeSegmentDTO `json:"segments"`
	Errors        []routeErrorDTO   `json:"errors"`
}
type routeResponse struct {
	Data routeData `json:"data"`
}
type httpStatusError struct {
	status   int
	endpoint string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("AIRAC.NET request to %s returned HTTP %d", e.endpoint, e.status)
}
func isHTTPStatus(err error, status int) bool {
	var value *httpStatusError
	return errors.As(err, &value) && value.status == status
}

func firstWaypoint(raw json.RawMessage) (waypointDTO, bool, error) {
	var one waypointDTO
	if err := json.Unmarshal(raw, &one); err == nil && one.Identifier != "" {
		return one, true, nil
	}
	var many []waypointDTO
	if err := json.Unmarshal(raw, &many); err != nil {
		return waypointDTO{}, false, err
	}
	if len(many) == 0 {
		return waypointDTO{}, false, nil
	}
	if len(many) != 1 {
		return waypointDTO{}, false, errors.New("ambiguous waypoint response")
	}
	return many[0], true, nil
}
func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	return parsed.UTC(), err
}
func coordinate(lat, lon float64, nested coordinateDTO) navdata.Coordinate {
	if nested.Lat != 0 || nested.Lon != 0 {
		lat, lon = nested.Lat, nested.Lon
	}
	return navdata.Coordinate{LatitudeDeg: lat, LongitudeDeg: lon}
}
func canonicalCourse(value float64) float64 {
	if value == 360 {
		return 0
	}
	return value
}
func canonicalKind(value string) navdata.ProcedureKind {
	switch upper(value) {
	case "SID":
		return navdata.ProcedureSID
	case "STAR":
		return navdata.ProcedureSTAR
	case "APP":
		return navdata.ProcedureApproach
	default:
		return ""
	}
}
func vendorKind(kind navdata.ProcedureKind) string {
	switch kind {
	case navdata.ProcedureSID:
		return "SID"
	case navdata.ProcedureSTAR:
		return "STAR"
	case navdata.ProcedureApproach:
		return "APP"
	default:
		return ""
	}
}
func kindsOrAll(kinds []navdata.ProcedureKind) []navdata.ProcedureKind {
	if len(kinds) > 0 {
		return kinds
	}
	return []navdata.ProcedureKind{navdata.ProcedureSID, navdata.ProcedureSTAR, navdata.ProcedureApproach}
}
func canonicalRunways(listing string, detail []string) []navdata.RunwayID {
	seen := map[navdata.RunwayID]struct{}{}
	result := []navdata.RunwayID{}
	for _, value := range append([]string{listing}, detail...) {
		runway := navdata.RunwayID(upper(value))
		if runway != "" {
			if _, ok := seen[runway]; !ok {
				seen[runway] = struct{}{}
				result = append(result, runway)
			}
		}
	}
	return result
}
func procedureLeg(segment segmentDTO, index int) (navdata.ProcedureLeg, bool) {
	terminator := navdata.PathTerminator(upper(segment.PathTerminator))
	incomplete := false
	if !terminator.Supported() || terminator.IsHolding() {
		terminator, incomplete = navdata.PathUnsupported, true
	}
	leg := navdata.ProcedureLeg{ID: fmt.Sprintf("LEG-%04d", sequence(segment.Sequence, index)), PathTerminator: terminator}
	if fix := navdata.FixID(upper(segment.FixIdentifier)); fix != "" {
		leg.ToFix = &fix
	}
	return leg, incomplete
}
func sequence(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback + 1
}
func matchesListing(item procedureListItem, query navdata.ProcedureQuery) bool {
	if navdata.AirportID(upper(item.Airport)) != query.Airport {
		return false
	}
	if len(query.Kinds) > 0 && !slices.Contains(query.Kinds, canonicalKind(item.Type.Code)) {
		return false
	}
	if len(query.Identifiers) > 0 && !slices.Contains(query.Identifiers, navdata.ProcedureID(upper(item.Identifier))) {
		return false
	}
	if len(query.Runways) > 0 && !slices.Contains(query.Runways, navdata.RunwayID(upper(item.Runway))) {
		return false
	}
	return true
}
func transformedRoute(query navdata.RouteQuery) string {
	tokens := []string{}
	for _, token := range strings.Fields(strings.ToUpper(query.FiledRoute)) {
		if token != "DCT" {
			tokens = append(tokens, token)
		}
	}
	if query.ArrivalProcedure != nil && !slices.Contains(tokens, string(*query.ArrivalProcedure)) {
		tokens = append(tokens, string(*query.ArrivalProcedure))
	}
	return strings.Join(tokens, " ")
}
func cloneValues(values url.Values) url.Values {
	result := url.Values{}
	for key, value := range values {
		result[key] = slices.Clone(value)
	}
	return result
}
func upper(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }
func nextPage(hasMore bool, page int) int {
	if hasMore {
		return page + 1
	}
	return 0
}
func retryableTransport(err error) bool {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var network net.Error
	return errors.As(err, &network) && (network.Timeout() || network.Temporary())
}
func wait(ctx context.Context, base time.Duration, attempt int) error {
	if attempt > 20 {
		attempt = 20
	}
	delay := base * time.Duration(1<<attempt)
	if delay < 0 || delay > time.Minute {
		delay = time.Minute
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func invalid(message string) error {
	return &aman.DomainError{Class: navdata.ErrorInvalidRequest, Message: message}
}
func notFound(message string) error {
	return &aman.DomainError{Class: navdata.ErrorNotFound, Message: message}
}
func incomplete(message string) error {
	return &aman.DomainError{Class: navdata.ErrorIncompleteGeometry, Message: message}
}
func unavailable(message string) error {
	return &aman.DomainError{Class: navdata.ErrorSourceUnavailable, Message: message}
}

var _ navdata.CycleSource = (*Adapter)(nil)
var _ navdata.AirportSource = (*Adapter)(nil)
var _ navdata.ProcedureSource = (*Adapter)(nil)
var _ navdata.FixSource = (*Adapter)(nil)
var _ navdata.RouteResolver = (*Adapter)(nil)

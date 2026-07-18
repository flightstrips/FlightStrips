package standstatus

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/standdiagnostics"
	"FlightStrips/internal/vatsim"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

type standStatusSessionRepository interface {
	List(context.Context) ([]*models.Session, error)
}

type standStatusAssignmentRepository interface {
	ListAssignments(context.Context, int32) ([]*models.StandAssignment, error)
	ListBlocks(context.Context, int32) ([]*models.StandBlock, error)
}

type standStatusFeed interface {
	Snapshot() vatsim.Snapshot
}

type standStatusFailureSource interface {
	List() []standdiagnostics.AllocationFailure
}

// WebAPIDiagnostics describes the configuration that was loaded at startup.
type WebAPIDiagnostics struct {
	AircraftTypes int `json:"aircraft_types"`
	Stands        int `json:"stands"`
	StandVariants int `json:"stand_variants"`
	AirlineRules  int `json:"airline_rules"`
	StandGroups   int `json:"stand_groups"`
	FallbackRules int `json:"fallback_rules"`
}

// WebAPIConfig contains the read-only dependencies for the SAT diagnostics page.
type WebAPIConfig struct {
	Auth        shared.AuthenticationService
	Sessions    standStatusSessionRepository
	Assignments standStatusAssignmentRepository
	Feed        standStatusFeed
	Enabled     bool
	Ready       bool
	Reason      string
	StaleAfter  time.Duration
	Diagnostics WebAPIDiagnostics
	Failures    standStatusFailureSource
}

// WebAPI exposes an authenticated, read-only snapshot of SAT's internal state.
type WebAPI struct {
	config WebAPIConfig
	now    func() time.Time
}

type standStatusResponse struct {
	GeneratedAt   string                               `json:"generated_at"`
	System        standStatusSystemResponse            `json:"system"`
	Configuration WebAPIDiagnostics                    `json:"configuration"`
	Feed          standStatusFeedResponse              `json:"feed"`
	Failures      []standdiagnostics.AllocationFailure `json:"failures"`
	Sessions      []standStatusSessionResponse         `json:"sessions"`
}

type standStatusSystemResponse struct {
	Enabled bool   `json:"enabled"`
	Ready   bool   `json:"ready"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
}

type standStatusFeedResponse struct {
	Status     string `json:"status"`
	SnapshotAt string `json:"snapshot_at,omitempty"`
	LastError  string `json:"last_error,omitempty"`
	Flights    int    `json:"flights"`
	Online     int    `json:"online"`
	Prefiles   int    `json:"prefiles"`
}

type standStatusSessionResponse struct {
	SessionID   int32                           `json:"session_id"`
	Name        string                          `json:"name"`
	Airport     string                          `json:"airport"`
	Assignments []standStatusAssignmentResponse `json:"assignments"`
	Blocks      []standStatusBlockResponse      `json:"blocks"`
}

type standStatusAssignmentResponse struct {
	ID             int64      `json:"id"`
	Callsign       string     `json:"callsign"`
	Stand          string     `json:"stand"`
	Direction      string     `json:"direction"`
	Stage          string     `json:"stage"`
	Source         string     `json:"source"`
	RuleID         *string    `json:"rule_id,omitempty"`
	Tier           *int32     `json:"tier,omitempty"`
	MatchedVariant *string    `json:"matched_variant,omitempty"`
	ConflictReason *string    `json:"conflict_reason,omitempty"`
	ETA            *time.Time `json:"eta,omitempty"`
	ETASource      *string    `json:"eta_source,omitempty"`
	AssignedAt     *time.Time `json:"assigned_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	Manual         bool       `json:"manual"`
	Acknowledged   bool       `json:"acknowledged"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy *string    `json:"acknowledged_by,omitempty"`
	VatsimCID      *int64     `json:"vatsim_cid,omitempty"`
	VatsimRevision *int64     `json:"vatsim_revision,omitempty"`
	Version        int32      `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type standStatusBlockResponse struct {
	ID        int64      `json:"id"`
	Stand     string     `json:"stand"`
	BlockType string     `json:"block_type"`
	Source    string     `json:"source"`
	Reason    *string    `json:"reason,omitempty"`
	Callsign  *string    `json:"callsign,omitempty"`
	CreatedBy *string    `json:"created_by,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Manual    bool       `json:"manual"`
	Version   int32      `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func NewWebAPI(config WebAPIConfig) *WebAPI {
	return &WebAPI{config: config, now: time.Now}
}

func (a *WebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/stand/status", a.handleStatus)
}

func (a *WebAPI) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.authenticate(w, r) {
		return
	}

	now := a.now().UTC()
	response := standStatusResponse{
		GeneratedAt:   now.Format(time.RFC3339),
		System:        a.systemStatus(),
		Configuration: a.config.Diagnostics,
		Feed:          a.feedStatus(),
		Failures:      []standdiagnostics.AllocationFailure{},
		Sessions:      []standStatusSessionResponse{},
	}
	if a.config.Failures != nil {
		if failures := a.config.Failures.List(); failures != nil {
			response.Failures = failures
		}
	}

	if a.config.Sessions != nil && a.config.Assignments != nil {
		sessions, err := a.config.Sessions.List(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to list sessions")
			return
		}
		for _, session := range sessions {
			if session == nil {
				continue
			}
			assignments, err := a.config.Assignments.ListAssignments(r.Context(), session.ID)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to list stand assignments")
				return
			}
			blocks, err := a.config.Assignments.ListBlocks(r.Context(), session.ID)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to list stand blocks")
				return
			}
			response.Sessions = append(response.Sessions, mapStandStatusSession(session, assignments, blocks, now))
		}
	}

	sort.SliceStable(response.Sessions, func(i, j int) bool {
		left, right := response.Sessions[i], response.Sessions[j]
		if left.Airport != right.Airport {
			return left.Airport < right.Airport
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.SessionID < right.SessionID
	})
	writeJSON(w, http.StatusOK, response)
}

func (a *WebAPI) systemStatus() standStatusSystemResponse {
	result := standStatusSystemResponse{
		Enabled: a.config.Enabled,
		Ready:   a.config.Ready,
		Status:  "disabled",
	}
	switch {
	case !a.config.Enabled:
	case !a.config.Ready:
		result.Status = "invalid_config"
		result.Reason = a.config.Reason
	default:
		result.Status = "ready"
		feed := a.feedStatus()
		if feed.Status != "ready" {
			result.Ready = false
			result.Status = feed.Status
			result.Reason = feed.LastError
			if result.Reason == "" {
				result.Reason = "VATSIM feed is unavailable"
			}
		}
	}
	return result
}

func (a *WebAPI) feedStatus() standStatusFeedResponse {
	if !a.config.Enabled {
		return standStatusFeedResponse{Status: "disabled"}
	}
	if a.config.Feed == nil {
		return standStatusFeedResponse{Status: "feed_unavailable", LastError: "VATSIM feed is unavailable"}
	}

	snapshot := a.config.Feed.Snapshot()
	result := standStatusFeedResponse{Status: "ready"}
	if !snapshot.Timestamp.IsZero() {
		result.SnapshotAt = snapshot.Timestamp.UTC().Format(time.RFC3339)
	}
	for _, flight := range snapshot.Flights() {
		result.Flights++
		if flight.Prefile() {
			result.Prefiles++
		} else {
			result.Online++
		}
	}

	switch {
	case snapshot.Timestamp.IsZero():
		result.Status = "feed_unavailable"
		result.LastError = "VATSIM feed has not produced a snapshot"
	case snapshot.LastRefreshError != nil:
		result.Status = "feed_failed"
		result.LastError = snapshot.LastRefreshError.Error()
	case a.config.StaleAfter > 0 && a.now().UTC().Sub(snapshot.Timestamp) > a.config.StaleAfter:
		result.Status = "feed_stale"
		result.LastError = "VATSIM snapshot is stale"
	}
	return result
}

func mapStandStatusSession(session *models.Session, assignments []*models.StandAssignment, blocks []*models.StandBlock, now time.Time) standStatusSessionResponse {
	response := standStatusSessionResponse{
		SessionID:   session.ID,
		Name:        session.Name,
		Airport:     session.Airport,
		Assignments: make([]standStatusAssignmentResponse, 0, len(assignments)),
		Blocks:      make([]standStatusBlockResponse, 0, len(blocks)),
	}
	for _, assignment := range assignments {
		if assignment == nil {
			continue
		}
		response.Assignments = append(response.Assignments, standStatusAssignmentResponse{
			ID: assignment.ID, Callsign: assignment.Callsign, Stand: assignment.Stand,
			Direction: assignment.Direction, Stage: assignment.Stage, Source: assignment.Source,
			RuleID: assignment.RuleID, Tier: assignment.Tier, MatchedVariant: assignment.MatchedVariant,
			ConflictReason: assignment.ConflictReason, ETA: assignment.ETA, ETASource: assignment.ETASource,
			AssignedAt: assignment.AssignedAt, ExpiresAt: assignment.ExpiresAt, Manual: assignment.Manual,
			Acknowledged: assignment.Acknowledged, AcknowledgedAt: assignment.AcknowledgedAt,
			AcknowledgedBy: assignment.AcknowledgedBy, VatsimCID: assignment.VatsimCID,
			VatsimRevision: assignment.VatsimRevision, Version: assignment.Version,
			CreatedAt: assignment.CreatedAt, UpdatedAt: assignment.UpdatedAt,
		})
	}
	for _, block := range blocks {
		if block == nil || (block.ExpiresAt != nil && !block.ExpiresAt.After(now)) {
			continue
		}
		response.Blocks = append(response.Blocks, standStatusBlockResponse{
			ID: block.ID, Stand: block.Stand, BlockType: block.BlockType, Source: block.Source,
			Reason: block.Reason, Callsign: block.Callsign, CreatedBy: block.CreatedBy,
			ExpiresAt: block.ExpiresAt, Manual: block.Manual, Version: block.Version,
			CreatedAt: block.CreatedAt, UpdatedAt: block.UpdatedAt,
		})
	}
	sort.SliceStable(response.Assignments, func(i, j int) bool {
		return response.Assignments[i].Callsign < response.Assignments[j].Callsign
	})
	sort.SliceStable(response.Blocks, func(i, j int) bool {
		if response.Blocks[i].Stand != response.Blocks[j].Stand {
			return response.Blocks[i].Stand < response.Blocks[j].Stand
		}
		return response.Blocks[i].ID < response.Blocks[j].ID
	})
	return response
}

func (a *WebAPI) authenticate(w http.ResponseWriter, r *http.Request) bool {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing authorization header")
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	if token == authHeader || token == "" {
		writeJSONError(w, http.StatusUnauthorized, "invalid authorization header")
		return false
	}
	if a.config.Auth == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return false
	}
	if _, err := a.config.Auth.Validate(token); err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid token")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

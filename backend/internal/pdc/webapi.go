package pdc

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WebAPI exposes JSON endpoints for web PDC (Bearer JWT).
type WebAPI struct {
	Auth     *services.AuthenticationService
	Pdc      *Service
	PostGate *postLimiter
}

type postLimiter struct {
	mu            sync.Mutex
	last          map[string]time.Time
	minInterval   time.Duration
	cleanupEvery  time.Duration
	lastCleanupAt time.Time
}

func newPostLimiter(minInterval time.Duration) *postLimiter {
	return &postLimiter{
		last:         make(map[string]time.Time),
		minInterval:  minInterval,
		cleanupEvery: 5 * time.Minute,
	}
}

func (l *postLimiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastCleanupAt) > l.cleanupEvery {
		cutoff := now.Add(-l.minInterval * 2)
		for k, t := range l.last {
			if t.Before(cutoff) {
				delete(l.last, k)
			}
		}
		l.lastCleanupAt = now
	}
	if t, ok := l.last[key]; ok && now.Sub(t) < l.minInterval {
		return false
	}
	l.last[key] = now
	return true
}

// RegisterHTTP registers /api/pdc/request, /api/pdc/status, and /api/pdc/acknowledge on mux.
func (a *WebAPI) RegisterHTTP(mux *http.ServeMux) {
	if a.PostGate == nil {
		a.PostGate = newPostLimiter(2 * time.Second)
	}
	mux.HandleFunc("/api/pdc/request", a.cors(a.handlePdcRequest))
	mux.HandleFunc("/api/pdc/status", a.cors(a.handlePdcStatus))
	mux.HandleFunc("/api/pdc/acknowledge", a.cors(a.handlePdcAcknowledge))
}

func (a *WebAPI) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o := r.Header.Get("Origin")
		if o != "" {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

type pdcRequestBody struct {
	Callsign string `json:"callsign"`
	Atis     string `json:"atis"`
	Stand    string `json:"stand"`
	Remarks  string `json:"remarks"`
}

type pdcRequestResponse struct {
	RequestID int64 `json:"request_id"`
}

type pdcStatusResponse struct {
	Status                string  `json:"status"`
	ClearanceText         *string `json:"clearance_text"`
	ErrorMessage          *string `json:"error_message,omitempty"`
	PilotAcknowledgedAt   *string `json:"pilot_acknowledged_at,omitempty"`
}

func (a *WebAPI) handlePdcRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Pdc == nil || a.Auth == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}
	ip, _, _ := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if ip == "" {
		ip = r.RemoteAddr
	}
	if !a.PostGate.allow(ip + ":" + user.GetCid()) {
		http.Error(w, "rate limit", http.StatusTooManyRequests)
		return
	}

	var body pdcRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	id, err := a.Pdc.SubmitWebPDCRequest(r.Context(), WebPDCSubmitInput{
		Callsign:  body.Callsign,
		Atis:      body.Atis,
		Stand:     body.Stand,
		Remarks:   body.Remarks,
		VatsimCID: user.GetCid(),
	})
	if err != nil {
		a.writeSubmitError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(pdcRequestResponse{RequestID: id})
}

func (a *WebAPI) writeSubmitError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrWebStripNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrWebAmbiguousCallsign):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		if strings.Contains(err.Error(), "invalid ATIS") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		slog.Error("web PDC submit", slog.Any("error", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (a *WebAPI) handlePdcStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Pdc == nil || a.Auth == nil || a.Pdc.queries == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}
	q := r.URL.Query().Get("request_id")
	if q == "" {
		http.Error(w, "missing request_id", http.StatusBadRequest)
		return
	}
	rid, err := strconv.ParseInt(q, 10, 64)
	if err != nil || rid < 1 {
		http.Error(w, "invalid request_id", http.StatusBadRequest)
		return
	}

	row, err := a.Pdc.queries.GetPdcWebRequestByID(r.Context(), rid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, ErrWebRequestNotFound.Error(), http.StatusNotFound)
			return
		}
		slog.Error("pdc web status", slog.Any("error", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if row.VatsimCid != user.GetCid() {
		http.Error(w, ErrWebRequestForbidden.Error(), http.StatusForbidden)
		return
	}
	if row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(time.Now()) && row.Status == WebRequestStatusPending {
		http.Error(w, "request expired", http.StatusGone)
		return
	}

	resp := pdcStatusResponse{
		Status:       row.Status,
		ClearanceText: row.ClearanceText,
		ErrorMessage:  row.ErrorMessage,
	}
	if row.PilotAcknowledgedAt.Valid {
		s := row.PilotAcknowledgedAt.Time.UTC().Format(time.RFC3339)
		resp.PilotAcknowledgedAt = &s
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

type pdcAckBody struct {
	RequestID int64 `json:"request_id"`
}

type pdcAckResponse struct {
	Acknowledged bool `json:"acknowledged"`
}

func (a *WebAPI) handlePdcAcknowledge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Pdc == nil || a.Auth == nil || a.Pdc.queries == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}
	var body pdcAckBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RequestID < 1 {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	row, err := a.Pdc.queries.GetPdcWebRequestByID(r.Context(), body.RequestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, ErrWebRequestNotFound.Error(), http.StatusNotFound)
			return
		}
		slog.Error("pdc web ack", slog.Any("error", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if row.VatsimCid != user.GetCid() {
		http.Error(w, ErrWebRequestForbidden.Error(), http.StatusForbidden)
		return
	}
	if row.Status != WebRequestStatusCleared {
		http.Error(w, "clearance not ready", http.StatusBadRequest)
		return
	}
	if row.PilotAcknowledgedAt.Valid {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(pdcAckResponse{Acknowledged: true})
		return
	}

	err = a.Pdc.queries.UpdatePdcWebRequestPilotAck(r.Context(), database.UpdatePdcWebRequestPilotAckParams{
		ID: body.RequestID,
		PilotAcknowledgedAt: pgtype.Timestamptz{
			Time:  time.Now().UTC(),
			Valid: true,
		},
		VatsimCid: user.GetCid(),
	})
	if err != nil {
		slog.Error("pdc web ack update", slog.Any("error", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(pdcAckResponse{Acknowledged: true})
}

func (a *WebAPI) authenticate(w http.ResponseWriter, r *http.Request) (shared.AuthenticatedUser, bool) {
	h := r.Header.Get("Authorization")
	if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return shared.AuthenticatedUser{}, false
	}
	raw := strings.TrimSpace(h[7:])
	u, err := a.Auth.Validate(raw)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return shared.AuthenticatedUser{}, false
	}
	return u, true
}

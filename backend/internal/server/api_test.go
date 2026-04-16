package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIMiddlewareHandlesPreflight(t *testing.T) {
	handler := APIMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("preflight request should not reach downstream handler")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/pdc/request", nil)
	req.Header.Set("Origin", "https://app.flightstrips.dk")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	response := recorder.Result()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, response.StatusCode)
	}
	if got := response.Header.Get("Access-Control-Allow-Origin"); got != "https://app.flightstrips.dk" {
		t.Fatalf("expected Access-Control-Allow-Origin header to echo origin, got %q", got)
	}
}

func TestAPIMiddlewareAddsCORSHeadersToNormalRequests(t *testing.T) {
	handler := APIMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/pdc/status", nil)
	req.Header.Set("Origin", "https://app.flightstrips.dk")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	response := recorder.Result()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, response.StatusCode)
	}
	if got := response.Header.Get("Access-Control-Allow-Methods"); got != "GET, POST, OPTIONS" {
		t.Fatalf("unexpected Access-Control-Allow-Methods header %q", got)
	}
	if got := response.Header.Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Fatalf("unexpected Access-Control-Allow-Headers header %q", got)
	}
}

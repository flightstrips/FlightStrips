package vatsim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCacheVerifyPilotOwnsCallsign(t *testing.T) {
	dataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pilots":[{"cid":1234567,"callsign":"dal123"}]}`))
	}))
	defer dataServer.Close()

	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"v3":["` + dataServer.URL + `"]}}`))
	}))
	defer statusServer.Close()

	cache := NewCache(statusServer.URL, time.Second, dataServer.Client())

	ok, err := cache.VerifyPilotOwnsCallsign(context.Background(), "1234567", "DAL123")
	if err != nil {
		t.Fatalf("VerifyPilotOwnsCallsign returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected callsign ownership verification to succeed")
	}

	ok, err = cache.VerifyPilotOwnsCallsign(context.Background(), "7654321", "DAL123")
	if err != nil {
		t.Fatalf("VerifyPilotOwnsCallsign returned error: %v", err)
	}
	if ok {
		t.Fatal("expected callsign ownership verification to fail for wrong CID")
	}
}

func TestCacheVerifyPilotOwnsCallsignReturnsFalseForUnknownPilot(t *testing.T) {
	dataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pilots":[]}`))
	}))
	defer dataServer.Close()

	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"v3":["` + dataServer.URL + `"]}}`))
	}))
	defer statusServer.Close()

	cache := NewCache(statusServer.URL, time.Second, dataServer.Client())

	ok, err := cache.VerifyPilotOwnsCallsign(context.Background(), "1234567", "DAL123")
	if err != nil {
		t.Fatalf("VerifyPilotOwnsCallsign returned error: %v", err)
	}
	if ok {
		t.Fatal("expected callsign ownership verification to fail for unknown pilot")
	}
}

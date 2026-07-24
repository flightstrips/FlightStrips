package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStartPprofServerDisabled(t *testing.T) {
	if server := startPprofServer(false); server != nil {
		t.Fatal("disabled pprof server should be nil")
	}
}

func TestPprofServerIsLoopbackOnlyAndServesHeapProfile(t *testing.T) {
	server := newPprofServer()
	if server.Addr != pprofAddr {
		t.Fatalf("pprof server address = %q, want %q", server.Addr, pprofAddr)
	}
	if !strings.HasPrefix(server.Addr, "127.0.0.1:") {
		t.Fatalf("pprof server must bind only to loopback, got %q", server.Addr)
	}

	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("heap profile status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.Len() == 0 {
		t.Fatal("heap profile response is empty")
	}
}

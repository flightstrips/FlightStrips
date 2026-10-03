package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStartPprofServerDisabled(t *testing.T) {
	t.Setenv("PPROF_ADDR", "0.0.0.0:6060")
	t.Setenv("PPROF_MUTEX_FRACTION", "invalid")
	if server := startPprofServer(false); server != nil {
		t.Fatal("disabled pprof server should be nil")
	}
}

func TestPprofSettingsKeepDefaultsAndRejectNonLoopback(t *testing.T) {
	address, mutex, block, err := pprofSettings("", "", "")
	if err != nil || address != pprofAddr || mutex != 0 || block != 0 {
		t.Fatalf("default profiler settings changed: %q %d %d %v", address, mutex, block, err)
	}
	for _, address := range []string{"127.0.0.1:12345", "[::1]:12345"} {
		got, mutex, block, err := pprofSettings(address, "5", "1000000")
		if err != nil || got != address || mutex != 5 || block != 1000000 {
			t.Fatalf("valid profiler settings rejected: %q %d %d %v", got, mutex, block, err)
		}
	}
	for _, address := range []string{"0.0.0.0:6060", "192.168.1.2:6060", "localhost:6060", "127.0.0.1:0", "127.0.0.1:65536", "127.0.0.1:invalid", "invalid"} {
		if _, _, _, err := pprofSettings(address, "", ""); err == nil {
			t.Fatalf("invalid profiler address accepted: %q", address)
		}
	}
	for _, settings := range [][2]string{{"-1", ""}, {"", "-1"}, {"invalid", ""}, {"", "invalid"}} {
		if _, _, _, err := pprofSettings("", settings[0], settings[1]); err == nil {
			t.Fatalf("invalid profiler sampling settings accepted: %v", settings)
		}
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

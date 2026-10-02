package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Diagnostic profiling is opt-in and excluded from qualification measurements.
// Only an owned fixture process is restarted; the fixed loopback profiler port
// must be free before enabling it, so another application's profiler is never read.
func profilePositionLoad(t *testing.T, f *entrypointFixture) {
	raw := os.Getenv("NATS_TASK23_PROFILE_NODE")
	if raw == "" {
		return
	}
	node, err := strconv.Atoi(raw)
	if err != nil || node < 0 || node >= len(f.apps) {
		t.Fatalf("invalid profile fixture node %q", raw)
	}
	listener, err := net.Listen("tcp", pprofAddr)
	if err != nil {
		t.Fatalf("refuse to collide with another profiler: %v", err)
	}
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	f.apps[node].stop()
	env := replaceFixtureEnv(f.env, map[string]string{"ENABLE_PPROF": "true"})
	f.apps[node] = startFixtureProcess(t, f.binary, f.backend, env, "-addr", f.addresses[node])
	f.ready()
	t.Logf("POSITION_CPU_PROFILE node=%d pid=%d", node, f.apps[node].command.Process.Pid)
	traceDone := make(chan error, 1)
	go func() {
		time.Sleep(10 * time.Second)
		dir := os.Getenv("NATS_TASK23_OUTPUT")
		if err := os.MkdirAll(dir, 0700); err != nil {
			traceDone <- err
			return
		}
		client := &http.Client{Timeout: 15 * time.Second}
		for _, artifact := range []struct{ endpoint, suffix string }{{"goroutine?debug=2", "goroutines.txt"}, {"trace?seconds=5", "trace"}} {
			response, err := client.Get("http://" + pprofAddr + "/debug/pprof/" + artifact.endpoint)
			if err != nil {
				traceDone <- err
				return
			}
			data, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				traceDone <- err
				return
			}
			if response.StatusCode != 200 {
				traceDone <- fmt.Errorf("profile HTTP status %d", response.StatusCode)
				return
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("backend-%d.%s", node, artifact.suffix)), data, 0600); err != nil {
				traceDone <- err
				return
			}
		}
		traceDone <- nil
	}()
	done := make(chan error, 1)
	go func() {
		response, err := (&http.Client{Timeout: 45 * time.Second}).Get("http://" + pprofAddr + "/debug/pprof/profile?seconds=30")
		if err != nil {
			done <- err
			return
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			done <- fmt.Errorf("profile HTTP status %d", response.StatusCode)
			return
		}
		profile, err := io.ReadAll(response.Body)
		if err == nil {
			dir := os.Getenv("NATS_TASK23_OUTPUT")
			err = os.MkdirAll(dir, 0700)
			if err == nil {
				err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("backend-%d.cpu", node)), profile, 0600)
			}
		}
		done <- err
	}()
	t.Cleanup(func() {
		if err := <-traceDone; err != nil {
			t.Errorf("owned backend runtime trace: %v", err)
		}
		if err := <-done; err != nil {
			t.Errorf("owned backend CPU profile: %v", err)
		}
	})
}

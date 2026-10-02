package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type loadMonitor struct {
	mu      sync.Mutex
	samples []map[string]any
	stop    chan struct{}
	done    chan struct{}
}

func (f *entrypointFixture) startLoadMonitor() *loadMonitor {
	m := &loadMonitor{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(m.done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			m.sample(f)
			select {
			case <-m.stop:
				return
			case <-ticker.C:
			}
		}
	}()
	return m
}
func (m *loadMonitor) sample(f *entrypointFixture) {
	sample := map[string]any{"utc": time.Now().UTC().Format(time.RFC3339Nano)}
	var counters map[string]any
	if runtime.GOOS == "windows" {
		ids := []string{}
		for _, p := range append(append([]*fixtureProcess{}, f.apps...), f.brokers...) {
			if p.command.Process != p.ownedProcess || p.command.Process.Pid != p.ownedPID {
				panic("unowned sample target")
			}
			ids = append(ids, strconv.Itoa(p.ownedPID))
		}
		command := "@{ processes=@(Get-Process -Id " + strings.Join(ids, ",") + " | Select-Object Id,CPU,WorkingSet64,PrivateMemorySize64); host_cpu=@(Get-CimInstance Win32_Processor | Select-Object LoadPercentage,NumberOfLogicalProcessors); host_memory=(Get-CimInstance Win32_OperatingSystem | Select-Object FreePhysicalMemory,TotalVisibleMemorySize) } | ConvertTo-Json -Depth 4 -Compress"
		bytes, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", command).Output()
		if err != nil {
			sample["process_error"] = err.Error()
		} else {
			if err = json.Unmarshal(bytes, &counters); err != nil {
				sample["process_error"] = err.Error()
			} else {
				sample["processes"] = counters["processes"]
				sample["host_cpu"] = counters["host_cpu"]
				sample["host_memory_kib"] = counters["host_memory"]
			}
		}
	}
	backends := []any{}
	for _, address := range f.addresses {
		readyStatus, readyData := entrypointStatus(address, "/readyz", "")
		var readiness map[string]string
		_ = json.Unmarshal(readyData, &readiness)
		status, data := entrypointStatus(address, "/metrics", "")
		values := map[string]float64{}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				continue
			}
			if value, err := strconv.ParseFloat(fields[1], 64); err == nil {
				values[fields[0]] = value
			}
		}
		// Exact readiness reasons stay in private, synthetic fixture evidence;
		// they distinguish original health failure from a later socket reset.
		backends = append(backends, map[string]any{"address": address, "status": status, "metrics": values, "ready_status": readyStatus, "readiness": readiness})
	}
	sample["backends"] = backends
	brokers := []any{}
	for _, address := range f.monitorAddresses {
		status, data := entrypointStatus(address, "/varz", "")
		var v map[string]any
		_ = json.Unmarshal(data, &v)
		counters := map[string]any{"address": address, "status": status}
		for _, key := range []string{"cpu", "mem", "in_bytes", "out_bytes", "in_msgs", "out_msgs", "connections", "jetstream"} {
			counters[key] = v[key]
		}
		// JetStream configuration contains no key value, but only retain usage and
		// API counters rather than copying its configuration into public evidence.
		if j, ok := v["jetstream"].(map[string]any); ok {
			counters["jetstream"] = map[string]any{"stats": j["stats"]}
		}
		brokers = append(brokers, counters)
	}
	sample["brokers"] = brokers
	sizes := map[string]int64{}
	for i := 0; i < 3; i++ {
		root := filepath.Join(f.dir, fmt.Sprintf("data-%d", i))
		var size int64
		_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				size += info.Size()
			}
			return nil
		})
		sizes[fmt.Sprintf("broker-%d", i)] = size
	}
	sample["store_file_bytes"] = sizes
	m.mu.Lock()
	m.samples = append(m.samples, sample)
	m.mu.Unlock()
}
func (m *loadMonitor) finish(f *entrypointFixture) []map[string]any {
	close(m.stop)
	<-m.done
	m.sample(f)
	return m.samples
}

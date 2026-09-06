package metrics

import (
	"context"
	"math"
	"runtime"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func collectRuntimeMetrics(t *testing.T) (*sdkmetric.ManualReader, metricdata.ResourceMetrics) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	if err := StartRuntimeMetrics(provider); err != nil {
		t.Fatalf("start runtime metrics: %v", err)
	}
	return reader, collectMetrics(t, reader)
}

func metricNames(rm metricdata.ResourceMetrics) map[string]struct{} {
	names := map[string]struct{}{}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			names[metric.Name] = struct{}{}
		}
	}
	return names
}

func TestStartRuntimeMetricsRequiresProvider(t *testing.T) {
	if err := StartRuntimeMetrics(nil); err == nil {
		t.Fatal("expected an error when no meter provider is supplied")
	}
}

func TestRuntimeMetricsPublishCPUAttribution(t *testing.T) {
	_, rm := collectRuntimeMetrics(t)

	names := metricNames(rm)
	for _, name := range []string{"go.cpu.time", "go.processor.limit", "go.mutex.wait.time", "go.gc.cycles"} {
		if _, ok := names[name]; !ok {
			t.Fatalf("expected %q to be published", name)
		}
	}

	// Every CPU class must be present, because a dashboard divides one class by
	// the total to attribute load and a missing class silently skews that.
	seen := map[string]bool{}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "go.cpu.time" {
				continue
			}
			data, ok := metric.Data.(metricdata.Sum[float64])
			if !ok {
				t.Fatalf("expected go.cpu.time to be a sum, got %T", metric.Data)
			}
			for _, point := range data.DataPoints {
				for _, kv := range point.Attributes.ToSlice() {
					if kv.Key == "class" {
						seen[kv.Value.AsString()] = true
					}
				}
				if point.Value < 0 {
					t.Fatalf("expected non-negative CPU seconds, got %v", point.Value)
				}
			}
		}
	}

	for _, class := range []string{"user", "gc", "scavenge", "idle"} {
		if !seen[class] {
			t.Fatalf("expected CPU class %q to be reported", class)
		}
	}
}

func TestProcessorLimitMatchesGOMAXPROCS(t *testing.T) {
	_, rm := collectRuntimeMetrics(t)

	want := int64(runtime.GOMAXPROCS(0))
	got := findInt64MetricValue(t, rm, "go.processor.limit", map[string]string{})
	if got != want {
		t.Fatalf("expected processor limit %d, got %d", want, got)
	}
}

func TestFirstCollectionReportsNoLatencyQuantiles(t *testing.T) {
	_, rm := collectRuntimeMetrics(t)

	// The first collection only establishes the histogram baseline, so process
	// startup cannot be reported as a scheduling latency spike.
	names := metricNames(rm)
	for _, name := range []string{"go.schedule.latency", "go.gc.pause.latency"} {
		if _, ok := names[name]; ok {
			t.Fatalf("expected no %q on the first collection", name)
		}
	}
}

// Whether a goroutine actually waits for a processor during a test is up to the
// machine, so the emit path is driven by resetting the baseline rather than by
// trying to provoke real scheduling delay.
func TestLatencyQuantilesAreEmittedOnceADeltaExists(t *testing.T) {
	sampler := newRuntimeSampler(latenciesName)
	sampler.read()

	buckets, counts, ok := sampler.histogramDelta(latenciesName)
	if ok {
		t.Fatal("expected the first read to establish a baseline only")
	}

	// Treat everything observed since process start as the interval's delta.
	sampler.previous[latenciesName] = make([]uint64, len(sampler.previous[latenciesName]))
	sampler.read()

	buckets, counts, ok = sampler.histogramDelta(latenciesName)
	if !ok {
		t.Fatal("expected a delta once a baseline exists")
	}

	var total uint64
	for _, count := range counts {
		total += count
	}
	if total == 0 {
		t.Skip("the runtime recorded no scheduling latency observations")
	}

	for _, quantile := range reportedQuantiles {
		value, ok := histogramQuantile(buckets, counts, quantile.value)
		if !ok {
			t.Fatalf("expected %s to be derivable from a non-empty delta", quantile.label)
		}
		if value < 0 || math.IsInf(value, 1) {
			t.Fatalf("expected a finite non-negative %s, got %v", quantile.label, value)
		}
	}
}

func TestRuntimeSamplerIgnoresUnsupportedMetrics(t *testing.T) {
	sampler := newRuntimeSampler(gomaxprocsName, "/not/a/real/metric:units")

	if len(sampler.samples) != 1 {
		t.Fatalf("expected unsupported metrics to be dropped, got %d samples", len(sampler.samples))
	}
	if _, ok := sampler.index["/not/a/real/metric:units"]; ok {
		t.Fatal("expected an unsupported metric to be absent from the index")
	}

	sampler.read()
	if _, ok := sampler.uint64Value(gomaxprocsName); !ok {
		t.Fatal("expected the supported metric to still be readable")
	}
	if _, ok := sampler.float64Value("/not/a/real/metric:units"); ok {
		t.Fatal("expected reading an unsupported metric to report absence")
	}
}

func TestHistogramQuantile(t *testing.T) {
	buckets := []float64{0, 1, 2, 3, math.Inf(1)}

	tests := []struct {
		name     string
		counts   []uint64
		quantile float64
		want     float64
		wantOK   bool
	}{
		{name: "median of a uniform distribution", counts: []uint64{10, 10, 10, 10}, quantile: 0.5, want: 2, wantOK: true},
		{name: "low quantile picks the first bucket", counts: []uint64{10, 10, 10, 10}, quantile: 0.01, want: 1, wantOK: true},
		{name: "sparse p99 picks the final observation", counts: []uint64{1, 1, 0, 0}, quantile: 0.99, want: 2, wantOK: true},
		{name: "unbounded bucket reports its finite lower bound", counts: []uint64{0, 0, 0, 5}, quantile: 0.99, want: 3, wantOK: true},
		{name: "no observations", counts: []uint64{0, 0, 0, 0}, quantile: 0.5, wantOK: false},
		{name: "mismatched bucket count", counts: []uint64{1, 2}, quantile: 0.5, wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := histogramQuantile(buckets, test.counts, test.quantile)
			if ok != test.wantOK {
				t.Fatalf("expected ok=%v, got %v", test.wantOK, ok)
			}
			if ok && got != test.want {
				t.Fatalf("expected quantile %v, got %v", test.want, got)
			}
			if ok && math.IsInf(got, 1) {
				t.Fatal("a quantile must never be reported as +Inf")
			}
		})
	}
}

func TestHistogramDeltaReportsOnlyNewObservations(t *testing.T) {
	sampler := newRuntimeSampler(latenciesName)
	sampler.read()

	if _, _, ok := sampler.histogramDelta(latenciesName); ok {
		t.Fatal("expected the first read to establish a baseline only")
	}

	sampler.read()
	buckets, counts, ok := sampler.histogramDelta(latenciesName)
	if !ok {
		t.Fatal("expected a delta on the second read")
	}
	if len(buckets) != len(counts)+1 {
		t.Fatalf("expected %d buckets for %d counts, got %d", len(counts)+1, len(counts), len(buckets))
	}
}

func TestRuntimeMetricsCallbackIsSafeToCollectRepeatedly(t *testing.T) {
	reader, _ := collectRuntimeMetrics(t)

	for range 3 {
		var rm metricdata.ResourceMetrics
		if err := reader.Collect(context.Background(), &rm); err != nil {
			t.Fatalf("collect metrics: %v", err)
		}
	}
}

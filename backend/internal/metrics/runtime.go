package metrics

import (
	"context"
	"errors"
	"math"
	"runtime/metrics"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Runtime metric names sampled from the Go runtime. Every name is validated
// against runtime/metrics.All() before it is read, so a toolchain that does not
// publish one of them degrades to "metric absent" instead of failing startup.
const (
	cpuUserName     = "/cpu/classes/user:cpu-seconds"
	cpuGCName       = "/cpu/classes/gc/total:cpu-seconds"
	cpuScavengeName = "/cpu/classes/scavenge/total:cpu-seconds"
	cpuIdleName     = "/cpu/classes/idle:cpu-seconds"

	gomaxprocsName = "/sched/gomaxprocs:threads"
	threadsName    = "/sched/threads/total:threads"
	latenciesName  = "/sched/latencies:seconds"

	mutexWaitName = "/sync/mutex/wait/total:seconds"

	gcPausesName    = "/gc/pauses:seconds"
	gcAutomaticName = "/gc/cycles/automatic:gc-cycles"
	gcForcedName    = "/gc/cycles/forced:gc-cycles"
)

// cpuClasses maps a runtime CPU class onto the "class" attribute of go.cpu.time.
// The classes together account for GOMAXPROCS * wall-clock seconds, so
// rate(go.cpu.time) / go.processor.limit is the CPU utilisation of that class.
var cpuClasses = []struct {
	metric string
	class  string
}{
	{cpuUserName, "user"},
	{cpuGCName, "gc"},
	{cpuScavengeName, "scavenge"},
	{cpuIdleName, "idle"},
}

// reportedQuantiles are the latency quantiles published for scheduling delay and
// garbage collector pauses. They are derived from the per-interval delta of the
// underlying cumulative histogram rather than its lifetime contents, so a long
// running process keeps reporting what happened in the last collection window.
var reportedQuantiles = []struct {
	label string
	value float64
}{
	{"p50", 0.50},
	{"p90", 0.90},
	{"p99", 0.99},
}

// minQuantileInterval is the shortest gap between two delta computations for the
// same histogram. The SDK invokes an observable callback once per registered
// reader, so without this a second reader would consume the baseline microseconds
// after the first and report a near-empty interval as though latency had
// collapsed. Any realistic collection interval is far longer than this.
const minQuantileInterval = time.Second

// runtimeSampler reads the Go runtime metrics once per collection cycle and
// shares that sample across every observable instrument registered below.
type runtimeSampler struct {
	mu           sync.Mutex
	samples      []metrics.Sample
	index        map[string]int
	previous     map[string][]uint64
	quantiles    map[string][]float64
	lastComputed map[string]time.Time
}

func newRuntimeSampler(names ...string) *runtimeSampler {
	supported := make(map[string]struct{}, len(metrics.All()))
	for _, description := range metrics.All() {
		supported[description.Name] = struct{}{}
	}

	sampler := &runtimeSampler{
		index:        make(map[string]int, len(names)),
		previous:     make(map[string][]uint64),
		quantiles:    make(map[string][]float64),
		lastComputed: make(map[string]time.Time),
	}
	for _, name := range names {
		if _, ok := supported[name]; !ok {
			continue
		}
		sampler.index[name] = len(sampler.samples)
		sampler.samples = append(sampler.samples, metrics.Sample{Name: name})
	}
	return sampler
}

// read refreshes the shared sample. Callers must hold the sampler lock.
func (s *runtimeSampler) read() {
	if len(s.samples) > 0 {
		metrics.Read(s.samples)
	}
}

func (s *runtimeSampler) float64Value(name string) (float64, bool) {
	position, ok := s.index[name]
	if !ok || s.samples[position].Value.Kind() != metrics.KindFloat64 {
		return 0, false
	}
	return s.samples[position].Value.Float64(), true
}

func (s *runtimeSampler) uint64Value(name string) (uint64, bool) {
	position, ok := s.index[name]
	if !ok || s.samples[position].Value.Kind() != metrics.KindUint64 {
		return 0, false
	}
	return s.samples[position].Value.Uint64(), true
}

// histogramDelta returns the bucket boundaries and the counts accumulated since
// the previous collection. The first call establishes the baseline and reports
// no observations, so process startup never appears as a latency spike.
func (s *runtimeSampler) histogramDelta(name string) ([]float64, []uint64, bool) {
	position, ok := s.index[name]
	if !ok || s.samples[position].Value.Kind() != metrics.KindFloat64Histogram {
		return nil, nil, false
	}

	histogram := s.samples[position].Value.Float64Histogram()
	if histogram == nil {
		return nil, nil, false
	}

	previous, seen := s.previous[name]
	current := make([]uint64, len(histogram.Counts))
	copy(current, histogram.Counts)
	s.previous[name] = current

	if !seen || len(previous) != len(current) {
		return nil, nil, false
	}

	delta := make([]uint64, len(current))
	for i := range current {
		if current[i] >= previous[i] {
			delta[i] = current[i] - previous[i]
		}
	}
	return histogram.Buckets, delta, true
}

// histogramQuantile approximates a quantile from runtime/metrics bucket
// boundaries. Buckets holds len(counts)+1 boundaries; the upper boundary of the
// matching bucket is reported, and the finite lower boundary is used for an
// unbounded final bucket so a single slow observation cannot report +Inf.
func histogramQuantile(buckets []float64, counts []uint64, quantile float64) (float64, bool) {
	if len(buckets) != len(counts)+1 {
		return 0, false
	}

	var total uint64
	for _, count := range counts {
		total += count
	}
	if total == 0 {
		return 0, false
	}

	target := uint64(math.Ceil(float64(total) * quantile))
	target = min(max(target, 1), total)

	var seen uint64
	for i, count := range counts {
		seen += count
		if seen < target {
			continue
		}
		if upper := buckets[i+1]; !math.IsInf(upper, 1) {
			return upper, true
		}
		return buckets[i], true
	}
	return buckets[len(buckets)-1], true
}

// StartRuntimeMetrics publishes Go runtime CPU attribution, scheduler pressure,
// and mutex contention as OpenTelemetry metrics.
//
// These answer questions the existing runtime instrumentation cannot: which
// class of work consumed the CPU (application code, garbage collection, or the
// scavenger), whether runnable goroutines are waiting for a processor, and
// whether time is lost to lock contention. They are sampled through
// runtime/metrics, which unlike runtime.ReadMemStats does not stop the world.
func StartRuntimeMetrics(provider metric.MeterProvider) error {
	if provider == nil {
		return errors.New("runtime metrics require a meter provider")
	}
	meter := provider.Meter("flightstrips/runtime")

	names := []string{gomaxprocsName, threadsName, latenciesName, mutexWaitName, gcPausesName, gcAutomaticName, gcForcedName}
	for _, class := range cpuClasses {
		names = append(names, class.metric)
	}
	sampler := newRuntimeSampler(names...)

	cpuTime, err := meter.Float64ObservableCounter(
		"go.cpu.time",
		metric.WithDescription("CPU time consumed by the Go runtime, split by the class of work that consumed it"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}
	processorLimit, err := meter.Int64ObservableUpDownCounter(
		"go.processor.limit",
		metric.WithDescription("GOMAXPROCS, the processor count go.cpu.time is measured against"),
		metric.WithUnit("{thread}"),
	)
	if err != nil {
		return err
	}
	osThreads, err := meter.Int64ObservableUpDownCounter(
		"go.thread.count",
		metric.WithDescription("Operating system threads owned by the Go runtime"),
		metric.WithUnit("{thread}"),
	)
	if err != nil {
		return err
	}
	scheduleLatency, err := meter.Float64ObservableGauge(
		"go.schedule.latency",
		metric.WithDescription("Time runnable goroutines waited for a processor during the last collection interval"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}
	mutexWait, err := meter.Float64ObservableCounter(
		"go.mutex.wait.time",
		metric.WithDescription("Total time goroutines spent blocked on runtime mutexes"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}
	gcPause, err := meter.Float64ObservableGauge(
		"go.gc.pause.latency",
		metric.WithDescription("Stop-the-world garbage collector pause duration during the last collection interval"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}
	gcCycles, err := meter.Int64ObservableCounter(
		"go.gc.cycles",
		metric.WithDescription("Completed garbage collection cycles by trigger"),
		metric.WithUnit("{cycle}"),
	)
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		sampler.mu.Lock()
		defer sampler.mu.Unlock()

		sampler.read()

		for _, class := range cpuClasses {
			if seconds, ok := sampler.float64Value(class.metric); ok {
				observer.ObserveFloat64(cpuTime, seconds, metric.WithAttributes(attribute.String("class", class.class)))
			}
		}
		if procs, ok := sampler.uint64Value(gomaxprocsName); ok {
			observer.ObserveInt64(processorLimit, int64(procs))
		}
		if threads, ok := sampler.uint64Value(threadsName); ok {
			observer.ObserveInt64(osThreads, int64(threads))
		}
		if seconds, ok := sampler.float64Value(mutexWaitName); ok {
			observer.ObserveFloat64(mutexWait, seconds)
		}
		if automatic, ok := sampler.uint64Value(gcAutomaticName); ok {
			observer.ObserveInt64(gcCycles, int64(automatic), metric.WithAttributes(attribute.String("trigger", "automatic")))
		}
		if forced, ok := sampler.uint64Value(gcForcedName); ok {
			observer.ObserveInt64(gcCycles, int64(forced), metric.WithAttributes(attribute.String("trigger", "forced")))
		}

		now := time.Now()
		observeQuantiles(observer, scheduleLatency, sampler, latenciesName, now)
		observeQuantiles(observer, gcPause, sampler, gcPausesName, now)
		return nil
	}, cpuTime, processorLimit, osThreads, scheduleLatency, mutexWait, gcPause, gcCycles)

	return err
}

// quantilesFor returns this interval's quantiles, in the order of
// reportedQuantiles. A computation within minQuantileInterval of the previous one
// reuses the cached result rather than consuming a fresh delta, so every reader
// collecting in the same cycle observes the same interval.
func (s *runtimeSampler) quantilesFor(name string, now time.Time) ([]float64, bool) {
	if computed, seen := s.lastComputed[name]; seen && now.Sub(computed) < minQuantileInterval {
		values, cached := s.quantiles[name]
		return values, cached
	}

	buckets, counts, ok := s.histogramDelta(name)
	if !ok {
		return nil, false
	}

	values := make([]float64, 0, len(reportedQuantiles))
	for _, quantile := range reportedQuantiles {
		value, ok := histogramQuantile(buckets, counts, quantile.value)
		if !ok {
			return nil, false
		}
		values = append(values, value)
	}

	s.quantiles[name] = values
	s.lastComputed[name] = now
	return values, true
}

func observeQuantiles(observer metric.Observer, gauge metric.Float64ObservableGauge, sampler *runtimeSampler, name string, now time.Time) {
	values, ok := sampler.quantilesFor(name, now)
	if !ok {
		return
	}
	for i, quantile := range reportedQuantiles {
		observer.ObserveFloat64(gauge, values[i], metric.WithAttributes(attribute.String("quantile", quantile.label)))
	}
}

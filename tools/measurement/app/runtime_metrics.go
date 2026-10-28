package app

import "runtime/metrics"

// Runtime counters and gauges are cheap scalar observations. Keep allocation
// counters separate from GC-delayed, sampled allocation-stack profiles.
func runtimeMetrics() map[string]any {
	names := []string{"/sched/goroutines:goroutines", "/memory/classes/heap/objects:bytes", "/gc/heap/live:bytes", "/gc/heap/allocs:bytes", "/gc/heap/allocs:objects", "/gc/heap/frees:bytes", "/gc/cycles/total:gc-cycles", "/cpu/classes/gc/total:cpu-seconds"}
	labels := []string{"go_goroutines", "go_heap_objects_bytes", "go_heap_live_bytes", "go_alloc_bytes_total", "go_alloc_objects_total", "go_free_bytes_total", "go_gc_cycles_total", "go_gc_cpu_seconds_total"}
	samples := make([]metrics.Sample, len(names))
	for i, n := range names {
		samples[i].Name = n
	}
	metrics.Read(samples)
	result := make(map[string]any, len(names))
	for i, s := range samples {
		switch s.Value.Kind() {
		case metrics.KindUint64:
			result[labels[i]] = s.Value.Uint64()
		case metrics.KindFloat64:
			result[labels[i]] = s.Value.Float64()
		}
	}
	return result
}

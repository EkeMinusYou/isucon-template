import assert from 'node:assert/strict'
import { test } from 'node:test'
import { runtimeDisplaySeries } from '../src/runtimeMetrics.ts'

test('computes rates from timestamps, preserving actual zero gauges', () => {
  const series = runtimeDisplaySeries([
    { elapsed_ms: 0, timestamp: '2026-01-01T00:00:00Z', values: { go_process_started_unix_ms: 1, go_alloc_bytes_total: 100, go_gc_cpu_seconds_total: 1, go_goroutines: 0 } },
    { elapsed_ms: 2000, timestamp: '2026-01-01T00:00:02Z', values: { go_process_started_unix_ms: 1, go_alloc_bytes_total: 500, go_gc_cpu_seconds_total: 1.2, go_goroutines: 7 } },
  ])
  assert.equal(series[0].go_goroutines, 0)
  assert.equal(series[0].allocation_bytes_per_sec, null)
  assert.equal(series[1].allocation_bytes_per_sec, 200)
  assert.ok(Math.abs(series[1].gc_cpu_seconds_per_sec! - 0.1) < 1e-10)
})

test('missing counters, resets and process changes do not become zero rates', () => {
  const series = runtimeDisplaySeries([
    { elapsed_ms: 0, timestamp: '2026-01-01T00:00:00Z', values: { go_process_started_unix_ms: 1, go_alloc_bytes_total: 100 } },
    { elapsed_ms: 1000, timestamp: '2026-01-01T00:00:01Z', values: { go_process_started_unix_ms: 1 } },
    { elapsed_ms: 2000, timestamp: '2026-01-01T00:00:02Z', values: { go_process_started_unix_ms: 1, go_alloc_bytes_total: 500 } },
    { elapsed_ms: 3000, timestamp: '2026-01-01T00:00:03Z', values: { go_process_started_unix_ms: 2, go_alloc_bytes_total: 600 } },
    { elapsed_ms: 4000, timestamp: '2026-01-01T00:00:04Z', values: { go_process_started_unix_ms: 2, go_alloc_bytes_total: 10 } },
  ])
  assert.ok(series.every((point) => point.allocation_bytes_per_sec === null))
  assert.ok(series.every((point) => point.go_heap_live_bytes === null))
})

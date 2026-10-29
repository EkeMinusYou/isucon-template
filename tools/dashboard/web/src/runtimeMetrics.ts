import type { RuntimePoint } from './api.ts'

export type RuntimeDisplayPoint = {
  elapsed_ms: number
  go_goroutines: number | null
  go_heap_objects_bytes: number | null
  go_heap_live_bytes: number | null
  allocation_bytes_per_sec: number | null
  gc_cpu_seconds_per_sec: number | null
}

export function runtimeDisplaySeries(points: RuntimePoint[]): RuntimeDisplayPoint[] {
  const sorted = [...points].sort((a, b) => Date.parse(a.timestamp) - Date.parse(b.timestamp))
  return sorted.map((point, index) => {
    const previous = sorted[index - 1]
    const value = (name: string): number | null => {
      const raw = point.values[name]
      return Number.isFinite(raw) ? raw : null
    }
    const rate = (name: string): number | null => {
      if (!previous || !Number.isFinite(point.values.go_process_started_unix_ms) ||
        point.values.go_process_started_unix_ms !== previous.values.go_process_started_unix_ms) return null
      const now = value(name)
      const before = previous.values[name]
      const seconds = (Date.parse(point.timestamp) - Date.parse(previous.timestamp)) / 1000
      if (now == null || !Number.isFinite(before) || now < before || !(seconds > 0)) return null
      return (now - before) / seconds
    }
    return {
      elapsed_ms: point.elapsed_ms,
      go_goroutines: value('go_goroutines'),
      go_heap_objects_bytes: value('go_heap_objects_bytes'),
      go_heap_live_bytes: value('go_heap_live_bytes'),
      allocation_bytes_per_sec: rate('go_alloc_bytes_total'),
      gc_cpu_seconds_per_sec: rate('go_gc_cpu_seconds_total'),
    }
  })
}

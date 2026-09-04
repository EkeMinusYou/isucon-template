// Categorical palette slots in fixed order (never cycled per-series by rank —
// callers must map a stable key -> index once and reuse it).
export const SERIES_COLORS = [
  'var(--series-1)',
  'var(--series-2)',
  'var(--series-3)',
  'var(--series-4)',
  'var(--series-5)',
  'var(--series-6)',
  'var(--series-7)',
  'var(--series-8)',
]

export function colorForIndex(i: number): string {
  return SERIES_COLORS[i % SERIES_COLORS.length]
}

// Assigns a stable color to each key the first time it is seen, in the
// order the keys are provided (e.g. sorted host/service names), so the same
// entity keeps the same color across charts and re-renders.
export function buildColorMap(keys: string[]): Map<string, string> {
  const sorted = [...new Set(keys)].sort()
  const map = new Map<string, string>()
  sorted.forEach((key, i) => map.set(key, colorForIndex(i)))
  return map
}

export const STATUS_COLUMN_ORDER = [
  'INVESTIGATE',
  'READY',
  'DOING',
  'VERIFY',
  'APPLIED',
  'BLOCKED',
  'VALIDATED',
  'REJECTED',
]

export const STATUS_COLORS: Record<string, string> = {
  INVESTIGATE: 'var(--series-4)',
  READY: 'var(--series-1)',
  DOING: 'var(--series-3)',
  VERIFY: 'var(--series-7)',
  APPLIED: 'var(--status-good)',
  BLOCKED: 'var(--status-critical)',
  VALIDATED: 'var(--series-6)',
  REJECTED: 'var(--chart-muted)',
}

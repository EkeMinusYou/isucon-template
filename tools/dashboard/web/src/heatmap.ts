import type { CSSProperties } from 'react'

// Sequential single-hue ramp (dataviz skill default: blue, light -> dark),
// applied as a translucent overlay so it reads correctly on both the light
// and dark table backgrounds without needing separate per-theme ramps.
const HEATMAP_RGB: [number, number, number] = [42, 120, 214]
const MIN_ALPHA = 0.05
const MAX_ALPHA = 0.42

export function columnRange(values: number[]): [number, number] {
  if (values.length === 0) return [0, 0]
  let min = values[0]
  let max = values[0]
  for (const v of values) {
    if (v < min) min = v
    if (v > max) max = v
  }
  return [min, max]
}

// Maps a value's position within [min, max] to a translucent background —
// an Excel-style "color scale": low values fade toward the surface, high
// values (the notable ones) darken toward the sequential hue.
export function heatmapStyle(value: number, [min, max]: [number, number]): CSSProperties | undefined {
  if (!Number.isFinite(value) || max <= min) return undefined
  const t = Math.min(1, Math.max(0, (value - min) / (max - min)))
  const alpha = MIN_ALPHA + t * (MAX_ALPHA - MIN_ALPHA)
  return { backgroundColor: `rgba(${HEATMAP_RGB[0]}, ${HEATMAP_RGB[1]}, ${HEATMAP_RGB[2]}, ${alpha.toFixed(3)})` }
}

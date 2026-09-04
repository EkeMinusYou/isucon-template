import { useMemo, useState } from 'react'

export type SortDir = 'asc' | 'desc'

// Client-side sort-by-column-click for a table. Clicking the active column
// again flips direction; clicking a new column switches to it (descending
// first, since the interesting rows for these tables are usually the
// largest values).
export function useSort<T extends Record<string, unknown>>(
  rows: T[],
  initialKey: keyof T,
  initialDir: SortDir = 'desc',
) {
  const [sortKey, setSortKey] = useState<keyof T>(initialKey)
  const [sortDir, setSortDir] = useState<SortDir>(initialDir)

  const sorted = useMemo(() => {
    const copy = [...rows]
    copy.sort((a, b) => {
      const av = a[sortKey]
      const bv = b[sortKey]
      let cmp: number
      if (typeof av === 'number' && typeof bv === 'number') {
        cmp = av - bv
      } else {
        cmp = String(av).localeCompare(String(bv))
      }
      return sortDir === 'asc' ? cmp : -cmp
    })
    return copy
  }, [rows, sortKey, sortDir])

  function toggle(key: keyof T) {
    if (key === sortKey) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(key)
      setSortDir('desc')
    }
  }

  return { sorted, sortKey, sortDir, toggle }
}

import { useCallback, useEffect, useState } from 'react'

export type ThemePref = 'system' | 'light' | 'dark'

const STORAGE_KEY = 'isucon-dashboard-theme'

function readStored(): ThemePref {
  const raw = localStorage.getItem(STORAGE_KEY)
  return raw === 'light' || raw === 'dark' ? raw : 'system'
}

/**
 * daisyUI のテーマを `data-theme` で切り替える。'system' のときは属性を外し、
 * daisyUI 側の `--prefersdark` と index.css の prefers-color-scheme に任せる。
 */
export function useTheme(): [ThemePref, (next: ThemePref) => void] {
  const [pref, setPref] = useState<ThemePref>(() => readStored())

  useEffect(() => {
    const root = document.documentElement
    if (pref === 'system') root.removeAttribute('data-theme')
    else root.setAttribute('data-theme', pref)
    localStorage.setItem(STORAGE_KEY, pref)
  }, [pref])

  const update = useCallback((next: ThemePref) => setPref(next), [])
  return [pref, update]
}

import type { RunInfo } from '../api'

type Props = {
  runs: RunInfo[]
  selected: string | null
  onChange: (runId: string) => void
  /** join でまとめるときに join-item を渡す */
  className?: string
}

export function RunSelector({ runs, selected, onChange, className = '' }: Props) {
  if (runs.length === 0) {
    return <span className="badge badge-ghost badge-sm">RUNなし</span>
  }
  return (
    <label className={`select select-sm w-72 ${className}`}>
      <span className="label">RUN</span>
      <select value={selected ?? ''} onChange={(e) => onChange(e.target.value)}>
        {runs.map((run) => (
          <option key={run.run_id} value={run.run_id}>
            {run.run_id}
          </option>
        ))}
      </select>
    </label>
  )
}

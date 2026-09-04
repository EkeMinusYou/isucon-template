import { marked } from 'marked'
import { useEffect, useState } from 'react'
import { api, type ReportInfo } from '../api'
import { EmptyState } from './EmptyState'

type Props = {
  reports: ReportInfo[]
}

export function ReportsPanel({ reports }: Props) {
  const [selected, setSelected] = useState<string | null>(null)
  const [content, setContent] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // `reports` loads asynchronously after this component first mounts, so the
  // initial selection has to be made once it arrives rather than at useState
  // init time (which would only ever see the empty initial `reports` prop).
  useEffect(() => {
    if (!selected && reports.length > 0) {
      setSelected(reports[0].filename)
    }
  }, [reports, selected])

  useEffect(() => {
    if (!selected) return
    let cancelled = false
    setLoading(true)
    setError(null)
    api
      .reportContent(selected)
      .then((res) => {
        if (!cancelled) setContent(res.content)
      })
      .catch((err) => {
        if (!cancelled) setError(String(err.message ?? err))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [selected])

  if (reports.length === 0) {
    return (
      <EmptyState
        title="docs/reports にレポートがありません"
        detail="isucon-analyze などの解析スキルを実行するとここに並びます"
      />
    )
  }

  const html = content ? marked.parse(content, { async: false }) : ''

  // `reports` arrives newest-first, so grouping in encounter order keeps both
  // the report types and the reports inside each type sorted by recency.
  const groups: { kind: string; reports: ReportInfo[] }[] = []
  for (const r of reports) {
    const kind = r.kind || '(直下)'
    const group = groups.find((g) => g.kind === kind)
    if (group) group.reports.push(r)
    else groups.push({ kind, reports: [r] })
  }

  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-[340px_1fr]">
      <ul className="menu h-[calc(100vh-14rem)] min-h-[320px] w-full flex-nowrap overflow-y-auto rounded-box border border-base-300 bg-base-100 p-2 text-base">
        {groups.map((g) => (
          <li key={g.kind}>
            <h2 className="menu-title font-mono text-xs">
              {g.kind}
              <span className="ml-2 opacity-60">{g.reports.length}</span>
            </h2>
            <ul>
              {g.reports.map((r) => (
                <li key={r.filename}>
                  <button
                    className={selected === r.filename ? 'menu-active' : ''}
                    onClick={() => setSelected(r.filename)}
                  >
                    <span className="flex flex-col items-start gap-0.5 py-1">
                      <span className="font-medium">{r.title}</span>
                      <span className="font-mono text-xs opacity-60">
                        {r.filename.split('/').pop()}
                      </span>
                      <span className="text-xs opacity-50">
                        {new Date(r.mod_time).toLocaleString()}
                      </span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ul>
      <div className="h-[calc(100vh-14rem)] min-h-[320px] overflow-y-auto rounded-box border border-base-300 bg-base-100 p-6">
        {loading && (
          <div className="flex flex-col gap-3">
            <div className="skeleton h-8 w-1/2" />
            <div className="skeleton h-4 w-full" />
            <div className="skeleton h-4 w-full" />
            <div className="skeleton h-4 w-4/5" />
          </div>
        )}
        {error && (
          <div className="alert alert-error text-base">
            <span>{error}</span>
          </div>
        )}
        {!loading && !error && (
          <div className="prose dark:prose-invert max-w-none" dangerouslySetInnerHTML={{ __html: html }} />
        )}
      </div>
    </div>
  )
}

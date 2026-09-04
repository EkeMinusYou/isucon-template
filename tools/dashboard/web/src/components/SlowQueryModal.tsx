import { useEffect, useRef } from 'react'
import type { SlowQueryClass } from '../api'

type Props = {
  query: SlowQueryClass | null
  onClose: () => void
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="stat px-3 py-2">
      <div className="stat-title text-xs">{label}</div>
      <div className="stat-value text-lg tabular-nums">{value}</div>
    </div>
  )
}

function MetricGroup({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mt-4">
      <div className="divider divider-start my-1 text-sm font-semibold">{title}</div>
      <div className="stats stats-horizontal w-full overflow-x-auto border border-base-300 bg-base-100">
        {children}
      </div>
    </div>
  )
}

const ms = (v: number, digits = 2) => `${(v * 1000).toFixed(digits)}ms`

export function SlowQueryModal({ query, onClose }: Props) {
  const dialogRef = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    if (query) {
      dialogRef.current?.showModal()
    } else {
      dialogRef.current?.close()
    }
  }, [query])

  return (
    <dialog ref={dialogRef} className="modal" onClose={onClose}>
      <div className="modal-box max-w-4xl">
        <form method="dialog">
          <button className="btn btn-sm btn-circle btn-ghost absolute top-3 right-3" aria-label="閉じる">
            ✕
          </button>
        </form>
        {query && (
          <>
            <div className="mb-1 flex items-start gap-3 pr-8">
              <h3 className="text-2xl font-bold">スロークエリ詳細</h3>
              <span className="badge badge-soft badge-primary badge-lg shrink-0 font-mono">
                {query.query_count.toLocaleString()} calls
              </span>
            </div>
            <p className="mb-4 text-sm text-base-content/50">
              slp（SQLパーサーでの抽象化）による正規化クエリ。リテラルは N / 'S' に置換済み
            </p>

            <div className="divider divider-start my-1 text-sm font-semibold">クエリ（正規化）</div>
            <pre className="overflow-x-auto rounded-box border border-base-300 bg-base-200 p-4 text-sm whitespace-pre-wrap">
              <code>{query.query}</code>
            </pre>

            <MetricGroup title="Query time">
              <Metric label="呼び出し回数" value={query.query_count.toLocaleString()} />
              <Metric label="全体に占める割合" value={`${query.query_time_pct.toFixed(1)}%`} />
              <Metric label="合計" value={`${query.query_time_sum.toFixed(3)}s`} />
              <Metric label="平均" value={ms(query.query_time_avg)} />
              <Metric label="最小" value={ms(query.query_time_min)} />
              <Metric label="95%ile" value={ms(query.query_time_p95)} />
              <Metric label="最大" value={ms(query.query_time_max)} />
            </MetricGroup>

            <MetricGroup title="Lock time">
              <Metric label="合計" value={ms(query.lock_time_sum, 3)} />
              <Metric label="平均" value={ms(query.lock_time_avg, 3)} />
              <Metric label="最大" value={ms(query.lock_time_max, 3)} />
            </MetricGroup>

            <MetricGroup title="Rows examined / sent">
              <Metric label="examined 合計" value={query.rows_examined_sum.toLocaleString()} />
              <Metric label="examined 平均" value={query.rows_examined_avg.toLocaleString()} />
              <Metric label="examined 最大" value={query.rows_examined_max.toLocaleString()} />
              <Metric label="sent 合計" value={query.rows_sent_sum.toLocaleString()} />
              <Metric label="sent 平均" value={query.rows_sent_avg.toLocaleString()} />
            </MetricGroup>
          </>
        )}
      </div>
      <form method="dialog" className="modal-backdrop">
        <button>close</button>
      </form>
    </dialog>
  )
}

import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { ScoreEntry } from '../api'
import { EmptyState } from './EmptyState'

type Props = {
  scores: ScoreEntry[]
}

export function ScoreTrend({ scores }: Props) {
  if (scores.length === 0) {
    return (
      <EmptyState
        title="スコア履歴がありません"
        detail="runs/scores.tsv にまだ記録がありません"
      />
    )
  }

  const data = scores.map((s) => ({ ...s, label: s.run_id.slice(4, 13) }))
  const best = scores.reduce((a, b) => (b.score > a.score ? b : a))
  const latest = scores[scores.length - 1]

  return (
    <>
    <div className="stats stats-horizontal w-full overflow-x-auto border border-base-300 bg-base-100">
      <div className="stat px-4 py-2">
        <div className="stat-title text-sm">最新スコア</div>
        <div className="stat-value text-2xl tabular-nums">{latest.score.toLocaleString()}</div>
        <div className="stat-desc font-mono text-xs">{latest.run_id}</div>
      </div>
      <div className="stat px-4 py-2">
        <div className="stat-title text-sm">ベストスコア</div>
        <div className="stat-value text-2xl tabular-nums text-success">{best.score.toLocaleString()}</div>
        <div className="stat-desc font-mono text-xs">{best.run_id}</div>
      </div>
      <div className="stat px-4 py-2">
        <div className="stat-title text-sm">RUN数</div>
        <div className="stat-value text-2xl tabular-nums">{scores.length}</div>
      </div>
    </div>
    <ResponsiveContainer width="100%" height={260}>
      <LineChart data={data} margin={{ top: 8, right: 16, bottom: 8, left: 0 }}>
        <CartesianGrid stroke="var(--chart-grid)" vertical={false} />
        <XAxis
          dataKey="label"
          tick={{ fontSize: 13, fill: 'var(--chart-muted)' }}
          axisLine={{ stroke: 'var(--chart-baseline)' }}
          tickLine={false}
        />
        <YAxis
          tick={{ fontSize: 13, fill: 'var(--chart-muted)' }}
          axisLine={false}
          tickLine={false}
          width={56}
        />
        <Tooltip
          cursor={{ stroke: 'var(--chart-cursor-line)', strokeWidth: 1 }}
          contentStyle={{
            background: 'var(--chart-tooltip-bg)',
            color: 'var(--chart-tooltip-text)',
            border: '1px solid var(--chart-grid)',
            borderRadius: 8,
            fontSize: 12,
          }}
          labelFormatter={(_, payload) => payload?.[0]?.payload?.run_id ?? ''}
          formatter={(value) => [Number(value).toLocaleString(), 'score']}
        />
        <Line
          type="monotone"
          dataKey="score"
          stroke="var(--series-1)"
          strokeWidth={2}
          dot={{ r: 3, fill: 'var(--series-1)' }}
          activeDot={{ r: 5 }}
        />
      </LineChart>
    </ResponsiveContainer>
    </>
  )
}

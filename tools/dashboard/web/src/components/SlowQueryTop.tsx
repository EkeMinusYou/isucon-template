import { useMemo, useState } from 'react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { SlowQueryClass, SlowQueryResponse } from '../api'
import { columnRange, heatmapStyle } from '../heatmap'
import { useSort } from '../useSort'
import { ChartCard } from './ChartCard'
import { EmptyState } from './EmptyState'
import { SlowQueryModal } from './SlowQueryModal'
import { SortableTh } from './SortableTh'
import { TableScroll } from './TableScroll'

type Props = {
  data: SlowQueryResponse
}

function shortText(s: string, max = 50): string {
  return s.length > max ? s.slice(0, max - 1) + '…' : s
}

export function SlowQueryTop({ data }: Props) {
  const [selected, setSelected] = useState<SlowQueryClass | null>(null)

  const top = useMemo(
    () => data.classes.slice(0, 10).map((c) => ({ ...c, label: shortText(c.query) })),
    [data.classes],
  )

  const ranges = useMemo(
    () => ({
      query_count: columnRange(data.classes.map((c) => c.query_count)),
      query_time_sum: columnRange(data.classes.map((c) => c.query_time_sum)),
      query_time_avg: columnRange(data.classes.map((c) => c.query_time_avg)),
      query_time_pct: columnRange(data.classes.map((c) => c.query_time_pct)),
    }),
    [data.classes],
  )

  const { sorted, sortKey, sortDir, toggle } = useSort<SlowQueryClass>(data.classes, 'query_time_sum', 'desc')
  const rowKey = (c: SlowQueryClass) => c.query

  if (!data.available || data.classes.length === 0) {
    return (
      <EmptyState
        title="slp.tsv がまだありません"
        detail="計測パイプライン更新前のRUN、preflight 失敗、slp 未インストール、または集計失敗の可能性があります（詳細は slp.stderr）"
      />
    )
  }

  const th = (label: string, key: keyof SlowQueryClass, align: 'left' | 'right' = 'right') => (
    <SortableTh label={label} align={align} active={sortKey === key} dir={sortDir} onClick={() => toggle(key)} />
  )

  return (
    <>
      <div className="stats stats-horizontal w-full overflow-x-auto border border-base-300 bg-base-100">
        <div className="stat px-4 py-2">
          <div className="stat-title text-sm">総クエリ数</div>
          <div className="stat-value text-2xl">{data.total_query_count.toLocaleString()}</div>
        </div>
        <div className="stat px-4 py-2">
          <div className="stat-title text-sm">ユニーク数</div>
          <div className="stat-value text-2xl">{data.unique_query_count.toLocaleString()}</div>
        </div>
        <div className="stat px-4 py-2">
          <div className="stat-title text-sm">総実行時間</div>
          <div className="stat-value text-2xl">{data.total_query_time_sum.toFixed(2)}s</div>
        </div>
        <div className="stat px-4 py-2">
          <div className="stat-title text-sm">総ロック時間</div>
          <div className="stat-value text-2xl">{data.total_lock_time_sum.toFixed(2)}s</div>
        </div>
        <div className="stat px-4 py-2">
          <div className="stat-title text-sm">総examined行数</div>
          <div className="stat-value text-2xl">{Math.round(data.total_rows_examined_sum).toLocaleString()}</div>
        </div>
        <div className="stat px-4 py-2">
          <div className="stat-title text-sm">総sent行数</div>
          <div className="stat-value text-2xl">{Math.round(data.total_rows_sent_sum).toLocaleString()}</div>
        </div>
        {data.truncated && (
          <div className="stat px-4 py-2">
            <div className="stat-title text-sm">表示件数</div>
            <div className="stat-value text-2xl">
              {data.classes.length} / {data.total_classes}
            </div>
          </div>
        )}
      </div>
      <ChartCard title="合計実行時間 上位10件" unit="s">
      <ResponsiveContainer width="100%" height={320}>
        <BarChart data={top} layout="vertical" margin={{ top: 4, right: 24, bottom: 4, left: 4 }}>
          <CartesianGrid stroke="var(--chart-grid)" horizontal={false} />
          <XAxis
            type="number"
            tick={{ fontSize: 13, fill: 'var(--chart-muted)' }}
            axisLine={{ stroke: 'var(--chart-baseline)' }}
            tickLine={false}
            label={{
              value: '合計実行時間 (s)',
              position: 'insideBottomRight',
              offset: -2,
              fontSize: 13,
              fill: 'var(--chart-muted)',
            }}
          />
          <YAxis
            type="category"
            dataKey="label"
            width={340}
            tick={{ fontSize: 13, fill: 'currentColor' }}
            axisLine={false}
            tickLine={false}
          />
          <Tooltip
            cursor={{ fill: 'var(--chart-cursor-fill)' }}
            contentStyle={{
              background: 'var(--chart-tooltip-bg)',
              color: 'var(--chart-tooltip-text)',
              border: '1px solid var(--chart-grid)',
              borderRadius: 8,
              fontSize: 13,
            }}
            formatter={(value, _name, item) => {
              const row = item.payload as SlowQueryClass
              return [
                `sum=${Number(value).toFixed(3)}s calls=${row.query_count} avg=${(row.query_time_avg * 1000).toFixed(2)}ms`,
                shortText(row.query),
              ]
            }}
          />
          <Bar
            dataKey="query_time_sum"
            fill="var(--series-7)"
            radius={[0, 4, 4, 0]}
            cursor="pointer"
            onClick={(entry) => setSelected(entry as unknown as SlowQueryClass)}
          />
        </BarChart>
      </ResponsiveContainer>
      </ChartCard>
      <TableScroll>
        <table className="table table-zebra table-pin-rows text-base">
          <thead>
            <tr>
              {th('query', 'query', 'left')}
              {th('calls', 'query_count')}
              {th('sum(s)', 'query_time_sum')}
              {th('avg(ms)', 'query_time_avg')}
              {th('% of total', 'query_time_pct')}
            </tr>
          </thead>
          <tbody>
            {sorted.map((c) => (
              <tr
                key={rowKey(c)}
                className="cursor-pointer hover:bg-base-200"
                onClick={() => setSelected(c)}
              >
                <td className="max-w-md truncate" title={c.query}>
                  {shortText(c.query)}
                </td>
                <td className="text-right tabular-nums" style={heatmapStyle(c.query_count, ranges.query_count)}>
                  {c.query_count.toLocaleString()}
                </td>
                <td
                  className="text-right tabular-nums"
                  style={heatmapStyle(c.query_time_sum, ranges.query_time_sum)}
                >
                  {c.query_time_sum.toFixed(3)}
                </td>
                <td
                  className="text-right tabular-nums"
                  style={heatmapStyle(c.query_time_avg, ranges.query_time_avg)}
                >
                  {(c.query_time_avg * 1000).toFixed(2)}
                </td>
                <td
                  className="text-right tabular-nums"
                  style={heatmapStyle(c.query_time_pct, ranges.query_time_pct)}
                >
                  {c.query_time_pct.toFixed(1)}%
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableScroll>
      <SlowQueryModal query={selected} onClose={() => setSelected(null)} />
    </>
  )
}

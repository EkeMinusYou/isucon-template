import { useMemo } from 'react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { AlpRow } from '../api'
import { columnRange, heatmapStyle } from '../heatmap'
import { useSort } from '../useSort'
import { ChartCard } from './ChartCard'
import { EmptyState } from './EmptyState'
import { SortableTh } from './SortableTh'
import { TableScroll } from './TableScroll'

type Props = {
  rows: AlpRow[]
}

function shortUri(uri: string, max = 50): string {
  return uri.length > max ? uri.slice(0, max - 1) + '…' : uri
}

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'] as const

function formatBytes(bytes: number | undefined): string {
  if (bytes == null || !Number.isFinite(bytes)) return '—'

  const sign = bytes < 0 ? '-' : ''
  let value = Math.abs(bytes)
  let unitIndex = 0
  while (value >= 1000 && unitIndex < BYTE_UNITS.length - 1) {
    value /= 1000
    unitIndex += 1
  }

  const maximumFractionDigits = unitIndex === 0 ? 0 : value >= 100 ? 0 : value >= 10 ? 1 : 2
  return `${sign}${value.toLocaleString('ja-JP', { maximumFractionDigits })} ${BYTE_UNITS[unitIndex]}`
}

function exactBytes(bytes: number | undefined): string | undefined {
  if (bytes == null || !Number.isFinite(bytes)) return undefined
  return `${bytes.toLocaleString('ja-JP')} bytes`
}

export function AlpTop({ rows }: Props) {
  const top = useMemo(
    () => rows.slice(0, 10).map((r) => ({ ...r, label: `${r.method} ${shortUri(r.uri)}` })),
    [rows],
  )

  const ranges = useMemo(
    () => ({
      count: columnRange(rows.map((r) => r.count)),
      sum: columnRange(rows.map((r) => r.sum)),
      avg: columnRange(rows.map((r) => r.avg)),
      max: columnRange(rows.map((r) => r.max)),
      p90: columnRange(rows.map((r) => r.p90)),
      p99: columnRange(rows.map((r) => r.p99)),
    }),
    [rows],
  )

  const showBody = rows.some((r) => r.sum_body != null || r.avg_body != null)
  const { sorted, sortKey, sortDir, toggle } = useSort<AlpRow>(rows, 'sum', 'desc')

  if (rows.length === 0) {
    return (
      <EmptyState
        title="alp.json がまだありません"
        detail="このRUNは計測パイプライン更新前のRUN、または preflight 失敗の可能性があります"
      />
    )
  }

  const th = (label: string, key: keyof AlpRow, align: 'left' | 'right' = 'right') => (
    <SortableTh label={label} align={align} active={sortKey === key} dir={sortDir} onClick={() => toggle(key)} />
  )

  return (
    <>
      <ChartCard title="SUM 上位10件" unit="s">
      <ResponsiveContainer width="100%" height={320}>
        <BarChart data={top} layout="vertical" margin={{ top: 4, right: 24, bottom: 4, left: 4 }}>
          <CartesianGrid stroke="var(--chart-grid)" horizontal={false} />
          <XAxis
            type="number"
            tick={{ fontSize: 13, fill: 'var(--chart-muted)' }}
            axisLine={{ stroke: 'var(--chart-baseline)' }}
            tickLine={false}
            label={{ value: 'SUM (s)', position: 'insideBottomRight', offset: -2, fontSize: 13, fill: 'var(--chart-muted)' }}
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
              const row = item.payload as AlpRow
              const bodyStats = showBody
                ? ` sum_body=${formatBytes(row.sum_body)} avg_body=${formatBytes(row.avg_body)}`
                : ''
              return [
                `sum=${value}s count=${row.count} avg=${row.avg}s max=${row.max}s p90=${row.p90}s p99=${row.p99}s${bodyStats} / 2xx=${row['2xx']} 3xx=${row['3xx']} 4xx=${row['4xx']} 5xx=${row['5xx']}`,
                row.uri,
              ]
            }}
          />
          <Bar dataKey="sum" fill="var(--series-2)" radius={[0, 4, 4, 0]} />
        </BarChart>
      </ResponsiveContainer>
      </ChartCard>
      <TableScroll>
        <table className="table table-zebra table-pin-rows text-base">
          <thead>
            <tr>
              {th('method', 'method', 'left')}
              {th('uri', 'uri', 'left')}
              {th('count', 'count')}
              {th('sum(s)', 'sum')}
              {th('avg(s)', 'avg')}
              {th('max(s)', 'max')}
              {th('p90(s)', 'p90')}
              {th('p99(s)', 'p99')}
              {showBody && th('sum(body)', 'sum_body')}
              {showBody && th('avg(body)', 'avg_body')}
              {th('2xx', '2xx')}
              {th('3xx', '3xx')}
              {th('4xx', '4xx')}
              {th('5xx', '5xx')}
            </tr>
          </thead>
          <tbody>
            {sorted.map((r) => (
              <tr key={`${r.method}-${r.uri}`}>
                <td>{r.method}</td>
                <td className="max-w-xs truncate">{r.uri}</td>
                <td className="text-right tabular-nums" style={heatmapStyle(r.count, ranges.count)}>
                  {r.count}
                </td>
                <td className="text-right tabular-nums" style={heatmapStyle(r.sum, ranges.sum)}>
                  {r.sum}
                </td>
                <td className="text-right tabular-nums" style={heatmapStyle(r.avg, ranges.avg)}>
                  {r.avg}
                </td>
                <td className="text-right tabular-nums" style={heatmapStyle(r.max, ranges.max)}>
                  {r.max}
                </td>
                <td className="text-right tabular-nums" style={heatmapStyle(r.p90, ranges.p90)}>
                  {r.p90}
                </td>
                <td className="text-right tabular-nums" style={heatmapStyle(r.p99, ranges.p99)}>
                  {r.p99}
                </td>
                {showBody && (
                  <td className="text-right tabular-nums" title={exactBytes(r.sum_body)}>
                    {formatBytes(r.sum_body)}
                  </td>
                )}
                {showBody && (
                  <td className="text-right tabular-nums" title={exactBytes(r.avg_body)}>
                    {formatBytes(r.avg_body)}
                  </td>
                )}
                <td className="text-right tabular-nums">{r['2xx']}</td>
                <td className={`text-right tabular-nums ${r['3xx'] > 0 ? 'text-info' : ''}`}>{r['3xx']}</td>
                <td className={`text-right tabular-nums ${r['4xx'] > 0 ? 'text-warning' : ''}`}>{r['4xx']}</td>
                <td className={`text-right tabular-nums ${r['5xx'] > 0 ? 'text-error' : ''}`}>{r['5xx']}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableScroll>
    </>
  )
}

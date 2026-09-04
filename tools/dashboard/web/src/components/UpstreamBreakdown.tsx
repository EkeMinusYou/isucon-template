import { useMemo } from 'react'
import type { UpstreamResponse, UpstreamRow } from '../api'
import { columnRange, heatmapStyle } from '../heatmap'
import { useSort } from '../useSort'
import { EmptyState } from './EmptyState'
import { SortableTh } from './SortableTh'
import { TableScroll } from './TableScroll'

const EMPTY_ROWS: UpstreamRow[] = []

export function UpstreamBreakdown({ data }: { data: UpstreamResponse }) {
  const rows = data.rows ?? EMPTY_ROWS
  const ranges = useMemo(
    () => ({
      requests: columnRange(rows.map((r) => r.requests)),
      response_time_avg_ms: columnRange(rows.map((r) => r.response_time_avg_ms)),
      upstream_time_avg_ms: columnRange(rows.map((r) => r.upstream_time_avg_ms)),
    }),
    [rows],
  )

  const { sorted, sortKey, sortDir, toggle } = useSort<UpstreamRow>(rows, 'requests', 'desc')

  if (!data.available || rows.length === 0) {
    return (
      <EmptyState
        title="upstream-breakdown.tsv がまだありません"
        detail="このRUNは計測パイプライン更新前のRUNの可能性があります"
      />
    )
  }

  const th = (label: string, key: keyof UpstreamRow, align: 'left' | 'right' = 'right') => (
    <SortableTh label={label} align={align} active={sortKey === key} dir={sortDir} onClick={() => toggle(key)} />
  )

  return (
    <TableScroll maxHeight={480}>
      <table className="table table-zebra table-pin-rows text-base">
        <thead>
          <tr>
            {th('upstream', 'upstream_addr', 'left')}
            {th('status', 'upstream_status', 'left')}
            {th('cache', 'cache_status', 'left')}
            {th('requests', 'requests')}
            {th('2xx', 'status_2xx')}
            {th('3xx', 'status_3xx')}
            {th('4xx', 'status_4xx')}
            {th('5xx', 'status_5xx')}
            {th('avg応答時間(ms)', 'response_time_avg_ms')}
            {th('avgupstream時間(ms)', 'upstream_time_avg_ms')}
          </tr>
        </thead>
        <tbody>
          {sorted.map((r, i) => (
            <tr key={`${r.upstream_addr}-${r.upstream_status}-${r.cache_status}-${i}`}>
              <td className="font-mono">{r.upstream_addr}</td>
              <td>{r.upstream_status}</td>
              <td>
                <span
                  className={`badge badge-sm badge-soft ${
                    r.cache_status === 'HIT'
                      ? 'badge-success'
                      : r.cache_status === 'MISS'
                        ? 'badge-warning'
                        : r.cache_status === 'EXPIRED'
                          ? 'badge-info'
                          : 'badge-neutral'
                  }`}
                >
                  {r.cache_status}
                </span>
              </td>
              <td className="text-right tabular-nums" style={heatmapStyle(r.requests, ranges.requests)}>
                {r.requests.toLocaleString()}
              </td>
              <td className="text-right tabular-nums">{r.status_2xx.toLocaleString()}</td>
              <td className={`text-right tabular-nums ${r.status_3xx > 0 ? 'text-info' : ''}`}>
                {r.status_3xx.toLocaleString()}
              </td>
              <td className={`text-right tabular-nums ${r.status_4xx > 0 ? 'text-warning' : ''}`}>
                {r.status_4xx.toLocaleString()}
              </td>
              <td className={`text-right tabular-nums ${r.status_5xx > 0 ? 'text-error' : ''}`}>
                {r.status_5xx.toLocaleString()}
              </td>
              <td
                className="text-right tabular-nums"
                style={heatmapStyle(r.response_time_avg_ms, ranges.response_time_avg_ms)}
              >
                {r.response_time_avg_ms.toFixed(3)}
              </td>
              <td
                className="text-right tabular-nums"
                style={heatmapStyle(r.upstream_time_avg_ms, ranges.upstream_time_avg_ms)}
              >
                {r.upstream_time_avg_ms.toFixed(3)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </TableScroll>
  )
}

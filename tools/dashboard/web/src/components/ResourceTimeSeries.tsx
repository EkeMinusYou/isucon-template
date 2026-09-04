import { useMemo, useState } from 'react'
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { HostPoint, MetricsResponse, ServicePoint } from '../api'
import { buildColorMap } from '../colors'
import { CategoryDivider, ChartCard } from './ChartCard'
import { EmptyState } from './EmptyState'
import { LegendRow } from './LegendRow'

type MetricDef = {
  key: string
  label: string
  unit: string
  category: string
  domain?: [number, number]
  scale?: number
  decimals?: number
}

const HOST_METRICS: MetricDef[] = [
  { key: 'cpu_busy_pct', label: 'CPU使用率（合計）', unit: '%', category: 'CPU', domain: [0, 100] },
  { key: 'cpu_user_pct', label: 'CPU使用率（user）', unit: '%', category: 'CPU', domain: [0, 100] },
  { key: 'cpu_system_pct', label: 'CPU使用率（system）', unit: '%', category: 'CPU', domain: [0, 100] },
  { key: 'cpu_iowait_pct', label: 'CPU iowait', unit: '%', category: 'CPU', domain: [0, 100] },
  { key: 'cpu_steal_pct', label: 'CPU steal', unit: '%', category: 'CPU', domain: [0, 100] },
  { key: 'load1', label: 'Load Average (1分)', unit: '', category: 'CPU', decimals: 2 },
  { key: 'load5', label: 'Load Average (5分)', unit: '', category: 'CPU', decimals: 2 },
  { key: 'load15', label: 'Load Average (15分)', unit: '', category: 'CPU', decimals: 2 },
  { key: 'procs_running', label: '実行中プロセス数', unit: '', category: 'CPU', decimals: 0 },
  { key: 'procs_blocked', label: 'ブロック中プロセス数', unit: '', category: 'CPU', decimals: 0 },
  { key: 'context_switches_per_sec', label: 'コンテキストスイッチ', unit: 'K/s', category: 'CPU', scale: 1 / 1_000 },
  { key: 'interrupts_per_sec', label: '割り込み', unit: 'K/s', category: 'CPU', scale: 1 / 1_000 },

  { key: 'mem_used_pct', label: 'メモリ使用率', unit: '%', category: 'メモリ', domain: [0, 100] },
  { key: 'mem_available_bytes', label: '利用可能メモリ', unit: 'MB', category: 'メモリ', scale: 1 / 1_000_000 },
  { key: 'buffers_bytes', label: 'buffers', unit: 'MB', category: 'メモリ', scale: 1 / 1_000_000 },
  { key: 'cached_bytes', label: 'cached', unit: 'MB', category: 'メモリ', scale: 1 / 1_000_000 },
  { key: 'swap_used_bytes', label: 'swap使用量', unit: 'MB', category: 'メモリ', scale: 1 / 1_000_000 },
  { key: 'swap_in_bytes_per_sec', label: 'swap in', unit: 'MB/s', category: 'メモリ', scale: 1 / 1_000_000 },
  { key: 'swap_out_bytes_per_sec', label: 'swap out', unit: 'MB/s', category: 'メモリ', scale: 1 / 1_000_000 },

  { key: 'disk_read_bytes_per_sec', label: 'ディスク読み取り', unit: 'MB/s', category: 'ディスク', scale: 1 / 1_000_000 },
  { key: 'disk_write_bytes_per_sec', label: 'ディスク書き込み', unit: 'MB/s', category: 'ディスク', scale: 1 / 1_000_000 },
  { key: 'disk_io_in_progress', label: 'ディスクI/O処理中数', unit: '', category: 'ディスク', decimals: 1 },
  { key: 'disk_io_time_millis_per_sec', label: 'ディスクI/O busy時間', unit: 'ms/s', category: 'ディスク' },

  { key: 'net_rx_bytes_per_sec', label: 'ネットワーク受信', unit: 'MB/s', category: 'ネットワーク', scale: 1 / 1_000_000 },
  { key: 'net_tx_bytes_per_sec', label: 'ネットワーク送信', unit: 'MB/s', category: 'ネットワーク', scale: 1 / 1_000_000 },
  { key: 'net_rx_packets_per_sec', label: '受信パケット数', unit: '/s', category: 'ネットワーク' },
  { key: 'net_tx_packets_per_sec', label: '送信パケット数', unit: '/s', category: 'ネットワーク' },
  { key: 'net_rx_errors_per_sec', label: '受信エラー', unit: '/s', category: 'ネットワーク', decimals: 3 },
  { key: 'net_tx_errors_per_sec', label: '送信エラー', unit: '/s', category: 'ネットワーク', decimals: 3 },
  { key: 'net_rx_drops_per_sec', label: '受信ドロップ', unit: '/s', category: 'ネットワーク', decimals: 3 },
  { key: 'net_tx_drops_per_sec', label: '送信ドロップ', unit: '/s', category: 'ネットワーク', decimals: 3 },

  { key: 'cpu_pressure_some_avg10', label: 'CPU pressure (some)', unit: '%', category: 'PSI', domain: [0, 100] },
  { key: 'memory_pressure_some_avg10', label: 'Memory pressure (some)', unit: '%', category: 'PSI', domain: [0, 100] },
  { key: 'memory_pressure_full_avg10', label: 'Memory pressure (full)', unit: '%', category: 'PSI', domain: [0, 100] },
  { key: 'io_pressure_some_avg10', label: 'IO pressure (some)', unit: '%', category: 'PSI', domain: [0, 100] },
  { key: 'io_pressure_full_avg10', label: 'IO pressure (full)', unit: '%', category: 'PSI', domain: [0, 100] },
]

const SERVICE_METRICS: MetricDef[] = [
  { key: 'cpu_host_pct', label: 'CPU使用率（ホスト内シェア）', unit: '%', category: 'CPU', domain: [0, 100] },
  { key: 'cpu_pct', label: 'CPU使用率（cgroup quota比）', unit: '%', category: 'CPU', domain: [0, 100] },
  { key: 'cpu_user_pct', label: 'CPU使用率（user）', unit: '%', category: 'CPU', domain: [0, 100] },
  { key: 'cpu_system_pct', label: 'CPU使用率（system）', unit: '%', category: 'CPU', domain: [0, 100] },
  { key: 'memory_pct_of_host', label: 'メモリ使用率（ホスト内シェア）', unit: '%', category: 'メモリ', domain: [0, 100] },
  { key: 'memory_current_bytes', label: 'メモリ使用量', unit: 'MB', category: 'メモリ', scale: 1 / 1_000_000 },
  { key: 'memory_peak_bytes', label: 'メモリ使用量ピーク', unit: 'MB', category: 'メモリ', scale: 1 / 1_000_000 },
  { key: 'io_read_bytes_per_sec', label: 'ディスク読み取り', unit: 'MB/s', category: 'ディスク', scale: 1 / 1_000_000 },
  { key: 'io_write_bytes_per_sec', label: 'ディスク書き込み', unit: 'MB/s', category: 'ディスク', scale: 1 / 1_000_000 },
  { key: 'tasks_current', label: 'タスク数', unit: '', category: 'その他', decimals: 0 },
]

function groupByCategory(metrics: MetricDef[]): Array<[string, MetricDef[]]> {
  const order: string[] = []
  const groups = new Map<string, MetricDef[]>()
  for (const m of metrics) {
    if (!groups.has(m.category)) {
      groups.set(m.category, [])
      order.push(m.category)
    }
    groups.get(m.category)!.push(m)
  }
  return order.map((c) => [c, groups.get(c)!])
}

type Series = { key: string; series: Array<HostPoint | ServicePoint> }

// Merges independently-sampled series into one array keyed by sample index
// (each sampler ticks at the same interval but with its own timestamps),
// projecting out the selected metric field and applying its display scale.
function mergeByIndex(seriesList: Series[], metric: MetricDef): Array<Record<string, number>> {
  const length = Math.max(0, ...seriesList.map((s) => s.series.length))
  const rows: Array<Record<string, number>> = []
  for (let i = 0; i < length; i++) {
    const row: Record<string, number> = {
      elapsed_s: Math.round((seriesList[0]?.series[i]?.elapsed_ms ?? i * 1000) / 1000),
    }
    for (const s of seriesList) {
      const point = s.series[i] as Record<string, number> | undefined
      if (point) {
        const raw = point[metric.key] ?? 0
        row[s.key] = metric.scale ? raw * metric.scale : raw
      }
    }
    rows.push(row)
  }
  return rows
}

function ResourceChart({
  seriesList,
  metric,
  colorMap,
}: {
  seriesList: Series[]
  metric: MetricDef
  colorMap: Map<string, string>
}) {
  const data = useMemo(() => mergeByIndex(seriesList, metric), [seriesList, metric])
  const decimals = metric.decimals ?? 1

  return (
    <ChartCard title={metric.label} unit={metric.unit}>
      <ResponsiveContainer width="100%" height={200}>
        <LineChart data={data} margin={{ top: 4, right: 12, bottom: 4, left: 0 }}>
          <CartesianGrid stroke="var(--chart-grid)" vertical={false} />
          <XAxis
            dataKey="elapsed_s"
            tick={{ fontSize: 11, fill: 'var(--chart-muted)' }}
            axisLine={{ stroke: 'var(--chart-baseline)' }}
            tickLine={false}
          />
          <YAxis
            tick={{ fontSize: 11, fill: 'var(--chart-muted)' }}
            axisLine={false}
            tickLine={false}
            width={48}
            domain={metric.domain ?? ['auto', 'auto']}
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
            formatter={(value) => [`${Number(value).toFixed(decimals)}${metric.unit}`, '']}
            labelFormatter={(label) => `${label}s`}
          />
          {seriesList.map((s) => (
            <Line
              key={s.key}
              type="monotone"
              dataKey={s.key}
              name={s.key}
              stroke={colorMap.get(s.key)}
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </ChartCard>
  )
}

export function ResourceTimeSeries({ metrics }: { metrics: MetricsResponse }) {
  const hosts = useMemo(
    () => [...metrics.hosts].sort((a, b) => a.host.localeCompare(b.host)),
    [metrics.hosts],
  )
  const [selectedHost, setSelectedHost] = useState<string | null>(hosts[0]?.host ?? null)

  const hostSeriesList: Series[] = useMemo(
    () => hosts.map((h) => ({ key: h.host, series: h.series })),
    [hosts],
  )
  const hostColorMap = useMemo(() => buildColorMap(hostSeriesList.map((s) => s.key)), [hostSeriesList])

  const activeHost = selectedHost ?? hosts[0]?.host ?? ''
  const serviceSeriesList: Series[] = useMemo(
    () =>
      metrics.services
        .filter((s) => s.host === activeHost)
        .sort((a, b) => a.service.localeCompare(b.service))
        .map((s) => ({ key: s.service.replace(/\.service$/, ''), series: s.series })),
    [metrics.services, activeHost],
  )
  const serviceColorMap = useMemo(
    () => buildColorMap(serviceSeriesList.map((s) => s.key)),
    [serviceSeriesList],
  )

  if (hosts.length === 0) {
    return (
      <EmptyState
        title="proc-metrics/service-metrics がまだありません"
        detail="task after-bench で sampler の結果が回収されているか確認してください"
      />
    )
  }

  return (
    <>
      <h3 className="text-base font-semibold">ホスト別</h3>
      <LegendRow entries={hostSeriesList.map((s) => ({ key: s.key, color: hostColorMap.get(s.key)! }))} />
      {groupByCategory(HOST_METRICS).map(([category, defs]) => (
        <div key={category} className="mb-5">
          <CategoryDivider label={category} />
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {defs.map((m) => (
              <ResourceChart key={m.key} seriesList={hostSeriesList} metric={m} colorMap={hostColorMap} />
            ))}
          </div>
        </div>
      ))}

      <div className="mt-8 mb-3 flex flex-wrap items-center gap-3">
        <h3 className="text-base font-semibold">サービス別</h3>
        <div role="tablist" className="tabs tabs-box tabs-sm bg-base-200">
          {hosts.map((h) => (
            <button
              key={h.host}
              role="tab"
              aria-selected={activeHost === h.host}
              className={`tab ${activeHost === h.host ? 'tab-active' : ''}`}
              onClick={() => setSelectedHost(h.host)}
            >
              {h.host}
            </button>
          ))}
        </div>
      </div>
      <LegendRow
        entries={serviceSeriesList.map((s) => ({ key: s.key, color: serviceColorMap.get(s.key)! }))}
      />
      {groupByCategory(SERVICE_METRICS).map(([category, defs]) => (
        <div key={category} className="mb-5">
          <CategoryDivider label={category} />
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
            {defs.map((m) => (
              <ResourceChart key={m.key} seriesList={serviceSeriesList} metric={m} colorMap={serviceColorMap} />
            ))}
          </div>
        </div>
      ))}
    </>
  )
}

import { useMemo } from 'react'
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { TimelineResponse } from '../api'
import { buildColorMap } from '../colors'
import { ChartCard } from './ChartCard'
import { EmptyState } from './EmptyState'
import { LegendRow } from './LegendRow'

const SYNC_ID = 'benchmark-timeline'

const tooltipStyle = {
  background: 'var(--chart-tooltip-bg)',
  color: 'var(--chart-tooltip-text)',
  border: '1px solid var(--chart-grid)',
  borderRadius: 8,
  fontSize: 12,
}

const lineCursor = { stroke: 'var(--chart-cursor-line)', strokeWidth: 1 }
const barCursor = { fill: 'var(--chart-cursor-fill)' }

function xAxisProps() {
  return {
    dataKey: 'elapsed_s' as const,
    tick: { fontSize: 11, fill: 'var(--chart-muted)' },
    axisLine: { stroke: 'var(--chart-baseline)' },
    tickLine: false,
    label: { value: '経過秒', position: 'insideBottomRight' as const, offset: -4, fontSize: 11, fill: 'var(--chart-muted)' },
  }
}

function yAxisProps(domain?: [number, number]) {
  return {
    tick: { fontSize: 11, fill: 'var(--chart-muted)' },
    axisLine: false,
    tickLine: false,
    width: 52,
    domain: domain ?? (['auto', 'auto'] as const),
  }
}

function Panel({ title, unit, children }: { title: string; unit?: string; children: React.ReactNode }) {
  return (
    <ChartCard title={title} unit={unit}>
      <ResponsiveContainer width="100%" height={190}>
        {children as React.ReactElement}
      </ResponsiveContainer>
    </ChartCard>
  )
}

export function BenchmarkTimeline({ timeline }: { timeline: TimelineResponse }) {
  const hosts = useMemo(() => [...timeline.hosts].sort((a, b) => a.host.localeCompare(b.host)), [timeline.hosts])
  const hostColorMap = useMemo(() => buildColorMap(hosts.map((h) => h.host)), [hosts])

  const data = useMemo(() => {
    const hostMaps = hosts.map((h) => ({
      host: h.host,
      byElapsed: new Map(h.series.map((p) => [p.elapsed_s, p])),
    }))
    return timeline.buckets.map((b) => {
      const row: Record<string, number | null> = {
        elapsed_s: b.elapsed_s,
        status_2xx: b.status_2xx,
        status_3xx: b.status_3xx,
        status_4xx: b.status_4xx,
        status_5xx: b.status_5xx,
        avg_response_ms: b.avg_response_ms,
        p95_response_ms: b.p95_response_ms,
        throughput_mb_s: b.bytes_per_sec / 1_000_000,
        scenario_warnings: b.scenario_warnings,
      }
      for (const { host, byElapsed } of hostMaps) {
        const p = byElapsed.get(b.elapsed_s)
        row[`cpu__${host}`] = p ? p.cpu_busy_pct : null
      }
      return row
    })
  }, [timeline.buckets, hosts])

  if (!timeline.available || timeline.buckets.length === 0) {
    return (
      <EmptyState
        title="表示できるアクセス記録がありません"
        detail="このRUNのログが未取得・空、または有効な時刻を読み取れませんでした"
      />
    )
  }

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <Panel title="リクエスト数（ステータス別）" unit="req/s">
        <AreaChart data={data} margin={{ top: 4, right: 12, bottom: 4, left: 0 }} syncId={SYNC_ID}>
          <CartesianGrid stroke="var(--chart-grid)" vertical={false} />
          <XAxis {...xAxisProps()} />
          <YAxis {...yAxisProps()} />
          <Tooltip cursor={lineCursor} contentStyle={tooltipStyle} labelFormatter={(label) => `${label}s`} />
          <Area type="monotone" dataKey="status_2xx" name="2xx" stackId="status" stroke="var(--status-good)" fill="var(--status-good)" fillOpacity={0.5} isAnimationActive={false} />
          <Area type="monotone" dataKey="status_3xx" name="3xx" stackId="status" stroke="var(--status-warning)" fill="var(--status-warning)" fillOpacity={0.5} isAnimationActive={false} />
          <Area type="monotone" dataKey="status_4xx" name="4xx" stackId="status" stroke="var(--status-serious)" fill="var(--status-serious)" fillOpacity={0.5} isAnimationActive={false} />
          <Area type="monotone" dataKey="status_5xx" name="5xx" stackId="status" stroke="var(--status-critical)" fill="var(--status-critical)" fillOpacity={0.5} isAnimationActive={false} />
        </AreaChart>
      </Panel>

      <Panel title="レスポンスタイム" unit="ms">
        <LineChart data={data} margin={{ top: 4, right: 12, bottom: 4, left: 0 }} syncId={SYNC_ID}>
          <CartesianGrid stroke="var(--chart-grid)" vertical={false} />
          <XAxis {...xAxisProps()} />
          <YAxis {...yAxisProps()} />
          <Tooltip cursor={lineCursor} contentStyle={tooltipStyle} labelFormatter={(label) => `${label}s`} formatter={(v) => Number(v).toFixed(1)} />
          <Line type="monotone" dataKey="avg_response_ms" name="avg" stroke="var(--series-1)" strokeWidth={2} dot={false} isAnimationActive={false} />
          <Line type="monotone" dataKey="p95_response_ms" name="p95" stroke="var(--series-8)" strokeWidth={2} dot={false} isAnimationActive={false} />
        </LineChart>
      </Panel>

      <Panel title="レスポンスボディ転送量" unit="MB/s">
        <AreaChart data={data} margin={{ top: 4, right: 12, bottom: 4, left: 0 }} syncId={SYNC_ID}>
          <CartesianGrid stroke="var(--chart-grid)" vertical={false} />
          <XAxis {...xAxisProps()} />
          <YAxis {...yAxisProps()} />
          <Tooltip cursor={lineCursor} contentStyle={tooltipStyle} labelFormatter={(label) => `${label}s`} formatter={(v) => Number(v).toFixed(2)} />
          <Area type="monotone" dataKey="throughput_mb_s" name="throughput" stroke="var(--series-3)" fill="var(--series-3)" fillOpacity={0.4} isAnimationActive={false} />
        </AreaChart>
      </Panel>

      <Panel title="シナリオ警告/エラー（bench.log）" unit="件/s">
        <BarChart data={data} margin={{ top: 4, right: 12, bottom: 4, left: 0 }} syncId={SYNC_ID}>
          <CartesianGrid stroke="var(--chart-grid)" vertical={false} />
          <XAxis {...xAxisProps()} />
          <YAxis {...yAxisProps()} />
          <Tooltip cursor={barCursor} contentStyle={tooltipStyle} labelFormatter={(label) => `${label}s`} />
          <Bar dataKey="scenario_warnings" name="警告/エラー" fill="var(--status-critical)" isAnimationActive={false} />
        </BarChart>
      </Panel>

      <div className="xl:col-span-2">
        <ChartCard title="ホストCPU使用率" unit="%">
          <LegendRow entries={hosts.map((h) => ({ key: h.host, color: hostColorMap.get(h.host)! }))} />
          <ResponsiveContainer width="100%" height={220}>
            <LineChart data={data} margin={{ top: 4, right: 12, bottom: 4, left: 0 }} syncId={SYNC_ID}>
              <CartesianGrid stroke="var(--chart-grid)" vertical={false} />
              <XAxis {...xAxisProps()} />
              <YAxis {...yAxisProps([0, 100])} />
              <Tooltip cursor={lineCursor} contentStyle={tooltipStyle} labelFormatter={(label) => `${label}s`} formatter={(v) => (v == null ? '-' : `${Number(v).toFixed(1)}%`)} />
              {hosts.map((h) => (
                <Line
                  key={h.host}
                  type="monotone"
                  dataKey={`cpu__${h.host}`}
                  name={h.host}
                  stroke={hostColorMap.get(h.host)}
                  strokeWidth={2}
                  dot={false}
                  connectNulls
                  isAnimationActive={false}
                />
              ))}
            </LineChart>
          </ResponsiveContainer>
        </ChartCard>
      </div>
    </div>
  )
}

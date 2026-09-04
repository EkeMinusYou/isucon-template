import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { ChartCard } from './ChartCard'

export type MetricDef<T> = {
  key: keyof T
  label: string
  unit: string
  category: string
  domain?: [number, number]
  scale?: number
  decimals?: number
}

export function groupByCategory<T>(metrics: MetricDef<T>[]): Array<[string, MetricDef<T>[]]> {
  const order: string[] = []
  const groups = new Map<string, MetricDef<T>[]>()
  for (const m of metrics) {
    if (!groups.has(m.category)) {
      groups.set(m.category, [])
      order.push(m.category)
    }
    groups.get(m.category)!.push(m)
  }
  return order.map((c) => [c, groups.get(c)!])
}

// A single-series (one line, no legend needed) small-multiple chart, used
// for host-wide time series that aren't broken down by entity (for example MySQL).
export function TinyLineChart<T extends { elapsed_ms: number }>({
  series,
  metric,
}: {
  series: T[]
  metric: MetricDef<T>
}) {
  const decimals = metric.decimals ?? 1
  const data = series.map((p) => ({
    elapsed_s: Math.round(p.elapsed_ms / 1000),
    value: (p[metric.key] as number) * (metric.scale ?? 1),
  }))

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
          <Line
            type="monotone"
            dataKey="value"
            stroke="var(--series-1)"
            strokeWidth={2}
            dot={false}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </ChartCard>
  )
}

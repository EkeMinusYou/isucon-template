import type { MetricsResponse } from '../api'
import { runtimeDisplaySeries, type RuntimeDisplayPoint } from '../runtimeMetrics'
import { EmptyState } from './EmptyState'
import { TinyLineChart, type MetricDef } from './TinyLineChart'

const METRICS: MetricDef<RuntimeDisplayPoint>[] = [
  { key: 'go_goroutines', label: 'goroutine数', unit: '', category: 'Go runtime', domain: [0, 'auto'], decimals: 0 },
  { key: 'go_heap_objects_bytes', label: 'heap使用量', unit: 'MiB', category: 'Go runtime', domain: [0, 'auto'], scale: 1 / 1024 ** 2 },
  { key: 'go_heap_live_bytes', label: '直前GC時のlive heap', unit: 'MiB', category: 'Go runtime', domain: [0, 'auto'], scale: 1 / 1024 ** 2 },
  { key: 'allocation_bytes_per_sec', label: 'allocation速度', unit: 'MiB/s', category: 'Go runtime', domain: [0, 'auto'], scale: 1 / 1024 ** 2 },
  { key: 'gc_cpu_seconds_per_sec', label: 'GCのCPU時間', unit: 'CPU秒/秒', category: 'Go runtime', domain: [0, 'auto'], decimals: 3 },
]

export function GoRuntimeMetrics({ metrics }: { metrics: MetricsResponse }) {
  const runtimes = (metrics.runtimes ?? []).filter((runtime) => runtime.series.length > 0)
  if (runtimes.length === 0) {
    return <EmptyState title="Go runtime指標がありません" detail="APP_HOSTSの計測listenerとruntime collector、RUNへの回収結果を確認してください。" />
  }
  return (
    <div className="space-y-6">
      <p className="text-sm text-base-content/70">
        {metrics.runtime_scope === 'load' ? '負荷期間のサンプルを表示しています。' : '負荷期間が未確定のため、採取全体を表示しています。'}
        {' '}未取得値と再起動をまたぐcounter差分は表示しません。
      </p>
      {runtimes.map(({ host, series }) => {
        const points = runtimeDisplaySeries(series)
        return (
          <div key={host}>
            <h3 className="mb-3 font-mono font-semibold">{host}</h3>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
              {METRICS.map((metric) => points.some((point) => point[metric.key] != null) ? (
                <TinyLineChart key={metric.key} series={points} metric={metric} />
              ) : (
                <EmptyState key={metric.key} title={`${metric.label}：未計測`} />
              ))}
            </div>
          </div>
        )
      })}
    </div>
  )
}

import { useMemo, useState } from 'react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import {
  api,
  type GoPprofFunction,
  type GoPprofKind,
  type GoPprofProfile,
  type GoPprofResponse,
} from '../api'
import { useSort } from '../useSort'
import { EmptyState } from './EmptyState'
import { SortableTh } from './SortableTh'
import { TableScroll } from './TableScroll'

type SortKey = 'flat_pct' | 'cum_pct'

const KIND_ORDER: GoPprofKind[] = ['cpu', 'heap', 'allocs', 'goroutine']
const KIND_LABELS: Record<GoPprofKind, string> = {
  cpu: 'CPU',
  heap: 'heap',
  allocs: 'allocs',
  goroutine: 'goroutine',
}

const tooltipStyle = {
  background: 'var(--chart-tooltip-bg)',
  color: 'var(--chart-tooltip-text)',
  border: '1px solid var(--chart-grid)',
  borderRadius: 8,
  fontSize: 13,
}

type ValueScale = {
  divisor: number
  label: string
}

function valueScale(unit: string, total: number): ValueScale {
  switch (unit.toLowerCase()) {
    case 'nanoseconds':
      return { divisor: 1_000_000, label: 'ms' }
    case 'microseconds':
      return { divisor: 1_000, label: 'ms' }
    case 'milliseconds':
      return { divisor: 1, label: 'ms' }
    case 'seconds':
      return { divisor: 1, label: 's' }
    case 'bytes':
      if (total >= 1024 ** 3) return { divisor: 1024 ** 3, label: 'GiB' }
      if (total >= 1024 ** 2) return { divisor: 1024 ** 2, label: 'MiB' }
      if (total >= 1024) return { divisor: 1024, label: 'KiB' }
      return { divisor: 1, label: 'B' }
    case 'count':
      return { divisor: 1, label: 'count' }
    default:
      return { divisor: 1, label: unit || 'value' }
  }
}

function formatValue(value: number, scale: ValueScale): string {
  const scaled = value / scale.divisor
  const digits = Math.abs(scaled) >= 100 ? 0 : Math.abs(scaled) >= 10 ? 1 : 2
  return `${scaled.toLocaleString(undefined, { maximumFractionDigits: digits })} ${scale.label}`
}

function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '—'
  if (seconds < 60) return `${seconds.toFixed(1)}s`
  return `${Math.floor(seconds / 60)}m ${(seconds % 60).toFixed(0)}s`
}

function shortFunctionName(name: string, max = 64): string {
  return name.length > max ? `${name.slice(0, max - 1)}…` : name
}

function ProfileStats({ profile }: { profile: GoPprofProfile }) {
  const scale = valueScale(profile.sample_unit, profile.total)
  return (
    <div className="stats stats-horizontal w-full overflow-x-auto border border-base-300 bg-base-100">
      <div className="stat px-4 py-2">
        <div className="stat-title text-sm">ホスト</div>
        <div className="stat-value text-2xl">{profile.host}</div>
      </div>
      <div className="stat px-4 py-2">
        <div className="stat-title text-sm">プロファイル時間</div>
        <div className="stat-value text-2xl">{formatDuration(profile.duration_sec)}</div>
      </div>
      <div className="stat px-4 py-2">
        <div className="stat-title text-sm">{profile.sample_type}</div>
        <div className="stat-value text-2xl">{formatValue(profile.total, scale)}</div>
      </div>
      <div className="stat px-4 py-2">
        <div className="stat-title text-sm">関数数</div>
        <div className="stat-value text-2xl">{profile.functions.length.toLocaleString()}</div>
      </div>
    </div>
  )
}

function ProfileChart({ profile, sortKey }: { profile: GoPprofProfile; sortKey: SortKey }) {
  const scale = valueScale(profile.sample_unit, profile.total)
  const valueKey = sortKey === 'flat_pct' ? 'flat' : 'cum'
  const data = useMemo(
    () =>
      [...profile.functions]
        .sort((a, b) => b[sortKey] - a[sortKey] || b.cum - a.cum || a.name.localeCompare(b.name))
        .slice(0, 20)
        .map((fn) => ({
          ...fn,
          label: shortFunctionName(fn.name),
          displayValue: fn[valueKey] / scale.divisor,
        })),
    [profile.functions, scale.divisor, sortKey, valueKey],
  )
  const valueLabel = sortKey === 'flat_pct' ? 'flat' : 'cum'
  const chartHeight = Math.max(340, data.length * 28 + 70)

  return (
    <ResponsiveContainer width="100%" height={chartHeight}>
      <BarChart data={data} layout="vertical" margin={{ top: 4, right: 24, bottom: 4, left: 4 }}>
        <CartesianGrid stroke="var(--chart-grid)" horizontal={false} />
        <XAxis
          type="number"
          tick={{ fontSize: 13, fill: 'var(--chart-muted)' }}
          axisLine={{ stroke: 'var(--chart-baseline)' }}
          tickLine={false}
          label={{ value: `${valueLabel} (${scale.label})`, position: 'insideBottomRight', offset: -2, fontSize: 13, fill: 'var(--chart-muted)' }}
        />
        <YAxis
          type="category"
          dataKey="label"
          width={360}
          tick={{ fontSize: 12, fill: 'currentColor' }}
          axisLine={false}
          tickLine={false}
        />
        <Tooltip
          cursor={{ fill: 'var(--chart-cursor-fill)' }}
          contentStyle={tooltipStyle}
          formatter={(_value, _name, item) => {
            const fn = item.payload as GoPprofFunction & { displayValue: number }
            const rawValue = sortKey === 'flat_pct' ? fn.flat : fn.cum
            const pct = sortKey === 'flat_pct' ? fn.flat_pct : fn.cum_pct
            return [`${formatValue(rawValue, scale)} (${pct.toFixed(1)}%)`, valueLabel]
          }}
          labelFormatter={(label) => String(label)}
        />
        <Bar dataKey="displayValue" name={valueLabel} fill={sortKey === 'flat_pct' ? 'var(--series-2)' : 'var(--series-7)'} radius={[0, 4, 4, 0]} />
      </BarChart>
    </ResponsiveContainer>
  )
}

function CallGraph({ runId, profile }: { runId: string; profile: GoPprofProfile }) {
  return (
    <div className="mt-6">
      <div className="mb-1 text-sm font-semibold">コールグラフ</div>
      <div className="mb-2 text-sm text-base-content/60">
        矢印は caller → callee、ノードとエッジの太さは{profile.sample_type}のサンプル量を表します
      </div>
      <iframe
        title={`${profile.host} ${profile.kind} pprof コールグラフ`}
        src={api.pprofGraph(runId, profile.kind, profile.host)}
        loading="lazy"
        className="h-[720px] w-full rounded-box border border-base-300 bg-base-100"
      />
    </div>
  )
}

function ProfilePanel({ runId, profile }: { runId: string; profile: GoPprofProfile }) {
  const [sortKey, setSortKey] = useState<SortKey>('flat_pct')
  const scale = valueScale(profile.sample_unit, profile.total)
  const { sorted, sortKey: tableSortKey, sortDir, toggle } = useSort<GoPprofFunction>(
    profile.functions,
    'flat_pct',
    'desc',
  )
  const th = (label: string, key: keyof GoPprofFunction, align: 'left' | 'right' = 'right') => (
    <SortableTh
      label={label}
      align={align}
      active={tableSortKey === key}
      dir={sortDir}
      onClick={() => toggle(key)}
    />
  )

  return (
    <div className="flex flex-col gap-4">
      <ProfileStats profile={profile} />
      <div className="flex flex-wrap items-center gap-3">
        <span className="badge badge-ghost badge-sm font-mono">
          {profile.source} / {profile.sample_type} ({profile.sample_unit})
        </span>
        <div role="tablist" className="tabs tabs-box tabs-sm ml-auto bg-base-200">
          <button
            role="tab"
            aria-selected={sortKey === 'flat_pct'}
            className={`tab ${sortKey === 'flat_pct' ? 'tab-active' : ''}`}
            onClick={() => setSortKey('flat_pct')}
          >
            flat
          </button>
          <button
            role="tab"
            aria-selected={sortKey === 'cum_pct'}
            className={`tab ${sortKey === 'cum_pct' ? 'tab-active' : ''}`}
            onClick={() => setSortKey('cum_pct')}
          >
            cumulative
          </button>
        </div>
      </div>
      <ProfileChart profile={profile} sortKey={sortKey} />
      <CallGraph runId={runId} profile={profile} />
      <TableScroll>
        <table className="table table-zebra table-pin-rows text-base">
          <thead>
            <tr>
              {th('function', 'name', 'left')}
              {th(`flat (${scale.label})`, 'flat')}
              {th('flat (%)', 'flat_pct')}
              {th(`cum (${scale.label})`, 'cum')}
              {th('cum (%)', 'cum_pct')}
            </tr>
          </thead>
          <tbody>
            {sorted.map((fn) => (
              <tr key={fn.name}>
                <td className="max-w-xl truncate" title={fn.name}>{fn.name}</td>
                <td className="text-right tabular-nums">{formatValue(fn.flat, scale)}</td>
                <td className="text-right tabular-nums">{fn.flat_pct.toFixed(1)}%</td>
                <td className="text-right tabular-nums">{formatValue(fn.cum, scale)}</td>
                <td className="text-right tabular-nums">{fn.cum_pct.toFixed(1)}%</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableScroll>
    </div>
  )
}

export function GoPprofTop({ data }: { data: GoPprofResponse }) {
  const availableKinds = KIND_ORDER.filter((kind) => data.profiles.some((profile) => profile.kind === kind))
  const [selectedKind, setSelectedKind] = useState<GoPprofKind | null>(availableKinds[0] ?? null)
  const activeKind = selectedKind && availableKinds.includes(selectedKind) ? selectedKind : availableKinds[0]
  const profiles = data.profiles
    .filter((profile) => profile.kind === activeKind)
    .sort((a, b) => a.host.localeCompare(b.host))
  const [selectedHost, setSelectedHost] = useState<string | null>(profiles[0]?.host ?? null)
  const activeHost = profiles.some((profile) => profile.host === selectedHost) ? selectedHost : profiles[0]?.host
  const activeProfile = profiles.find((profile) => profile.host === activeHost)

  if (!data.available || availableKinds.length === 0 || !activeKind || !activeProfile) {
    return (
      <EmptyState
        title="Go pprofがまだありません"
        detail="このRUNでは CPU・heap・allocs・goroutine profileが未収集、または収集失敗の可能性があります"
      />
    )
  }

  return (
    <>
      <div className="flex flex-wrap gap-3">
        <div role="tablist" className="tabs tabs-box tabs-sm w-fit bg-base-200">
          {availableKinds.map((kind) => (
            <button
              key={kind}
              role="tab"
              aria-selected={activeKind === kind}
              className={`tab ${activeKind === kind ? 'tab-active' : ''}`}
              onClick={() => setSelectedKind(kind)}
            >
              {KIND_LABELS[kind]}
            </button>
          ))}
        </div>
        <div role="tablist" className="tabs tabs-box tabs-sm w-fit bg-base-200">
          {profiles.map((profile) => (
            <button
              key={profile.host}
              role="tab"
              aria-selected={activeHost === profile.host}
              className={`tab ${activeHost === profile.host ? 'tab-active' : ''}`}
              onClick={() => setSelectedHost(profile.host)}
            >
              {profile.host}
            </button>
          ))}
        </div>
      </div>
      <ProfilePanel key={`${activeKind}:${activeProfile.host}`} runId={data.run_id} profile={activeProfile} />
    </>
  )
}

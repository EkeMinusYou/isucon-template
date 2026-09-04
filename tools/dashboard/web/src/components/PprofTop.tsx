import { useMemo, useState } from 'react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api, type FgprofFunction, type FgprofProfile, type FgprofResponse } from '../api'
import { useSort } from '../useSort'
import { EmptyState } from './EmptyState'
import { SortableTh } from './SortableTh'
import { TableScroll } from './TableScroll'

type SortKey = 'flat_pct' | 'cum_pct'

const tooltipStyle = {
  background: 'var(--chart-tooltip-bg)',
  color: 'var(--chart-tooltip-text)',
  border: '1px solid var(--chart-grid)',
  borderRadius: 8,
  fontSize: 13,
}

function shortFunctionName(name: string, max = 64): string {
  return name.length > max ? `${name.slice(0, max - 1)}…` : name
}

function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '—'
  if (seconds < 60) return `${seconds.toFixed(1)}s`
  return `${Math.floor(seconds / 60)}m ${(seconds % 60).toFixed(0)}s`
}

function ProfileStats({ profile }: { profile: FgprofProfile }) {
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
        <div className="stat-title text-sm">wall-clockサンプル</div>
        <div className="stat-value text-2xl">{profile.total_ms.toFixed(0)}ms</div>
      </div>
      <div className="stat px-4 py-2">
        <div className="stat-title text-sm">関数数</div>
        <div className="stat-value text-2xl">{profile.functions.length.toLocaleString()}</div>
      </div>
    </div>
  )
}

function ProfileChart({ profile, sortKey }: { profile: FgprofProfile; sortKey: SortKey }) {
  const valueKey = sortKey === 'flat_pct' ? 'flat_ms' : 'cum_ms'
  const data = useMemo(
    () =>
      [...profile.functions]
        .sort((a, b) => b[sortKey] - a[sortKey] || b.cum_ms - a.cum_ms || a.name.localeCompare(b.name))
        .slice(0, 20)
        .map((fn) => ({ ...fn, label: shortFunctionName(fn.name) })),
    [profile.functions, sortKey],
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
          label={{ value: `${valueLabel} (ms)`, position: 'insideBottomRight', offset: -2, fontSize: 13, fill: 'var(--chart-muted)' }}
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
          formatter={(value, _name, item) => {
            const fn = item.payload as FgprofFunction
            const valueMs = Number(value)
            const pct = sortKey === 'flat_pct' ? fn.flat_pct : fn.cum_pct
            return [`${valueMs.toFixed(1)}ms (${pct.toFixed(1)}%)`, valueLabel]
          }}
          labelFormatter={(label) => String(label)}
        />
        <Bar dataKey={valueKey} name={valueLabel} fill={sortKey === 'flat_pct' ? 'var(--series-2)' : 'var(--series-7)'} radius={[0, 4, 4, 0]} />
      </BarChart>
    </ResponsiveContainer>
  )
}

function FgprofCallGraph({ runId, host }: { runId: string; host: string }) {
  return (
    <div className="mt-6">
      <div className="mb-1 text-sm font-semibold">コールグラフ</div>
      <div className="mb-2 text-sm text-base-content/60">矢印は caller → callee、ノードとエッジの太さはwall-clockサンプル量を表します</div>
      <iframe
        title={`${host} fgprof コールグラフ`}
        src={api.fgprofGraph(runId, host)}
        loading="lazy"
        className="h-[720px] w-full rounded-box border border-base-300 bg-base-100"
      />
    </div>
  )
}

function ProfilePanel({ runId, profile }: { runId: string; profile: FgprofProfile }) {
  const [sortKey, setSortKey] = useState<SortKey>('flat_pct')
  const { sorted, sortKey: tableSortKey, sortDir, toggle } = useSort<FgprofFunction>(
    profile.functions,
    'flat_pct',
    'desc',
  )

  const th = (label: string, key: keyof FgprofFunction, align: 'left' | 'right' = 'right') => (
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
      <FgprofCallGraph runId={runId} host={profile.host} />
      <TableScroll>
        <table className="table table-zebra table-pin-rows text-base">
          <thead>
            <tr>
              {th('function', 'name', 'left')}
              {th('flat (ms)', 'flat_ms')}
              {th('flat (%)', 'flat_pct')}
              {th('cum (ms)', 'cum_ms')}
              {th('cum (%)', 'cum_pct')}
            </tr>
          </thead>
          <tbody>
            {sorted.map((fn) => (
              <tr key={fn.name}>
                <td className="max-w-xl truncate" title={fn.name}>
                  {fn.name}
                </td>
                <td className="text-right tabular-nums">{fn.flat_ms.toFixed(1)}</td>
                <td className="text-right tabular-nums">{fn.flat_pct.toFixed(1)}%</td>
                <td className="text-right tabular-nums">{fn.cum_ms.toFixed(1)}</td>
                <td className="text-right tabular-nums">{fn.cum_pct.toFixed(1)}%</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableScroll>
    </div>
  )
}

export function FgprofTop({ data }: { data: FgprofResponse }) {
  const profiles = useMemo(
    () => [...data.profiles].sort((a, b) => a.host.localeCompare(b.host)),
    [data.profiles],
  )
  const [selectedHost, setSelectedHost] = useState<string | null>(profiles[0]?.host ?? null)
  const activeHost = profiles.some((profile) => profile.host === selectedHost)
    ? selectedHost!
    : profiles[0]?.host ?? ''
  const activeProfile = profiles.find((profile) => profile.host === activeHost)

  if (!data.available || profiles.length === 0 || !activeProfile) {
    return (
      <EmptyState
        title="*-fgprof.pprof がまだありません"
        detail="このRUNでは fgprof 未収集、または収集失敗の可能性があります"
      />
    )
  }

  return (
    <>
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
      <ProfilePanel runId={data.run_id} profile={activeProfile} />
    </>
  )
}

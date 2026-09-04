import { useCallback, useEffect, useRef, useState } from 'react'
import {
  api,
  type AlpResponse,
  type BacklogResponse,
  type MetricsResponse,
  type MysqlResponse,
  type FgprofResponse,
  type GoPprofResponse,
  type ReportInfo,
  type RunInfo,
  type ScoreEntry,
  type SlowQueryResponse,
  type TimelineResponse,
  type UpstreamResponse,
  type UserTransitionsResponse,
} from './api'
import { AlpTop } from './components/AlpTop'
import { BacklogBoard } from './components/BacklogBoard'
import { BenchmarkTimeline } from './components/BenchmarkTimeline'
import { MysqlStatus } from './components/MysqlStatus'
import { FgprofTop } from './components/PprofTop'
import { GoPprofTop } from './components/GoPprofTop'
import { ResourceTimeSeries } from './components/ResourceTimeSeries'
import { ReportsPanel } from './components/ReportsPanel'
import { RunSelector } from './components/RunSelector'
import { SectionNav, type SectionDef } from './components/SectionNav'
import { ScoreTrend } from './components/ScoreTrend'
import { ChartSkeleton, TableSkeleton } from './components/Skeletons'
import { SlowQueryTop } from './components/SlowQueryTop'
import { ThemeToggle } from './components/ThemeToggle'
import { UpstreamBreakdown } from './components/UpstreamBreakdown'
import { UserTransitions } from './components/UserTransitions'
import { useTheme } from './useTheme'

type RunData = {
  alp: AlpResponse
  slowquery: SlowQueryResponse
  metrics: MetricsResponse
  fgprof: FgprofResponse
  pprof: GoPprofResponse
  mysql: MysqlResponse
  upstream: UpstreamResponse
  userTransitions: UserTransitionsResponse
  timeline: TimelineResponse
}

// ジャンプバーの並び順とラベル。各 Section の id と 1:1 で対応させる
const SECTIONS: SectionDef[] = [
  { id: 'score-trend', title: 'スコア推移', navLabel: 'スコア' },
  { id: 'benchmark-timeline', title: 'ベンチマーカー挙動タイムライン', navLabel: 'タイムライン' },
  { id: 'backlog', title: 'バックログボード', navLabel: 'バックログ' },
  { id: 'alp', title: 'alp トップボトルネック', navLabel: 'alp' },
  { id: 'upstream', title: 'nginx upstream/キャッシュ内訳', navLabel: 'upstream' },
  { id: 'user-transitions', title: 'Cookie ユーザー遷移', navLabel: 'ユーザー遷移' },
  { id: 'slowquery', title: 'slow query トップ', navLabel: 'slow query' },
  { id: 'mysql', title: 'MySQL ステータス', navLabel: 'MySQL' },
  { id: 'resources', title: 'ホスト/サービス別リソース時系列', navLabel: 'リソース' },
  { id: 'pprof', title: 'Go pprof', navLabel: 'pprof' },
  { id: 'fgprof', title: 'fgprof wall-clock', navLabel: 'fgprof' },
]

function Section({
  id,
  title,
  badge,
  extra,
  children,
}: {
  id?: string
  title: string
  badge?: string
  extra?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section
      id={id}
      // sticky ヘッダー + ジャンプバーの高さぶん、ジャンプ先のスクロール位置を下げる
      style={{ scrollMarginTop: 'calc(var(--dash-header-h, 7rem) + 1rem)' }}
      className="card card-border mb-6 border-base-300 bg-base-100 shadow-sm"
    >
      <div className="card-body gap-4 p-6">
        <div className="flex flex-wrap items-center gap-3">
          <h2 className="card-title text-xl">{title}</h2>
          {badge && <span className="badge badge-soft badge-neutral badge-sm">{badge}</span>}
          {extra}
        </div>
        {children}
      </div>
    </section>
  )
}

function App() {
  const [runs, setRuns] = useState<RunInfo[]>([])
  const [selectedRun, setSelectedRun] = useState<string | null>(null)
  const [scores, setScores] = useState<ScoreEntry[]>([])
  const [runData, setRunData] = useState<RunData | null>(null)
  const [backlog, setBacklog] = useState<BacklogResponse | null>(null)
  const [reports, setReports] = useState<ReportInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [lastLoaded, setLastLoaded] = useState<Date | null>(null)
  const [showClosedBacklog, setShowClosedBacklog] = useState(false)
  const [view, setView] = useState<'dashboard' | 'reports'>('dashboard')
  const headerRef = useRef<HTMLDivElement>(null)
  const [headerHeight, setHeaderHeight] = useState(0)
  const [theme, setTheme] = useTheme()
  const selectedScore = scores.find((s) => s.run_id === selectedRun)?.score ?? null

  // sticky ヘッダーの実寸を測り、ジャンプ先の scroll-margin と現在地判定に使う
  useEffect(() => {
    const el = headerRef.current
    if (!el) return
    const observer = new ResizeObserver(() => {
      const height = el.getBoundingClientRect().height
      setHeaderHeight(height)
      document.documentElement.style.setProperty('--dash-header-h', `${height}px`)
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const loadAll = useCallback(async (runIdOverride?: string) => {
    setLoading(true)
    setError(null)
    try {
      const [runList, scoreList, backlogData, reportList] = await Promise.all([
        api.runs(),
        api.scores(),
        api.backlog(showClosedBacklog),
        api.reports(),
      ])
      setRuns(runList)
      setScores(scoreList)
      setBacklog(backlogData)
      setReports(reportList)

      const runId = runIdOverride ?? selectedRun ?? runList[0]?.run_id ?? null
      setSelectedRun(runId)

      if (runId) {
        const [alp, slowquery, metrics, pprof, fgprof, mysql, upstream, userTransitions, timeline] = await Promise.all([
          api.alp(runId),
          api.slowQuery(runId),
          api.metrics(runId),
          api.pprof(runId),
          api.fgprof(runId),
          api.mysql(runId),
          api.upstream(runId),
          api.userTransitions(runId),
          api.timeline(runId),
        ])
        setRunData({ alp, slowquery, metrics, pprof, fgprof, mysql, upstream, userTransitions, timeline })
      } else {
        setRunData(null)
      }
      setLastLoaded(new Date())
    } catch (err) {
      setError(String((err as Error).message ?? err))
    } finally {
      setLoading(false)
    }
  }, [selectedRun, showClosedBacklog])

  useEffect(() => {
    loadAll()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const handleToggleClosedBacklog = async (checked: boolean) => {
    setShowClosedBacklog(checked)
    try {
      const backlogData = await api.backlog(checked)
      setBacklog(backlogData)
    } catch (err) {
      setError(String((err as Error).message ?? err))
    }
  }

  const handleRunChange = async (runId: string) => {
    setSelectedRun(runId)
    setLoading(true)
    setError(null)
    try {
      const [alp, slowquery, metrics, pprof, fgprof, mysql, upstream, userTransitions, timeline] = await Promise.all([
        api.alp(runId),
        api.slowQuery(runId),
        api.metrics(runId),
        api.pprof(runId),
        api.fgprof(runId),
        api.mysql(runId),
        api.upstream(runId),
        api.userTransitions(runId),
        api.timeline(runId),
      ])
      setRunData({ alp, slowquery, metrics, pprof, fgprof, mysql, upstream, userTransitions, timeline })
      setLastLoaded(new Date())
    } catch (err) {
      setError(String((err as Error).message ?? err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-base-200">
      <div ref={headerRef} className="sticky top-0 z-20 border-b border-base-300 bg-base-100/90 backdrop-blur">
        <div className="navbar gap-4 px-8">
          <div className="navbar-start gap-4">
            {/* ghost ボタンにする以上は押せる必要があるので、ダッシュボード先頭へ戻す */}
            <button
              className="btn btn-ghost gap-2.5 px-2 text-lg font-bold"
              onClick={() => {
                setView('dashboard')
                window.scrollTo({ top: 0, behavior: 'smooth' })
              }}
            >
              <img src="/favicon.svg" alt="" className="size-9 object-contain" />
              ISUCON 計測ダッシュボード
            </button>
            <div className="divider divider-horizontal mx-0" />
            {/* 2択なので tabs ではなく join のセグメンテッドコントロール。
                active を btn-neutral にして、明色テーマでも選択状態が下地に溶けないようにする */}
            <div className="join">
              <button
                aria-pressed={view === 'dashboard'}
                className={`btn btn-sm join-item ${view === 'dashboard' ? 'btn-neutral' : ''}`}
                onClick={() => setView('dashboard')}
              >
                ダッシュボード
              </button>
              <button
                aria-pressed={view === 'reports'}
                className={`btn btn-sm join-item ${view === 'reports' ? 'btn-neutral' : ''}`}
                onClick={() => setView('reports')}
              >
                計測レポート
              </button>
            </div>
          </div>

          <div className="navbar-end gap-3">
            {/* 数値は色付きピルではなく見出し付きの stat で出す。
                stat 同士の区切り線がそのままグループの仕切りになる */}
            <div className="stats stats-horizontal bg-transparent">
              {view === 'dashboard' && selectedScore != null && (
                <div className="stat gap-0 px-3 py-0">
                  <div className="stat-title text-xs">選択中RUNのスコア</div>
                  <div className="stat-value text-xl tabular-nums">{selectedScore.toLocaleString()}</div>
                </div>
              )}
              <div className="stat gap-0 px-3 py-0">
                <div className="stat-title text-xs">最終更新</div>
                {/* スコアより一段軽くする。主役はスコアの方 */}
                <div className="stat-value text-base font-medium tabular-nums text-base-content/70">
                  {lastLoaded ? lastLoaded.toLocaleTimeString('ja-JP') : '—'}
                </div>
              </div>
            </div>

            {/* RUN の選択と再読み込みは一続きの操作なので join でまとめる */}
            <div className="join">
              {view === 'dashboard' && (
                <RunSelector
                  className="join-item"
                  runs={runs}
                  selected={selectedRun}
                  onChange={handleRunChange}
                />
              )}
              <button
                className="btn btn-primary btn-sm join-item"
                disabled={loading}
                onClick={() => loadAll(selectedRun ?? undefined)}
              >
                {loading ? (
                  <span className="loading loading-spinner loading-xs" />
                ) : (
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" className="size-4">
                    <path d="M21 12a9 9 0 1 1-2.6-6.4M21 3v6h-6" />
                  </svg>
                )}
                {loading ? '更新中…' : '再読み込み'}
              </button>
            </div>
            <ThemeToggle value={theme} onChange={setTheme} />
          </div>
        </div>
        {view === 'dashboard' && <SectionNav sections={SECTIONS} offset={headerHeight} />}
      </div>

      <div className="mx-auto max-w-[1920px] px-8 pt-6 pb-12">
        {error && (
          <div className="alert alert-error mb-5 text-base">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="size-5 shrink-0">
              <circle cx="12" cy="12" r="9" />
              <path d="M12 7v6m0 3.5v.5" />
            </svg>
            <span>{error}</span>
            <button className="btn btn-sm btn-ghost" onClick={() => setError(null)}>
              閉じる
            </button>
          </div>
        )}

        {view === 'dashboard' ? (
          <>
            <Section id="score-trend" title="スコア推移" badge={`${scores.length} RUN`}>
              <ScoreTrend scores={scores} />
            </Section>

            <Section id="benchmark-timeline" title="ベンチマーカー挙動タイムライン">
              {runData ? <BenchmarkTimeline timeline={runData.timeline} /> : <ChartSkeleton />}
            </Section>

            <Section
              id="backlog"
              title="バックログボード"
              badge={backlog ? `${backlog.cards.length} 件` : undefined}
              extra={
                <label className="label ml-auto cursor-pointer gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="toggle toggle-sm toggle-primary"
                    checked={showClosedBacklog}
                    onChange={(e) => handleToggleClosedBacklog(e.target.checked)}
                  />
                  VALIDATED / REJECTED も表示
                </label>
              }
            >
              {backlog ? (
                <BacklogBoard backlog={backlog} showClosed={showClosedBacklog} />
              ) : (
                <ChartSkeleton height={180} columns={4} />
              )}
            </Section>

            <Section id="alp" title="alp トップボトルネック">
              {runData ? <AlpTop rows={runData.alp.rows} /> : <TableSkeleton />}
            </Section>

            <Section id="upstream" title="nginx upstream/キャッシュ内訳">
              {runData ? <UpstreamBreakdown data={runData.upstream} /> : <TableSkeleton />}
            </Section>

            <Section id="user-transitions" title="Cookie ユーザー遷移">
              {runData ? <UserTransitions data={runData.userTransitions} /> : <TableSkeleton />}
            </Section>

            <Section id="slowquery" title="slow query トップ">
              {runData ? <SlowQueryTop data={runData.slowquery} /> : <TableSkeleton />}
            </Section>

            <Section id="mysql" title="MySQL ステータス">
              {runData ? <MysqlStatus data={runData.mysql} /> : <ChartSkeleton columns={3} />}
            </Section>

            <Section id="resources" title="ホスト/サービス別リソース時系列">
              {runData ? <ResourceTimeSeries metrics={runData.metrics} /> : <ChartSkeleton columns={3} />}
            </Section>

            <Section id="pprof" title="Go pprof">
              {runData ? <GoPprofTop data={runData.pprof} /> : <TableSkeleton />}
            </Section>

            <Section id="fgprof" title="fgprof wall-clock">
              {runData ? <FgprofTop data={runData.fgprof} /> : <TableSkeleton />}
            </Section>

          </>
        ) : (
          <Section title="計測レポート（docs/reports）" badge={`${reports.length} 件`}>
            <ReportsPanel reports={reports} />
          </Section>
        )}
      </div>
    </div>
  )
}

export default App

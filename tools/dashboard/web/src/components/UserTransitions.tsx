import { useState } from 'react'
import type { UserScenario, UserTransitionEdge, UserTransitionsResponse } from '../api'
import { EmptyState } from './EmptyState'
import { TransitionNodeGraph } from './TransitionNodeGraph'

const EMPTY_EDGES: UserTransitionEdge[] = []
const EMPTY_SCENARIOS: UserScenario[] = []
const SCENARIO_CARD_LIMIT = 8

function percent(part: number, total: number) {
  return total > 0 ? (part / total) * 100 : 0
}

export function UserTransitions({ data }: { data: UserTransitionsResponse }) {
  const edges = data.edges ?? EMPTY_EDGES
  const scenarios = data.scenarios ?? EMPTY_SCENARIOS
  const [selectedScenarioID, setSelectedScenarioID] = useState<string | null>(null)
  const effectiveScenarioID = selectedScenarioID === 'all'
    ? 'all'
    : scenarios.some((scenario) => scenario.id === selectedScenarioID)
      ? selectedScenarioID!
      : scenarios[0]?.id ?? 'all'
  const selectedScenario = effectiveScenarioID === 'all'
    ? null
    : scenarios.find((scenario) => scenario.id === effectiveScenarioID) ?? null
  const viewEdges = selectedScenario?.edges ?? edges
  const scenarioRank = selectedScenario ? scenarios.findIndex((scenario) => scenario.id === selectedScenario.id) + 1 : 0
  const firstNodes = [...(selectedScenario?.nodes ?? [])]
    .filter((node) => node.first_sessions > 0)
    .sort((a, b) => b.first_sessions - a.first_sessions)
  const lastNodes = [...(selectedScenario?.nodes ?? [])]
    .filter((node) => node.last_sessions > 0)
    .sort((a, b) => b.last_sessions - a.last_sessions)

  if (!data.available) {
    return (
      <EmptyState
        title="user-transitions.json がまだありません"
        detail="このRUNはユーザー遷移集計の導入前か、access log 集計に失敗した可能性があります"
      />
    )
  }

  const summary = data.summary
  const coverage = percent(summary.requests_with_identity, summary.classified_requests)
  const overlapRate = percent(summary.overlapping_transitions, summary.transitions)

  return (
    <div className="space-y-5">
      <div className="stats stats-horizontal w-full overflow-x-auto bg-base-200 shadow-sm">
        <div className="stat py-4">
          <div className="stat-title">Cookie セッション</div>
          <div className="stat-value text-2xl tabular-nums">{summary.sessions.toLocaleString()}</div>
        </div>
        <div className="stat py-4">
          <div className="stat-title">API 遷移</div>
          <div className="stat-value text-2xl tabular-nums">{summary.transitions.toLocaleString()}</div>
        </div>
        <div className="stat py-4">
          <div className="stat-title">シナリオ群</div>
          <div className="stat-value text-2xl tabular-nums">{summary.scenario_groups.toLocaleString()}</div>
          <div className="stat-desc">利用API集合が同じCookie群</div>
        </div>
        <div className="stat py-4">
          <div className="stat-title">Cookie 観測率</div>
          <div className="stat-value text-2xl tabular-nums">{coverage.toFixed(1)}%</div>
          <div className="stat-desc">正規化済みAPIリクエスト比</div>
        </div>
        <div className="stat py-4">
          <div className="stat-title">重複実行を含む遷移</div>
          <div className="stat-value text-2xl tabular-nums">{overlapRate.toFixed(1)}%</div>
          <div className="stat-desc">次の開始が直前の完了より早い</div>
        </div>
      </div>

      {edges.length === 0 ? (
        <EmptyState title="Cookie で結べる API 遷移がありません" detail="summary の欠損・未分類件数を確認してください" />
      ) : (
        <>
          {scenarios.length > 0 && (
            <div className="space-y-3">
              <div className="flex flex-wrap items-end justify-between gap-3">
                <div>
                  <h3 className="font-semibold">Cookieジャーニーのシナリオ群</h3>
                  <div className="text-xs text-base-content/60">
                    同じ正規化API集合を利用したCookieセッションをまとめています。反復回数や同時刻内の順序では分割しません。
                  </div>
                </div>
                <label className="select select-sm w-80">
                  <span className="label">表示</span>
                  <select value={effectiveScenarioID} onChange={(event) => setSelectedScenarioID(event.target.value)}>
                    {scenarios.map((scenario, index) => (
                      <option key={scenario.id} value={scenario.id}>
                        シナリオ {index + 1} — {scenario.sessions.toLocaleString()} セッション / {scenario.signature.length} API
                      </option>
                    ))}
                    <option value="all">全Cookieセッション</option>
                  </select>
                </label>
              </div>

              <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
                {scenarios.slice(0, SCENARIO_CARD_LIMIT).map((scenario, index) => {
                  const selected = scenario.id === effectiveScenarioID
                  return (
                    <button
                      key={scenario.id}
                      className={`card border text-left transition-colors ${selected ? 'border-primary bg-primary/10' : 'border-base-300 bg-base-100 hover:border-primary/50'}`}
                      aria-pressed={selected}
                      onClick={() => setSelectedScenarioID(scenario.id)}
                    >
                      <div className="card-body gap-1 p-3">
                        <div className="flex items-center justify-between gap-2">
                          <span className="font-semibold">シナリオ {index + 1}</span>
                          <span className="badge badge-sm badge-neutral">{scenario.signature.length} API</span>
                        </div>
                        <div className="text-xl font-bold tabular-nums">{scenario.sessions.toLocaleString()} <span className="text-xs font-normal">sessions</span></div>
                        <div className="text-xs text-base-content/60">
                          平均 {scenario.requests_per_session_avg.toFixed(1)} req / p50 {(scenario.duration_p50_ms / 1000).toFixed(1)}秒
                        </div>
                      </div>
                    </button>
                  )
                })}
              </div>

              {selectedScenario && (
                <div className="rounded-box border border-base-300 bg-base-200/50 p-4">
                  <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                    <div><div className="text-xs text-base-content/60">対象</div><div className="font-semibold">シナリオ {scenarioRank}</div></div>
                    <div><div className="text-xs text-base-content/60">Cookieセッション</div><div className="font-semibold tabular-nums">{selectedScenario.sessions.toLocaleString()}</div></div>
                    <div><div className="text-xs text-base-content/60">リクエスト</div><div className="font-semibold tabular-nums">{selectedScenario.requests.toLocaleString()}</div></div>
                    <div><div className="text-xs text-base-content/60">観測継続時間 p50 / p95</div><div className="font-semibold tabular-nums">{(selectedScenario.duration_p50_ms / 1000).toFixed(1)}秒 / {(selectedScenario.duration_p95_ms / 1000).toFixed(1)}秒</div></div>
                  </div>
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    {selectedScenario.signature.map((node) => <span key={node} className="badge badge-soft badge-sm font-mono">{node}</span>)}
                  </div>
                  <div className="mt-3 grid gap-3 lg:grid-cols-2">
                    <div>
                      <div className="mb-1 text-xs font-semibold text-base-content/60">Cookie観測開始API</div>
                      <div className="flex flex-wrap gap-1.5">
                        {firstNodes.map((node) => (
                          <span key={`${node.method} ${node.route}`} className="badge badge-success badge-outline badge-sm font-mono">
                            {node.method} {node.route} · {node.first_sessions.toLocaleString()}
                          </span>
                        ))}
                      </div>
                    </div>
                    <div>
                      <div className="mb-1 text-xs font-semibold text-base-content/60">Cookie観測終了API</div>
                      <div className="flex flex-wrap gap-1.5">
                        {lastNodes.map((node) => (
                          <span key={`${node.method} ${node.route}`} className="badge badge-error badge-outline badge-sm font-mono">
                            {node.method} {node.route} · {node.last_sessions.toLocaleString()}
                          </span>
                        ))}
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}

          <TransitionNodeGraph
            key={effectiveScenarioID}
            edges={viewEdges}
            timelineNodes={selectedScenario?.nodes}
            title={selectedScenario ? `シナリオ ${scenarioRank} のAPI遷移` : '全CookieセッションのAPI遷移'}
          />
        </>
      )}

      <div className="text-xs leading-relaxed text-base-content/60">
        Cookie のないリクエストは遷移・シナリオの対象外です。各セッションはCookie付きの最初から最後までを観測し、
        Cookie 値とユーザー別履歴は保存していません。リクエスト開始時刻順で集計し、同時刻は応答終了時刻と入力順で決定します。
        未分類 {summary.unmatched_api_requests.toLocaleString()} 件、Cookieなし {summary.missing_identity_requests.toLocaleString()} 件、
        順序曖昧 {summary.ambiguous_order_transitions.toLocaleString()} 遷移、壊れたログ {summary.malformed_lines.toLocaleString()} 行です。
      </div>
    </div>
  )
}

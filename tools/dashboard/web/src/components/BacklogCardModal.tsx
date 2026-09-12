import { useEffect, useRef, useState } from 'react'
import { api, type BacklogCardDetail } from '../api'
import { STATUS_COLORS } from '../colors'

type Props = {
  id: string | null
  onClose: () => void
}

const META_FIELDS: Array<[keyof BacklogCardDetail, string]> = [
  ['source_runs', 'source runs'],
  ['compare_run', 'compare runs'],
  ['observed_runs', 'observed runs'],
]

export function BacklogCardModal({ id, onClose }: Props) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const [detail, setDetail] = useState<BacklogCardDetail | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) {
      dialogRef.current?.close()
      return
    }
    dialogRef.current?.showModal()
    setDetail(null)
    setError(null)
    setLoading(true)
    api
      .backlogDetail(id)
      .then(setDetail)
      .catch((err) => setError(String(err.message ?? err)))
      .finally(() => setLoading(false))
  }, [id])

  const sections = detail?.sections.filter((s) => s.body.trim() !== '') ?? []
  const metaEntries = detail
    ? META_FIELDS.filter(([key]) => String(detail[key] ?? '').trim() !== '')
    : []

  return (
    <dialog ref={dialogRef} className="modal" onClose={onClose}>
      <div className="modal-box max-w-4xl">
        <form method="dialog">
          <button className="btn btn-sm btn-circle btn-ghost absolute top-3 right-3" aria-label="閉じる">
            ✕
          </button>
        </form>
        {loading && (
          <div className="flex flex-col gap-3 py-2">
            <div className="skeleton h-8 w-2/3" />
            <div className="skeleton h-4 w-full" />
            <div className="skeleton h-4 w-full" />
            <div className="skeleton h-4 w-3/4" />
          </div>
        )}
        {error && (
          <div className="alert alert-error text-base">
            <span>{error}</span>
          </div>
        )}
        {detail && !loading && (
          <>
            <div className="mb-1 flex items-start gap-3 pr-8">
              <span
                className="status status-lg mt-2"
                style={{
                  background: STATUS_COLORS[detail.status] ?? 'var(--chart-muted)',
                  color: STATUS_COLORS[detail.status] ?? 'var(--chart-muted)',
                }}
              />
              <div className="min-w-0">
                <div className="font-mono text-sm text-base-content/60">
                  {detail.id} · Intervention · {detail.status}
                </div>
                <h3 className="text-2xl font-bold">{detail.title}</h3>
              </div>
            </div>

            {detail.closed && detail.targets.length === 0 && (
              <p className="mb-4 text-sm text-base-content/70">
                過去の判定を保持したカードです。現在のTargetへの関連はありません。
              </p>
            )}
            {detail.targets.length > 0 && (
              <div className="mb-4 space-y-3">
                <div className="divider divider-start my-1 text-sm font-semibold">Targets</div>
                {detail.targets.map((target) => (
                  <div key={target.id} className="rounded-box border border-base-300 p-3 space-y-2">
                    <p className="font-semibold">
                      {target.id} · {target.status} · {target.title}
                      {target.is_primary && <span className="badge badge-soft ml-2">主対象</span>}
                    </p>
                    {target.rationale && <p className="text-base-content/80">{target.rationale}</p>}
                    <dl className="text-sm space-y-1">
                      {([
                        ['対象範囲', target.scope],
                        ['評価軸', target.axis],
                        ['今回の目標', target.goal],
                        ['評価条件', target.evaluation],
                        ['Evidence・現在の状況', target.evidence],
                        ['前回のTarget', target.previous_target_id],
                      ] as const).filter(([, value]) => value).map(([label, value]) => (
                        <div key={label}>
                          <dt className="font-semibold">{label}</dt>
                          <dd className="whitespace-pre-wrap text-base-content/80">{value}</dd>
                        </div>
                      ))}
                    </dl>
                    {detail.objectives.filter((objective) => objective.target_id === target.id).map((objective) => (
                      <p key={objective.id} className="text-sm text-base-content/80">
                        Objective: {objective.id} · {objective.status} · {objective.mode} · {objective.title}
                        {objective.is_primary && '（主Objective）'}
                        {objective.rationale && ` — ${objective.rationale}`}
                      </p>
                    ))}
                  </div>
                ))}
              </div>
            )}
            <div className="mb-4 flex flex-wrap gap-1.5 text-sm">
              {detail.priority && <span className="badge badge-soft badge-primary">{detail.priority}</span>}
              {detail.area && <span className="badge badge-ghost">area: {detail.area}</span>}
              {detail.owner && <span className="badge badge-ghost">owner: {detail.owner}</span>}
              <span className="badge badge-ghost">
                updated: {detail.updated} ({detail.updated_by})
              </span>
            </div>

            {sections.length > 0 && (
              <div className="mb-4 space-y-4">
                {sections.map((s) => (
                  <div key={s.name}>
                    <div className="divider divider-start my-1 text-sm font-semibold">{s.name}</div>
                    <p className="text-base whitespace-pre-wrap text-base-content/80">{s.body}</p>
                  </div>
                ))}
              </div>
            )}

            {(detail.dependencies.length > 0 || detail.unblocks.length > 0) && (
              <div className="mb-4 space-y-3">
                {detail.dependencies.length > 0 && (
                  <div>
                    <div className="divider divider-start my-1 text-sm font-semibold">Depends on</div>
                    {detail.dependencies.map((dependency) => (
                      <p key={dependency.depends_on_card_id} className="text-base text-base-content/80">
                        {dependency.depends_on_card_id} · {dependency.mode} · requires {dependency.required_status} · current {dependency.status}
                        {dependency.reason && ` — ${dependency.reason}`}
                      </p>
                    ))}
                  </div>
                )}
                {detail.unblocks.length > 0 && (
                  <div>
                    <div className="divider divider-start my-1 text-sm font-semibold">Unblocks</div>
                    {detail.unblocks.map((dependency) => (
                      <p key={dependency.card_id} className="text-base text-base-content/80">
                        {dependency.card_id} · {dependency.mode} · requires {dependency.required_status} · current {dependency.status}
                        {dependency.reason && ` — ${dependency.reason}`}
                      </p>
                    ))}
                  </div>
                )}
              </div>
            )}

            {metaEntries.length > 0 && (
              <div className="mb-4 overflow-x-auto rounded-box border border-base-300">
                <table className="table text-base">
                  <tbody>
                    {metaEntries.map(([key, label]) => (
                      <tr key={key}>
                        <th className="w-36 whitespace-nowrap">{label}</th>
                        <td className="break-all">{String(detail[key])}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {detail.history.length > 0 && (
              <div>
                <div className="divider divider-start my-1 text-sm font-semibold">History</div>
                <ul className="list rounded-box bg-base-200/50">
                  {detail.history.map((h) => (
                    <li key={h.position} className="list-row text-sm">
                      <div className="text-xs whitespace-nowrap text-base-content/60">
                        {h.occurred_at}
                        <div className="opacity-70">{h.actor}</div>
                      </div>
                      <div className="list-col-grow whitespace-pre-wrap text-base-content/80">{h.body}</div>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </>
        )}
      </div>
      <form method="dialog" className="modal-backdrop">
        <button>close</button>
      </form>
    </dialog>
  )
}

import { useState } from 'react'
import type { BacklogResponse } from '../api'
import { STATUS_COLORS, STATUS_COLUMN_ORDER } from '../colors'
import { BacklogCardModal } from './BacklogCardModal'

const CLOSED_STATUSES = new Set(['VALIDATED', 'REJECTED'])
const VISIBLE_PER_COLUMN = 20

type Props = {
  backlog: BacklogResponse
  showClosed: boolean
}

export function BacklogBoard({ backlog, showClosed }: Props) {
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const columns = showClosed
    ? STATUS_COLUMN_ORDER
    : STATUS_COLUMN_ORDER.filter((status) => !CLOSED_STATUSES.has(status))

  const byStatus = new Map<string, typeof backlog.cards>()
  for (const status of columns) byStatus.set(status, [])
  for (const card of backlog.cards) {
    if (!byStatus.has(card.status)) byStatus.set(card.status, [])
    byStatus.get(card.status)!.push(card)
  }

  return (
    <>
      <div
        className="grid gap-3 overflow-x-auto pb-1"
        style={{ gridTemplateColumns: `repeat(${columns.length}, minmax(200px, 1fr))` }}
      >
        {[...byStatus.entries()].map(([status, cards]) => (
          <div key={status} className="card card-border min-w-0 border-base-300 bg-base-200/50">
            <div className="card-body gap-2 p-3">
              <div className="flex items-center gap-2">
                <span
                  className="status status-md"
                  style={{
                    background: STATUS_COLORS[status] ?? 'var(--chart-muted)',
                    color: STATUS_COLORS[status] ?? 'var(--chart-muted)',
                  }}
                />
                <span className="text-sm font-semibold tracking-wide">{status}</span>
                <span className="badge badge-ghost badge-xs ml-auto tabular-nums">{cards.length}</span>
              </div>

              {cards.length === 0 && (
                <p className="py-2 text-center text-xs text-base-content/40">なし</p>
              )}

              {cards.slice(0, VISIBLE_PER_COLUMN).map((card) => (
                <button
                  key={card.id}
                  onClick={() => setSelectedId(card.id)}
                  title={card.title}
                  className="card card-border card-xs w-full border-base-300 bg-base-100 text-left transition hover:border-primary hover:shadow-md"
                >
                  <div className="card-body gap-1 p-2.5">
                    <div className="flex items-center gap-1.5">
                      <span className="font-mono text-xs text-base-content/50">{card.id}</span>
                      {card.priority && (
                        <span className="badge badge-outline badge-xs">{card.priority}</span>
                      )}
                      {card.kind === 'MEASUREMENT' && (
                        <span className="badge badge-info badge-xs">measurement</span>
                      )}
                    </div>
                    <span className="line-clamp-2 text-sm text-base-content">{card.title}</span>
                  </div>
                </button>
              ))}

              {cards.length > VISIBLE_PER_COLUMN && (
                <p className="text-center text-xs text-base-content/50">
                  ほか {cards.length - VISIBLE_PER_COLUMN} 件
                </p>
              )}
            </div>
          </div>
        ))}
      </div>
      <BacklogCardModal id={selectedId} onClose={() => setSelectedId(null)} />
    </>
  )
}

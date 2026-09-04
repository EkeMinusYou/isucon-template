/** 系列の凡例。daisyUI の badge + status ドットで表示する */
export function LegendRow({ entries }: { entries: Array<{ key: string; color: string }> }) {
  if (entries.length === 0) return null
  return (
    <div className="mb-3 flex flex-wrap gap-1.5">
      {entries.map((e) => (
        <span key={e.key} className="badge badge-sm badge-ghost gap-1.5 font-normal">
          <span className="status status-sm" style={{ background: e.color, color: e.color }} />
          {e.key}
        </span>
      ))}
    </div>
  )
}

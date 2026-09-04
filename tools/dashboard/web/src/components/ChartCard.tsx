/**
 * グラフ 1 枚分の枠。ResourceTimeSeries / TinyLineChart / BenchmarkTimeline で
 * 別々に書かれていた枠を daisyUI の card に揃えたもの。
 */
export function ChartCard({
  title,
  unit,
  actions,
  children,
}: {
  title: string
  unit?: string
  actions?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="card card-border border-base-300 bg-base-100">
      {/* min-w-0: グリッド/フレックス内で recharts のチャートが縮めるようにする */}
      <div className="card-body min-w-0 gap-2 p-3">
        <div className="flex items-baseline gap-2">
          <h4 className="card-title text-sm">
            {title}
            {unit && <span className="ml-1 font-normal text-base-content/50">({unit})</span>}
          </h4>
          {actions && <div className="ml-auto">{actions}</div>}
        </div>
        {children}
      </div>
    </div>
  )
}

/** カテゴリ見出し（CPU / メモリ / …）。daisyUI の divider で区切る */
export function CategoryDivider({ label }: { label: string }) {
  return (
    <div className="divider divider-start my-2 text-sm font-semibold text-base-content/60">{label}</div>
  )
}

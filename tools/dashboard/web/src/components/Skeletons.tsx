/** RUN データ取得中のプレースホルダ。グラフ主体のセクション向け */
export function ChartSkeleton({ height = 220, columns = 2 }: { height?: number; columns?: number }) {
  return (
    <div className={`grid gap-4 ${columns >= 2 ? 'md:grid-cols-2' : ''}`}>
      {Array.from({ length: columns }, (_, i) => (
        <div key={i} className="skeleton w-full" style={{ height }} />
      ))}
    </div>
  )
}

/** 表主体のセクション向けプレースホルダ */
export function TableSkeleton({ rows = 6 }: { rows?: number }) {
  return (
    <div className="flex flex-col gap-2">
      <div className="skeleton h-8 w-full" />
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="skeleton h-6 w-full" />
      ))}
    </div>
  )
}

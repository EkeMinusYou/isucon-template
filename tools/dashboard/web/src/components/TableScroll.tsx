/**
 * 表の共通ラッパ。枠を rounded-box で閉じ、縦横ともスクロールさせる。
 * 中の `table` に `table-pin-rows` を付けると thead がスクロール中も残る。
 */
export function TableScroll({ maxHeight = 384, children }: { maxHeight?: number; children: React.ReactNode }) {
  return (
    <div
      className="overflow-auto rounded-box border border-base-300 bg-base-100"
      style={{ maxHeight }}
    >
      {children}
    </div>
  )
}

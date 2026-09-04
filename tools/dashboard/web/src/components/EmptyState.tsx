type Props = {
  /** 「〜がまだありません」の主文 */
  title: string
  /** 理由・切り分けの補足 */
  detail?: string
  tone?: 'info' | 'warning'
}

const ICONS = {
  info: (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="size-5 shrink-0">
      <circle cx="12" cy="12" r="9" />
      <path d="M12 11v5m0-8.5v.5" />
    </svg>
  ),
  warning: (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="size-5 shrink-0">
      <path d="M10.3 4.3 2.6 17.5A2 2 0 0 0 4.3 20.5h15.4a2 2 0 0 0 1.7-3L13.7 4.3a2 2 0 0 0-3.4 0z" />
      <path d="M12 9v4m0 3.5v.5" />
    </svg>
  ),
}

/** 計測ファイル未取得などで中身を出せないときの共通表示 */
export function EmptyState({ title, detail, tone = 'info' }: Props) {
  return (
    <div className={`alert alert-soft ${tone === 'warning' ? 'alert-warning' : 'alert-info'} text-base`}>
      {ICONS[tone]}
      <div>
        <div className="font-medium">{title}</div>
        {detail && <div className="text-sm opacity-70">{detail}</div>}
      </div>
    </div>
  )
}

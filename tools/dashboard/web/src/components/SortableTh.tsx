import type { SortDir } from '../useSort'

type Props = {
  label: string
  active: boolean
  dir: SortDir
  onClick: () => void
  align?: 'left' | 'right'
}

export function SortableTh({ label, active, dir, onClick, align = 'left' }: Props) {
  return (
    <th className={align === 'right' ? 'text-right' : ''}>
      <button
        className={`btn btn-ghost btn-xs -mx-1 gap-1 font-semibold ${active ? 'text-base-content' : ''}`}
        onClick={onClick}
      >
        {label}
        <span className={active ? 'text-primary' : 'opacity-25'}>
          {active && dir === 'asc' ? '▲' : '▼'}
        </span>
      </button>
    </th>
  )
}

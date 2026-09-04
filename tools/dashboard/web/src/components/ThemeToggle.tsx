import type { ThemePref } from '../useTheme'

const OPTIONS: Array<{ value: ThemePref; label: string; icon: React.ReactNode }> = [
  {
    value: 'system',
    label: 'システム',
    icon: (
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="size-4">
        <rect x="2" y="4" width="20" height="13" rx="2" />
        <path d="M8 21h8m-4-4v4" />
      </svg>
    ),
  },
  {
    value: 'light',
    label: 'ライト',
    icon: (
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="size-4">
        <circle cx="12" cy="12" r="4" />
        <path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
      </svg>
    ),
  },
  {
    value: 'dark',
    label: 'ダーク',
    icon: (
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="size-4">
        <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
      </svg>
    ),
  },
]

type Props = {
  value: ThemePref
  onChange: (next: ThemePref) => void
}

export function ThemeToggle({ value, onChange }: Props) {
  const active = OPTIONS.find((o) => o.value === value) ?? OPTIONS[0]

  return (
    <div className="dropdown dropdown-end">
      <div tabIndex={0} role="button" className="btn btn-ghost btn-circle" aria-label={`テーマ: ${active.label}`}>
        {active.icon}
      </div>
      <ul tabIndex={0} className="dropdown-content menu z-30 mt-2 w-40 rounded-box border border-base-300 bg-base-100 p-2 shadow-lg">
        <li className="menu-title">テーマ</li>
        {OPTIONS.map((o) => (
          <li key={o.value}>
            <button className={value === o.value ? 'menu-active' : ''} onClick={() => onChange(o.value)}>
              {o.icon}
              {o.label}
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}

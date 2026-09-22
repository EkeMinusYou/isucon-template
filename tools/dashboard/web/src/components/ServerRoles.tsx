import type { RunInfo } from '../api'
import { EmptyState } from './EmptyState'

export function ServerRoles({ run }: { run?: RunInfo }) {
  const roles = run?.roles
  if (!roles) {
    return <EmptyState title="サーバーの役割情報がありません" detail="選択中RUNの run.json に保存された構成を表示します。" />
  }

  const hosts = new Map<string, Set<string>>()
  const add = (names: string[] | null | undefined, role?: string) => {
    for (const host of names ?? []) {
      if (!host) continue
      if (!hosts.has(host)) hosts.set(host, new Set())
      if (role) hosts.get(host)!.add(role)
    }
  }
  add(roles.additional?.all)
  add(roles.app, 'app')
  add(roles.nginx, 'nginx')
  add(roles.additional?.mysql_all, 'mysql')
  add([roles.mysql], 'mysql')

  if (hosts.size === 0) {
    return <EmptyState title="サーバーの役割情報がありません" />
  }

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
      {[...hosts].sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true })).map(([host, labels]) => (
        <div key={host} className="rounded-box border border-base-300 bg-base-200/50 p-4">
          <h3 className="mb-3 font-mono font-semibold">{host}</h3>
          <div className="flex flex-wrap gap-2">
            {labels.size > 0 ? [...labels].map((label) => (
              <span key={label} className="badge badge-soft badge-primary badge-sm">{label}</span>
            )) : <span className="text-sm text-base-content/60">役割の記録なし</span>}
          </div>
        </div>
      ))}
    </div>
  )
}

import type { RunRoles } from './api.ts'

export function serverRoles(roles: RunRoles): { host: string; labels: string[] }[] {
  const hosts = new Map<string, Set<string>>()
  const add = (names: string[] | null | undefined, label?: string) => {
    for (const host of names ?? []) {
      if (!host) continue
      if (!hosts.has(host)) hosts.set(host, new Set())
      if (label) hosts.get(host)!.add(label)
    }
  }
  add(roles.additional?.all)
  add(roles.app, 'Go')
  add(roles.ingress, 'HTTP入口')
  add(roles.additional?.nginx, 'nginx')
  add(roles.app_traffic, 'API処理')
  add(roles.entry ? [roles.entry] : [], 'ベンチ入口')
  for (const [name, members] of Object.entries(roles.additional ?? {})) {
    if (!['all', 'nginx', 'app', 'app_traffic', 'ingress', 'entry'].includes(name)) add(members, name)
  }
  return [...hosts].sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true }))
    .map(([host, labels]) => ({ host, labels: [...labels] }))
}

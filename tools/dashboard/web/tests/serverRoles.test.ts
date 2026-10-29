import assert from 'node:assert/strict'
import { test } from 'node:test'
import { serverRoles } from '../src/serverRoles.ts'

test('shows separate nginx, Go and additional services from the saved RUN', () => {
  const hosts = serverRoles({ app: ['h2'], app_traffic: ['h2'], ingress: ['h1'], entry: 'h1', additional: { all: ['h1', 'h2', 'h3'], nginx: ['h1'], worker: ['h2'] } })
  assert.deepEqual(hosts, [
    { host: 'h1', labels: ['HTTP入口', 'nginx', 'ベンチ入口'] },
    { host: 'h2', labels: ['Go', 'API転送先', 'worker'] },
    { host: 'h3', labels: [] },
  ])
})

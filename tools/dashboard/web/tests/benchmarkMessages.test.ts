import assert from 'node:assert/strict'
import { test } from 'node:test'
import { groupBenchmarkMessages } from '../src/benchmarkMessages.ts'

test('groups timestamps and path IDs while preserving every original log line', () => {
  const messages = [
    '2026-01-01T00:00:01.123Z\twarn\tworker\tapi.go:10\tPOST /api/items/123 expected:400 actual:201',
    '2026-01-01T00:00:02.456+09:00\twarn\tworker\tapi.go:10\tPOST /api/items/456 expected:400 actual:201',
  ]
  const groups = groupBenchmarkMessages([...messages, messages[0]])
  assert.equal(groups.length, 1)
  assert.deepEqual(groups[0].messages, [...messages, messages[0]])
  assert.equal(groups[0].representative, messages[0].split('\t').slice(1).join('\t'))
})

test('keeps severity, source, status codes, and ordinary numeric values distinct', () => {
  const messages = [
    'warn worker HTTP expected:400 actual:201 retries:1',
    'error worker HTTP expected:400 actual:201 retries:1',
    'warn other HTTP expected:400 actual:201 retries:1',
    'warn worker HTTP expected:400 actual:500 retries:1',
    'warn worker HTTP expected:400 actual:201 retries:2',
  ]
  assert.equal(groupBenchmarkMessages(messages).length, messages.length)
})

test('handles plain text, Go timestamps, and bracketed timestamps without contest-specific rules', () => {
  assert.equal(groupBenchmarkMessages([
    'request timed out',
    '2026/01/01 01:02:03 request timed out',
    '[2026-01-01T01:02:03Z] request timed out',
  ])[0].messages.length, 3)
  assert.equal(groupBenchmarkMessages(['unrecognized message A', 'unrecognized message B']).length, 2)
  assert.deepEqual(groupBenchmarkMessages([]), [])
})

test('groups numeric and UUID URL segments but preserves query values and route names', () => {
  const groups = groupBenchmarkMessages([
    'GET https://example.test/items/123?limit=10 failed',
    'GET https://example.test/items/987?limit=10 failed',
    'GET https://example.test/items/123?limit=20 failed',
    'GET https://example.test/users/123?limit=10 failed',
    'GET /jobs/550e8400-e29b-41d4-a716-446655440000 failed',
    'GET /jobs/550e8400-e29b-41d4-a716-446655440001 failed',
  ])
  assert.deepEqual(groups.map((group) => group.messages.length), [2, 2, 1, 1])
})

test('sorts by count and preserves first occurrence order for ties', () => {
  const messages = ['first', 'second', 'second', 'third', 'third', 'first', 'second']
  assert.deepEqual(groupBenchmarkMessages(messages).map((group) => group.representative), ['second', 'first', 'third'])
  assert.deepEqual(messages, ['first', 'second', 'second', 'third', 'third', 'first', 'second'])
})

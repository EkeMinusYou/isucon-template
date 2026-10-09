export type BenchmarkMessageGroup = {
  key: string
  representative: string
  messages: string[]
}

// Recognize common ISO and Go log timestamps without depending on a benchmark's text.
const timestampPrefix = /^\[?\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?\]?(?:\t| +)/
const uuid = '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'
const pathID = new RegExp(`(/)(?:[0-9]+|${uuid})(?=[/?#\\s"'):,]|$)`, 'gi')

function withoutTimestamp(message: string): string {
  return message.trim().replace(timestampPrefix, '')
}

export function groupBenchmarkMessages(messages: readonly string[]): BenchmarkMessageGroup[] {
  const groups = new Map<string, BenchmarkMessageGroup>()
  for (const message of messages) {
    const representative = withoutTimestamp(message)
    // Preserve severity, source, status codes, and other values. Only path IDs vary.
    const key = representative.replace(pathID, '$1:id')
    const group = groups.get(key)
    if (group) {
      group.messages.push(message)
    } else {
      groups.set(key, { key, representative, messages: [message] })
    }
  }
  // Stable sorting keeps first-occurrence order when counts are equal.
  return [...groups.values()].sort((a, b) => b.messages.length - a.messages.length)
}

import { useMemo, useState } from 'react'
import { groupBenchmarkMessages } from '../benchmarkMessages'
import type { BenchmarkMessageGroup } from '../benchmarkMessages'
import { ChartCard } from './ChartCard'

const PREVIEW_GROUPS = 5

function MessageGroup({ group }: { group: BenchmarkMessageGroup }) {
  const [open, setOpen] = useState(false)
  return (
    <details onToggle={(event) => setOpen(event.currentTarget.open)} className="rounded border border-base-300 p-2">
      <summary className="cursor-pointer text-sm">
        <span className="mr-2 inline-block font-semibold tabular-nums">{group.messages.length.toLocaleString()}件</span>
        <span className="whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{group.representative}</span>
      </summary>
      {open && (
        <ol className="mt-2 max-h-64 space-y-2 overflow-auto border-t border-base-300 pt-2 font-mono text-xs" aria-label="この種類の元ログ">
          {group.messages.map((message, index) => (
            <li key={index} className="whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{message}</li>
          ))}
        </ol>
      )}
    </details>
  )
}

export function BenchmarkMessages({ messages }: { messages: string[] }) {
  const groups = useMemo(() => groupBenchmarkMessages(messages), [messages])
  const [showRemaining, setShowRemaining] = useState(false)
  if (groups.length === 0) return null

  return (
    <ChartCard title="ベンチ結果メッセージ">
      <p className="text-sm text-base-content/60">
        {messages.length.toLocaleString()}件・{groups.length.toLocaleString()}種類（ログ行数）。件数の多い順に表示しています。
      </p>
      <p className="text-xs text-base-content/60">時刻とURL内の数値ID・UUIDを除いて集約しています。各行を開くと元ログを確認できます。</p>
      <div className="space-y-2">
        {groups.slice(0, PREVIEW_GROUPS).map((group) => <MessageGroup key={group.key} group={group} />)}
      </div>
      {groups.length > PREVIEW_GROUPS && (
        <details onToggle={(event) => setShowRemaining(event.currentTarget.open)}>
          <summary className="cursor-pointer text-sm">残り{(groups.length - PREVIEW_GROUPS).toLocaleString()}種類を表示</summary>
          {showRemaining && (
            <div className="mt-2 max-h-96 space-y-2 overflow-auto">
              {groups.slice(PREVIEW_GROUPS).map((group) => <MessageGroup key={group.key} group={group} />)}
            </div>
          )}
        </details>
      )}
    </ChartCard>
  )
}

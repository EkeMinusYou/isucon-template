import { useId, useMemo, useState } from 'react'
import type { AlpRow, UserScenarioNode, UserTransitionEdge } from '../api'

const COLUMN_WIDTH = 250
const ROW_HEIGHT = 124
const MARGIN_X = 150
const MARGIN_Y = 100
const MIN_GRAPH_WIDTH = 920
const MIN_GRAPH_HEIGHT = 460
const WAVE_AMPLITUDE = 200
const ORDER_PASSES = 6

type EdgeKind = 'forward' | 'sibling' | 'back' | 'self'

type GraphNode = {
  id: string
  method: string
  route: string
  weight: number
  radius: number
  layer: number
  x: number
  y: number
}

type GraphEdge = {
  edge: UserTransitionEdge
  source: GraphNode
  target: GraphNode
  width: number
  path: string
  kind: EdgeKind
}

type Graph = {
  nodes: GraphNode[]
  edges: GraphEdge[]
  width: number
  height: number
  columnX: number[]
  labelChars: number
  chronological: boolean
}

const EMPTY_GRAPH: Graph = {
  nodes: [],
  edges: [],
  width: MIN_GRAPH_WIDTH,
  height: MIN_GRAPH_HEIGHT,
  columnX: [],
  labelChars: 40,
  chronological: false,
}

function nodeID(method: string, route: string) {
  return `${method} ${route}`
}

function pairKey(from: string, to: string) {
  return `${from}\u0000${to}`
}

function methodColor(method: string) {
  switch (method) {
    case 'GET':
      return 'var(--series-1)'
    case 'POST':
      return 'var(--series-2)'
    case 'PUT':
    case 'PATCH':
      return 'var(--series-4)'
    case 'DELETE':
      return 'var(--series-8)'
    default:
      return 'var(--series-7)'
  }
}

// Drop the /api prefix so the distinguishing tail of the route survives the
// per-column label budget. Tooltips and aria labels keep the full route.
function shortRoute(route: string) {
  const trimmed = route.replace(/^\/api(?=\/|$)/, '')
  return trimmed === '' ? '/' : trimmed
}

function truncateLabel(value: string, max: number) {
  return value.length > max ? `${value.slice(0, Math.max(1, max - 1))}…` : value
}

// Match the same route shape without evaluating arbitrary ALP regular expressions.
function apiLatency(rows: AlpRow[], method: string, route: string) {
  const pattern = `^${route.replace(/:[^/]+/g, '[^/]+')}$`
  const matches = rows.filter((row) => row.method === method && (row.uri === route || row.uri === pattern))
  // Percentiles cannot be merged from already aggregated rows.
  return matches.length === 1 && matches[0].count > 0 ? matches[0] : undefined
}

function milliseconds(seconds: number | undefined) {
  return seconds != null && Number.isFinite(seconds) ? `${(seconds * 1000).toFixed(1)} ms` : '未計測'
}

function latencyLabel(row: AlpRow | undefined) {
  return `平均 ${milliseconds(row?.avg)} / p99 ${milliseconds(row?.p99)}`
}

// Left-to-right layered layout. Columns follow observed chronology: when the
// scenario reports where each API first appears in a session, every edge that
// would run backwards in time is treated as a return edge, so the remaining
// graph ranks strictly forward. Without that timeline the ranking falls back to
// a DFS that only strips cycles. Deterministic either way, so switching RUN or
// scenario keeps the same shape.
function layoutGraph(edges: UserTransitionEdge[], timelineNodes?: UserScenarioNode[]): Graph {
  const parts = new Map<string, { method: string; route: string; weight: number; net: number }>()
  for (const edge of edges) {
    const from = nodeID(edge.from_method, edge.from_route)
    const to = nodeID(edge.to_method, edge.to_route)
    const fromNode = parts.get(from) ?? { method: edge.from_method, route: edge.from_route, weight: 0, net: 0 }
    const toNode = parts.get(to) ?? { method: edge.to_method, route: edge.to_route, weight: 0, net: 0 }
    fromNode.weight += edge.transitions
    toNode.weight += edge.transitions
    if (from !== to) {
      fromNode.net += edge.transitions
      toNode.net -= edge.transitions
    }
    parts.set(from, fromNode)
    parts.set(to, toNode)
  }

  const ids = [...parts.keys()].sort(
    (a, b) => parts.get(b)!.weight - parts.get(a)!.weight || a.localeCompare(b),
  )
  if (ids.length === 0) return EMPTY_GRAPH

  const successors = new Map<string, { to: string; transitions: number }[]>(ids.map((id) => [id, []]))
  const indegree = new Map<string, number>(ids.map((id) => [id, 0]))
  for (const edge of edges) {
    const from = nodeID(edge.from_method, edge.from_route)
    const to = nodeID(edge.to_method, edge.to_route)
    if (from === to) continue
    successors.get(from)!.push({ to, transitions: edge.transitions })
    indegree.set(to, indegree.get(to)! + 1)
  }
  for (const list of successors.values()) {
    list.sort((a, b) => b.transitions - a.transitions || a.to.localeCompare(b.to))
  }

  const timeline = new Map<string, UserScenarioNode>()
  for (const node of timelineNodes ?? []) {
    timeline.set(nodeID(node.method, node.route), node)
  }
  const chronological = ids.every((id) => timeline.has(id))
  const backEdges = new Set<string>()

  if (chronological) {
    const timeRank = new Map<string, number>()
    ;[...ids]
      .sort((a, b) => {
        const left = timeline.get(a)!
        const right = timeline.get(b)!
        return left.first_position_avg - right.first_position_avg
          || left.first_offset_avg_ms - right.first_offset_avg_ms
          || parts.get(b)!.weight - parts.get(a)!.weight
          || a.localeCompare(b)
      })
      .forEach((id, index) => timeRank.set(id, index))
    for (const id of ids) {
      for (const { to } of successors.get(id)!) {
        if (timeRank.get(to)! < timeRank.get(id)!) backEdges.add(pairKey(id, to))
      }
    }
  } else {
    // Sessions whose Cookie was first seen on this API are the natural entry
    // points. Without that hint fall back to net outflow, then to weight.
    const seedRank = (a: string, b: string) =>
      (timeline.get(b)?.first_sessions ?? 0) - (timeline.get(a)?.first_sessions ?? 0)
      || parts.get(b)!.net - parts.get(a)!.net
      || parts.get(b)!.weight - parts.get(a)!.weight
      || a.localeCompare(b)
    let seeds = ids.filter((id) => indegree.get(id) === 0).sort(seedRank)
    if (seeds.length === 0) seeds = [[...ids].sort(seedRank)[0]]
    const seedSet = new Set(seeds)
    const color = new Map<string, number>()
    for (const start of [...seeds, ...ids.filter((id) => !seedSet.has(id))]) {
      if (color.get(start)) continue
      color.set(start, 1)
      const stack = [{ id: start, index: 0 }]
      while (stack.length > 0) {
        const frame = stack[stack.length - 1]
        const neighbours = successors.get(frame.id)!
        if (frame.index >= neighbours.length) {
          color.set(frame.id, 2)
          stack.pop()
          continue
        }
        const next = neighbours[frame.index].to
        frame.index += 1
        const state = color.get(next) ?? 0
        if (state === 1) {
          backEdges.add(pairKey(frame.id, next))
          continue
        }
        if (state === 2) continue
        color.set(next, 1)
        stack.push({ id: next, index: 0 })
      }
    }
  }

  const forwardOut = new Map<string, string[]>(ids.map((id) => [id, []]))
  const forwardIn = new Map<string, string[]>(ids.map((id) => [id, []]))
  for (const id of ids) {
    for (const { to } of successors.get(id)!) {
      if (backEdges.has(pairKey(id, to))) continue
      forwardOut.get(id)!.push(to)
      forwardIn.get(to)!.push(id)
    }
  }

  const layer = new Map<string, number>(ids.map((id) => [id, 0]))
  const pending = new Map<string, number>(ids.map((id) => [id, forwardIn.get(id)!.length]))
  const queue = ids.filter((id) => pending.get(id) === 0)
  for (let head = 0; head < queue.length; head += 1) {
    const id = queue[head]
    for (const next of forwardOut.get(id)!) {
      layer.set(next, Math.max(layer.get(next)!, layer.get(id)! + 1))
      const left = pending.get(next)! - 1
      pending.set(next, left)
      if (left === 0) queue.push(next)
    }
  }

  const layerCount = Math.max(...ids.map((id) => layer.get(id)!)) + 1
  const columns: string[][] = Array.from({ length: layerCount }, () => [])
  for (const id of ids) columns[layer.get(id)!].push(id)
  const position = new Map<string, number>()
  const refreshPositions = () => {
    for (const column of columns) column.forEach((id, index) => position.set(id, index))
  }
  refreshPositions()

  // Barycenter passes: pull each node next to the rows of the neighbours it is
  // connected to, which keeps branches from crossing each other.
  for (let pass = 0; pass < ORDER_PASSES; pass += 1) {
    const downward = pass % 2 === 0
    const indexes = columns.map((_, index) => index)
    for (const li of downward ? indexes : indexes.reverse()) {
      const column = columns[li]
      const neighbours = downward ? forwardIn : forwardOut
      const barycenter = new Map<string, number>()
      column.forEach((id, index) => {
        const linked = neighbours.get(id)!.filter((other) => layer.get(other) !== li)
        const mean = linked.length > 0
          ? linked.reduce((sum, other) => sum + position.get(other)!, 0) / linked.length
          : index
        barycenter.set(id, mean)
      })
      column.sort(
        (a, b) => barycenter.get(a)! - barycenter.get(b)!
          || parts.get(b)!.weight - parts.get(a)!.weight
          || a.localeCompare(b),
      )
      refreshPositions()
    }
  }

  // A chain of single-node columns would otherwise sit on one flat line, which
  // hides where the flow branches. Offset each column vertically, damped by how
  // many nodes it already stacks.
  const rawWaves = columns.map((column, li) => Math.sin(li * 0.95 + 0.45) * WAVE_AMPLITUDE / Math.sqrt(column.length))
  // Recentre the wave, otherwise a run of same-sign offsets pushes the whole
  // graph off the canvas centre and wastes the opposite half.
  const waveMid = (Math.max(...rawWaves) + Math.min(...rawWaves)) / 2
  const waves = rawWaves.map((wave) => wave - waveMid)
  const waveSpan = Math.max(...waves) - Math.min(...waves)
  const rows = Math.max(...columns.map((column) => column.length))
  const width = Math.max(MIN_GRAPH_WIDTH, MARGIN_X * 2 + (layerCount - 1) * COLUMN_WIDTH)
  const height = Math.max(MIN_GRAPH_HEIGHT, MARGIN_Y * 2 + (rows - 1) * ROW_HEIGHT + waveSpan)
  const spacing = layerCount > 1 ? (width - MARGIN_X * 2) / (layerCount - 1) : width - MARGIN_X * 2
  const columnX = columns.map((_, index) => (layerCount > 1 ? MARGIN_X + index * spacing : width / 2))
  const labelChars = Math.min(52, Math.max(18, Math.floor((spacing - 20) / 5.6)))

  const maxWeight = Math.max(1, ...ids.map((id) => parts.get(id)!.weight))
  const nodes: GraphNode[] = []
  columns.forEach((column, li) => {
    column.forEach((id, index) => {
      const value = parts.get(id)!
      nodes.push({
        id,
        method: value.method,
        route: value.route,
        weight: value.weight,
        radius: 10 + 9 * (Math.log1p(value.weight) / Math.log1p(maxWeight)),
        layer: li,
        x: columnX[li],
        y: height / 2 + waves[li] + (index - (column.length - 1) / 2) * ROW_HEIGHT,
      })
    })
  })

  const nodeByID = new Map(nodes.map((node) => [node.id, node]))
  const maxTransitions = Math.max(1, ...edges.map((edge) => edge.transitions))
  let backIndex = 0
  const graphEdges = edges.map((edge) => {
    const source = nodeByID.get(nodeID(edge.from_method, edge.from_route))!
    const target = nodeByID.get(nodeID(edge.to_method, edge.to_route))!
    const kind: EdgeKind = source.id === target.id
      ? 'self'
      : target.layer > source.layer
        ? 'forward'
        : target.layer === source.layer
          ? 'sibling'
          : 'back'
    const path = kind === 'self'
      ? selfPath(source)
      : kind === 'forward'
        ? forwardPath(source, target)
        : kind === 'sibling'
          ? siblingPath(source, target)
          : backPath(source, target, height, backIndex++)
    return {
      edge,
      source,
      target,
      width: 0.8 + 5.2 * (Math.log1p(edge.transitions) / Math.log1p(maxTransitions)),
      path,
      kind,
    }
  })

  return { nodes, edges: graphEdges, width, height, columnX, labelChars, chronological }
}

function selfPath(node: GraphNode) {
  const r = node.radius
  return `M ${node.x + r * 0.55} ${node.y - r * 0.8} C ${node.x + r * 3.4} ${node.y - r * 3.8}, ${node.x - r * 3.4} ${node.y - r * 3.8}, ${node.x - r * 0.55} ${node.y - r * 0.8}`
}

function forwardPath(source: GraphNode, target: GraphNode) {
  const startX = source.x + source.radius
  const endX = target.x - (target.radius + 6)
  const bend = Math.max(26, (endX - startX) * 0.45)
  return `M ${startX} ${source.y} C ${startX + bend} ${source.y}, ${endX - bend} ${target.y}, ${endX} ${target.y}`
}

function siblingPath(source: GraphNode, target: GraphNode) {
  const downward = target.y >= source.y
  const startY = source.y + (downward ? source.radius : -source.radius)
  const endY = target.y + (downward ? -(target.radius + 6) : target.radius + 6)
  const bend = 46
  return `M ${source.x} ${startY} C ${source.x + bend} ${startY}, ${target.x + bend} ${endY}, ${target.x} ${endY}`
}

function backPath(source: GraphNode, target: GraphNode, height: number, index: number) {
  const startX = source.x - source.radius
  const endX = target.x + (target.radius + 6)
  const midY = (source.y + target.y) / 2
  const bow = (52 + (index % 3) * 18) * (midY > height / 2 ? 1 : -1)
  return `M ${startX} ${source.y} C ${startX - 70} ${source.y + bow}, ${endX + 70} ${target.y + bow}, ${endX} ${target.y}`
}

export function TransitionNodeGraph({
  edges,
  timelineNodes,
  alpRows,
  title = 'API 遷移ノードグラフ',
}: {
  edges: UserTransitionEdge[]
  timelineNodes?: UserScenarioNode[]
  alpRows: AlpRow[]
  title?: string
}) {
  const [hoveredNode, setHoveredNode] = useState<string | null>(null)
  const [selectedNode, setSelectedNode] = useState<string | null>(null)
  const markerID = `transition-arrow-${useId().replaceAll(':', '')}`
  const graph = useMemo(() => layoutGraph(edges, timelineNodes), [edges, timelineNodes])
  const latencies = useMemo(() => new Map(graph.nodes.map((node) => [
    node.id, apiLatency(alpRows, node.method, node.route),
  ])), [graph.nodes, alpRows])
  const selectedVisible = selectedNode && graph.nodes.some((node) => node.id === selectedNode) ? selectedNode : null
  const activeNode = selectedVisible ?? hoveredNode
  const activeDetails = useMemo(() => {
    if (!selectedVisible) return null
    let inbound = 0
    let outbound = 0
    let inboundSessions = 0
    let outboundSessions = 0
    for (const edge of edges) {
      if (nodeID(edge.from_method, edge.from_route) === selectedVisible) {
        outbound += edge.transitions
        outboundSessions += edge.sessions
      }
      if (nodeID(edge.to_method, edge.to_route) === selectedVisible) {
        inbound += edge.transitions
        inboundSessions += edge.sessions
      }
    }
    return { inbound, outbound, inboundSessions, outboundSessions }
  }, [edges, selectedVisible])

  return (
    <div className="space-y-3">
      <div>
        <h3 className="font-semibold">{title}</h3>
        <div className="text-xs text-base-content/60">
          {graph.chronological
            ? 'セッション内で最初に現れた時刻の順に左から右へ並べています。'
            : '遷移の依存順に左から右へ並べています。'}
          矢印の太さは遷移件数、ノードの大きさは入出力件数です（{graph.nodes.length} ノード / {graph.edges.length} 辺）
        </div>
        <div className="text-xs text-base-content/60">
          ノード下はAPI応答時間の平均 / p99（ms）。同じRUNのalp集計で、Cookieなし・エラー応答を含むRUN全体の値です。
          シナリオ別の値ではありません。対応する計測がないAPIは「未計測」と表示します。
        </div>
      </div>

      <div className="overflow-x-auto rounded-box border border-base-300 bg-base-200/50">
        <svg
          viewBox={`0 0 ${graph.width} ${graph.height}`}
          className="h-auto min-w-[920px] text-base-content"
          role="img"
          aria-label={`API遷移ノードグラフ。左から右へ${graph.columnX.length}段、${graph.nodes.length}ノード、${graph.edges.length}辺`}
          onClick={() => setSelectedNode(null)}
        >
          <defs>
            <marker id={markerID} markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto" markerUnits="strokeWidth">
              <path d="M 0 0 L 8 4 L 0 8 z" fill="context-stroke" />
            </marker>
          </defs>
          <g stroke="var(--chart-grid)" strokeWidth="1" strokeDasharray="3 7" opacity="0.7">
            {graph.columnX.map((x) => (
              <line key={x} x1={x} y1={MARGIN_Y * 0.42} x2={x} y2={graph.height - MARGIN_Y * 0.42} />
            ))}
          </g>
          <g fill="var(--chart-muted)" fontSize="10" textAnchor="middle">
            {graph.columnX.map((x, index) => (
              <text key={x} x={x} y={MARGIN_Y * 0.42 - 6}>{`${index + 1} 段目`}</text>
            ))}
          </g>
          <g fill="none">
            {graph.edges.map((item) => {
              const related = !activeNode || item.source.id === activeNode || item.target.id === activeNode
              const errors = item.edge.to_status_4xx + item.edge.to_status_5xx
              const returning = item.kind === 'back'
              return (
                <path
                  key={pairKey(item.source.id, item.target.id)}
                  d={item.path}
                  stroke={activeNode && related ? 'var(--series-1)' : errors > 0 ? 'var(--status-warning)' : 'var(--chart-muted)'}
                  strokeWidth={item.width}
                  strokeOpacity={related ? (returning ? 0.5 : 0.7) : 0.08}
                  strokeDasharray={returning ? '7 5' : undefined}
                  markerEnd={`url(#${markerID})`}
                  vectorEffect="non-scaling-stroke"
                >
                  <title>
                    {`${item.source.id} → ${item.target.id}${returning ? ' (前段への戻り)' : ''}\n${item.edge.transitions.toLocaleString()}遷移 / ${item.edge.sessions.toLocaleString()}セッション\n開始間隔 p50 ${item.edge.start_gap_p50_ms.toFixed(1)}ms / p95 ${item.edge.start_gap_p95_ms.toFixed(1)}ms\n重複 ${item.edge.overlap_transitions.toLocaleString()} / 順序曖昧 ${item.edge.ambiguous_order_transitions.toLocaleString()} / 次が4xx・5xx ${errors.toLocaleString()}`}
                  </title>
                </path>
              )
            })}
          </g>
          <g>
            {graph.nodes.map((node) => {
              const connected = !activeNode || node.id === activeNode || graph.edges.some(
                (edge) => (edge.source.id === activeNode && edge.target.id === node.id) || (edge.target.id === activeNode && edge.source.id === node.id),
              )
              const isActive = node.id === activeNode
              const latency = latencies.get(node.id)
              return (
                <g
                  key={node.id}
                  transform={`translate(${node.x} ${node.y})`}
                  opacity={connected ? 1 : 0.18}
                  className="cursor-pointer outline-none"
                  role="button"
                  tabIndex={0}
                  aria-label={`${node.id}、${latencyLabel(latency)}、${node.layer + 1}段目、入出力延べ${node.weight.toLocaleString()}件`}
                  onClick={(event) => {
                    event.stopPropagation()
                    setSelectedNode((current) => current === node.id ? null : node.id)
                  }}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault()
                      setSelectedNode((current) => current === node.id ? null : node.id)
                    }
                  }}
                  onMouseEnter={() => setHoveredNode(node.id)}
                  onMouseLeave={() => setHoveredNode(null)}
                >
                  <circle
                    r={node.radius + (isActive ? 4 : 0)}
                    fill={methodColor(node.method)}
                    stroke={isActive ? 'var(--chart-tooltip-text)' : 'var(--chart-tooltip-bg)'}
                    strokeWidth={isActive ? 3 : 1.5}
                    vectorEffect="non-scaling-stroke"
                  />
                  <text
                    y={node.radius + 14}
                    textAnchor="middle"
                    fontSize="9.5"
                    fontWeight="700"
                    letterSpacing="0.04em"
                    fill="currentColor"
                  >
                    {node.method}
                  </text>
                  <text
                    y={node.radius + 26}
                    textAnchor="middle"
                    fontSize="10"
                    fontWeight={isActive ? 700 : 400}
                    fill="currentColor"
                  >
                    {truncateLabel(shortRoute(node.route), graph.labelChars)}
                  </text>
                  <text
                    y={node.radius + 41}
                    textAnchor="middle"
                    fontSize="10"
                    fontWeight="600"
                    fill="currentColor"
                    stroke="var(--color-base-200)"
                    strokeWidth="3"
                    paintOrder="stroke"
                  >
                    {latencyLabel(latency)}
                  </text>
                  <title>{`${node.id}\n${latencyLabel(latency)}\n${node.layer + 1}段目\n入出力延べ ${node.weight.toLocaleString()}件\nクリックで固定`}</title>
                </g>
              )
            })}
          </g>
        </svg>
      </div>

      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-base-content/70">
        {['GET', 'POST', 'PUT/PATCH', 'DELETE', 'OTHER'].map((method) => (
          <span key={method} className="inline-flex items-center gap-1.5">
            <span className="size-2.5 rounded-full" style={{ backgroundColor: methodColor(method === 'PUT/PATCH' ? 'PUT' : method) }} />
            {method}
          </span>
        ))}
        <span>破線: 前の段へ戻る遷移</span>
        <span className="ml-auto">橙色の矢印: 次のAPIに4xx/5xxを含む</span>
      </div>

      {selectedVisible && activeDetails && (
        <div className="alert alert-soft alert-info">
          <div className="w-full">
            <div className="break-all font-mono font-medium">{selectedVisible}</div>
            <div className="mt-1 text-sm tabular-nums">
              応答時間（RUN全体）: {latencyLabel(latencies.get(selectedVisible))}
              {' / '}p90 {milliseconds(latencies.get(selectedVisible)?.p90)}
              {' / '}最大 {milliseconds(latencies.get(selectedVisible)?.max)}
              {' / '}リクエスト数 {latencies.get(selectedVisible)?.count.toLocaleString() ?? '未計測'}
            </div>
            <div className="mt-1 flex flex-wrap gap-x-5 gap-y-1 text-sm">
              <span>流入 {activeDetails.inbound.toLocaleString()} 遷移</span>
              <span>流出 {activeDetails.outbound.toLocaleString()} 遷移</span>
              <span>流入セッション延べ {activeDetails.inboundSessions.toLocaleString()}</span>
              <span>流出セッション延べ {activeDetails.outboundSessions.toLocaleString()}</span>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

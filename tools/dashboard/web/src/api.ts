export type RunInfo = {
  run_id: string
  has_alp: boolean
  has_slowquery: boolean
  has_metrics: boolean
  has_fgprof: boolean
  has_pprof: boolean
}

export type ScoreEntry = {
  run_id: string
  score: number | null
  app: string
  nginx: string
  mysql: string
}

export type AlpRow = {
  count: number
  method: string
  uri: string
  '2xx': number
  '3xx': number
  '4xx': number
  '5xx': number
  max: number
  avg: number
  sum: number
  p90: number
  p99: number
  sum_body?: number
  avg_body?: number
}

export type AlpResponse = {
  run_id: string
  available: boolean
  rows: AlpRow[]
}

export type SlowQueryClass = {
  query: string
  query_count: number

  query_time_sum: number
  query_time_avg: number
  query_time_max: number
  query_time_min: number
  query_time_p95: number
  query_time_pct: number

  lock_time_sum: number
  lock_time_avg: number
  lock_time_max: number

  rows_examined_sum: number
  rows_examined_avg: number
  rows_examined_max: number

  rows_sent_sum: number
  rows_sent_avg: number
}

export type SlowQueryResponse = {
  run_id: string
  available: boolean
  total_query_count: number
  unique_query_count: number
  total_query_time_sum: number
  total_lock_time_sum: number
  total_rows_examined_sum: number
  total_rows_sent_sum: number
  classes: SlowQueryClass[]
  total_classes: number
  truncated: boolean
}

export type HostPoint = {
  elapsed_ms: number
  cpu_busy_pct: number
  cpu_user_pct: number
  cpu_system_pct: number
  cpu_iowait_pct: number
  cpu_steal_pct: number
  mem_used_pct: number
  mem_available_bytes: number
  buffers_bytes: number
  cached_bytes: number
  swap_used_bytes: number
  swap_in_bytes_per_sec: number
  swap_out_bytes_per_sec: number
  load1: number
  load5: number
  load15: number
  procs_running: number
  procs_blocked: number
  context_switches_per_sec: number
  interrupts_per_sec: number
  disk_read_bytes_per_sec: number
  disk_write_bytes_per_sec: number
  disk_io_in_progress: number
  disk_io_time_millis_per_sec: number
  net_rx_bytes_per_sec: number
  net_tx_bytes_per_sec: number
  net_rx_packets_per_sec: number
  net_tx_packets_per_sec: number
  net_rx_errors_per_sec: number
  net_tx_errors_per_sec: number
  net_rx_drops_per_sec: number
  net_tx_drops_per_sec: number
  cpu_pressure_some_avg10: number
  memory_pressure_some_avg10: number
  memory_pressure_full_avg10: number
  io_pressure_some_avg10: number
  io_pressure_full_avg10: number
}

export type ServicePoint = {
  elapsed_ms: number
  available: boolean
  cpu_pct: number
  cpu_host_pct: number
  cpu_user_pct: number
  cpu_system_pct: number
  memory_current_bytes: number
  memory_pct_of_host: number
  memory_peak_bytes: number
  io_read_bytes_per_sec: number
  io_write_bytes_per_sec: number
  tasks_current: number
}

export type HostMetrics = {
  host: string
  series: HostPoint[]
}

export type ServiceMetrics = {
  host: string
  service: string
  series: ServicePoint[]
}

export type MetricsResponse = {
  run_id: string
  hosts: HostMetrics[]
  services: ServiceMetrics[]
}

export type FgprofFunction = {
  name: string
  flat_ms: number
  flat_pct: number
  cum_ms: number
  cum_pct: number
}

export type FgprofProfile = {
  host: string
  source: string
  sample_type: string
  sample_unit: string
  duration_sec: number
  total_ms: number
  functions: FgprofFunction[]
}

export type FgprofResponse = {
  run_id: string
  available: boolean
  profiles: FgprofProfile[]
}

export type GoPprofKind = 'cpu' | 'heap' | 'allocs' | 'goroutine'

export type GoPprofFunction = {
  name: string
  flat: number
  flat_pct: number
  cum: number
  cum_pct: number
}

export type GoPprofProfile = {
  kind: GoPprofKind
  host: string
  source: string
  sample_type: string
  sample_unit: string
  duration_sec: number
  total: number
  functions: GoPprofFunction[]
}

export type GoPprofResponse = {
  run_id: string
  available: boolean
  profiles: GoPprofProfile[]
}

export type MysqlPoint = {
  elapsed_ms: number
  threads_connected: number
  threads_running: number
  threads_cached: number
  threads_created_per_sec: number
  connections_per_sec: number
  aborted_connects_per_sec: number
  questions_per_sec: number
  com_select_per_sec: number
  com_insert_per_sec: number
  com_update_per_sec: number
  com_delete_per_sec: number
  bytes_received_per_sec: number
  bytes_sent_per_sec: number
  buffer_pool_pages_total: number
  buffer_pool_pages_free: number
  buffer_pool_pages_dirty: number
  buffer_pool_read_requests_per_sec: number
  buffer_pool_reads_per_sec: number
  buffer_pool_hit_pct: number
  row_lock_current_waits: number
  row_lock_waits_per_sec: number
  row_lock_time_ms_per_sec: number
  innodb_log_waits_per_sec: number
  created_tmp_tables_per_sec: number
  created_tmp_disk_tables_per_sec: number
  tmp_disk_ratio_pct: number
}

export type MysqlResponse = {
  run_id: string
  available: boolean
  series: MysqlPoint[]
}

export type UpstreamRow = {
  upstream_addr: string
  upstream_status: string
  cache_status: string
  requests: number
  status_2xx: number
  status_3xx: number
  status_4xx: number
  status_5xx: number
  status_other: number
  response_time_sum_ms: number
  response_time_avg_ms: number
  upstream_time_sum_ms: number
  upstream_time_avg_ms: number
}

export type UpstreamResponse = {
  run_id: string
  available: boolean
  rows: UpstreamRow[]
}

export type UserTransitionSummary = {
  input_files: number
  input_lines: number
  malformed_lines: number
  invalid_time_lines: number
  api_requests: number
  classified_requests: number
  unmatched_api_requests: number
  requests_with_identity: number
  missing_identity_requests: number
  sessions: number
  transitions: number
  overlapping_transitions: number
  ambiguous_order_transitions: number
  window_start_unix_ms: number
  window_end_unix_ms: number
  scenario_groups: number
  scenarios_emitted: number
  scenario_sessions_omitted: number
}

export type UserTransitionEdge = {
  from_method: string
  from_route: string
  to_method: string
  to_route: string
  transitions: number
  sessions: number
  overlap_transitions: number
  ambiguous_order_transitions: number
  start_gap_avg_ms: number
  start_gap_p50_ms: number
  start_gap_p95_ms: number
  idle_gap_avg_ms: number
  from_status_2xx: number
  from_status_3xx: number
  from_status_4xx: number
  from_status_5xx: number
  from_status_other: number
  to_status_2xx: number
  to_status_3xx: number
  to_status_4xx: number
  to_status_5xx: number
  to_status_other: number
}

export type UserTransitionsResponse = {
  run_id: string
  available: boolean
  schema_version: number
  identity_field: string
  ordering: string
  scenario_grouping: string
  summary: UserTransitionSummary
  edges: UserTransitionEdge[]
  scenarios: UserScenario[]
}

export type UserScenarioNode = {
  method: string
  route: string
  requests: number
  sessions: number
  first_sessions: number
  last_sessions: number
  first_position_avg: number
  first_offset_avg_ms: number
}

export type UserScenario = {
  id: string
  signature: string[]
  sessions: number
  requests: number
  transitions: number
  overlap_transitions: number
  ambiguous_order_transitions: number
  requests_per_session_avg: number
  duration_p50_ms: number
  duration_p95_ms: number
  nodes: UserScenarioNode[]
  edges: UserTransitionEdge[]
}

export type TimelineBucket = {
  elapsed_s: number
  requests: number
  status_2xx: number
  status_3xx: number
  status_4xx: number
  status_5xx: number
  avg_response_ms: number
  p95_response_ms: number
  bytes_per_sec: number
  scenario_warnings: number
}

export type TimelineHostPoint = {
  elapsed_s: number
  cpu_busy_pct: number
  load1: number
  mem_used_pct: number
}

export type TimelineHost = {
  host: string
  series: TimelineHostPoint[]
}

export type TimelineResponse = {
  run_id: string
  available: boolean
  start_time: string
  buckets: TimelineBucket[]
  hosts: TimelineHost[]
}

export type BacklogCard = {
  id: string
  status: string
  title: string
  closed: boolean
  priority: string
  owner: string
  area: string
  updated: string
  updated_by: string
}

export type BacklogResponse = {
  counts: Record<string, number>
  cards: BacklogCard[]
}

export type BacklogSection = {
  name: string
  position: number
  body: string
}

export type BacklogHistoryEntry = {
  position: number
  occurred_at: string
  actor: string
  body: string
}

export type BacklogDependency = {
  card_id: string
  depends_on_card_id: string
  required_status: string
  mode: string
  reason: string
  status: string
}

export type BacklogObjectiveRelation = {
  id: string
  status: string
  mode: string
  title: string
  rationale: string
}

export type BacklogConstraintRelation = {
  id: string
  status: string
  title: string
  role: string
  rationale: string
}

export type BacklogCardDetail = {
  id: string
  status: string
  title: string
  closed: boolean
  priority: string
  owner: string
  area: string
  source_runs: string
  compare_run: string
  observed_runs: string
  updated: string
  updated_by: string
  sections: BacklogSection[]
  history: BacklogHistoryEntry[]
  dependencies: BacklogDependency[]
  unblocks: BacklogDependency[]
  objectives: BacklogObjectiveRelation[]
  constraints: BacklogConstraintRelation[]
}

export type ReportInfo = {
  /** path relative to docs/reports, e.g. "isucon-analyze/20260828-235318-r2.md" */
  filename: string
  /** directory the report is filed under (the report type), "" for the top level */
  kind: string
  title: string
  mod_time: string
  size: number
}

export type ReportContent = {
  filename: string
  content: string
}

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error ?? `${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export const api = {
  runs: () => getJSON<RunInfo[]>('/api/runs'),
  scores: () => getJSON<ScoreEntry[]>('/api/scores'),
  alp: (runId: string) => getJSON<AlpResponse>(`/api/runs/${encodeURIComponent(runId)}/alp`),
  slowQuery: (runId: string) =>
    getJSON<SlowQueryResponse>(`/api/runs/${encodeURIComponent(runId)}/slowquery`),
  metrics: (runId: string) =>
    getJSON<MetricsResponse>(`/api/runs/${encodeURIComponent(runId)}/metrics`),
  fgprof: (runId: string) =>
    getJSON<FgprofResponse>(`/api/runs/${encodeURIComponent(runId)}/fgprof`),
  fgprofGraph: (runId: string, host: string) =>
    `/api/runs/${encodeURIComponent(runId)}/fgprof/${encodeURIComponent(host)}/graph.svg`,
  pprof: (runId: string) =>
    getJSON<GoPprofResponse>(`/api/runs/${encodeURIComponent(runId)}/pprof`),
  pprofGraph: (runId: string, kind: GoPprofKind, host: string) =>
    `/api/runs/${encodeURIComponent(runId)}/pprof/${encodeURIComponent(kind)}/${encodeURIComponent(host)}/graph.svg`,
  timeline: (runId: string) =>
    getJSON<TimelineResponse>(`/api/runs/${encodeURIComponent(runId)}/timeline`),
  mysql: (runId: string) => getJSON<MysqlResponse>(`/api/runs/${encodeURIComponent(runId)}/mysql`),
  upstream: (runId: string) =>
    getJSON<UpstreamResponse>(`/api/runs/${encodeURIComponent(runId)}/upstream`),
  userTransitions: (runId: string) =>
    getJSON<UserTransitionsResponse>(`/api/runs/${encodeURIComponent(runId)}/user-transitions`),
  backlog: (includeClosed = false) =>
    getJSON<BacklogResponse>(`/api/backlog${includeClosed ? '?all=1' : ''}`),
  backlogDetail: (id: string) => getJSON<BacklogCardDetail>(`/api/backlog/${encodeURIComponent(id)}`),
  reports: () => getJSON<ReportInfo[]>('/api/reports'),
  reportContent: (filename: string) =>
    // reports live in per-type subdirectories, so encode each path segment
    // and keep the separators.
    getJSON<ReportContent>(
      `/api/reports/${filename.split('/').map(encodeURIComponent).join('/')}`,
    ),
}

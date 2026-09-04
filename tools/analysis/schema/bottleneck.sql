-- Bounded, reusable evidence views for single-RUN bottleneck discovery.
-- These views select and reconcile observations; they deliberately do not
-- decide the bottleneck, classify benchmark failures, or propose changes.

create or replace view bottleneck_run_health as
with artifact_rollup as (
    select
        run_id,
        count(*) as artifact_count,
        count(*) filter (where artifact_status <> 'ok') as non_ok_artifacts,
        count(*) filter (where periodic_expected and quality_status <> 'valid') as invalid_periodic_artifacts,
        string_agg(artifact_name || '=' || artifact_status, ', ' order by artifact_name)
            filter (where artifact_status <> 'ok') as artifact_failures
    from measurement_quality
    group by run_id
), applied as (
    select
        run_id,
        string_agg(card_id, ',' order by card_id) as applied_cards
    from run_applied_cards
    group by run_id
)
select
    m.run_id,
    m.score,
    m.passed,
    b.final_check_succeeded,
    w.started_at as load_started_at,
    w.ended_at as load_ended_at,
    w.duration_seconds as load_window_seconds,
    w.status as load_window_status,
    coalesce(a.artifact_count, 0) as artifact_count,
    coalesce(a.non_ok_artifacts, 0) as non_ok_artifacts,
    coalesce(a.invalid_periodic_artifacts, 0) as invalid_periodic_artifacts,
    coalesce(a.artifact_failures, '') as artifact_failures,
    m.commit,
    m.dirty,
    coalesce(p.applied_cards, '') as applied_cards,
    m.app_hosts,
    m.app_traffic_hosts,
    m.nginx_hosts,
    m.mysql_host
from manifests m
join load_windows w using (run_id)
left join bench_summaries b using (run_id)
left join artifact_rollup a using (run_id)
left join applied p using (run_id);

create or replace view bottleneck_bench_summary as
with scenarios as (
    select
        run_id,
        sum(successes) as scenario_successes,
        sum(failures) as scenario_failures
    from bench_scenarios
    group by run_id
)
select
    b.*,
    coalesce(s.scenario_successes, 0) as scenario_successes,
    coalesce(s.scenario_failures, 0) as scenario_failures
from bench_summaries b
left join scenarios s using (run_id);

create or replace view bottleneck_capacity_summary as
with hosts as (
    select
        d.*,
        row_number() over (partition by d.run_id order by d.utilization_ratio desc, d.host) as busy_rank
    from resource_demand d
    where d.resource_kind = 'host'
)
select
    c.run_id,
    c.capacity_core_seconds,
    c.host_demand_core_seconds,
    c.idle_core_seconds,
    c.utilization_ratio,
    c.service_demand_core_seconds,
    c.unaccounted_core_seconds,
    h.host as busiest_host,
    h.demand_core_seconds as busiest_host_demand_core_seconds,
    h.capacity_core_seconds as busiest_host_capacity_core_seconds,
    h.utilization_ratio as busiest_host_utilization_ratio,
    s.idle_min_pct as busiest_host_idle_min_pct,
    s.idle_mean_pct as busiest_host_idle_mean_pct,
    s.run_queue_over_cpu_samples as busiest_host_run_queue_over_cpu_samples,
    s.blocked_samples as busiest_host_blocked_samples,
    s.cpu_pressure_some_avg10_mean as busiest_host_cpu_pressure_some_avg10_mean,
    c.load_window_seconds
from capacity_ledger c
join hosts h on h.run_id = c.run_id and h.busy_rank = 1
left join host_saturation s on s.run_id = c.run_id and s.host = h.host;

create or replace view bottleneck_wait_axes as
select
    run_id,
    'proc' as source,
    host,
    cast(null as varchar) as entity,
    'run_queue_over_cpu_samples' as metric,
    run_queue_over_cpu_samples::double as value,
    'samples' as unit,
    samples::double as denominator,
    load_window_seconds
from host_saturation
union all
select run_id, 'proc', host, null, 'blocked_samples', blocked_samples::double,
       'samples', samples::double, load_window_seconds
from host_saturation
union all
select run_id, 'proc', host, null, 'cpu_pressure_some_avg10_mean', cpu_pressure_some_avg10_mean,
       'pct', samples::double, load_window_seconds
from host_saturation
union all
select run_id, 'proc', host, null, 'io_pressure_some_avg10_mean', io_pressure_some_avg10_mean,
       'pct', samples::double, load_window_seconds
from host_saturation
union all
select run_id, source, host, entity, metric,
       coalesce(window_total, mean_value) as value,
       case when window_total is not null then 'per-window' else 'gauge' end as unit,
       samples::double as denominator,
       load_window_seconds
from window_metrics
where (source = 'mysql' and metric in (
           'threads_running', 'row_lock_current_waits', 'row_lock_waits_per_sec',
           'row_lock_time_ms_per_sec', 'innodb_log_waits_per_sec'))
   or (source = 'disk' and metric in ('avg_queue_size', 'io_util_pct', 'read_await_ms', 'write_await_ms'));

create or replace view bottleneck_endpoint_ranking as
select
    e.run_id,
    row_number() over (partition by e.run_id order by e.response_time_sum_seconds desc, e.requests desc) as response_time_rank,
    e.method,
    e.route,
    e.requests,
    e.response_time_sum_seconds,
    e.response_time_avg_ms,
    e.response_time_max_ms,
    e.p90_ms,
    e.p99_ms,
    e.sum_body_bytes,
    e.avg_body_bytes,
    e.status_2xx,
    e.status_3xx,
    e.status_4xx,
    e.status_5xx,
    e.source_artifact
from endpoint_cost e;

create or replace view bottleneck_consumers as
select
    run_id,
    host,
    resource_kind,
    service,
    demand_core_seconds,
    capacity_core_seconds,
    utilization_ratio,
    cpu_busy_pct,
    cpu_pressure_some_avg10,
    load_window_seconds,
    row_number() over (partition by run_id, resource_kind order by demand_core_seconds desc, host, service) as demand_rank
from resource_demand;

create or replace view bottleneck_state_audit as
select
    run_id,
    source,
    host,
    entity,
    metric,
    samples,
    mean_value,
    min_value,
    max_value,
    window_total,
    load_window_seconds
from window_metrics
where (source = 'mysql' and metric in (
           'threads_connected', 'threads_running', 'threads_created_per_sec',
           'connections_per_sec', 'questions_per_sec', 'com_select_per_sec',
           'com_insert_per_sec', 'com_update_per_sec', 'com_delete_per_sec',
           'created_tmp_tables_per_sec', 'created_tmp_disk_tables_per_sec',
           'tmp_disk_ratio_pct', 'buffer_pool_hit_pct', 'row_lock_current_waits',
           'row_lock_waits_per_sec', 'row_lock_time_ms_per_sec',
           'innodb_log_waits_per_sec', 'aborted_connects_per_sec',
           'bytes_received_per_sec', 'bytes_sent_per_sec'))
   or (source = 'disk' and metric in (
           'avg_queue_size', 'io_util_pct', 'io_in_progress',
           'read_bytes_per_sec', 'write_bytes_per_sec',
           'read_iops', 'write_iops', 'read_await_ms', 'write_await_ms'));

create or replace view bottleneck_serialization_screen as
with run_ids as (
    select run_id from manifests
), mysql_values as (
    select
        run_id,
        max(window_total) filter (where metric = 'connections_per_sec') as connections,
        max(window_total) filter (where metric = 'threads_created_per_sec') as threads_created,
        max(window_total) filter (where metric = 'questions_per_sec') as questions
    from window_metrics
    where source = 'mysql'
    group by run_id
), request_values as (
    select
        run_id,
        sum(requests) as requests
    from endpoint_cost
    group by run_id
), query_values as (
    select run_id, sum(calls) as query_calls
    from query_cost
    where source = 'slp'
    group by run_id
), upstream_values as (
    select
        run_id,
        sum(requests) as upstream_requests,
        max(requests) as busiest_upstream_requests
    from upstreams
    where upstream_addr <> 'NONE'
    group by run_id
)
select r.run_id, 'mysql_connections_per_question' as screen,
       m.connections / nullif(m.questions, 0) as value, 'ratio' as unit,
       m.connections as numerator, m.questions as denominator,
       'mysql window totals' as calculation
from run_ids r left join mysql_values m using (run_id)
union all
select r.run_id, 'mysql_threads_created_per_question',
       m.threads_created / nullif(m.questions, 0), 'ratio',
       m.threads_created, m.questions, 'mysql window totals'
from run_ids r left join mysql_values m using (run_id)
union all
select r.run_id, 'slp_calls_per_request',
       q.query_calls / nullif(v.requests, 0), 'calls/request',
       q.query_calls, v.requests, 'slp.tsv calls / alp requests'
from run_ids r left join query_values q using (run_id) left join request_values v using (run_id)
union all
select r.run_id, 'busiest_upstream_request_share',
       u.busiest_upstream_requests / nullif(u.upstream_requests, 0), 'ratio',
       u.busiest_upstream_requests, u.upstream_requests, 'upstream-breakdown.tsv excluding NONE'
from run_ids r left join upstream_values u using (run_id);

create or replace view bottleneck_profile_metadata as
select
    run_id,
    host,
    source,
    profile_sha256,
    sample_type,
    sample_unit as original_sample_unit,
    duration_seconds,
    total_wall_seconds,
    period_seconds,
    sample_count,
    incomplete_samples,
    embedded_function_names,
    external_binary_used,
    binary_path,
    binary_sha256,
    binary_matches_run,
    incomplete_samples = 0 and embedded_function_names > 0 as symbolization_complete,
    'goroutine wall-clock seconds; concurrent samples overlap and are not CPU/core-seconds' as interpretation
from profile_metadata;

create or replace view bottleneck_profile_functions as
select
    f.run_id,
    f.host,
    f.function,
    f.file,
    f.line,
    f.flat_wall_seconds,
    f.cumulative_wall_seconds,
    f.flat_wall_seconds / nullif(m.total_wall_seconds, 0) as flat_profile_share,
    f.cumulative_wall_seconds / nullif(m.total_wall_seconds, 0) as cumulative_profile_share,
    row_number() over (partition by f.run_id, f.host order by f.cumulative_wall_seconds desc, f.function) as cumulative_rank,
    'wall_seconds' as unit
from profile_functions f
join profile_metadata m using (run_id, host);

create or replace view bottleneck_profile_edges as
select
    e.run_id,
    e.host,
    e.caller,
    e.callee,
    e.wall_seconds,
    e.sample_occurrences,
    e.wall_seconds / nullif(m.total_wall_seconds, 0) as profile_share,
    row_number() over (partition by e.run_id, e.host order by e.wall_seconds desc, e.caller, e.callee) as edge_rank,
    'wall_seconds' as unit
from profile_edges e
join profile_metadata m using (run_id, host);

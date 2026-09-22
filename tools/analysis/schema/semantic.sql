-- Stable analysis semantics over the materialized collector tables. Keep units,
-- scope, and provenance visible so consumers can drill back to raw artifacts.

-- Failed runs and collector-free runs can have no input files for these tables.
-- Preserve an empty relation; artifact status remains the source of availability.
create table if not exists metrics (
    run_id varchar, ts timestamptz, elapsed_ms bigint, host varchar,
    source varchar, entity varchar, metric varchar, value double
);

create table if not exists upstreams (
    run_id varchar,
    upstream_addr varchar,
    upstream_status varchar,
    cache_status varchar,
    requests bigint,
    status_2xx bigint,
    status_3xx bigint,
    status_4xx bigint,
    status_5xx bigint,
    status_other bigint,
    response_time_sum_ms double,
    response_time_avg_ms double,
    upstream_time_sum_ms double,
    upstream_time_avg_ms double
);

-- analysisctl derives this window from ingress request-rate transitions. The
-- manifest and bench.log markers remain the wider collection-window fallback.
create or replace view load_windows as
select
    m.run_id,
    coalesce(a.started_at, m.load_started_at, b.started_at) as started_at,
    coalesce(a.ended_at, m.load_ended_at, b.ended_at) as ended_at,
    coalesce(
        date_diff('millisecond', a.started_at, a.ended_at) / 1000.0,
        m.load_duration_ms / 1000.0,
        date_diff('millisecond', b.started_at, b.ended_at) / 1000.0
    ) as duration_seconds,
    case
        when a.status = 'ok' and a.started_at is not null and a.ended_at > a.started_at then a.source
        when m.load_window_status = 'ok' then m.load_window_source
        when b.started_at is not null and b.ended_at > b.started_at then 'bench.log-backfill'
        else coalesce(m.load_window_source, b.source, 'unavailable')
    end as source,
    case
        when a.status = 'ok'
             and a.started_at is not null
             and a.ended_at > a.started_at then 'ok'
        when m.load_window_status = 'ok'
             and m.load_started_at is not null
             and m.load_ended_at > m.load_started_at then 'ok'
        when b.started_at is not null and b.ended_at > b.started_at then 'ok'
        else 'unavailable'
    end as status,
    case
        when a.status = 'ok' then a.reason
        when m.load_window_status = 'ok'
             and m.load_started_at is not null
             and m.load_ended_at > m.load_started_at then ''
        when b.started_at is not null and b.ended_at > b.started_at then 'derived from historical benchmark log'
        else coalesce(nullif(m.load_window_reason, ''), 'load phase markers are unavailable')
    end as reason
from manifests m
left join bench_phases b using (run_id)
left join analysis_windows a using (run_id);

create or replace view measurement_quality as
with periodic_points as (
    select distinct run_id, source, host, ts
    from metrics
    where source in ('proc', 'service', 'disk', 'mysql', 'sql_pool')
),
window_points as (
    select
        p.*,
        lag(p.ts) over (partition by p.run_id, p.source, p.host order by p.ts) as previous_ts
    from periodic_points p
    join load_windows w using (run_id)
    where w.status = 'ok' and p.ts >= w.started_at and p.ts < w.ended_at
),
periodic_values as (
    select
        m.run_id,
        m.source,
        m.host,
        count(*) filter (where not isfinite(m.value)) as non_finite_values
    from metrics m
    join load_windows w using (run_id)
    where w.status = 'ok'
      and m.ts >= w.started_at and m.ts < w.ended_at
      and m.source in ('proc', 'service', 'disk', 'mysql', 'sql_pool')
    group by all
),
periodic as (
    select
        p.run_id,
        p.source,
        p.host,
        count(*) as in_window_samples,
        coalesce(max(date_diff('millisecond', p.previous_ts, p.ts)), 0) as max_gap_ms,
        coalesce(v.non_finite_values, 0) as non_finite_values
    from window_points p
    left join periodic_values v using (run_id, source, host)
    group by all
),
derived as (
    select
        p.run_id,
        case p.source
            when 'proc' then p.host || '-proc-metrics.tsv'
            when 'service' then p.host || '-service-metrics.tsv'
            when 'disk' then p.host || '-disk-metrics.tsv'
            when 'mysql' then
                case
                    when exists (
                        select 1
                        from artifacts a
                        where a.run_id = p.run_id
                          and a.name = p.host || '-mysql-status.tsv'
                    ) then p.host || '-mysql-status.tsv'
                    else 'mysql-status.tsv'
                end
            when 'sql_pool' then p.host || '-sql-pool-metrics.tsv'
        end as artifact_name,
        p.in_window_samples,
        greatest(1, floor(w.duration_seconds)::bigint) as expected_samples,
        least(100.0, p.in_window_samples * 100.0 / greatest(1, floor(w.duration_seconds))) as window_coverage_pct,
        p.max_gap_ms,
        p.non_finite_values
    from periodic p
    join load_windows w using (run_id)
),
classified as (
    select
        a.*,
        case
            when a.name = 'user-transitions.json' then 'transition'
            -- ends_with keeps the suffix out of a LIKE pattern: measurectl
            -- artifacts -check reads SQL string literals as artifact names, and
            -- a wildcard pattern there looks like an undeclared artifact to it.
            when ends_with(lower(a.name), '.pprof') then 'profile'
            when lower(a.name) = 'mysql-status.tsv'
              or lower(a.name) like '%-mysql-status.tsv'
              or lower(a.name) like '%-proc-metrics.tsv'
              or lower(a.name) like '%-service-metrics.tsv'
              or lower(a.name) like '%-disk-metrics.tsv'
              or lower(a.name) like '%-sql-pool-metrics.tsv' then 'periodic'
            else 'capture'
        end as measurement_kind
    from artifacts a
)
select
    c.run_id,
    c.name as artifact_name,
    c.status as artifact_status,
    c.measurement_kind,
    c.measurement_kind = 'periodic' as periodic_expected,
    coalesce(c.quality_rows, 0) as rows_recorded,
    coalesce(d.in_window_samples, c.quality_in_window_samples, 0) as in_window_samples,
    coalesce(d.expected_samples, c.quality_expected_samples, 0) as expected_samples,
    coalesce(d.window_coverage_pct, c.quality_window_coverage_pct, 0) as window_coverage_pct,
    coalesce(d.max_gap_ms, c.quality_max_gap_ms, 0) as max_gap_ms,
    coalesce(c.quality_monotonic, true) as monotonic,
    coalesce(d.non_finite_values = 0, c.quality_finite, true) as finite,
    case
        when c.status <> 'ok' then 'invalid'
        when c.measurement_kind <> 'periodic' then
            case
                when c.quality_status in ('valid', 'invalid', 'unavailable') then c.quality_status
                else 'artifact-ok'
            end
        when w.status <> 'ok' then 'unavailable'
        when coalesce(d.in_window_samples, c.quality_in_window_samples, 0)
             < greatest(1, coalesce(d.expected_samples, c.quality_expected_samples, 1) - 1) then 'invalid'
        when coalesce(d.max_gap_ms, c.quality_max_gap_ms, 0) > 2500 then 'invalid'
        when not coalesce(c.quality_monotonic, true) then 'invalid'
        when not coalesce(d.non_finite_values = 0, c.quality_finite, true) then 'invalid'
        else 'valid'
    end as quality_status,
    coalesce(nullif(c.quality_reason, ''), nullif(c.reason, ''), '') as reason,
    case when d.artifact_name is not null then 'filtered-metrics+load_windows' else 'run.json' end as quality_source
from classified c
join load_windows w using (run_id)
left join derived d on d.run_id = c.run_id and d.artifact_name = c.name;

create or replace view measurement_quality_periodic as
select * from measurement_quality where measurement_kind = 'periodic';

create or replace view measurement_quality_profile as
select * from measurement_quality where measurement_kind = 'profile';

create or replace view measurement_quality_transition as
select * from measurement_quality where measurement_kind = 'transition';

create or replace view valid_runs as
with artifact_summary as (
    select
        run_id,
        count(*) as artifact_count,
        count(*) filter (where artifact_status <> 'ok') as bad_artifacts,
        count(*) filter (where periodic_expected) as periodic_artifacts,
        count(*) filter (where periodic_expected and quality_status <> 'valid') as bad_periodic_artifacts
    from measurement_quality
    group by run_id
)
select
    m.run_id,
    m.passed,
    w.status as load_window_status,
    coalesce(a.artifact_count, 0) as artifact_count,
    coalesce(a.bad_artifacts, 0) as bad_artifacts,
    coalesce(a.periodic_artifacts, 0) as periodic_artifacts,
    coalesce(a.bad_periodic_artifacts, 0) as bad_periodic_artifacts,
    case
        when m.passed is not true then 'invalid'
        when w.status <> 'ok' then 'invalid'
        when coalesce(a.artifact_count, 0) = 0 then 'invalid'
        when coalesce(a.bad_artifacts, 0) <> 0 then 'invalid'
        when coalesce(a.periodic_artifacts, 0) = 0 then 'invalid'
        when coalesce(a.bad_periodic_artifacts, 0) <> 0 then 'invalid'
        else 'valid'
    end as overall_status,
    concat_ws('; ',
        case when m.passed is not true then 'benchmark did not pass' end,
        case when w.status <> 'ok' then 'load window unavailable' end,
        case when coalesce(a.artifact_count, 0) = 0 then 'no artifacts recorded' end,
        case when coalesce(a.bad_artifacts, 0) <> 0 then a.bad_artifacts || ' artifact(s) are not ok' end,
        case when coalesce(a.periodic_artifacts, 0) = 0 then 'no periodic artifacts recorded' end,
        case when coalesce(a.bad_periodic_artifacts, 0) <> 0 then a.bad_periodic_artifacts || ' periodic artifact(s) failed quality' end
    ) as reasons
from manifests m
join load_windows w using (run_id)
left join artifact_summary a using (run_id);

-- These two standard artifacts are introduced after historical RUNs already
-- exist. Keep typed empty tables so semantic views and incremental sync remain
-- usable until the first new RUN containing the artifacts is imported.
create table if not exists endpoints_by_ingress (
    run_id varchar,
    ingress_host varchar,
    method varchar,
    uri varchar,
    count bigint,
    sum_time_sec double,
    avg_time_sec double,
    max_time_sec double,
    p90_time_sec double,
    p99_time_sec double,
    sum_body_bytes double,
    avg_body_bytes double,
    status_2xx bigint,
    status_3xx bigint,
    status_4xx bigint,
    status_5xx bigint
);

create table if not exists upstreams_by_ingress (
    run_id varchar,
    ingress_host varchar,
    upstream_addr varchar,
    upstream_status varchar,
    cache_status varchar,
    requests bigint,
    status_2xx bigint,
    status_3xx bigint,
    status_4xx bigint,
    status_5xx bigint,
    status_other bigint,
    response_time_sum_ms double,
    response_time_avg_ms double,
    upstream_time_sum_ms double,
    upstream_time_avg_ms double
);

create or replace view endpoint_cost as
select
    e.run_id,
    e.method,
    e.uri as route,
    e.count as requests,
    e.sum_time_sec as response_time_sum_seconds,
    e.avg_time_sec * 1000 as response_time_avg_ms,
    e.max_time_sec * 1000 as response_time_max_ms,
    e.p90_time_sec * 1000 as p90_ms,
    cast(null as double) as p95_ms,
    e.p99_time_sec * 1000 as p99_ms,
    e.sum_body_bytes,
    e.avg_body_bytes,
    e.status_2xx,
    e.status_3xx,
    e.status_4xx,
    e.status_5xx,
    e.sum_time_sec / nullif(sum(e.sum_time_sec) over (partition by e.run_id), 0) as response_time_share,
    w.duration_seconds as load_window_seconds,
    'benchmark-lifecycle' as scope,
    'alp.json' as source_artifact
from endpoints e
left join load_windows w using (run_id);

-- Whole-RUN nginx routing/cache breakdown. Like endpoint_cost, this is an
-- access-log aggregate for the benchmark lifecycle rather than a periodic
-- load-window sample. NONE means that nginx logged no upstream for the group;
-- it does not by itself prove an application response or a cache hit.
create or replace view upstream_cost as
select
    u.run_id,
    u.upstream_addr,
    u.upstream_status,
    u.cache_status,
    u.requests,
    u.status_2xx,
    u.status_3xx,
    u.status_4xx,
    u.status_5xx,
    u.status_other,
    u.response_time_sum_ms / 1000.0 as response_time_sum_seconds,
    u.response_time_avg_ms,
    u.upstream_time_sum_ms / 1000.0 as upstream_time_sum_seconds,
    u.upstream_time_avg_ms,
    u.response_time_sum_ms / nullif(sum(u.response_time_sum_ms) over (partition by u.run_id), 0) as response_time_share,
    u.upstream_time_sum_ms / nullif(sum(u.upstream_time_sum_ms) over (partition by u.run_id), 0) as upstream_time_share,
    w.duration_seconds as load_window_seconds,
    'benchmark-lifecycle' as scope,
    'upstream-breakdown.tsv' as source_artifact
from upstreams u
left join load_windows w using (run_id);

create or replace view endpoint_cost_by_ingress as
select
    e.run_id,
    e.ingress_host,
    e.method,
    e.uri as route,
    e.count as requests,
    e.sum_time_sec as response_time_sum_seconds,
    e.avg_time_sec * 1000 as response_time_avg_ms,
    e.max_time_sec * 1000 as response_time_max_ms,
    e.p90_time_sec * 1000 as p90_ms,
    e.p99_time_sec * 1000 as p99_ms,
    e.sum_body_bytes,
    e.avg_body_bytes,
    e.status_2xx,
    e.status_3xx,
    e.status_4xx,
    e.status_5xx,
    e.sum_time_sec / nullif(sum(e.sum_time_sec) over (partition by e.run_id, e.ingress_host), 0) as ingress_response_time_share,
    w.duration_seconds as load_window_seconds,
    'benchmark-lifecycle' as scope,
    'alp-by-ingress.tsv' as source_artifact
from endpoints_by_ingress e
left join load_windows w using (run_id);

create or replace view upstream_cost_by_ingress as
select
    u.run_id,
    u.ingress_host,
    u.upstream_addr,
    u.upstream_status,
    u.cache_status,
    u.requests,
    u.status_2xx,
    u.status_3xx,
    u.status_4xx,
    u.status_5xx,
    u.status_other,
    u.response_time_sum_ms / 1000.0 as response_time_sum_seconds,
    u.response_time_avg_ms,
    u.upstream_time_sum_ms / 1000.0 as upstream_time_sum_seconds,
    u.upstream_time_avg_ms,
    u.response_time_sum_ms / nullif(sum(u.response_time_sum_ms) over (partition by u.run_id, u.ingress_host), 0) as ingress_response_time_share,
    w.duration_seconds as load_window_seconds,
    'benchmark-lifecycle' as scope,
    'upstream-breakdown-by-ingress.tsv' as source_artifact
from upstreams_by_ingress u
left join load_windows w using (run_id);

create table if not exists http_traffic (
    run_id varchar,
    ingress_host varchar,
    ts timestamptz,
    bucket_seconds integer,
    method varchar,
    route varchar,
    requests bigint,
    status_2xx bigint,
    status_3xx bigint,
    status_4xx bigint,
    status_5xx bigint,
    sum_time_sec double,
    max_time_sec double,
    sum_upstream_time_sec double,
    upstream_time_samples bigint,
    sum_body_bytes double
);

-- endpoint_cost の時系列版。負荷区間外の bucket も含むので in_load_window で絞る。
create or replace view http_traffic_series as
select
    t.run_id,
    t.ingress_host,
    t.ts,
    date_diff('millisecond', w.started_at, t.ts) / 1000.0 as load_offset_seconds,
    coalesce(w.status = 'ok' and t.ts >= w.started_at and t.ts < w.ended_at, false) as in_load_window,
    t.method,
    t.route,
    t.bucket_seconds,
    t.requests,
    t.requests / t.bucket_seconds as requests_per_second,
    t.sum_time_sec as response_time_sum_seconds,
    t.sum_time_sec * 1000 / nullif(t.requests, 0) as response_time_avg_ms,
    t.max_time_sec * 1000 as response_time_max_ms,
    t.sum_upstream_time_sec * 1000 / nullif(t.upstream_time_samples, 0) as upstream_time_avg_ms,
    t.upstream_time_samples,
    t.status_2xx,
    t.status_3xx,
    t.status_4xx,
    t.status_5xx,
    t.sum_body_bytes,
    w.duration_seconds as load_window_seconds,
    'access-log time bucket' as scope,
    'raw/access-<host>.log' as source_artifact
from http_traffic t
left join load_windows w using (run_id);

create or replace view resource_demand as
with in_window as (
    select m.*
    from metrics m
    join load_windows w using (run_id)
    where w.status = 'ok' and m.ts >= w.started_at and m.ts < w.ended_at
),
host_summary as (
    select
        run_id,
        host,
        avg(value) filter (where metric = 'cpu_busy_pct') as cpu_busy_pct,
        sum(value) filter (where metric = 'cpu_busy_pct') as cpu_busy_pct_samples,
        count(*) filter (where metric = 'cpu_busy_pct') as samples,
        max(value) filter (where metric = 'cpu_count') as cpu_count,
        avg(value) filter (where metric = 'cpu_pressure_some_avg10') as cpu_pressure_some_avg10
    from in_window
    where source = 'proc'
    group by run_id, host
),
-- A unit that is not running on this host still emits samples. Counting them
-- would dilute every service mean with zeros, so availability gates the unit.
available_service_samples as (
    select run_id, host, entity, ts
    from in_window
    where source = 'service' and metric = 'available' and value = 1
),
service_summary as (
    select
        m.run_id,
        m.host,
        m.entity as service,
        avg(m.value) filter (where m.metric = 'cpu_pct') as cpu_pct,
        sum(m.value) filter (where m.metric = 'cpu_pct') as cpu_pct_samples,
        count(*) filter (where m.metric = 'cpu_pct') as samples
    from in_window m
    join available_service_samples a using (run_id, host, entity, ts)
    where m.source = 'service'
    group by m.run_id, m.host, m.entity
)
select
    h.run_id,
    h.host,
    'host' as resource_kind,
    cast(null as varchar) as service,
    h.cpu_busy_pct_samples / 100.0 * h.cpu_count as demand_core_seconds,
    h.cpu_count * h.samples as capacity_core_seconds,
    h.cpu_busy_pct / 100.0 as utilization_ratio,
    h.cpu_busy_pct,
    h.cpu_pressure_some_avg10,
    w.duration_seconds as load_window_seconds,
    'proc metrics within load_windows' as calculation
from host_summary h
join load_windows w using (run_id)
union all
select
    s.run_id,
    s.host,
    'service' as resource_kind,
    s.service,
    s.cpu_pct_samples / 100.0 as demand_core_seconds,
    h.cpu_count * s.samples as capacity_core_seconds,
    s.cpu_pct / nullif(100.0 * h.cpu_count, 0) as utilization_ratio,
    s.cpu_pct / nullif(h.cpu_count, 0) as cpu_busy_pct,
    h.cpu_pressure_some_avg10,
    w.duration_seconds as load_window_seconds,
    'service cpu_pct within load_windows' as calculation
from service_summary s
join host_summary h using (run_id, host)
join load_windows w using (run_id);

create or replace view query_cost as
select
    q.run_id,
    q.host,
    q.source,
    q.query as query_fingerprint,
    q.count as calls,
    q.sum_time_sec as sum_time_seconds,
    q.sum_time_sec * 1000 / nullif(q.count, 0) as avg_time_ms,
    q.p95_time_sec * 1000 as p95_time_ms,
    q.max_time_sec * 1000 as max_time_ms,
    q.sum_lock_sec as lock_time_seconds,
    q.rows_examined,
    q.rows_sent,
    q.rows_affected,
    q.no_index_used,
    q.no_good_index_used,
    q.tmp_disk_tables,
    q.tmp_tables,
    q.sort_merge_passes,
    q.errors,
    q.sum_time_sec / nullif(sum(q.sum_time_sec) over (partition by q.run_id, q.source, q.host), 0) as query_time_share,
    case q.source when 'slp' then 'slp.tsv' when 'digest' then 'mysql-digest.tsv' else q.source end as source_artifact
from queries q;

-- Host saturation signals that need two metrics compared inside one sample, so
-- they cannot be derived from a single-metric aggregate. Core-seconds follow
-- resource_demand and treat one sample as one second of the load window.
create or replace view host_saturation as
with in_window as (
    select m.*
    from metrics m
    join load_windows w using (run_id)
    where w.status = 'ok' and m.ts >= w.started_at and m.ts < w.ended_at
      and m.source = 'proc'
),
samples as (
    select
        run_id,
        host,
        ts,
        max(value) filter (where metric = 'cpu_count') as cpu_count,
        max(value) filter (where metric = 'cpu_busy_pct') as cpu_busy_pct,
        max(value) filter (where metric = 'cpu_idle_pct') as cpu_idle_pct,
        max(value) filter (where metric = 'cpu_iowait_pct') as cpu_iowait_pct,
        max(value) filter (where metric = 'procs_running') as procs_running,
        max(value) filter (where metric = 'procs_blocked') as procs_blocked,
        max(value) filter (where metric = 'cpu_pressure_some_avg10') as cpu_pressure_some_avg10,
        max(value) filter (where metric = 'io_pressure_some_avg10') as io_pressure_some_avg10,
        max(value) filter (where metric = 'memory_pressure_some_avg10') as memory_pressure_some_avg10
    from in_window
    group by all
), host_summary as (
    select
        run_id,
        host,
        count(*) as samples,
        max(cpu_count) as cpu_count,
        sum(cpu_busy_pct) as cpu_busy_pct_sum,
        min(cpu_idle_pct) as idle_min_pct,
        avg(cpu_idle_pct) as idle_mean_pct,
        avg(cpu_iowait_pct) as iowait_mean_pct,
        count(*) filter (where procs_running > cpu_count) as run_queue_over_cpu_samples,
        count(*) filter (where procs_blocked > 0) as blocked_samples,
        avg(cpu_pressure_some_avg10) as cpu_pressure_some_avg10_mean,
        avg(io_pressure_some_avg10) as io_pressure_some_avg10_mean,
        avg(memory_pressure_some_avg10) as memory_pressure_some_avg10_mean,
        count(*) filter (where cpu_busy_pct >= 80.0) as cpu_busy_ge80_samples
    from samples
    group by run_id, host
), host_peaks as (
    select
        run_id,
        host,
        max(cpu_busy_pct) as cpu_busy_peak_pct,
        max(cpu_pressure_some_avg10) as cpu_pressure_some_avg10_max
    from samples
    group by run_id, host
), host_peak_times as (
    select
        p.run_id,
        p.host,
        min(s.ts) as cpu_busy_peak_at
    from samples s
    join host_peaks p using (run_id, host)
    where s.cpu_busy_pct = p.cpu_busy_peak_pct
    group by p.run_id, p.host
)
select
    h.run_id,
    h.host,
    h.samples,
    h.cpu_count,
    -- Idle stays capacity minus busy so it reconciles with resource_demand and
    -- capacity_ledger; cpu_idle_pct is reported separately as the raw gauge.
    h.cpu_count * h.samples - h.cpu_busy_pct_sum / 100.0 * h.cpu_count as idle_core_seconds,
    p.cpu_busy_peak_pct,
    t.cpu_busy_peak_at,
    date_diff(
        'millisecond',
        w.started_at,
        t.cpu_busy_peak_at
    ) / 1000.0 as cpu_busy_peak_offset_seconds,
    h.cpu_busy_ge80_samples,
    h.cpu_busy_ge80_samples::double / nullif(h.samples, 0) as cpu_busy_ge80_ratio,
    h.idle_min_pct,
    h.idle_mean_pct,
    h.iowait_mean_pct,
    h.run_queue_over_cpu_samples,
    h.blocked_samples,
    h.cpu_pressure_some_avg10_mean,
    p.cpu_pressure_some_avg10_max,
    h.io_pressure_some_avg10_mean,
    h.memory_pressure_some_avg10_mean,
    w.duration_seconds as load_window_seconds,
    'proc metrics within load_windows' as calculation
from host_summary h
join host_peaks p using (run_id, host)
join host_peak_times t using (run_id, host)
join load_windows w using (run_id)
;

-- Every remaining periodic metric, aggregated over the same window without a
-- hardcoded metric list, so a new collector column is queryable the day it
-- lands. window_total integrates rate metrics back to a count for the window;
-- gauges leave it null and are read through mean/min/max.
create or replace view window_metrics as
with in_window as (
    select m.*
    from metrics m
    join load_windows w using (run_id)
    where w.status = 'ok' and m.ts >= w.started_at and m.ts < w.ended_at
),
available_service_samples as (
    select run_id, host, entity, ts
    from in_window
    where source = 'service' and metric = 'available' and value = 1
),
observed as (
    select m.*
    from in_window m
    left join available_service_samples a using (run_id, host, entity, ts)
    where m.source <> 'service' or a.ts is not null
)
select
    o.run_id,
    o.source,
    o.host,
    o.entity,
    o.metric,
    count(*) as samples,
    avg(o.value) as mean_value,
    min(o.value) as min_value,
    max(o.value) as max_value,
    case when ends_with(o.metric, '_per_sec') then avg(o.value) * w.duration_seconds end as window_total,
    w.duration_seconds as load_window_seconds,
    'metrics within load_windows; service rows keep only available samples' as calculation
from observed o
join load_windows w using (run_id)
group by o.run_id, o.source, o.host, o.entity, o.metric, w.duration_seconds;

-- Whole-configuration CPU ledger. unaccounted is host demand that no sampled
-- unit claims, which is the check that the service breakdown explains the host.
create or replace view capacity_ledger as
with host_rollup as (
    select
        run_id,
        count(*) as hosts,
        sum(capacity_core_seconds) as capacity_core_seconds,
        sum(demand_core_seconds) as host_demand_core_seconds
    from resource_demand
    where resource_kind = 'host'
    group by all
),
service_rollup as (
    select run_id, sum(demand_core_seconds) as service_demand_core_seconds
    from resource_demand
    where resource_kind = 'service'
    group by all
)
select
    h.run_id,
    h.hosts,
    h.capacity_core_seconds,
    h.host_demand_core_seconds,
    h.capacity_core_seconds - h.host_demand_core_seconds as idle_core_seconds,
    h.host_demand_core_seconds / nullif(h.capacity_core_seconds, 0) as utilization_ratio,
    coalesce(s.service_demand_core_seconds, 0) as service_demand_core_seconds,
    h.host_demand_core_seconds - coalesce(s.service_demand_core_seconds, 0) as unaccounted_core_seconds,
    w.duration_seconds as load_window_seconds,
    'resource_demand rolled up per RUN' as calculation
from host_rollup h
left join service_rollup s using (run_id)
join load_windows w using (run_id);

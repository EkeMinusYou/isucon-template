-- Standard reports use only the selected finalized RUNs. NULL means unknown,
-- including a route/host absent from either RUN; never fill missing data with 0.
select b.run_id as base_run, n.run_id as new_run,
 b.passed as base_passed, n.passed as new_passed,
 b.score as base_score, n.score as new_score,
 n.score-b.score as score_delta,
 round(100.0*(n.score-b.score)/nullif(b.score,0),2) as score_delta_pct,
 (b.passed and n.passed) as both_passed
from manifests b, manifests n
where b.run_id=getvariable('base_run') and n.run_id=getvariable('new_run');

-- Check configuration and load-window differences before interpreting deltas.
select m.run_id, m.commit, m.app_hosts, m.app_traffic_hosts, m.nginx_hosts,
 m.entry_host, m.mysql_host, m.additional_roles,
 m.collectors_disabled, m.profiles_enabled, m.collector_clean,
 w.status as window_status, w.source as window_source,
 w.duration_seconds, w.reason as window_reason
from manifests m left join load_windows w using(run_id)
where m.run_id in (getvariable('base_run'),getvariable('new_run'))
order by m.run_id;

-- These sources remain RUN artifacts; absence is not a measured zero.
select run_id,table_name as source,status,reason from measurement_sources
where run_id in (getvariable('base_run'),getvariable('new_run')) and status='excluded'
union all
select run_id,source,status,reason from go_profile_import_scope
where run_id in (getvariable('base_run'),getvariable('new_run')) and status='excluded'
order by run_id,source;

-- Empty output means no recorded quality warnings, not proof of completeness.
select q.run_id,q.artifact_name,q.artifact_status,q.quality_status,q.reason,
 q.window_coverage_pct
from measurement_quality q
where q.run_id in (getvariable('base_run'),getvariable('new_run'))
 and (q.artifact_status is distinct from 'ok'
      or q.quality_status is null or q.quality_status not in ('valid','artifact-ok'))
union all
select m.run_id, required.name,'missing','unavailable',
 'required artifact has no quality record',null
from manifests m, unnest(m.required_artifacts) required(name)
where m.run_id in (getvariable('base_run'),getvariable('new_run'))
 and not exists (select 1 from measurement_quality q
                 where q.run_id=m.run_id and q.artifact_name=required.name)
order by run_id,artifact_name;

-- Request starts are filtered before aggregation into up-to-5-second buckets.
-- Rank the 20 routes with the largest request counts; detail remains in task q.
with totals as (
 select h.run_id,h.method,h.route,sum(h.requests) as requests,
  sum(h.status_4xx) as errors_4xx,sum(h.status_5xx) as errors_5xx,
  round(1000.0*sum(h.sum_time_sec)/nullif(sum(h.requests),0),3) as mean_ms
 from http_traffic h join load_windows w using(run_id)
 where h.run_id in (getvariable('base_run'),getvariable('new_run'))
  and w.status='ok' and h.ts>=w.started_at and h.ts<w.ended_at
 group by h.run_id,h.method,h.route
), b as (select * from totals where run_id=getvariable('base_run')),
 n as (select * from totals where run_id=getvariable('new_run'))
select coalesce(b.method,n.method) as method,coalesce(b.route,n.route) as route,
 b.requests as base_requests,n.requests as new_requests,
 n.requests-b.requests as requests_delta,
 b.mean_ms as base_mean_ms,n.mean_ms as new_mean_ms,
 round(n.mean_ms-b.mean_ms,3) as mean_ms_delta,
 b.errors_4xx as base_4xx,n.errors_4xx as new_4xx,
 b.errors_5xx as base_5xx,n.errors_5xx as new_5xx
from b full outer join n using(method,route)
order by greatest(b.requests,n.requests) desc,method,route limit 20;

-- Host gauges share load_windows with HTTP; means are not score estimates.
with b as (
 select * from window_metrics where run_id=getvariable('base_run')
  and source='proc' and metric in ('cpu_busy_pct','cpu_idle_pct','cpu_iowait_pct','mem_used_bytes')
), n as (
 select * from window_metrics where run_id=getvariable('new_run')
  and source='proc' and metric in ('cpu_busy_pct','cpu_idle_pct','cpu_iowait_pct','mem_used_bytes')
)
select coalesce(b.host,n.host) as host,coalesce(b.metric,n.metric) as metric,
 b.samples as base_samples,n.samples as new_samples,
 round(b.mean_value,3) as base_mean,round(n.mean_value,3) as new_mean,
 round(n.mean_value-b.mean_value,3) as mean_delta
from b full outer join n using(host,metric) order by host,metric;

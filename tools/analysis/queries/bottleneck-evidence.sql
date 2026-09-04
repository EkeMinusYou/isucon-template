.mode box
.maxrows 80
.print '== RUN HEALTH =='
select * from bottleneck_run_health where run_id = getvariable('run_id');

.print '== BENCH SUMMARY =='
select * from bottleneck_bench_summary where run_id = getvariable('run_id');
select scenario, successes, failures
from bench_scenarios
where run_id = getvariable('run_id')
order by scenario;

.print '== CAPACITY =='
select * from bottleneck_capacity_summary where run_id = getvariable('run_id');

.print '== CONSUMERS =='
select host, resource_kind, service, demand_core_seconds, capacity_core_seconds,
       utilization_ratio, cpu_busy_pct, cpu_pressure_some_avg10, demand_rank
from bottleneck_consumers
where run_id = getvariable('run_id')
order by resource_kind, demand_rank;

.print '== WAIT AXES =='
select source, host, entity, metric, value, unit, denominator
from bottleneck_wait_axes
where run_id = getvariable('run_id')
  and (source <> 'disk' or (entity not like 'loop%' and entity not like 'ram%'))
order by source, host, entity, metric
limit 120;

.print '== ENDPOINT RANKING (TOP 20 BY SUM) =='
select response_time_rank, method, route, requests, response_time_sum_seconds,
       response_time_avg_ms, response_time_max_ms, p90_ms, p99_ms,
       sum_body_bytes, avg_body_bytes, status_2xx, status_3xx, status_4xx,
       status_5xx
from bottleneck_endpoint_ranking
where run_id = getvariable('run_id') and response_time_rank <= 20
order by response_time_rank;

.print '== UPSTREAM/CACHE RANKING (TOP 20 BY RESPONSE SUM) =='
select upstream_addr, upstream_status, cache_status, requests,
       response_time_sum_seconds, response_time_avg_ms,
       upstream_time_sum_seconds, upstream_time_avg_ms,
       status_2xx, status_3xx, status_4xx, status_5xx, status_other
from upstream_cost
where run_id = getvariable('run_id')
order by response_time_sum_seconds desc, requests desc
limit 20;

.print '== QUERY RANKING (TOP 20 PER SOURCE) =='
with ranked as (
    select *, row_number() over (partition by source order by sum_time_seconds desc, calls desc) as cost_rank
    from query_cost
    where run_id = getvariable('run_id')
)
select source, cost_rank, left(query_fingerprint, 180) as query_fingerprint, calls, sum_time_seconds,
       avg_time_ms, p95_time_ms, lock_time_seconds, rows_examined, rows_sent
from ranked
where cost_rank <= 20
order by source, cost_rank;

.print '== FGPROF METADATA (WALL-CLOCK, NOT CPU) =='
select host, source, sample_type, original_sample_unit, duration_seconds,
       total_wall_seconds, sample_count, incomplete_samples,
       embedded_function_names, external_binary_used, binary_matches_run,
       symbolization_complete, interpretation
from bottleneck_profile_metadata
where run_id = getvariable('run_id')
order by host;

.print '== FGPROF FUNCTIONS (TOP 20 CUMULATIVE PER HOST) =='
select host, cumulative_rank, function, flat_wall_seconds,
       cumulative_wall_seconds, flat_profile_share, cumulative_profile_share, unit
from bottleneck_profile_functions
where run_id = getvariable('run_id') and cumulative_rank <= 20
order by host, cumulative_rank;

.print '== FGPROF EDGES (TOP 20 PER HOST) =='
select host, edge_rank, caller, callee, wall_seconds,
       sample_occurrences, profile_share, unit
from bottleneck_profile_edges
where run_id = getvariable('run_id') and edge_rank <= 20
order by host, edge_rank;

.print '== SERIALIZATION SCREEN =='
select screen, value, unit, numerator, denominator, calculation
from bottleneck_serialization_screen
where run_id = getvariable('run_id')
order by screen;

.print '== STATE AUDIT =='
select source, host, entity, metric, samples, mean_value, min_value, max_value, window_total
from bottleneck_state_audit
where run_id = getvariable('run_id')
  and (source <> 'disk' or (entity not like 'loop%' and entity not like 'ram%'))
order by source, host, entity, metric
limit 160;

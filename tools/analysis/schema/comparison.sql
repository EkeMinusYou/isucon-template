-- Candidate control RUNs and normalized same-axis deltas. These views do not
-- attribute a delta to a code change; they only make compatibility and units
-- explicit so analysis skills do not each reinvent control selection.
create or replace view comparable_runs as
with card_sets as (
    select
        run_id,
        string_agg(concat_ws(':', card_id, change_boundary_hash), ',' order by card_id, change_boundary_hash) as applied_snapshot
    from run_applied_cards
    group by run_id
), eligible as (
    select
        m.*,
        v.load_window_status,
        v.bad_artifacts,
        v.bad_periodic_artifacts,
        (m.passed is true
         and v.load_window_status = 'ok'
         and v.bad_artifacts = 0
         and v.bad_periodic_artifacts = 0) as measurement_valid,
        coalesce(c.applied_snapshot, '') as applied_snapshot
    from manifests m
    join valid_runs v using (run_id)
    left join card_sets c using (run_id)
), pairs as (
    select
        t.run_id,
        c.run_id as comparison_run_id,
        t.score,
        c.score as comparison_score,
        t.score - c.score as score_delta,
        t.measurement_valid,
        c.measurement_valid as comparison_measurement_valid,
        (t.app_hosts = c.app_hosts
         and t.app_traffic_hosts = c.app_traffic_hosts
         and t.nginx_hosts = c.nginx_hosts
         and t.mysql_host = c.mysql_host) as role_compatible,
        t.commit = c.commit as same_commit,
        (t.commit = c.commit and t.dirty is false and c.dirty is false) as source_compatible,
        t.applied_snapshot = c.applied_snapshot as applied_snapshot_compatible,
        t.started_at,
        c.started_at as comparison_started_at
    from eligible t
    join eligible c on c.run_id < t.run_id
)
select
    *,
    case
        when not measurement_valid or not comparison_measurement_valid then 'invalid-measurement'
        when not role_compatible then 'role-incompatible'
        when source_compatible and applied_snapshot_compatible then 'snapshot-compatible'
        else 'role-compatible'
    end as compatibility_status,
    row_number() over (
        partition by run_id
        order by
            measurement_valid and comparison_measurement_valid and role_compatible desc,
            source_compatible and applied_snapshot_compatible desc,
            comparison_run_id desc
    ) as comparison_rank
from pairs;

create or replace view endpoint_cost_comparison as
select
    p.run_id,
    p.comparison_run_id,
    p.compatibility_status,
    p.comparison_rank,
    t.method,
    t.route,
    t.requests,
    c.requests as comparison_requests,
    t.requests - c.requests as requests_delta,
    t.response_time_sum_seconds,
    c.response_time_sum_seconds as comparison_response_time_sum_seconds,
    t.response_time_sum_seconds - c.response_time_sum_seconds as response_time_sum_delta_seconds,
    t.response_time_avg_ms,
    c.response_time_avg_ms as comparison_response_time_avg_ms,
    t.response_time_avg_ms - c.response_time_avg_ms as response_time_avg_delta_ms,
    t.p90_ms,
    c.p90_ms as comparison_p90_ms,
    t.p99_ms,
    c.p99_ms as comparison_p99_ms,
    t.sum_body_bytes,
    c.sum_body_bytes as comparison_sum_body_bytes,
    t.sum_body_bytes - c.sum_body_bytes as sum_body_delta_bytes,
    t.scope,
    t.source_artifact
from comparable_runs p
join endpoint_cost t on t.run_id = p.run_id
join endpoint_cost c on c.run_id = p.comparison_run_id
                    and c.method = t.method and c.route = t.route
where p.measurement_valid and p.comparison_measurement_valid and p.role_compatible
  and t.scope = c.scope and t.source_artifact = c.source_artifact;

create or replace view resource_demand_comparison as
select
    p.run_id,
    p.comparison_run_id,
    p.compatibility_status,
    p.comparison_rank,
    t.host,
    t.resource_kind,
    t.service,
    t.demand_core_seconds,
    c.demand_core_seconds as comparison_demand_core_seconds,
    t.demand_core_seconds - c.demand_core_seconds as demand_delta_core_seconds,
    t.capacity_core_seconds,
    c.capacity_core_seconds as comparison_capacity_core_seconds,
    t.utilization_ratio,
    c.utilization_ratio as comparison_utilization_ratio,
    t.utilization_ratio - c.utilization_ratio as utilization_delta_ratio,
    t.load_window_seconds,
    c.load_window_seconds as comparison_load_window_seconds,
    t.calculation
from comparable_runs p
join resource_demand t on t.run_id = p.run_id
join resource_demand c on c.run_id = p.comparison_run_id
                      and c.host = t.host
                      and c.resource_kind = t.resource_kind
                      and c.service is not distinct from t.service
where p.measurement_valid and p.comparison_measurement_valid and p.role_compatible
  and t.calculation = c.calculation;

create or replace view query_cost_comparison as
select
    p.run_id,
    p.comparison_run_id,
    p.compatibility_status,
    p.comparison_rank,
    t.source,
    t.query_fingerprint,
    t.calls,
    c.calls as comparison_calls,
    t.calls - c.calls as calls_delta,
    t.sum_time_seconds,
    c.sum_time_seconds as comparison_sum_time_seconds,
    t.sum_time_seconds - c.sum_time_seconds as sum_time_delta_seconds,
    t.avg_time_ms,
    c.avg_time_ms as comparison_avg_time_ms,
    t.avg_time_ms - c.avg_time_ms as avg_time_delta_ms,
    t.rows_examined,
    c.rows_examined as comparison_rows_examined,
    t.rows_examined - c.rows_examined as rows_examined_delta,
    t.source_artifact
from comparable_runs p
join query_cost t on t.run_id = p.run_id
join query_cost c on c.run_id = p.comparison_run_id
                 and c.source = t.source and c.query_fingerprint = t.query_fingerprint
where p.measurement_valid and p.comparison_measurement_valid and p.role_compatible
  and t.source_artifact = c.source_artifact;

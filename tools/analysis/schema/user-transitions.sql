-- Consecutive normalized API calls for the same Cookie identity. The artifact
-- contains aggregates only; Cookie values and per-user histories are not
-- persisted. `user_transition_edges` contains all-session edges, while
-- `user_transition_scenarios` exposes the scenario cohorts.
create or replace view user_transition_edges as
with reports as (
    select
        regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
        summary,
        edges
    from read_json(
        getvariable('run_glob') || '/user-transitions.json',
        filename = true,
        columns = {
            schema_version: 'INTEGER',
            identity_field: 'VARCHAR',
            ordering: 'VARCHAR',
            scenario_grouping: 'VARCHAR',
            summary: 'JSON',
            edges: 'JSON',
            scenarios: 'JSON'
        }
    )
    where schema_version = 3
), flattened as (
    select reports.run_id, reports.summary, item.value as edge
    from reports, json_each(reports.edges) as item
)
select
    run_id,
    json_extract_string(edge, '$.from_method') as from_method,
    json_extract_string(edge, '$.from_route') as from_route,
    json_extract_string(edge, '$.to_method') as to_method,
    json_extract_string(edge, '$.to_route') as to_route,
    json_extract_string(edge, '$.transitions')::ubigint as transitions,
    json_extract_string(edge, '$.sessions')::ubigint as sessions,
    json_extract_string(edge, '$.overlap_transitions')::ubigint as overlap_transitions,
    json_extract_string(edge, '$.ambiguous_order_transitions')::ubigint as ambiguous_order_transitions,
    json_extract_string(edge, '$.start_gap_avg_ms')::double as start_gap_avg_ms,
    json_extract_string(edge, '$.start_gap_p50_ms')::double as start_gap_p50_ms,
    json_extract_string(edge, '$.start_gap_p95_ms')::double as start_gap_p95_ms,
    json_extract_string(edge, '$.idle_gap_avg_ms')::double as idle_gap_avg_ms,
    json_extract_string(edge, '$.from_status_2xx')::ubigint as from_status_2xx,
    json_extract_string(edge, '$.from_status_3xx')::ubigint as from_status_3xx,
    json_extract_string(edge, '$.from_status_4xx')::ubigint as from_status_4xx,
    json_extract_string(edge, '$.from_status_5xx')::ubigint as from_status_5xx,
    json_extract_string(edge, '$.from_status_other')::ubigint as from_status_other,
    json_extract_string(edge, '$.to_status_2xx')::ubigint as to_status_2xx,
    json_extract_string(edge, '$.to_status_3xx')::ubigint as to_status_3xx,
    json_extract_string(edge, '$.to_status_4xx')::ubigint as to_status_4xx,
    json_extract_string(edge, '$.to_status_5xx')::ubigint as to_status_5xx,
    json_extract_string(edge, '$.to_status_other')::ubigint as to_status_other,
    json_extract_string(summary, '$.api_requests')::ubigint as total_api_requests,
    json_extract_string(summary, '$.classified_requests')::ubigint as total_classified_requests,
    json_extract_string(summary, '$.requests_with_identity')::ubigint as total_requests_with_identity,
    json_extract_string(summary, '$.sessions')::ubigint as total_sessions,
    json_extract_string(summary, '$.transitions')::ubigint as total_transitions,
    json_extract_string(summary, '$.overlapping_transitions')::ubigint as total_overlapping_transitions,
    json_extract_string(summary, '$.ambiguous_order_transitions')::ubigint as total_ambiguous_order_transitions,
    json_extract_string(summary, '$.scenario_groups')::ubigint as total_scenario_groups,
    json_extract_string(summary, '$.scenarios_emitted')::ubigint as scenarios_emitted,
    json_extract_string(summary, '$.scenario_sessions_omitted')::ubigint as scenario_sessions_omitted,
    json_extract_string(summary, '$.unmatched_api_requests')::ubigint as unmatched_api_requests,
    json_extract_string(summary, '$.missing_identity_requests')::ubigint as missing_identity_requests,
    json_extract_string(summary, '$.malformed_lines')::ubigint as malformed_lines,
    json_extract_string(summary, '$.invalid_time_lines')::ubigint as invalid_time_lines
from flattened;

create or replace view user_transition_scenarios as
with reports as (
    select
        regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
        scenarios
    from read_json(
        getvariable('run_glob') || '/user-transitions.json',
        filename = true,
        columns = {
            schema_version: 'INTEGER',
            identity_field: 'VARCHAR',
            ordering: 'VARCHAR',
            scenario_grouping: 'VARCHAR',
            summary: 'JSON',
            edges: 'JSON',
            scenarios: 'JSON'
        }
    )
    where schema_version = 3
), scenario_rows as (
    select reports.run_id, scenario.key::integer as scenario_rank_zero_based, scenario.value as scenario
    from reports, json_each(reports.scenarios) as scenario
), scenario_edges as (
    select scenario_rows.*, edge.value as edge
    from scenario_rows
    left join json_each(json_extract(scenario_rows.scenario, '$.edges')) as edge on true
)
select
    run_id,
    scenario_rank_zero_based + 1 as scenario_rank,
    json_extract_string(scenario, '$.id') as scenario_id,
    json_extract(scenario, '$.signature') as signature,
    json_extract(scenario, '$.nodes') as nodes,
    json_extract_string(scenario, '$.sessions')::ubigint as scenario_sessions,
    json_extract_string(scenario, '$.requests')::ubigint as scenario_requests,
    json_extract_string(scenario, '$.transitions')::ubigint as scenario_transitions,
    json_extract_string(scenario, '$.requests_per_session_avg')::double as requests_per_session_avg,
    json_extract_string(scenario, '$.duration_p50_ms')::double as duration_p50_ms,
    json_extract_string(scenario, '$.duration_p95_ms')::double as duration_p95_ms,
    json_extract_string(edge, '$.from_method') as from_method,
    json_extract_string(edge, '$.from_route') as from_route,
    json_extract_string(edge, '$.to_method') as to_method,
    json_extract_string(edge, '$.to_route') as to_route,
    json_extract_string(edge, '$.transitions')::ubigint as transitions,
    json_extract_string(edge, '$.sessions')::ubigint as sessions,
    json_extract_string(edge, '$.overlap_transitions')::ubigint as overlap_transitions,
    json_extract_string(edge, '$.ambiguous_order_transitions')::ubigint as ambiguous_order_transitions,
    json_extract_string(edge, '$.start_gap_avg_ms')::double as start_gap_avg_ms,
    json_extract_string(edge, '$.start_gap_p50_ms')::double as start_gap_p50_ms,
    json_extract_string(edge, '$.start_gap_p95_ms')::double as start_gap_p95_ms,
    json_extract_string(edge, '$.idle_gap_avg_ms')::double as idle_gap_avg_ms,
    json_extract_string(edge, '$.from_status_4xx')::ubigint as from_status_4xx,
    json_extract_string(edge, '$.from_status_5xx')::ubigint as from_status_5xx,
    json_extract_string(edge, '$.to_status_4xx')::ubigint as to_status_4xx,
    json_extract_string(edge, '$.to_status_5xx')::ubigint as to_status_5xx
from scenario_edges;

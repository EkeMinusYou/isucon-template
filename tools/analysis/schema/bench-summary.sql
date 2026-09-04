-- Generic benchmark fields. BENCHMARK_* markers can be added by a local
-- wrapper without depending on the contest benchmarker's private format.
create or replace view bench_summaries_raw as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    try_cast(regexp_extract(content, '(?:スコア|SCORE|Score):? ([0-9]+)', 1) as bigint) as score,
    regexp_matches(content, '(BENCHMARK_PASS|最終チェックが成功しました)') as final_check_succeeded,
    regexp_matches(content, 'BENCHMARK_END') as stop_marker_present,
    'bench.log' as source_artifact
from read_text(getvariable('run_glob') || '/bench.log');

create or replace view bench_scenarios_raw as
with logs as (
    select
        regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
        content
    from read_text(getvariable('run_glob') || '/bench.log')
), matches as (
    select
        run_id,
        unnest(regexp_extract_all(
            content,
            'SCENARIO ([^ ]+) success=([0-9]+)(?: failure=([0-9]+))?',
            ['scenario', 'successes', 'failures']
        )) as matched
    from logs
)
select
    run_id,
    matched.scenario as scenario,
    try_cast(matched.successes as bigint) as successes,
    coalesce(try_cast(matched.failures as bigint), 0) as failures,
    'bench.log' as source_artifact
from matches;

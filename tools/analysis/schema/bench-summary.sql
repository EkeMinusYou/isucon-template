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

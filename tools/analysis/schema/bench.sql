-- Benchmark load-phase timestamps. New RUNs persist these in run.json; this
-- parser keeps historical RUNs queryable without rewriting their artifacts.
create or replace view bench_phases as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    try_cast(regexp_extract(content,
        '([0-9]{4}-[0-9]{2}-[0-9]{2}T[^\t ]+).*BENCHMARK_START', 1)
        as timestamptz) as started_at,
    try_cast(regexp_extract(content,
        '([0-9]{4}-[0-9]{2}-[0-9]{2}T[^\t ]+).*BENCHMARK_END', 1)
        as timestamptz) as ended_at,
    'bench.log' as source
from read_text(getvariable('run_glob') || '/bench.log');

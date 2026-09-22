-- Preserve log line order; completion summaries do not carry occurrence times.
create or replace view bench_error_logs_raw as
select regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    content, 'bench.log' as source_artifact
from read_text(getvariable('run_glob') || '/bench.log');

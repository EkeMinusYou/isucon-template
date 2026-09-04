-- ホスト全体の /proc サンプル。host はファイル名、run_id はディレクトリ名から取る。
-- union_by_name は collector に列が増えた過去/未来の RUN を混ぜて読むために要る。
create or replace view proc_metrics as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1)            as run_id,
    regexp_extract(filename, '/([^/]+)-proc-metrics\.tsv$', 1) as host,
    * exclude (filename)
from read_csv(getvariable('run_glob') || '/*-proc-metrics.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true,
              -- A stopped sampler can leave one truncated tail row. The
              -- manifest quality contract still marks its coverage invalid;
              -- omit only that unreadable row so other RUNs remain queryable.
              ignore_errors = true);

create or replace view metrics_proc as
select run_id, ts, elapsed_ms, host, source, entity, metric, value from (
    unpivot (
        select run_id, host, 'proc' as source,
               cast(null as varchar) as entity,
               timestamp as ts, elapsed_ms,
               * exclude (run_id, host, sample, timestamp, elapsed_ms)
        from proc_metrics
    ) on columns (* exclude (run_id, ts, elapsed_ms, host, source, entity))
      into name metric value value
);

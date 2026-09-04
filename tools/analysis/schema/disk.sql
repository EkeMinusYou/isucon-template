-- ブロックデバイスごとの I/O サンプル。entity はデバイス名。
create or replace view disk_metrics as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1)            as run_id,
    regexp_extract(filename, '/([^/]+)-disk-metrics\.tsv$', 1) as host,
    * exclude (filename)
from read_csv(getvariable('run_glob') || '/*-disk-metrics.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true);

create or replace view metrics_disk as
select run_id, ts, elapsed_ms, host, source, entity, metric, value from (
    unpivot (
        select run_id, host, 'disk' as source,
               device as entity,
               timestamp as ts, elapsed_ms,
               * exclude (run_id, host, sample, timestamp, elapsed_ms, device)
        from disk_metrics
    ) on columns (* exclude (run_id, ts, elapsed_ms, host, source, entity))
      into name metric value value
);

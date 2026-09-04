-- systemd unit ごとの cgroup サンプル。entity は unit 名。
create or replace view service_metrics as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1)               as run_id,
    regexp_extract(filename, '/([^/]+)-service-metrics\.tsv$', 1) as host,
    * exclude (filename)
from read_csv(getvariable('run_glob') || '/*-service-metrics.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true);

create or replace view metrics_service as
select run_id, ts, elapsed_ms, host, source, entity, metric, value from (
    unpivot (
        select run_id, host, 'service' as source,
               service as entity,
               timestamp as ts, elapsed_ms,
               * exclude (run_id, host, sample, timestamp, elapsed_ms, service)
        from service_metrics
    ) on columns (* exclude (run_id, ts, elapsed_ms, host, source, entity))
      into name metric value value
);

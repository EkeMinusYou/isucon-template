-- MySQL の SHOW STATUS サンプル。host は役割 (runs.mysql_host) から引く。
create or replace view mysql_status as
select
    r.run_id,
    r.mysql_host as host,
    m.* exclude (filename)
from read_csv(getvariable('run_glob') || '/mysql-status.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true) m
left join runs r
  on r.run_id = regexp_extract(m.filename, 'runs/([^/]+)/', 1);

create or replace view metrics_mysql as
select run_id, ts, elapsed_ms, host, source, entity, metric, value from (
    unpivot (
        select run_id, host, 'mysql' as source,
               cast(null as varchar) as entity,
               timestamp as ts, elapsed_ms,
               * exclude (run_id, host, sample, timestamp, elapsed_ms)
        from mysql_status
    ) on columns (* exclude (run_id, ts, elapsed_ms, host, source, entity))
      into name metric value value
);

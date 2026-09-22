-- Application-side database/sql pool samples. The collector emits one row per
-- pool and metric so pool identity remains queryable without high-cardinality
-- request labels.
create or replace view sql_pool_metrics as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    regexp_extract(filename, '/([^/]+)-sql-pool-metrics\.tsv$', 1) as host,
    * exclude (filename)
from read_csv(getvariable('run_glob') || '/*-sql-pool-metrics.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true,
              ignore_errors = true);

create or replace view metrics_sql_pool as
select
    run_id,
    timestamp as ts,
    elapsed_ms,
    host,
    'sql_pool' as source,
    database_host || ':' || role || ':' || pool || ':' || cast(shard as varchar) as entity,
    metric,
    value
from sql_pool_metrics;

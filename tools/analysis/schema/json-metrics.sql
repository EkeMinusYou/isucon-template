-- Bounded application snapshots emitted by json-metrics-collector.
create or replace view application_json_metrics as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    regexp_extract(filename, '/([^/]+)-json-metrics\.tsv$', 1) as host,
    * exclude (filename)
from read_csv(getvariable('run_glob') || '/*-json-metrics.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true,
              types = {'sample': 'UBIGINT', 'timestamp': 'TIMESTAMPTZ',
                       'elapsed_ms': 'BIGINT', 'generation': 'UBIGINT',
                       'final': 'BOOLEAN', 'scope': 'VARCHAR',
                       'entity_id': 'BIGINT', 'group_id': 'BIGINT',
                       'window_start_ms': 'BIGINT', 'metric': 'VARCHAR',
                       'value': 'BIGINT'});

create or replace view metrics_application_json as
select run_id, timestamp as ts, elapsed_ms, host,
       'application_json' as source,
       scope || ':' || cast(entity_id as varchar) || ':' || cast(group_id as varchar) as entity,
       metric, cast(value as double) as value
from application_json_metrics;

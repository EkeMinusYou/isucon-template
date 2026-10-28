-- Scalar runtime and application metrics captured from the Go process.
create or replace view metrics_runtime as
select regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
 timestamp as ts, elapsed_ms,
 regexp_extract(filename, '/([^/]+)-runtime-metrics\.tsv$', 1) as host,
 'runtime' as source, 'application' as entity, metric, value
from read_csv(getvariable('run_glob') || '/*-runtime-metrics.tsv', delim='\t',
 header=true, filename=true, types={'timestamp':'TIMESTAMPTZ','value':'DOUBLE'});

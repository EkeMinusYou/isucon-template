-- D-state task attribution sampled by proc-metrics. A pid/tid of zero is the
-- explicit no-blocked-task marker for that sample.
create or replace view task_states as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    regexp_extract(filename, '/([^/]+)-task-states\.tsv$', 1) as host,
    * exclude (filename)
from read_csv(getvariable('run_glob') || '/*-task-states.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true);

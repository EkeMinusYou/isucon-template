-- Live InnoDB data-lock wait edges. Repeated wait_key values are multiple
-- observations of the same wait and can be deduplicated by analysis queries.
create or replace view mysql_lock_waits as
select
    r.run_id,
    r.mysql_host as host,
    m.* exclude (filename)
from read_csv(getvariable('run_glob') || '/mysql-lock-waits.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true) m
left join runs r
  on r.run_id = regexp_extract(m.filename, 'runs/([^/]+)/', 1);

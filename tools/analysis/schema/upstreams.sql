-- nginx access log から作った upstream / cache 別内訳。
create or replace view upstreams as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    * exclude (filename)
from read_csv(getvariable('run_glob') || '/upstream-breakdown.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true);

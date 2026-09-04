-- performance_schema の statement digest。slp とは grouping が独立した別集計なので、
-- queries テーブルでは source='digest' として区別する。
create or replace view queries_digest as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    'digest'                                       as source,
    digest_text                                    as query,
    count,
    sum_time_sec,
    max_time_sec,
    cast(null as double)                           as p95_time_sec,
    rows_examined,
    rows_sent,
    sum_lock_sec,
    sum_lock_sec / nullif(count, 0)                as avg_lock_sec
from read_csv(getvariable('run_glob') || '/mysql-digest.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true);

-- performance_schema の statement digest。slp とは grouping が独立した別集計なので、
-- queries テーブルでは source='digest' として区別する。
create or replace view queries_digest as
select
    regexp_extract(m.filename, 'runs/([^/]+)/', 1) as run_id,
    coalesce(nullif(regexp_extract(m.filename, '/([^/]+)-mysql-digest\.tsv$', 1), ''), r.mysql_host) as host,
    'digest'                                        as source,
    fold_repeated_sql(m.digest_text)                as query,
    count,
    sum_time_sec,
    max_time_sec,
    cast(null as double)                           as p95_time_sec,
    rows_examined,
    rows_sent,
    sum_lock_sec,
    sum_lock_sec / nullif(count, 0)                as avg_lock_sec,
    cast(rows_affected as bigint)                   as rows_affected,
    cast(no_index_used as bigint)                   as no_index_used,
    cast(no_good_index_used as bigint)              as no_good_index_used,
    cast(tmp_disk_tables as bigint)                 as tmp_disk_tables,
    cast(tmp_tables as bigint)                      as tmp_tables,
    cast(sort_merge_passes as bigint)               as sort_merge_passes,
    cast(errors as bigint)                          as errors
from read_csv(getvariable('run_glob') || '/*mysql-digest.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true) m
left join runs r
  on r.run_id = regexp_extract(m.filename, 'runs/([^/]+)/', 1);

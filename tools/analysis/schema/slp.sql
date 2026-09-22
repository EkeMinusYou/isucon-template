-- slp (SQL パーサーでの抽象化) によるスロークエリ集計。
create or replace view queries_slp as
select
    regexp_extract(m.filename, 'runs/([^/]+)/', 1) as run_id,
    coalesce(nullif(regexp_extract(m.filename, '/([^/]+)-slp\.tsv$', 1), ''), r.mysql_host) as host,
    'slp'                                           as source,
    fold_repeated_sql(m."Query")                    as query,
    cast("Count" as bigint) as count,
    cast("Sum(QueryTime)" as double) as sum_time_sec,
    cast("Max(QueryTime)" as double) as max_time_sec,
    cast("P95(QueryTime)" as double) as p95_time_sec,
    cast("Sum(RowsExamined)" as double) as rows_examined,
    cast("Sum(RowsSent)" as double) as rows_sent,
    cast("Sum(LockTime)" as double) as sum_lock_sec,
    cast("Avg(LockTime)" as double) as avg_lock_sec,
    cast(null as bigint) as rows_affected,
    cast(null as bigint) as no_index_used,
    cast(null as bigint) as no_good_index_used,
    cast(null as bigint) as tmp_disk_tables,
    cast(null as bigint) as tmp_tables,
    cast(null as bigint) as sort_merge_passes,
    cast(null as bigint) as errors
from read_csv(getvariable('run_glob') || '/*slp.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true) m
left join runs r
  on r.run_id = regexp_extract(m.filename, 'runs/([^/]+)/', 1);

-- slp (SQL パーサーでの抽象化) によるスロークエリ集計。
create or replace view queries_slp as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    'slp'                                          as source,
    "Query"                                        as query,
    cast("Count" as bigint) as count,
    cast("Sum(QueryTime)" as double) as sum_time_sec,
    cast("Max(QueryTime)" as double) as max_time_sec,
    cast("P95(QueryTime)" as double) as p95_time_sec,
    cast("Sum(RowsExamined)" as double) as rows_examined,
    cast("Sum(RowsSent)" as double) as rows_sent,
    cast("Sum(LockTime)" as double) as sum_lock_sec,
    cast("Avg(LockTime)" as double) as avg_lock_sec
from read_csv(getvariable('run_glob') || '/slp.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true);

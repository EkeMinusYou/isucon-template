-- slp (SQL パーサーでの抽象化) によるスロークエリ集計。
create or replace view queries_slp as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    'slp'                                          as source,
    "Query"                                        as query,
    "Count"                                        as count,
    "Sum(QueryTime)"                               as sum_time_sec,
    "Max(QueryTime)"                               as max_time_sec,
    "P95(QueryTime)"                               as p95_time_sec,
    "Sum(RowsExamined)"                            as rows_examined,
    "Sum(RowsSent)"                                as rows_sent,
    "Sum(LockTime)"                                as sum_lock_sec,
    "Avg(LockTime)"                                as avg_lock_sec
from read_csv(getvariable('run_glob') || '/slp.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true);

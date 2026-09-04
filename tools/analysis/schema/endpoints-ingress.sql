-- alp aggregate per saved nginx access-log file. The filename-derived ingress
-- host is retained by tools/analysis/alp-by-ingress.sh.
create or replace view endpoints_by_ingress as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    ingress_host,
    method,
    uri,
    try_cast(count as bigint) as count,
    try_cast(sum as double) as sum_time_sec,
    try_cast(avg as double) as avg_time_sec,
    try_cast(max as double) as max_time_sec,
    try_cast(p90 as double) as p90_time_sec,
    try_cast(p99 as double) as p99_time_sec,
    try_cast(sum_body as double) as sum_body_bytes,
    try_cast(avg_body as double) as avg_body_bytes,
    try_cast("2xx" as bigint) as status_2xx,
    try_cast("3xx" as bigint) as status_3xx,
    try_cast("4xx" as bigint) as status_4xx,
    try_cast("5xx" as bigint) as status_5xx
from read_csv(getvariable('run_glob') || '/alp-by-ingress.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true, all_varchar = true);

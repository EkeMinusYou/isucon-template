-- nginx upstream/cache aggregate retaining the ingress host that owned each
-- source access log.
create or replace view upstreams_by_ingress as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
    ingress_host,
    upstream_addr,
    upstream_status,
    cache_status,
    try_cast(requests as bigint) as requests,
    try_cast(status_2xx as bigint) as status_2xx,
    try_cast(status_3xx as bigint) as status_3xx,
    try_cast(status_4xx as bigint) as status_4xx,
    try_cast(status_5xx as bigint) as status_5xx,
    try_cast(status_other as bigint) as status_other,
    try_cast(response_time_sum_ms as double) as response_time_sum_ms,
    try_cast(response_time_avg_ms as double) as response_time_avg_ms,
    try_cast(upstream_time_sum_ms as double) as upstream_time_sum_ms,
    try_cast(upstream_time_avg_ms as double) as upstream_time_avg_ms
from read_csv(getvariable('run_glob') || '/upstream-breakdown-by-ingress.tsv', delim = '\t',
              header = true, filename = true, union_by_name = true, all_varchar = true);

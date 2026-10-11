-- nginx access log から作った upstream / cache 別内訳。
-- 列を明示する。推定に任せると、空の TSV が混ざったとき column0 が足されて取り込み先の表と合わなくなる。
create or replace view upstreams as
select
    regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
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
from read_csv(getvariable('run_glob') || '/upstream-breakdown.tsv', delim = '\t',
              header = true, filename = true, auto_detect = false,
              columns = {
                  'upstream_addr': 'VARCHAR', 'upstream_status': 'VARCHAR', 'cache_status': 'VARCHAR',
                  'requests': 'VARCHAR', 'status_2xx': 'VARCHAR', 'status_3xx': 'VARCHAR',
                  'status_4xx': 'VARCHAR', 'status_5xx': 'VARCHAR', 'status_other': 'VARCHAR',
                  'response_time_sum_ms': 'VARCHAR', 'response_time_avg_ms': 'VARCHAR',
                  'upstream_time_sum_ms': 'VARCHAR', 'upstream_time_avg_ms': 'VARCHAR'
              })
where upstream_addr is not null;

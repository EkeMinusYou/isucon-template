-- ingress nginx の生 access log (raw/access-<host>.log[.zst]) の 5 秒 bucket。
-- alp.json は走行 1 回ぶんの集計なので、処理量と応答時間の時間発展はここで見る。
-- route 正規化は alp.yml の matching_groups が正本。setup で当日の route を入れるときは
-- 両方へ同じ正規化を書く。どの pattern にも一致しない URI は、そのまま route になる。
create or replace view http_traffic_buckets as
with routes as (
    select [
        -- # >>> contest values >>>
        -- プレースホルダー。当日の path を、動的セグメントを正規化して、
        -- より具体的なものから並べる。alp.yml の matching_groups と同じ内容にする。
        '^/initialize$'
        -- # <<< contest values <<<
    ] as patterns
),
requests as (
    select
        regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
        regexp_replace(regexp_extract(filename, '/access-([^/]+)$', 1),
                       '\.log(\.zst)?$', '') as ingress_host,
        ends_with(filename, '.zst') as compressed,
        try_cast(msec as double) as logged_at_epoch,
        coalesce(try_cast(response_time as double), 0.0) as response_time_sec,
        -- upstream を複数回試行した行は "0.001, 0.002" になる。NULL として数え、
        -- 母数は upstream_time_samples で示す。
        try_cast(upstream_time as double) as upstream_time_sec,
        method,
        uri,
        try_cast(status as bigint) as status,
        coalesce(try_cast(body_bytes as double), 0.0) as body_bytes
    from read_json(
        getvariable('run_glob') || '/raw/access-*.log*',
        format = 'newline_delimited',
        filename = true,
        -- 中断した RUN は末尾 1 行が切れていることがある。
        ignore_errors = true,
        columns = {
            'msec': 'VARCHAR',
            'method': 'VARCHAR',
            'uri': 'VARCHAR',
            'status': 'BIGINT',
            'response_time': 'DOUBLE',
            'body_bytes': 'DOUBLE',
            'upstream_time': 'VARCHAR',
            'upstream_addr': 'VARCHAR',
            'upstream_status': 'VARCHAR',
            'cache_status': 'VARCHAR',
            'session_id': 'VARCHAR'
        })
),
buckets as (
    select
        r.run_id,
        r.ingress_host,
        r.compressed,
        -- $msec は応答を書き出した時刻。traffic_window.go と同じく開始時刻へ戻す。
        cast(to_timestamp(floor((r.logged_at_epoch - r.response_time_sec) / 5) * 5)
             as timestamptz) as ts,
        cast(5 as integer) as bucket_seconds,
        r.method,
        coalesce(list_filter(g.patterns, lambda p: regexp_matches(r.uri, p))[1], r.uri) as route,
        count(*) as requests,
        count(*) filter (where r.status between 200 and 299) as status_2xx,
        count(*) filter (where r.status between 300 and 399) as status_3xx,
        count(*) filter (where r.status between 400 and 499) as status_4xx,
        count(*) filter (where r.status between 500 and 599) as status_5xx,
        sum(r.response_time_sec) as sum_time_sec,
        max(r.response_time_sec) as max_time_sec,
        sum(r.upstream_time_sec) as sum_upstream_time_sec,
        count(r.upstream_time_sec) as upstream_time_samples,
        sum(r.body_bytes) as sum_body_bytes
    from requests r, routes g
    where r.logged_at_epoch is not null
    group by all
),
-- 平文と .zst が両方ある RUN は平文を採る (accessLogFilesForWindow と同じ順)。
deduplicated as (
    select b.*, min(b.compressed) over (partition by b.run_id, b.ingress_host) as preferred
    from buckets b
)
select * exclude (compressed, preferred)
from deduplicated
where compressed = preferred;

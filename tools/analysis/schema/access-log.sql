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
requests as materialized (
    with windows as (
 select json_extract_string(value,'$.run_id') as run_id,
  try_cast(json_extract_string(value,'$.started_at') as timestamptz) as started_at,
  try_cast(json_extract_string(value,'$.ended_at') as timestamptz) as ended_at,
  json_extract_string(value,'$.status') as status
 from json_each(coalesce(getvariable('analysis_windows')::varchar,'[]'))
)
    select
        regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
        (select max(started_at) from windows where run_id=regexp_extract(filename,'runs/([^/]+)/',1)) as analysis_started_at,
        (select max(ended_at) from windows where run_id=regexp_extract(filename,'runs/([^/]+)/',1)) as analysis_ended_at,
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
        coalesce(try_cast(body_bytes as double), 0.0) as body_bytes,
        try_cast(connection_requests as bigint) as connection_requests,
        ssl_session_reused,
        -- HTTP/1.1 は 1 接続で順に処理するので、同じ接続の前の応答書き出しから
        -- この要求の開始までがクライアント側の間隔になる。接続番号は nginx 再起動で
        -- 振り直されるため、ファイル単位で区切る。接続番号のない旧ログは NULL。
        case when try_cast(connection as bigint) is not null then
            try_cast(msec as double) - coalesce(try_cast(response_time as double), 0.0)
                - lag(try_cast(msec as double)) over (
                    partition by filename, try_cast(connection as bigint)
                    order by try_cast(msec as double) - coalesce(try_cast(response_time as double), 0.0)
                )
        end as client_gap_sec
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
            'session_id': 'VARCHAR',
            'connection': 'VARCHAR',
            'connection_requests': 'VARCHAR',
            'ssl_session_reused': 'VARCHAR'
        })
    where getvariable('analysis_windows') is null or exists (
      select 1 from windows w
      where w.run_id=regexp_extract(filename,'runs/([^/]+)/',1) and w.status='ok'
       and to_timestamp(try_cast(msec as double)-coalesce(try_cast(response_time as double),0))>=w.started_at
       and to_timestamp(try_cast(msec as double)-coalesce(try_cast(response_time as double),0))<w.ended_at
    )
),
-- Parse each log once and normalize each distinct URI once. Repeated requests
-- often share the same URI throughout a RUN.
normalized_routes as materialized (
    select
        r.uri,
        coalesce(list_filter(g.patterns, lambda p: regexp_matches(r.uri, p))[1], r.uri) as route
    from (select distinct uri from requests) r, routes g
),
buckets as (
    select
        r.run_id,
        r.ingress_host,
        r.compressed,
        -- $msec は応答を書き出した時刻。traffic_window.go と同じく開始時刻へ戻す。
        greatest(to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5),
                 coalesce(r.analysis_started_at,to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5))) as ts,
        epoch(least(to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5+5),
                    coalesce(r.analysis_ended_at,to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5+5)))
             -greatest(to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5),
                    coalesce(r.analysis_started_at,to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5)))) as bucket_seconds,
        r.method,
        g.route,
        count(*) as requests,
        count(*) filter (where r.status between 200 and 299) as status_2xx,
        count(*) filter (where r.status between 300 and 399) as status_3xx,
        count(*) filter (where r.status between 400 and 499) as status_4xx,
        count(*) filter (where r.status between 500 and 599) as status_5xx,
        sum(r.response_time_sec) as sum_time_sec,
        max(r.response_time_sec) as max_time_sec,
        sum(r.upstream_time_sec) as sum_upstream_time_sec,
        count(r.upstream_time_sec) as upstream_time_samples,
        sum(r.body_bytes) as sum_body_bytes,
        -- 接続の 1 本目の要求の route に新規接続を計上する。母数は connection_samples。
        count(r.connection_requests) as connection_samples,
        count(*) filter (where r.connection_requests = 1) as new_connections,
        count(*) filter (where r.connection_requests = 1 and r.ssl_session_reused = 'r')
            as resumed_tls_connections,
        sum(r.client_gap_sec) as sum_client_gap_sec,
        count(r.client_gap_sec) as client_gap_samples
    from requests r
    join normalized_routes g on r.uri is not distinct from g.uri
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

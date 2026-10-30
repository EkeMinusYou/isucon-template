-- Parse each access log once per import. This temporary table is also used by
-- operation progress and connection reuse; identities never enter stored tables.
set variable access_files = (
    select list(f.file) from glob(getvariable('run_glob') || '/raw/access-*.log*') f
    where not ends_with(f.file,'.zst') or not exists (
        select 1 from glob(getvariable('run_glob') || '/raw/access-*.log*') plain
        where plain.file=regexp_replace(f.file,'\.zst$','')
    )
);
set variable operation_methods = (
    select list_distinct(flatten(list(methods))) from (
        select [json_extract_string(p.value,'$.from.method'),json_extract_string(p.value,'$.to.method')] as methods
        from json_each(coalesce(getvariable('operation_definitions')::varchar,'[]')) d,
             json_each(json_extract(d.value,'$.config.operation_pairs')) p
        union all
        select [json_extract_string(p.value,'$.start.method'),json_extract_string(p.value,'$.end.method')]
        from json_each(coalesce(getvariable('operation_definitions')::varchar,'[]')) d,
             json_each(json_extract(d.value,'$.config.operation_intervals')) p
    )
);
create or replace temp table http_access_requests as
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
        try_cast(msec as double) as logged_at_epoch,
        coalesce(try_cast(response_time as double), 0.0) as response_time_sec,
        isfinite(try_cast(msec as double))
          and response_time is not null and isfinite(response_time) and response_time >= 0 as valid_timing,
        row_number() over () as event_id,
        case when session_id not in ('','-')
          and list_contains(getvariable('operation_methods')::varchar[],upper(method))
          then sha256(session_id) end as identity_key,
        try_cast(connection as bigint) as connection_id,
        upper(method) as method,
        uri,
        try_cast(status as bigint) as status,
        coalesce(try_cast(body_bytes as double), 0.0) as body_bytes,
        try_cast(connection_requests as bigint) as connection_requests,
        ssl_session_reused,
        protocol,
        tcp,
        upstream,
        case when request_body_bytes >= 0 and isfinite(request_body_bytes) then request_body_bytes end as request_body_bytes,
        case when request_content_length >= 0 and isfinite(request_content_length) then request_content_length end as request_content_length
    from read_json(
        getvariable('access_files'),
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
            'request_body_bytes': 'DOUBLE',
            'request_content_length': 'DOUBLE',
            'session_id': 'VARCHAR',
            'connection': 'VARCHAR',
            'connection_requests': 'VARCHAR',
            'ssl_session_reused': 'VARCHAR',
            'protocol': 'VARCHAR',
            'tcp': 'STRUCT(rtt_us UBIGINT, rtt_variance_us UBIGINT, total_retransmits UBIGINT)',
            'upstream': 'STRUCT(address VARCHAR, reused BOOLEAN, connect_seconds DOUBLE, tls_seconds DOUBLE, header_seconds DOUBLE, total_seconds DOUBLE, body_bytes BIGINT, error VARCHAR)[]'
        })
    where getvariable('analysis_windows') is null or exists (
      select 1 from windows w
      where w.run_id=regexp_extract(filename,'runs/([^/]+)/',1) and w.status='ok'
       and to_timestamp(try_cast(msec as double)-coalesce(try_cast(response_time as double),0))>=w.started_at
       and to_timestamp(try_cast(msec as double)-coalesce(try_cast(response_time as double),0))<w.ended_at
    );

-- Reuse this RUN's captured definitions across traffic, progress and concurrency.
create or replace temp table http_normalized_routes as
with configs as (
    select json_extract_string(value,'$.run_id') as run_id,
           json_extract(value,'$.config') as config
    from json_each(coalesce(getvariable('operation_definitions')::varchar,'[]'))
), paths as (
    select distinct run_id, uri, coalesce(try(url_decode(split_part(uri,'?',1))),split_part(uri,'?',1)) as path
    from http_access_requests
)
select p.run_id,p.uri,p.path,c.config is not null as config_available,
       starts_with(p.path,json_extract_string(c.config,'$.api_prefix')) as is_api,
       (select json_extract_string(r.value,'$.name')
        from json_each(json_extract(c.config,'$.routes')) r
        where regexp_matches(case when coalesce(json_extract(r.value,'$.match_query')::boolean,false)
                                  then p.uri else p.path end,json_extract_string(r.value,'$.pattern'))
        order by r.key::integer limit 1) as matched_route
from paths p left join configs c using(run_id);

-- Filter starts before bucketing; unmatched APIs stay visible without ID labels.
create or replace view http_traffic_buckets as
with requests as materialized (
    select *,
        case when connection_id is not null and (protocol is null or protocol in ('HTTP/1.0','HTTP/1.1')) then
            logged_at_epoch-response_time_sec-lag(logged_at_epoch) over (
                partition by run_id,ingress_host,connection_id
                order by logged_at_epoch-response_time_sec
            )
        end as client_gap_sec
    from http_access_requests
),
buckets as (
    select
        r.run_id,
        r.ingress_host,
        -- $msec は応答を書き出した時刻。traffic_window.go と同じく開始時刻へ戻す。
        greatest(to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5),
                 coalesce(r.analysis_started_at,to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5))) as ts,
        epoch(least(to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5+5),
                    coalesce(r.analysis_ended_at,to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5+5)))
             -greatest(to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5),
                    coalesce(r.analysis_started_at,to_timestamp(floor((r.logged_at_epoch-r.response_time_sec)/5)*5)))) as bucket_seconds,
        r.method,
        coalesce(g.matched_route,case when g.is_api then '<unclassified-api>' else g.path end) as route,
        count(*) as requests,
        count(*) filter (where r.status between 200 and 299) as status_2xx,
        count(*) filter (where r.status between 300 and 399) as status_3xx,
        count(*) filter (where r.status between 400 and 499) as status_4xx,
        count(*) filter (where r.status between 500 and 599) as status_5xx,
        sum(r.response_time_sec) as sum_time_sec,
        max(r.response_time_sec) as max_time_sec,
        sum(r.body_bytes) as sum_body_bytes,
        sum(r.request_body_bytes) as sum_request_body_bytes,
        count(r.request_body_bytes) as request_body_samples,
        sum(r.request_content_length) as sum_request_content_length,
        count(r.request_content_length) as request_content_length_samples,
        -- 接続の 1 本目の要求の route に新規接続を計上する。母数は connection_samples。
        count(r.connection_requests) as connection_samples,
        count(*) filter (where r.connection_requests = 1) as new_connections,
        count(*) filter (where r.connection_requests = 1 and r.ssl_session_reused = 'r')
            as resumed_tls_connections,
        sum(r.client_gap_sec) as sum_client_gap_sec,
        count(r.client_gap_sec) as client_gap_samples
    from requests r
    join http_normalized_routes g on r.run_id=g.run_id and r.uri is not distinct from g.uri
    where r.logged_at_epoch is not null
    group by all
)
select * from buckets;

-- Counts use the same RUN, host and load window as http_traffic_buckets.
create or replace view http_route_coverage as
select r.run_id,r.ingress_host,
       case when not n.config_available then 'definitions-missing'
            when r.uri is null then 'invalid-uri'
            when n.matched_route is not null then 'classified'
            when n.is_api then 'unclassified-api' else 'outside-api' end as status,
       count(*) as requests,
       count(*) filter (where r.logged_at_epoch is not null) as traffic_requests
from http_access_requests r join http_normalized_routes n
 on r.run_id=n.run_id and r.uri is not distinct from n.uri
group by all;

-- Normalize only operation-related requests once for progress and concurrency.
create or replace temp table operation_requests as
with configs as (
    select json_extract_string(value,'$.run_id') as run_id,
           json_extract(value,'$.config') as config
    from json_each(coalesce(getvariable('operation_definitions')::varchar,'[]'))
), paths as materialized (
    select distinct r.run_id, split_part(r.uri,'?',1) as path
    from http_access_requests r
    where list_contains(getvariable('operation_methods')::varchar[],r.method)
), normalized as materialized (
    select p.run_id, p.path,
           (select json_extract_string(route.value,'$.name')
            from json_each(json_extract(c.config,'$.routes')) route
            where regexp_matches(p.path,json_extract_string(route.value,'$.pattern'))
            order by route.key::integer limit 1) as route
    from paths p join configs c using(run_id)
)
select r.run_id,r.event_id,r.identity_key,r.method,r.status,n.route,
       r.logged_at_epoch,r.response_time_sec
from http_access_requests r join normalized n
 on n.run_id=r.run_id and n.path=split_part(r.uri,'?',1)
where list_contains(getvariable('operation_methods')::varchar[],r.method)
 and r.valid_timing and to_timestamp(r.logged_at_epoch)<=r.analysis_ended_at;

-- Selected operations can have other API calls between them. Definitions are
-- captured with the RUN, rather than read from today's contest configuration.
create or replace view operation_progress as
with configs as (
    select json_extract_string(value,'$.run_id') as run_id,
           json_extract(value,'$.config') as config
    from json_each(coalesce(getvariable('operation_definitions')::varchar,'[]'))
), pairs as (
    select c.run_id, c.config, p.value as definition,
           json_extract_string(p.value,'$.name') as name
    from configs c, json_each(json_extract(c.config,'$.operation_pairs')) p
), matched as (
    select r.*, p.name, p.definition,
           r.method=json_extract_string(p.definition,'$.from.method')
             and r.route=json_extract_string(p.definition,'$.from.route')
             and case when coalesce(json_array_length(p.definition,'$.from.statuses'),0)=0
                 then r.status between 200 and 299
                 else list_contains(from_json(json_extract(p.definition,'$.from.statuses'),'["INTEGER"]'),r.status::integer) end as is_from,
           r.method=json_extract_string(p.definition,'$.to.method')
             and r.route=json_extract_string(p.definition,'$.to.route')
             and case when coalesce(json_array_length(p.definition,'$.to.statuses'),0)=0
                 then r.status between 200 and 299
                 else list_contains(from_json(json_extract(p.definition,'$.to.statuses'),'["INTEGER"]'),r.status::integer) end as is_to
    from operation_requests r join pairs p using(run_id)
), ordered as (
    select *, row_number() over (
        partition by run_id,name,identity_key
        order by logged_at_epoch-response_time_sec,logged_at_epoch,event_id
    ) as seq
    from matched where identity_key is not null and (is_from or is_to)
), following as (
    select *,
        min(seq) filter(where is_from) over w as next_from,
        first_value(case when is_to then struct_pack(seq:=seq,started_at:=logged_at_epoch-response_time_sec) end ignore nulls)
            over w as target
    from ordered
    window w as (partition by run_id,name,identity_key order by seq rows between 1 following and unbounded following)
), outcomes as (
    select *, case
        when target is not null and (next_from is null or target.seq<=next_from)
            then case when target.started_at<=logged_at_epoch-response_time_sec
                            or target.started_at<logged_at_epoch then 'overlap' else 'reached' end
        when next_from is not null then 'superseded'
        else 'not_observed' end as outcome,
        (target.started_at-logged_at_epoch)*1000 as gap_ms
    from following where is_from
), unidentified as (
    select run_id,name,count(*) as unidentified_from_events
    from matched where is_from and identity_key is null group by run_id,name
), aggregates as (
    select run_id,name,count(*) as from_events,
        count(distinct identity_key) as from_sessions,
        count(*) filter(where outcome='reached') as reached_events,
        count(distinct identity_key) filter(where outcome='reached') as reached_sessions,
        count(*) filter(where outcome='not_observed') as not_observed_events,
        count(*) filter(where outcome='superseded') as superseded_events,
        count(*) filter(where outcome='overlap') as overlap_events,
        avg(gap_ms) filter(where outcome='reached') as gap_avg_ms,
        quantile_cont(gap_ms,0.5) filter(where outcome='reached') as gap_p50_ms,
        quantile_cont(gap_ms,0.95) filter(where outcome='reached') as gap_p95_ms,
        count(*) filter(where outcome='reached' and gap_ms<1) as gap_lt_1ms,
        count(*) filter(where outcome='reached' and gap_ms>=1 and gap_ms<10) as gap_1_10ms,
        count(*) filter(where outcome='reached' and gap_ms>=10 and gap_ms<100) as gap_10_100ms,
        count(*) filter(where outcome='reached' and gap_ms>=100 and gap_ms<1000) as gap_100_1000ms,
        count(*) filter(where outcome='reached' and gap_ms>=1000 and gap_ms<10000) as gap_1_10s,
        count(*) filter(where outcome='reached' and gap_ms>=10000) as gap_ge_10s
    from outcomes group by run_id,name
), windows as (
    select json_extract_string(value,'$.run_id') as run_id,
        try_cast(json_extract_string(value,'$.started_at') as timestamptz) as window_started_at,
        try_cast(json_extract_string(value,'$.ended_at') as timestamptz) as window_ended_at
    from json_each(coalesce(getvariable('analysis_windows')::varchar,'[]'))
    where json_extract_string(value,'$.status')='ok'
)
select p.run_id,p.name,p.definition,w.window_started_at,w.window_ended_at,
    coalesce(a.from_events,0) as from_events,coalesce(a.from_sessions,0) as from_sessions,
    coalesce(u.unidentified_from_events,0) as unidentified_from_events,
    coalesce(a.reached_events,0) as reached_events,coalesce(a.reached_sessions,0) as reached_sessions,
    coalesce(a.not_observed_events,0) as not_observed_events,
    coalesce(a.superseded_events,0) as superseded_events,coalesce(a.overlap_events,0) as overlap_events,
    a.gap_avg_ms,a.gap_p50_ms,a.gap_p95_ms,
    coalesce(a.gap_lt_1ms,0) as gap_lt_1ms,coalesce(a.gap_1_10ms,0) as gap_1_10ms,
    coalesce(a.gap_10_100ms,0) as gap_10_100ms,coalesce(a.gap_100_1000ms,0) as gap_100_1000ms,
    coalesce(a.gap_1_10s,0) as gap_1_10s,coalesce(a.gap_ge_10s,0) as gap_ge_10s
from pairs p join windows w using(run_id) left join aggregates a using(run_id,name)
left join unidentified u using(run_id,name);

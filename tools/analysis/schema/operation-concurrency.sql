-- An interval is the observed state from a successful start response to the
-- corresponding end response. One session may have one active interval per
-- definition. Unknown starts and ambiguous/repeated events mark the result partial.
create or replace view operation_concurrency as
with definitions as (
    select json_extract_string(d.value,'$.run_id') as run_id,
           json_extract_string(i.value,'$.name') as name,i.value as definition
    from json_each(coalesce(getvariable('operation_definitions')::varchar,'[]')) d,
         json_each(json_extract(d.value,'$.config.operation_intervals')) i
), windows as (
    select json_extract_string(value,'$.run_id') as run_id,
           try_cast(json_extract_string(value,'$.started_at') as timestamptz) as window_started_at,
           try_cast(json_extract_string(value,'$.ended_at') as timestamptz) as window_ended_at
    from json_each(coalesce(getvariable('analysis_windows')::varchar,'[]'))
    where json_extract_string(value,'$.status')='ok'
), matched as (
    select r.*,d.name,
        r.method=json_extract_string(d.definition,'$.start.method')
          and r.route=json_extract_string(d.definition,'$.start.route')
          and case when coalesce(json_array_length(d.definition,'$.start.statuses'),0)=0
            then r.status between 200 and 299
            else list_contains(from_json(json_extract(d.definition,'$.start.statuses'),'["INTEGER"]'),r.status::integer) end as is_start,
        r.method=json_extract_string(d.definition,'$.end.method')
          and r.route=json_extract_string(d.definition,'$.end.route')
          and case when coalesce(json_array_length(d.definition,'$.end.statuses'),0)=0
            then r.status between 200 and 299
            else list_contains(from_json(json_extract(d.definition,'$.end.statuses'),'["INTEGER"]'),r.status::integer) end as is_end
    from operation_requests r join definitions d using(run_id)
), events as (
    select *,lag(is_start) over w as previous_start,
        lag(logged_at_epoch) over w as previous_at,
        min(logged_at_epoch) filter(where is_end) over (
            partition by run_id,name,identity_key order by logged_at_epoch,event_id
            rows between 1 following and unbounded following
        ) as next_end
    from matched where identity_key is not null and (is_start or is_end)
    window w as (partition by run_id,name,identity_key order by logged_at_epoch,event_id)
), intervals as (
    select e.run_id,e.name,e.logged_at_epoch as begins,
        coalesce(e.next_end,epoch(w.window_ended_at)) as ends,e.next_end is null as open_ended
    from events e join windows w using(run_id)
    where is_start and previous_start is distinct from true
), event_quality as (
    select run_id,name,
        count(*) filter(where is_end and previous_start is distinct from true) as orphan_ends,
        count(*) filter(where is_start and previous_start) as repeated_starts,
        count(*) filter(where logged_at_epoch=previous_at) as ambiguous_events
    from events group by run_id,name
), missing_identity as (
    select run_id,name,count(*) as unidentified_events
    from matched where identity_key is null and (is_start or is_end) group by run_id,name
), interval_counts as (
    select run_id,name,count(*) as started_intervals,
        count(*) filter(where not open_ended) as closed_intervals,
        count(*) filter(where open_ended) as open_intervals
    from intervals group by run_id,name
), changes as (
    select run_id,name,begins as point_at,1 as delta from intervals where ends>begins
    union all select run_id,name,ends,-1 from intervals where ends>begins
    union all select d.run_id,d.name,epoch(w.window_started_at),0 from definitions d join windows w using(run_id)
    union all select d.run_id,d.name,epoch(w.window_ended_at),0 from definitions d join windows w using(run_id)
), points as (
    select run_id,name,point_at,sum(delta) as delta from changes group by run_id,name,point_at
), spans as (
    select run_id,name,point_at,lead(point_at) over w as until,
        sum(delta) over (partition by run_id,name order by point_at rows unbounded preceding) as concurrent
    from points window w as (partition by run_id,name order by point_at)
), totals as (
    select run_id,name,max(concurrent) as peak_concurrent,
        sum(concurrent*(until-point_at)) as active_person_seconds,
        sum(case when concurrent=0 then until-point_at else 0 end) as observed_idle_seconds
    from spans where until>point_at group by run_id,name
), result as (
    select d.*,w.window_started_at,w.window_ended_at,
        epoch(w.window_ended_at-w.window_started_at) as window_seconds,
        coalesce(c.started_intervals,0) as started_intervals,
        coalesce(c.closed_intervals,0) as closed_intervals,
        coalesce(c.open_intervals,0) as open_intervals,
        coalesce(q.orphan_ends,0) as orphan_ends,
        coalesce(q.repeated_starts,0) as repeated_starts,
        coalesce(q.ambiguous_events,0) as ambiguous_events,
        coalesce(m.unidentified_events,0) as unidentified_events,
        t.peak_concurrent,t.active_person_seconds,t.observed_idle_seconds
    from definitions d join windows w using(run_id)
    left join interval_counts c using(run_id,name)
    left join event_quality q using(run_id,name)
    left join missing_identity m using(run_id,name)
    left join totals t using(run_id,name)
)
select *,active_person_seconds/nullif(window_seconds,0) as avg_concurrent,
    case when orphan_ends+repeated_starts+ambiguous_events+unidentified_events=0 then observed_idle_seconds end as idle_seconds,
    active_person_seconds/nullif(peak_concurrent*window_seconds,0) as observed_peak_occupancy_ratio,
    case when orphan_ends+repeated_starts+ambiguous_events+unidentified_events=0 then 'ok' else 'partial' end as status
from result;

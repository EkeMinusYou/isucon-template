-- Logger counters share the ordinary runtime collector and the load window.
-- Missing metrics remain NULL; pending output is not classified as dropped.
create or replace view access_log_health as
with selected as (
    select *,max(value) filter(where metric='go_process_started_unix_ms')
        over(partition by run_id,host,ts) as process_started
    from metrics where source='runtime'
), samples as (
    select *,lag(value) over w as previous_value,
        lag(ts) over w as previous_at,lag(process_started) over w as previous_process
    from selected where metric in (
        'access_log_enqueued_total','access_log_written_total',
        'access_log_dropped_total','access_log_queue_length'
    )
    window w as (partition by run_id,host,metric order by ts)
), deltas as (
    select *,case when process_started=previous_process and value>=previous_value
        and ts>previous_at then value-previous_value end as delta
    from samples
)
select run_id,host,count(distinct ts) as observations,
    min(ts) as first_observed_at,max(ts) as last_observed_at,
    sum(delta) filter(where metric='access_log_enqueued_total') as enqueued_delta,
    sum(delta) filter(where metric='access_log_written_total') as written_delta,
    sum(delta) filter(where metric='access_log_dropped_total') as dropped_delta,
    max(value) filter(where metric='access_log_queue_length') as queue_max,
    last(value order by ts) filter(where metric='access_log_queue_length') as queue_last,
    count(*) filter(where ends_with(metric,'_total') and previous_process=process_started and value<previous_value) as unexpected_resets
from deltas group by run_id,host;

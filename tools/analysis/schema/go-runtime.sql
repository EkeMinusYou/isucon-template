-- Scalars and counters are observed only inside load_windows. Allocation rates
-- are runtime counter deltas, not sums of cumulative memory-profile snapshots.
create or replace view go_runtime_series as
with runtime as (
 select m.*,max(value) filter(where metric='go_process_started_unix_ms')
  over(partition by run_id,host,ts) as process_started_ms
 from metrics m where source='sql_pool' and starts_with(metric,'go_')
), previous as (
 select *,lag(value) over w as previous_value,lag(ts) over w as previous_at,
  lag(process_started_ms) over w as previous_process_started_ms
 from runtime window w as(partition by run_id,host,metric order by ts)
)
select *,ends_with(metric,'_total') as counter,
 case when ends_with(metric,'_total') and ts>previous_at
  and process_started_ms=previous_process_started_ms and value>=previous_value
  then value-previous_value end as delta,
 case when ends_with(metric,'_total') and ts>previous_at
  and process_started_ms=previous_process_started_ms and value>=previous_value
  then (value-previous_value)/epoch(ts-previous_at) end as per_second
from previous;

create or replace view go_runtime_window as
select s.run_id,s.host,s.metric,count(*) as observations,
 min(s.ts) as first_observed_at,max(s.ts) as last_observed_at,
 case when not s.counter then avg(s.value) end as mean_observed,
 case when not s.counter then max(s.value) end as max_observed,
 sum(s.delta) as measured_delta,
 sum(epoch(s.ts-s.previous_at)) filter(where s.delta is not null) as measured_seconds,
 sum(s.delta)/nullif(sum(epoch(s.ts-s.previous_at)) filter(where s.delta is not null),0) as measured_per_second
from go_runtime_series s group by s.run_id,s.host,s.metric,s.counter;

-- A point is a sampled state, not waiting duration. The allocation sample types
-- are differences between adjacent snapshots wholly within the load window;
-- memory stack accounting may lag by GC cycles, unlike runtime byte counters.
create or replace view go_profile_function_window as
with denominators as (
 select run_id,host,profile_type,sample_type,value_unit,count(*) as observations,
  sum((interval_end_ns-interval_start_ns)/1e9) as measured_seconds
 from go_profile_points where valid group by all
), function_values as (
 select run_id,host,profile_type,sample_type,value_unit,function,file,line,
  sum(flat_value) as flat_sum,sum(cumulative_value) as cumulative_sum,
  max(cumulative_value) as max_observed
 from go_profile_point_functions where valid group by all
)
select v.*,d.observations,d.measured_seconds,
 case when ends_with(v.sample_type,'_delta') then v.flat_sum/nullif(d.measured_seconds,0) end as allocated_per_second,
 case when not ends_with(v.sample_type,'_delta') then v.cumulative_sum/d.observations end as mean_observed
from function_values v join denominators d using(run_id,host,profile_type,sample_type,value_unit);

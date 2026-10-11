-- fgprof views for `analysisctl profile`. Values are goroutine wall-clock
-- seconds, not CPU time.

create or replace view bottleneck_profile_functions as
select
    f.run_id,
    f.host,
    f.function,
    f.file,
    f.line,
    f.flat_wall_seconds,
    f.cumulative_wall_seconds,
    f.flat_wall_seconds / nullif(m.total_wall_seconds, 0) as flat_profile_share,
    f.cumulative_wall_seconds / nullif(m.total_wall_seconds, 0) as cumulative_profile_share,
    row_number() over (partition by f.run_id, f.host order by f.cumulative_wall_seconds desc, f.function) as cumulative_rank,
    'wall_seconds' as unit
from profile_functions f
join profile_metadata m using (run_id, host);

create or replace view bottleneck_profile_edges as
select
    e.run_id,
    e.host,
    e.caller,
    e.callee,
    e.wall_seconds,
    e.sample_occurrences,
    e.wall_seconds / nullif(m.total_wall_seconds, 0) as profile_share,
    row_number() over (partition by e.run_id, e.host order by e.wall_seconds desc, e.caller, e.callee) as edge_rank,
    'wall_seconds' as unit
from profile_edges e
join profile_metadata m using (run_id, host);

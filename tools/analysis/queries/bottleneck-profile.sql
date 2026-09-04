.mode box
.maxrows 120
select function, file, line, flat_wall_seconds, cumulative_wall_seconds,
       flat_profile_share, cumulative_profile_share, cumulative_rank, unit
from bottleneck_profile_functions
where run_id = getvariable('run_id') and host = getvariable('host_name')
  and regexp_matches(function, getvariable('symbol_pattern'))
order by cumulative_wall_seconds desc, function
limit 80;

.print '== CALLER/CALLEE EDGES =='
select caller, callee, wall_seconds, sample_occurrences, profile_share, edge_rank, unit
from bottleneck_profile_edges
where run_id = getvariable('run_id') and host = getvariable('host_name')
  and (regexp_matches(caller, getvariable('symbol_pattern')) or regexp_matches(callee, getvariable('symbol_pattern')))
order by wall_seconds desc, caller, callee
limit 120;


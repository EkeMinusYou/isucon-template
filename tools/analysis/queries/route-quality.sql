-- Classification gaps and count reconciliation are not performance effects.
select q.run_id,q.ingress_host,q.status,q.requests,q.traffic_requests
from route_coverage q
where q.run_id in (getvariable('base_run'),getvariable('new_run'))
order by q.run_id,q.ingress_host,q.status;

with expected as (
 select run_id,ingress_host,sum(traffic_requests) as expected_requests
 from route_coverage group by run_id,ingress_host
), actual as (
 select run_id,ingress_host,sum(requests) as aggregated_requests
 from http_traffic group by run_id,ingress_host
)
select e.*,a.aggregated_requests,a.aggregated_requests-e.expected_requests as count_delta
from expected e left join actual a using(run_id,ingress_host)
where e.run_id in (getvariable('base_run'),getvariable('new_run'));

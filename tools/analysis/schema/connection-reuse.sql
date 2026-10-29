-- Counts describe observed requests inside the load window, not a connection's
-- full lifetime. Connection IDs are scoped to one host/process epoch.
create or replace view connection_reuse as
with connections as (
    select run_id,ingress_host,connection_id,count(*) as observed_requests,
           min(connection_requests) as first_request_number,
           max(connection_requests) as last_request_number
    from http_access_requests
    where connection_id is not null
    group by all
), totals as (
    select run_id,ingress_host,count(*) as requests,
           count(*) filter(where connection_id is null) as requests_without_connection,
           count(*) filter(where connection_id is not null and connection_requests is null) as requests_without_number
    from http_access_requests group by all
), stats as (
    select run_id,ingress_host,count(*) as observed_connections,
        count(*) filter(where observed_requests>1) as reused_connections,
        count(*) filter(where first_request_number=1) as connections_with_first_request,
        avg(observed_requests) as requests_per_connection_avg,
        quantile_cont(observed_requests,0.5) as requests_per_connection_p50,
        quantile_cont(observed_requests,0.95) as requests_per_connection_p95,
        max(observed_requests) as requests_per_connection_max
    from connections group by run_id,ingress_host
)
select t.*,coalesce(s.observed_connections,0) as observed_connections,
    coalesce(s.reused_connections,0) as reused_connections,
    coalesce(s.connections_with_first_request,0) as connections_with_first_request,
    s.requests_per_connection_avg,s.requests_per_connection_p50,
    s.requests_per_connection_p95,s.requests_per_connection_max
from totals t left join stats s using(run_id,ingress_host);

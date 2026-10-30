-- The shared access-log table is already restricted to the derived load window.
-- These are bounded OS samples; missing and unsupported samples have no rows.
create or replace view tcp_samples as
select run_id,ingress_host,connection_id,connection_requests,protocol,
    to_timestamp(logged_at_epoch) as ts,
    tcp.rtt_us as rtt_us,tcp.rtt_variance_us as rtt_variance_us,
    tcp.total_retransmits as total_retransmits
from http_access_requests where tcp is not null;

-- A direct local request has no upstream row, rather than a fabricated zero.
create or replace view upstream_requests as
select r.run_id,r.ingress_host,r.connection_id,r.method,
    coalesce(g.matched_route,case when g.is_api then '<unclassified-api>' else g.path end) as route,
    to_timestamp(r.logged_at_epoch) as ts,
    u.address,u.reused,u.connect_seconds,u.tls_seconds,u.header_seconds,
    u.total_seconds,u.body_bytes,u.error
from http_access_requests r
left join http_normalized_routes g on g.run_id=r.run_id and g.uri is not distinct from r.uri,
unnest(r.upstream) as spans(u);

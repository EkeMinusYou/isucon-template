package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGoTransportSamplesAndHTTP2TimingRemainDistinct(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs", "20260101-120000")
	writeTestFile(t, dir, "raw/access-entry.log", `{"msec":1001,"response_time":0.1,"method":"GET","uri":"/api/item","status":200,"protocol":"HTTP/2.0","connection":1,"connection_requests":1,"tcp":{"rtt_us":300,"rtt_variance_us":20,"total_retransmits":0},"upstream":[{"address":"host:80","reused":true,"connect_seconds":0,"tls_seconds":0,"header_seconds":0.01,"total_seconds":0.02,"body_bytes":8}]}
{"msec":1001.1,"response_time":0.2,"method":"GET","uri":"/api/item","status":200,"protocol":"HTTP/2.0","connection":1,"connection_requests":2}
{"msec":1002,"response_time":0.1,"method":"GET","uri":"/api/item","status":200,"protocol":"HTTP/1.1","connection":2,"connection_requests":1}
{"msec":1002.5,"response_time":0.1,"method":"GET","uri":"/api/item","status":200,"protocol":"HTTP/1.1","connection":2,"connection_requests":2}
{"msec":1021,"response_time":0.1,"method":"GET","uri":"/api/item","status":200,"protocol":"HTTP/1.1","tcp":{"rtt_us":900,"total_retransmits":3}}
`)
	script := "set variable run_glob=" + sqlString(filepath.ToSlash(dir)) + ";set variable analysis_windows='[{\"run_id\":\"20260101-120000\",\"status\":\"ok\",\"started_at\":\"1970-01-01T00:16:40Z\",\"ended_at\":\"1970-01-01T00:17:00Z\"}]';\n" + metricSchema(t, "access-log.sql", "connection-reuse.sql", "transport.sql") + `
select count(*),min(rtt_us) from tcp_samples;
select count(*),sum(body_bytes) from upstream_requests;
select sum(client_gap_samples),round(sum(sum_client_gap_sec),1) from http_traffic_buckets;
select observed_connections,reused_connections from connection_reuse;`
	got := runMetricSQL(t, script)
	if got != "1|300\n1|8\n1|0.4\n2|2" {
		t.Fatalf("transport and protocol analysis:\n%s", got)
	}
	if strings.Contains(got, "900") {
		t.Fatal("out-of-window sample imported")
	}
}

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTTPTrafficPreservesRoutesBucketsAndLogSelection(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	zstd, err := exec.LookPath("zstd")
	if err != nil {
		t.Skip("zstd is not installed")
	}
	schema, err := os.ReadFile("../analysis/schema/access-log.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Use overlapping generic routes to check that the first pattern wins.
	requests := strings.Index(string(schema), "\nrequests as ")
	if requests < 0 {
		t.Fatal("access log schema has no requests CTE")
	}
	schema = []byte("create or replace view http_traffic_buckets as\nwith routes as (select ['^/items/', '^/items/[^/]+$'] as patterns)," + string(schema[requests:]))
	runDir := filepath.Join(t.TempDir(), "runs", "20260901-120000")
	writeTestFile(t, runDir, "raw/access-entry-a.log", `{"msec":"1800000005.25","response_time":0.5,"method":"GET","uri":"/items/a","status":200,"body_bytes":10,"upstream_time":"0.2"}
{"msec":"1800000004.9","response_time":0.4,"method":"GET","uri":"/items/b","status":404,"body_bytes":20,"upstream_time":"0.001, 0.002"}
{"msec":"1800000005","response_time":0.25,"method":"GET","uri":"/items/a","status":302,"body_bytes":30,"upstream_time":null}
{"msec":"1800000006","response_time":null,"method":"POST","uri":"/outside","status":500,"body_bytes":null,"upstream_time":null}
{"msec":"1800000008","response_time":1,"method":"GET","uri":null,"status":999}
{"msec":null,"response_time":0,"method":"GET","uri":"/items/invalid","status":200}
{malformed
`)
	compress := func(name, data string) {
		t.Helper()
		path := filepath.Join(runDir, "raw", name)
		cmd := exec.Command(zstd, "-q", "-o", path)
		cmd.Stdin = strings.NewReader(data)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("compress fixture: %v: %s", err, output)
		}
	}
	// Plain and compressed copies must not double count one ingress host.
	compress("access-entry-a.log.zst", `{"msec":"1800000006","method":"GET","uri":"/ignored","status":503}`+"\n")
	compress("access-entry-b.log.zst", `{"msec":"1800000007","response_time":2,"method":"GET","uri":"/outside","status":503,"body_bytes":40,"upstream_time":"0.3"}`+"\n")
	query := "set variable run_glob = " + sqlString(filepath.ToSlash(runDir)) + ";\n" + string(schema) + `
create temp table actual as
select run_id, ingress_host, epoch(ts)::bigint as bucket, bucket_seconds,
       method, route, requests, status_2xx, status_3xx, status_4xx, status_5xx,
       round(sum_time_sec, 6) as sum_time_sec, max_time_sec,
       round(sum_upstream_time_sec, 6) as sum_upstream_time_sec,
       upstream_time_samples, sum_body_bytes
from http_traffic_buckets;
create temp table expected as
select * from (values
 ('20260901-120000', 'entry-a', 1800000000, 5, 'GET', '^/items/', 3, 1, 1, 1, 0, 1.15, 0.5, 0.2, 1, 60),
 ('20260901-120000', 'entry-a', 1800000005, 5, 'POST', '/outside', 1, 0, 0, 0, 1, 0, 0, NULL, 0, 0),
 ('20260901-120000', 'entry-a', 1800000005, 5, 'GET', NULL, 1, 0, 0, 0, 0, 1, 1, NULL, 0, 0),
 ('20260901-120000', 'entry-b', 1800000005, 5, 'GET', '/outside', 1, 0, 0, 0, 1, 2, 2, 0.3, 1, 40)
);
select count(*) from (
 (select * from actual except all select * from expected)
 union all
 (select * from expected except all select * from actual)
);
`
	cmd := exec.Command(duckdb, "-noheader", "-list")
	cmd.Stdin = strings.NewReader(query)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("aggregate access logs: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != "0" {
		t.Fatalf("traffic aggregate differs from expected rows: %s", output)
	}
}

func TestHTTPTrafficCountsConnectionsAndClientGaps(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	schema, err := os.ReadFile("../analysis/schema/access-log.sql")
	if err != nil {
		t.Fatal(err)
	}
	requests := strings.Index(string(schema), "\nrequests as ")
	if requests < 0 {
		t.Fatal("access log schema has no requests CTE")
	}
	schema = []byte("create or replace view http_traffic_buckets as\nwith routes as (select ['^/a$'] as patterns)," + string(schema[requests:]))
	runDir := filepath.Join(t.TempDir(), "runs", "20260901-120000")
	// Connection 7 carries two requests; connection 8 resumes TLS; the last line
	// predates the connection fields and must not join any connection.
	writeTestFile(t, runDir, "raw/access-entry-a.log", `{"msec":"1800000000.1","response_time":0.1,"method":"GET","uri":"/a","status":200,"connection":"7","connection_requests":"1","ssl_session_reused":"."}
{"msec":"1800000000.5","response_time":0.2,"method":"GET","uri":"/a","status":200,"connection":"7","connection_requests":"2","ssl_session_reused":"."}
{"msec":"1800000001","response_time":0.1,"method":"GET","uri":"/a","status":200,"connection":"8","connection_requests":"1","ssl_session_reused":"r"}
{"msec":"1800000002","response_time":0.1,"method":"GET","uri":"/a","status":200}
`)
	query := "set variable run_glob = " + sqlString(filepath.ToSlash(runDir)) + ";\n" + string(schema) + `
select requests, connection_samples, new_connections, resumed_tls_connections,
       round(sum_client_gap_sec, 6), client_gap_samples
from http_traffic_buckets;
`
	cmd := exec.Command(duckdb, "-noheader", "-list")
	cmd.Stdin = strings.NewReader(query)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("aggregate access logs: %v: %s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "4|3|2|1|0.2|1" {
		t.Fatalf("connection aggregate = %q, want 4|3|2|1|0.2|1", got)
	}
}

func TestHTTPImportFiltersRequestsBeforeBucketing(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb not installed")
	}
	schema, err := os.ReadFile("../analysis/schema/access-log.sql")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "runs", "20260901-120000")
	writeTestFile(t, dir, "raw/access-host.log", `{"msec":"1800000000.9","response_time":0.1,"method":"GET","uri":"/outside","status":200}
{"msec":"1800000001.2","response_time":0.1,"method":"GET","uri":"/inside","status":200}
{"msec":"1800000004.2","response_time":0.1,"method":"GET","uri":"/outside","status":200}
`)
	script := "set variable run_glob=" + sqlString(filepath.ToSlash(dir)) + ";set variable analysis_windows=" + sqlString(`[{"run_id":"20260901-120000","started_at":"2027-01-15T08:00:01Z","ended_at":"2027-01-15T08:00:04Z","status":"ok"}]`) + ";\n" + string(schema) + "\nselect route,requests,bucket_seconds from http_traffic_buckets;"
	// Use epoch-derived timestamps instead of relying on a hand-converted date.
	script = strings.ReplaceAll(script, "2027-01-15T08:00:01Z", time.Unix(1800000001, 0).UTC().Format(time.RFC3339))
	script = strings.ReplaceAll(script, "2027-01-15T08:00:04Z", time.Unix(1800000004, 0).UTC().Format(time.RFC3339))
	cmd := exec.Command(duckdb, "-noheader", "-list")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("query: %v: %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "/inside|1|3.0" {
		t.Fatalf("outside requests leaked: %s", out)
	}
}

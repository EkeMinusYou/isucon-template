package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runMetricSQL(t *testing.T, script string) string {
	t.Helper()
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb not installed")
	}
	cmd := exec.Command(duckdb, "-noheader", "-list")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("query: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func metricSchema(t *testing.T, names ...string) string {
	t.Helper()
	var s strings.Builder
	for _, name := range names {
		body, err := os.ReadFile("../analysis/schema/" + name)
		if err != nil {
			t.Fatal(err)
		}
		s.Write(body)
		s.WriteByte('\n')
	}
	return s.String()
}

func TestOperationConcurrencyAndIncompleteObservations(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs", "20260901-120000")
	var log strings.Builder
	add := func(at int, method, route, session string, status int) {
		row, _ := json.Marshal(map[string]any{"msec": at, "response_time": 0, "method": method, "uri": route, "session_id": session, "status": status})
		log.Write(row)
		log.WriteByte('\n')
	}
	add(1001, "POST", "/api/start", "a", 201)
	add(1003, "POST", "/api/start", "b", 201)
	add(1005, "DELETE", "/api/end", "a", 204)
	add(1007, "DELETE", "/api/end", "b", 204)
	add(1010, "POST", "/api/start", "c", 201)
	add(1021, "DELETE", "/api/end", "c", 204) // Right-censored at the load end.
	add(1009, "POST", "/api/start", "failed", 500)
	// Partial observations must not assert that all remaining time was idle.
	add(999, "POST", "/api/unknown-start", "before", 201)
	add(1004, "DELETE", "/api/unknown-end", "before", 204)
	add(1008, "POST", "/api/unknown-start", "", 201)
	add(1011, "POST", "/api/unknown-start", "repeat", 201)
	add(1012, "POST", "/api/unknown-start", "repeat", 201)
	add(1014, "DELETE", "/api/unknown-end", "repeat", 204)
	writeTestFile(t, dir, "raw/access-entry.log", log.String())
	routes := []map[string]string{}
	for _, path := range []string{"/api/start", "/api/end", "/api/unknown-start", "/api/unknown-end"} {
		routes = append(routes, map[string]string{"name": path, "pattern": "^" + path + "$"})
	}
	intervals := []map[string]any{}
	for _, d := range []struct{ name, start, end string }{{"active", "/api/start", "/api/end"}, {"partial", "/api/unknown-start", "/api/unknown-end"}} {
		intervals = append(intervals, map[string]any{"name": d.name, "start": map[string]string{"method": "POST", "route": d.start}, "end": map[string]string{"method": "DELETE", "route": d.end}})
	}
	defs, _ := json.Marshal([]any{map[string]any{"run_id": "20260901-120000", "config": map[string]any{"routes": routes, "operation_intervals": intervals}}})
	window := fmt.Sprintf(`[{"run_id":"20260901-120000","status":"ok","started_at":%q,"ended_at":%q}]`, time.Unix(1000, 0).UTC().Format(time.RFC3339), time.Unix(1020, 0).UTC().Format(time.RFC3339))
	script := "set variable run_glob=" + sqlString(filepath.ToSlash(dir)) + ";set variable analysis_windows=" + sqlString(window) + ";set variable operation_definitions=" + sqlString(string(defs)) + ";\n" +
		metricSchema(t, "access-log.sql", "operation-progress.sql", "operation-concurrency.sql") + `
select name,started_intervals,closed_intervals,open_intervals,peak_concurrent,avg_concurrent,
 active_person_seconds,idle_seconds,round(observed_peak_occupancy_ratio,2),status
from operation_concurrency where name='active';
select name,orphan_ends,repeated_starts,unidentified_events,idle_seconds,status
from operation_concurrency where name='partial';`
	got := runMetricSQL(t, script)
	want := "active|3|2|1|2|0.9|18.0|4.0|0.45|ok\npartial|1|1|1|NULL|partial"
	if got != want {
		t.Fatalf("concurrency:\n%s\nwant:\n%s", got, want)
	}
}

func TestRequestBytesKeepActualDeclaredAndMissingSeparate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs", "20260901-120000")
	writeTestFile(t, dir, "raw/access-entry.log", `{"msec":1001,"response_time":0,"method":"POST","uri":"/upload","status":200,"request_body_bytes":4,"request_content_length":6}
{"msec":1002,"response_time":0,"method":"POST","uri":"/upload","status":200,"request_body_bytes":10,"request_content_length":-1}
{"msec":1003,"response_time":0,"method":"POST","uri":"/upload","status":200,"request_content_length":8}
{"msec":1004,"response_time":0,"method":"POST","uri":"/upload","status":200}
{"msec":1004,"response_time":0,"method":"GET","uri":"/legacy","status":200}
`)
	script := "set variable run_glob=" + sqlString(filepath.ToSlash(dir)) + ";\n" + metricSchema(t, "access-log.sql") + `
select route,requests,sum_request_body_bytes,request_body_samples,sum_request_content_length,request_content_length_samples
from http_traffic_buckets order by route;`
	want := "/legacy|1|NULL|0|NULL|0\n/upload|4|14.0|2|14.0|2"
	if got := runMetricSQL(t, script); got != want {
		t.Fatalf("body sizes: %s", got)
	}
}

func TestLogHealthIgnoresRestartAndKeepsMissingCountersNull(t *testing.T) {
	script := `create table metrics(run_id varchar,host varchar,source varchar,ts timestamptz,metric varchar,value double);
insert into metrics select 'run','a','runtime',to_timestamp(n),metric,value
from (values (0),(1),(2),(3),(4)) times(n), lateral (
 values ('go_process_started_unix_ms',case when n<3 then 100 else 200 end),
 ('access_log_enqueued_total',case when n<3 then n*10 else (n-3)*5 end),
 ('access_log_written_total',case when n<3 then n*9 else (n-3)*5 end),
 ('access_log_dropped_total',0),('access_log_queue_length',case when n<3 then n else 0 end)
) values_by_time(metric,value);
insert into metrics values ('run','b','runtime',to_timestamp(0),'access_log_queue_length',2);
` + metricSchema(t, "access-log-health.sql") + `
select host,enqueued_delta,written_delta,dropped_delta,queue_max,queue_last,unexpected_resets
from access_log_health order by host; select count(output_errors) from access_log_health;`
	want := "a|25.0|23.0|0.0|2.0|0.0|0\nb|NULL|NULL|NULL|2.0|2.0|0\n0"
	if got := runMetricSQL(t, script); got != want {
		t.Fatalf("log health: %s", got)
	}
}

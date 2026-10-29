package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOperationProgressAcrossIntermediateCallsAndLoadBoundaries(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb not installed")
	}
	dir := filepath.Join(t.TempDir(), "runs", "20260901-120000")
	var log strings.Builder
	add := func(start, duration float64, method, path, identity string, status int, conn any, number any) {
		row := map[string]any{"msec": start + duration, "response_time": duration, "method": method, "uri": path,
			"session_id": identity, "status": status, "connection": conn, "connection_requests": number}
		body, _ := json.Marshal(row)
		log.Write(body)
		log.WriteByte('\n')
	}
	// Only successful, fully observed operations within [1000,1020) count.
	add(999, 0.1, "DELETE", "/api/exit/1", "outside", 200, 1, 1)
	add(1001, 0.1, "DELETE", "/api/exit/1", "a", 200, 1, 2)
	add(1002, 0.1, "GET", "/api/search", "a", 200, 1, 3)
	add(1004, 0.1, "POST", "/api/enter/2", "a", 200, 1, 4)
	add(1005, 0.1, "DELETE", "/api/exit/1", "b", 200, 2, 1)
	add(1006, 0.1, "DELETE", "/api/exit/2", "b", 200, 2, 2)
	add(1008, 0.1, "POST", "/api/enter/3", "b", 200, 2, 3)
	add(1009, 0.1, "DELETE", "/api/exit/1", "c", 200, 3, 8)
	add(1010, 0.1, "POST", "/api/enter/2", "c", 500, 3, 9)
	add(1010, 2, "DELETE", "/api/exit/1", "overlap", 200, 4, 1)
	add(1011, 0.1, "POST", "/api/enter/2", "overlap", 200, 4, 2)
	add(1018, 0.1, "DELETE", "/api/exit/1", "cutoff", 200, 5, 1)
	add(1021, 0.1, "POST", "/api/enter/2", "cutoff", 200, 5, 2)
	add(1014, 0.1, "POST", "/api/enter/4", "a", 200, 1, 5)
	add(1013, 0.1, "DELETE", "/api/exit/1", "", 200, nil, nil)
	writeTestFile(t, dir, "raw/access-entry.log", log.String())
	defs := `[{"run_id":"20260901-120000","config":{"routes":[{"name":"/api/exit/:id","pattern":"^/api/exit/[0-9]+$"},{"name":"/api/enter/:id","pattern":"^/api/enter/[0-9]+$"}],"operation_pairs":[{"name":"next_visit","from":{"method":"DELETE","route":"/api/exit/:id"},"to":{"method":"POST","route":"/api/enter/:id"}},{"name":"next_entry","from":{"method":"POST","route":"/api/enter/:id"},"to":{"method":"POST","route":"/api/enter/:id"}}]}}]`
	window := fmt.Sprintf(`[{"run_id":"20260901-120000","status":"ok","started_at":%q,"ended_at":%q}]`, time.Unix(1000, 0).UTC().Format(time.RFC3339), time.Unix(1020, 0).UTC().Format(time.RFC3339))
	query := "set variable run_glob=" + sqlString(filepath.ToSlash(dir)) + ";set variable analysis_windows=" + sqlString(window) + ";set variable operation_definitions=" + sqlString(defs) + ";\n"
	for _, file := range []string{"access-log.sql", "operation-progress.sql", "connection-reuse.sql"} {
		data, err := os.ReadFile("../analysis/schema/" + file)
		if err != nil {
			t.Fatal(err)
		}
		query += string(data) + "\n"
	}
	query += `select name,from_events,reached_events,not_observed_events,superseded_events,overlap_events,round(gap_p50_ms),round(gap_p95_ms) from operation_progress order by name;
select requests,requests_without_connection,observed_connections,reused_connections,connections_with_first_request,requests_per_connection_max from connection_reuse;
select count(*) from operation_progress where from_events<>reached_events+not_observed_events+superseded_events+overlap_events;
`
	cmd := exec.Command(duckdb, "-noheader", "-list")
	cmd.Stdin = strings.NewReader(query)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("query: %v: %s", err, out)
	}
	want := "next_entry|4|1|3|0|0|9900.0|9900.0\nnext_visit|6|2|2|1|1|2400.0|2850.0\n13|1|5|4|3|4\n0"
	if strings.TrimSpace(string(out)) != want {
		t.Fatalf("unexpected progress or connection counts:\n%s\nwant:\n%s", out, want)
	}

	// The ordinary import must read the saved definitions and materialize only
	// load-window aggregates; this also covers rebuilds of saved RUNs.
	var definitions []struct {
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal([]byte(defs), &definitions); err != nil {
		t.Fatal(err)
	}
	var savedConfig map[string]any
	if err := json.Unmarshal(definitions[0].Config, &savedConfig); err != nil {
		t.Fatal(err)
	}
	savedConfig["operation_intervals"] = []any{map[string]any{
		"name": "visit", "start": map[string]any{"method": "POST", "route": "/api/enter/:id"},
		"end": map[string]any{"method": "DELETE", "route": "/api/exit/:id"},
	}}
	body, _ := json.Marshal(map[string]any{"operation_config": savedConfig})
	writeTestFile(t, dir, "user-transitions.json", string(body))
	stamp := func(sec int64) string { return time.Unix(sec, 0).UTC().Format(time.RFC3339) }
	writeTestFile(t, dir, "bench.log", stamp(998)+"\tBENCHMARK_START\n"+stamp(1000)+"\tload begins\n"+stamp(1020)+"\tload stops\n"+stamp(1022)+"\tBENCHMARK_END\n")
	base := filepath.Join(t.TempDir(), "base.sql")
	os.WriteFile(base, []byte("set variable run_glob = getenv('ISUCON_RUN_GLOB'); create view runs as select '20260901-120000'::varchar as run_id;"), 0600)
	cfg := config{BaseSchema: base, loadPhase: loadPhaseConfig{StartPattern: `\tload begins$`, EndPattern: `\tload stops$`}}
	for _, item := range []struct{ table, schema, view string }{
		{"http_traffic", "access-log.sql", "http_traffic_buckets"},
		{"operation_progress", "operation-progress.sql", "operation_progress"},
		{"operation_concurrency", "operation-concurrency.sql", "operation_concurrency"},
		{"connection_reuse", "connection-reuse.sql", "connection_reuse"},
	} {
		schema, _ := filepath.Abs("../analysis/schema/" + item.schema)
		cfg.Sources = append(cfg.Sources, sourceConfig{Table: item.table, Schema: schema, View: item.view, Match: "raw/access-*.log*"})
	}
	if err := cfg.loadPhase.compile(); err != nil {
		t.Fatal(err)
	}
	r := runner{options: options{db: filepath.Join(t.TempDir(), "analysis.duckdb"), duckdb: duckdb, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}, config: cfg}
	if err := r.syncSelection(filepath.ToSlash(dir), []string{dir}, []string{"20260901-120000"}); err != nil {
		t.Fatal(err)
	}
	got, err := r.duckdbOutput(nil, "-readonly", "-noheader", "-list", r.db, "-c", "select reached_events,unidentified_from_events from operation_progress where name='next_visit'; select count(*) from information_schema.tables where table_name in ('http_access_requests','operation_requests'); select count(*) from operation_concurrency;")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != "2|1\n0\n1" {
		t.Fatalf("saved aggregate or temporary raw data: %s", got)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareRunsReportsMissingDataWithoutZeroFill(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	dir := t.TempDir()
	base, current := "20261028-100000", "20261028-110000"
	for _, id := range []string{base, current} {
		writeTestFile(t, dir, id+"/run.json", `{"phase":"finalized"}`)
	}
	db := filepath.Join(dir, "analysis.duckdb")
	sql := `
 create table ingested(run_id varchar);
 insert into ingested values ('20261028-100000'),('20261028-110000');
 create table manifests as select run_id,run_id='20261028-100000' as passed,
 case when run_id='20261028-100000' then 100 else 150 end as score,
 'commit' as commit, ['app'] as app_hosts,['app'] as app_traffic_hosts,
 ['nginx'] as nginx_hosts,'nginx' as entry_host,'mysql' as mysql_host,
 '{}'::json as additional_roles,false as collectors_disabled,true as profiles_enabled,
 true as collector_clean,['missing.tsv'] as required_artifacts from ingested;
 create table load_windows as select run_id,'ok' as status,'test' as source,
 timestamptz '2026-10-28 00:00:00Z' as started_at,
 timestamptz '2026-10-28 00:01:00Z' as ended_at,60.0 as duration_seconds,'' as reason from ingested;
 create table measurement_quality(run_id varchar,artifact_name varchar,artifact_status varchar,
 quality_status varchar,reason varchar,window_coverage_pct double);
 create table http_traffic as select '20261028-110000' as run_id,'GET' as method,'/new-route' as route,
 10 as requests,0 as status_4xx,0 as status_5xx,0.1 as sum_time_sec,
 timestamptz '2026-10-28 00:00:30Z' as ts;
 create table window_metrics(run_id varchar,source varchar,host varchar,metric varchar,samples bigint,mean_value double);
 `
	if output, err := exec.Command(duckdb, db, "-c", sql).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, output)
	}
	report, err := os.ReadFile("../analysis/queries/compare-runs.sql")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "compare.sql", ".mode json\n"+string(report))
	var out, stderr bytes.Buffer
	r := runner{options: options{db: db, results: dir, duckdb: duckdb, stdout: &out, stderr: &stderr},
		config: config{baseDir: dir, Reports: reportConfig{CompareRuns: "compare.sql"}}}
	if err := r.compareRuns(base, current); err != nil {
		t.Fatalf("compare: %v: %s", err, stderr.String())
	}
	data := out.String()
	decoder := json.NewDecoder(strings.NewReader(data[strings.Index(data, "["):]))
	var score, missing, route bool
	for {
		var rows []map[string]any
		if err := decoder.Decode(&rows); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decode: %v\n%s", err, data)
		}
		for _, row := range rows {
			if v, ok := row["score_delta"]; ok {
				score = v == float64(50) && row["both_passed"] == false
			}
			if row["artifact_name"] == "missing.tsv" {
				missing = row["artifact_status"] == "missing"
			}
			if row["route"] == "/new-route" {
				route = row["base_requests"] == nil && row["requests_delta"] == nil && fmt.Sprint(row["new_requests"]) == "10"
			}
		}
	}
	if !score || !missing || !route {
		t.Fatalf("score=%v missing=%v route=%v\n%s", score, missing, route, data)
	}
	r.checkOnly = true
	r.config.Reports.CompareRuns = "not-yet-installed.sql"
	if err := r.compareRuns(base, current); err != nil {
		t.Fatalf("check-only loaded report: %v", err)
	}
	if _, err := exec.Command(duckdb, db, "-c", "delete from ingested where run_id='"+current+"'").CombinedOutput(); err != nil {
		t.Fatal(err)
	}
	if err := r.compareRuns(base, current); err == nil || !strings.Contains(err.Error(), "not indexed") {
		t.Fatalf("expected missing index: %v", err)
	}
}

func TestCompareRunsRejectsInvalidAndUnfinalizedRuns(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "20261028-100000/run.json", `{"phase":"started"}`)
	r := runner{options: options{results: dir}, config: config{Reports: reportConfig{CompareRuns: "report.sql"}}}
	for _, id := range []string{"../escape", "20261028-100000"} {
		if err := r.compareRuns(id, id); err == nil {
			t.Fatalf("accepted RUN %q", id)
		}
	}
}

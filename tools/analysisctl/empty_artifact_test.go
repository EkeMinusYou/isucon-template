package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A zero-byte artifact in an older RUN must not change a table's columns during a
// full rebuild, or every later RUN fails to insert into it.
func TestEmptyArtifactDoesNotChangeRebuiltColumns(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	cfg, err := loadConfig("../analysis/sources.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	results := filepath.Join(tmp, "runs")
	base, err := os.ReadFile(cfg.path(cfg.BaseSchema))
	if err != nil {
		t.Fatal(err)
	}
	base = []byte(strings.ReplaceAll(string(base), "'runs/", "'"+filepath.ToSlash(results)+"/"))
	writeTestFile(t, tmp, "base.sql", string(base))
	cfg.BaseSchema = filepath.Join(tmp, "base.sql")
	writeTestFile(t, tmp, "runs/scores.tsv", "run_id\tscore\tapp\tnginx\tmysql\tapp_traffic\n")
	header := "upstream_addr\tupstream_status\tcache_status\trequests\tstatus_2xx\tstatus_3xx\tstatus_4xx\tstatus_5xx\tstatus_other\tresponse_time_sum_ms\tresponse_time_avg_ms\tupstream_time_sum_ms\tupstream_time_avg_ms\n"
	row := "127.0.0.1:8080\t200\tMISS\t1\t1\t0\t0\t0\t0\t2.5\t2.5\t1.5\t1.5\n"
	for _, run := range []struct{ id, upstreams string }{{"20260901-120000", header + row}, {"20260901-120100", ""}, {"20260901-120200", header + row}} {
		writeTestFile(t, tmp, "runs/"+run.id+"/run.json", `{"schema_version":4,"run_id":"`+run.id+`","phase":"finalized"}`)
		writeTestFile(t, tmp, "runs/"+run.id+"/bench.log", "2026-09-01T12:00:00Z\tBENCHMARK_START\n2026-09-01T12:01:00Z\tBENCHMARK_END\n")
		writeTestFile(t, tmp, "runs/"+run.id+"/alp.json", "[]\n")
		writeTestFile(t, tmp, "runs/"+run.id+"/slp.tsv", "Count\tQuery\tSum(QueryTime)\tMax(QueryTime)\tP95(QueryTime)\tSum(RowsExamined)\tSum(RowsSent)\tSum(LockTime)\tAvg(LockTime)\n")
		writeTestFile(t, tmp, "runs/"+run.id+"/upstream-breakdown.tsv", run.upstreams)
	}
	r := runner{options: options{db: filepath.Join(tmp, "analysis.duckdb"), results: results, duckdb: duckdb, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}, config: cfg}
	if err := r.rebuildRuns([]string{"20260901-120000", "20260901-120100"}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(results, "20260901-120200")
	if err := r.syncSelection(dir, []string{dir}, []string{"20260901-120200"}); err != nil {
		t.Fatalf("a later RUN must still import: %v", err)
	}
	output, err := r.duckdbOutput(nil, "-noheader", "-list", r.db, "-c", "select count(*) from information_schema.columns where table_name = 'upstreams' and column_name like 'column%'; select count(*) from upstreams")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(output); len(got) != 2 || got[0] != "0" || got[1] != "0" {
		t.Fatalf("upstreams columns and rows: %q", output)
	}
}

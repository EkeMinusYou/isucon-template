package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLPoolHeaderOnlyRunsKeepDeclaredTypes(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	schema, err := filepath.Abs("../analysis/schema/sql-pool.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, emptyFirst := range []bool{false, true} {
		name := "measured_then_empty"
		if emptyFirst {
			name = "empty_then_measured"
		}
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			results := filepath.Join(tmp, "runs")
			writeTestFile(t, tmp, "base.sql", "set variable run_glob = getenv('ISUCON_RUN_GLOB');\ncreate view runs as select 'run-1'::varchar as run_id union all select 'run-2';\n")
			r := runner{
				options: options{db: filepath.Join(tmp, "analysis.duckdb"), results: results, duckdb: duckdb, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}},
				config: config{Version: 1, BaseSchema: "base.sql", baseDir: tmp,
					Sources: []sourceConfig{{Table: "metrics", Schema: schema, Match: "*-sql-pool-metrics.tsv", View: "metrics_sql_pool", Windowed: true, WindowColumn: "ts"}}},
			}
			for i, runID := range []string{"run-1", "run-2"} {
				dir := filepath.Join(results, runID)
				writeTestFile(t, dir, "run.json", `{"phase":"finalized"}`)
				writeTestFile(t, dir, "bench.log", "2026-09-01T12:00:00Z\tBENCHMARK_START\n2026-09-01T12:01:00Z\tBENCHMARK_END\n")
				data := "sample\ttimestamp\telapsed_ms\tdatabase_host\tpool\trole\tshard\tmetric\tvalue\n"
				if (i == 0) != emptyFirst {
					data += "1\t2026-09-01T12:00:01Z\t1000\tdb.local\tisupipe\tprimary\t0\tin_use\t2.000000\n"
				}
				writeTestFile(t, dir, "host1-sql-pool-metrics.tsv", data)
				if err := r.syncSelection(dir, []string{dir}, []string{runID}); err != nil {
					t.Fatal(err)
				}
			}
			output, err := r.duckdbOutput(nil, "-noheader", "-list", r.db, "-c", "select typeof(ts), typeof(value), value from metrics")
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(output) != "TIMESTAMP WITH TIME ZONE|DOUBLE|2.0" {
				t.Fatalf("header-only import changed types or fabricated samples: %s", output)
			}
		})
	}
}

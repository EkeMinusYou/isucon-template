package main

import (
	"bytes"
	"encoding/csv"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMySQLLockDataStaysTextAcrossRuns(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	schema, err := filepath.Abs("../analysis/schema/mysql-lock.sql")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	results := filepath.Join(tmp, "runs")
	writeTestFile(t, tmp, "base.sql", `
set variable run_glob = getenv('ISUCON_RUN_GLOB');
create view runs as select '20260901-120000' as run_id, 'host1' as mysql_host
union all select '20260901-120100', 'host1';
`)
	header := strings.Split("sample timestamp elapsed_ms captured_waits capture_error wait_key requesting_trx_id blocking_trx_id object_schema object_table object_index request_lock_type request_lock_mode request_lock_data blocking_lock_type blocking_lock_mode blocking_lock_data requesting_thread_id requesting_processlist_id requesting_sql blocking_thread_id blocking_processlist_id blocking_sql", " ")
	writeRun := func(runID, requestData, blockingData string) string {
		t.Helper()
		dir := filepath.Join(results, runID)
		writeTestFile(t, dir, "run.json", `{"phase":"finalized"}`)
		var content bytes.Buffer
		writer := csv.NewWriter(&content)
		writer.Comma = '\t'
		if err := writer.Write(header); err != nil {
			t.Fatal(err)
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "host2-mysql-lock-waits.tsv", content.String())
		if err := writer.Write([]string{
			"0", "2026-09-01T12:00:01Z", "1000", "1", "", "wait-1", "28333", "28334",
			"db", "items", "idx_items_user", "RECORD", "X", requestData,
			"RECORD", "X", blockingData, "1377", "1284", "UPDATE items", "1204", "1111", "COMMIT",
		}); err != nil {
			t.Fatal(err)
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "host1-mysql-lock-waits.tsv", content.String())
		return dir
	}
	r := runner{
		options: options{db: filepath.Join(tmp, "analysis.duckdb"), results: results, duckdb: duckdb, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}},
		config: config{
			Version: 1, BaseSchema: "base.sql", baseDir: tmp,
			Sources: []sourceConfig{{Table: "mysql_lock_waits", Schema: schema, Match: "*-mysql-lock-waits.tsv", View: "mysql_lock_waits"}},
		},
	}
	first := writeRun("20260901-120000", "00123", "42")
	if err := r.syncSelection(first, []string{first}, []string{"20260901-120000"}); err != nil {
		t.Fatal(err)
	}
	second := writeRun("20260901-120100", "supremum pseudo-record", "1137, 938")
	if err := r.syncSelection(second, []string{second}, []string{"20260901-120100"}); err != nil {
		t.Fatalf("import opaque lock data after numeric lock data: %v", err)
	}
	output, err := r.duckdbOutput(nil, "-noheader", "-list", r.db, "-c", "select host, request_lock_data, blocking_lock_data, typeof(request_lock_data), typeof(blocking_lock_data) from mysql_lock_waits order by run_id")
	if err != nil {
		t.Fatal(err)
	}
	want := "host1|00123|42|VARCHAR|VARCHAR\nhost1|supremum pseudo-record|1137, 938|VARCHAR|VARCHAR"
	if strings.TrimSpace(output) != want {
		t.Fatalf("lock data changed during import: got %q, want %q", output, want)
	}
}

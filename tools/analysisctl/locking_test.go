package main

import (
	"bufio"
	"bytes"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func lockedTestDatabase(t *testing.T) (runner, func()) {
	t.Helper()
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	db := filepath.Join(t.TempDir(), "analysis.duckdb")
	sql := "create table values_table as select 1 as value;" +
		"create table ingested(run_id varchar primary key); insert into ingested values ('run-1');" +
		"create table analysis_metadata(key varchar primary key, value varchar not null);" +
		"insert into analysis_metadata values ('import_schema_version', '" + importSchemaVersion + "');"
	if output, err := exec.Command(duckdb, db, "-c", sql).CombinedOutput(); err != nil {
		t.Fatalf("create database: %v: %s", err, output)
	}
	cmd := exec.Command(duckdb, "-readonly", "-noheader", "-list", db)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			input.Close()
			cmd.Wait()
		})
	}
	t.Cleanup(release)
	if _, err := io.WriteString(input, "select value from values_table;\n"); err != nil {
		t.Fatal(err)
	}
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		if scanner.Scan() {
			ready <- scanner.Text()
		} else {
			ready <- ""
		}
	}()
	select {
	case line := <-ready:
		if line != "1" {
			release()
			t.Fatalf("reader did not acquire the database: %q: %s", line, stderr.String())
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		release()
		t.Fatal("reader startup timed out")
	}
	return runner{options: options{db: db, duckdb: duckdb, lockWait: 2 * time.Second}}, release
}

func TestMetadataReadsShareDatabaseWithReader(t *testing.T) {
	r, _ := lockedTestDatabase(t)
	current, err := r.importSchemaCurrent()
	if err != nil || !current {
		t.Fatalf("schema check with concurrent reader: current=%v, err=%v", current, err)
	}
	missing, err := r.missingTables([]sourceConfig{{Table: "values_table"}})
	if err != nil || len(missing) != 0 {
		t.Fatalf("table check with concurrent reader: missing=%v, err=%v", missing, err)
	}
	indexed, err := r.indexedRuns()
	if err != nil || !indexed["run-1"] {
		t.Fatalf("RUN check with concurrent reader: indexed=%v, err=%v", indexed, err)
	}
}

type lockWaitNotice chan struct{}

func (n lockWaitNotice) Write(p []byte) (int, error) {
	select {
	case n <- struct{}{}:
	default:
	}
	return len(p), nil
}

func TestDuckDBWriteWaitsForReader(t *testing.T) {
	r, release := lockedTestDatabase(t)
	notice := make(lockWaitNotice, 1)
	r.stderr = notice
	done := make(chan error, 1)
	go func() {
		_, err := r.duckdbOutput(nil, r.db, "-c", "update values_table set value = value + 1")
		done <- err
	}()
	select {
	case <-notice:
	case err := <-done:
		t.Fatalf("write finished before reader released: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("write did not report the lock wait")
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("write failed after reader released: %v", err)
	}
	value, err := r.queryCount("select value from values_table")
	if err != nil || value != 2 {
		t.Fatalf("write was not applied exactly once: value=%d, err=%v", value, err)
	}
}

func TestDuckDBImportWaitsForReader(t *testing.T) {
	r, release := lockedTestDatabase(t)
	dir := t.TempDir()
	writeTestFile(t, dir, "base.sql", "create view runs as select 'run-2'::varchar as run_id;\n")
	writeTestFile(t, dir, "source.sql", "create view incoming as select 'run-2'::varchar as run_id, 2::integer as value;\n")
	writeTestFile(t, dir, "run/data.tsv", "present\n")
	r.config = config{
		BaseSchema: "base.sql",
		Sources:    []sourceConfig{{Table: "imported_values", Schema: "source.sql", Match: "data.tsv", View: "incoming"}},
		baseDir:    dir,
	}
	notice := make(lockWaitNotice, 1)
	r.stderr = notice
	done := make(chan error, 1)
	go func() {
		runDir := filepath.Join(dir, "run")
		done <- r.syncSelection(runDir, []string{runDir}, []string{"run-2"})
	}()
	select {
	case <-notice:
	case err := <-done:
		t.Fatalf("import finished before reader released: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("import did not report the lock wait")
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("import failed after reader released: %v", err)
	}
	count, err := r.queryCount("select count(*) from imported_values where run_id = 'run-2'")
	if err != nil || count != 1 {
		t.Fatalf("import was not applied exactly once: count=%d, err=%v", count, err)
	}
	count, err = r.queryCount("select count(*) from ingested where run_id = 'run-2'")
	if err != nil || count != 1 {
		t.Fatalf("import marker missing or duplicated: count=%d, err=%v", count, err)
	}
}

func TestDuckDBLockWaitExpiresWithoutWriting(t *testing.T) {
	r, release := lockedTestDatabase(t)
	r.lockWait = 100 * time.Millisecond
	_, err := r.duckdbOutput(nil, r.db, "-c", "update values_table set value = value + 1")
	if err == nil || !strings.Contains(err.Error(), "lock wait timed out") {
		t.Fatalf("expected bounded lock wait: %v", err)
	}
	release()
	value, err := r.queryCount("select value from values_table")
	if err != nil || value != 1 {
		t.Fatalf("timed-out write changed data: value=%d, err=%v", value, err)
	}
}

func TestDuckDBSQLFailureDoesNotWait(t *testing.T) {
	r, release := lockedTestDatabase(t)
	release()
	var stderr bytes.Buffer
	r.stderr = &stderr
	_, err := r.duckdbOutput(nil, "-readonly", r.db, "-c", "select * from missing_table")
	if err == nil || !strings.Contains(err.Error(), "missing_table") {
		t.Fatalf("expected the SQL error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("SQL failure triggered a lock wait: %s", stderr.String())
	}
}

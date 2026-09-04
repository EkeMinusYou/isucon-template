package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildImportSQLMarksIngestedLast(t *testing.T) {
	r := runner{
		options: options{db: "index.duckdb"},
		config:  config{Profiles: profileConfig{Tables: []string{"profile_samples"}}},
	}
	sql := r.buildImportSQL([]sourceConfig{{Table: "metrics", View: "metrics_proc"}}, []string{"20260902-171648"})

	begin := strings.Index(sql, "begin transaction")
	sourceInsert := strings.Index(sql, "insert into db.metrics")
	profileInsert := strings.Index(sql, "insert into db.profile_samples")
	mark := strings.Index(sql, "insert or ignore into db.ingested")
	commit := strings.Index(sql, "commit;")
	if !(begin >= 0 && begin < sourceInsert && sourceInsert < profileInsert && profileInsert < mark && mark < commit) {
		t.Fatalf("unexpected transaction order:\n%s", sql)
	}
}

func TestLoadConfigRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.yaml")
	data := []byte("version: 1\nbase_schema: base.sql\nunknown: true\nsources:\n  - table: t\n    schema: t.sql\n    match: t.tsv\n    view: t_raw\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("loadConfig accepted an unknown field")
	}
}

func TestFailedImportRollsBackDataAndMarker(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	dir := t.TempDir()
	writeTestFile(t, dir, "base.sql", "create view runs as select 'run-1'::varchar as run_id;\n")
	writeTestFile(t, dir, "ok.sql", "create view ok_view as select 'run-1'::varchar as run_id, 1::integer as value;\n")
	writeTestFile(t, dir, "bad.sql", "create view bad_view as select 'run-1'::varchar as run_id, 'not-an-integer'::varchar as value;\n")
	writeTestFile(t, dir, "run/ok.tsv", "present\n")
	writeTestFile(t, dir, "run/bad.tsv", "present\n")

	db := filepath.Join(dir, "analysis.duckdb")
	r := runner{
		options: options{db: db, results: dir, duckdb: duckdb, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}},
		config: config{
			Version:    1,
			BaseSchema: "base.sql",
			Sources: []sourceConfig{
				{Table: "values_table", Schema: "ok.sql", Match: "ok.tsv", View: "ok_view"},
				{Table: "values_table", Schema: "bad.sql", Match: "bad.tsv", View: "bad_view"},
			},
			baseDir: dir,
		},
	}
	if err := r.syncSelection(filepath.Join(dir, "run"), []string{filepath.Join(dir, "run")}, []string{"run-1"}); err == nil {
		t.Fatal("syncSelection unexpectedly succeeded")
	}
	output, err := r.duckdbOutput(nil, "-noheader", "-list", db, "-c", "select count(*) from information_schema.tables where table_name in ('values_table','ingested')")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output) != "0" {
		t.Fatalf("partial import survived rollback: table count=%s", strings.TrimSpace(output))
	}
}

func TestFailedRebuildKeepsPreviousDatabase(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	dir := t.TempDir()
	db := filepath.Join(dir, "analysis.duckdb")
	cmd := exec.Command(duckdb, db, "-c", "create table preserved as select 42 as value")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create previous database: %v: %s", err, output)
	}
	writeTestFile(t, dir, "base.sql", "create view runs as select 'run-1'::varchar as run_id;\n")
	writeTestFile(t, dir, "bad.sql", "create view bad_view as select error('forced rebuild failure') as value;\n")
	writeTestFile(t, dir, "run-1/bad.tsv", "present\n")
	r := runner{
		options: options{db: db, results: dir, duckdb: duckdb, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}},
		config: config{
			Version:    1,
			BaseSchema: "base.sql",
			Sources:    []sourceConfig{{Table: "bad", Schema: "bad.sql", Match: "bad.tsv", View: "bad_view"}},
			baseDir:    dir,
		},
	}
	if err := r.rebuildRuns([]string{"run-1"}); err == nil {
		t.Fatal("rebuildRuns unexpectedly succeeded")
	}
	output, err := r.duckdbOutput(nil, "-noheader", "-list", db, "-c", "select value from preserved")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output) != "42" {
		t.Fatalf("previous database was not preserved: %q", output)
	}
}

func TestMissingTablesChecksOnlyActiveSources(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	dir := t.TempDir()
	db := filepath.Join(dir, "analysis.duckdb")
	cmd := exec.Command(duckdb, db, "-c", "create table active_source as select 1 as value")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create database: %v: %s", err, output)
	}
	r := runner{
		options: options{db: db, duckdb: duckdb, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}},
		config: config{
			Sources: []sourceConfig{
				{Table: "active_source"},
				{Table: "optional_source"},
			},
		},
	}
	missing, err := r.missingTables([]sourceConfig{{Table: "active_source"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("inactive optional source was treated as missing: %v", missing)
	}
	missing, err = r.missingTables(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("empty active source set returned missing tables: %v", missing)
	}
}

func TestImportSchemaVersionDetectsStaleDatabase(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	dir := t.TempDir()
	db := filepath.Join(dir, "analysis.duckdb")
	if output, err := exec.Command(duckdb, db, "-c", "create table stale as select 1 as value").CombinedOutput(); err != nil {
		t.Fatalf("create stale database: %v: %s", err, output)
	}
	r := runner{options: options{db: db, duckdb: duckdb}}
	current, err := r.importSchemaCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if current {
		t.Fatal("stale database unexpectedly has the current import schema")
	}
	if output, err := exec.Command(duckdb, db, "-c", "create table analysis_metadata(key varchar primary key, value varchar); insert into analysis_metadata values ('import_schema_version', '"+importSchemaVersion+"')").CombinedOutput(); err != nil {
		t.Fatalf("write schema version: %v: %s", err, output)
	}
	current, err = r.importSchemaCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if !current {
		t.Fatal("current import schema was not detected")
	}
}

func writeTestFile(t *testing.T, base, name, content string) {
	t.Helper()
	path := filepath.Join(base, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

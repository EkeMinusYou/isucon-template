package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSmokeUsesConfiguredSourcesWithoutRegisteringRun(t *testing.T) {
	duckdb, err := exec.LookPath("duckdb")
	if err != nil {
		t.Skip("duckdb is not installed")
	}
	cfg, err := loadConfig("../analysis/sources.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.BaseSchema, err = filepath.Abs(cfg.path(cfg.BaseSchema))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "setup-smoke-20261012-120000")
	schema := filepath.Join(tmp, "custom.sql")
	writeTestFile(t, tmp, "custom.sql", `create view custom_a as select
 regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id, value
 from read_csv(getvariable('run_glob') || '/a.tsv', delim='\t', filename=true);
create view custom_b as select
 regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id, value
 from read_csv(getvariable('run_glob') || '/b.tsv', delim='\t', filename=true);
`)
	writeTestFile(t, dir, "a.tsv", "value\n7\n")
	writeTestFile(t, dir, "b.tsv", "value\n11\n")
	cfg.Sources = []sourceConfig{
		{Table: "custom", Schema: schema, Match: "a.tsv", View: "custom_a"},
		{Table: "custom", Schema: schema, Match: "b.tsv", View: "custom_b"},
	}
	cfg.Profiles = profileConfig{}
	normalResults := filepath.Join(tmp, "normal-results")
	r := runner{options: options{results: normalResults, duckdb: duckdb, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}, config: cfg}
	if err := r.smoke(dir); err != nil {
		t.Fatal(err)
	}
	r.db = filepath.Join(dir, "analysis.duckdb")
	profileSQL(t, r, "select count(*),sum(value),min(run_id) from smoke_custom", "2|18|setup-smoke-20261012-120000")
	profileSQL(t, r, "select count(*) from runs", "0")
	for _, path := range []string{normalResults, filepath.Join(dir, "run.json"), filepath.Join(dir, "scores.tsv")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unexpected RUN state at %s: %v", path, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "a.tsv"))
	if err != nil || string(data) != "value\n7\n" {
		t.Fatalf("source artifact changed: %q, %v", data, err)
	}
}

func TestSmokeRefusesRunManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "setup-smoke-20261012-120000")
	writeTestFile(t, dir, "run.json", `{"phase":"finalized"}`)
	r := runner{}
	if err := r.smoke(dir); err == nil {
		t.Fatal("smoke accepted a RUN manifest")
	}
}

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/pprof/profile"
)

func profileTestRunner(t *testing.T) (runner, string) {
	t.Helper()
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
	dir := filepath.Join(results, "20260906-120000")
	writeTestFile(t, tmp, "base.sql", "create view runs as select '20260906-120000'::varchar as run_id;")
	writeTestFile(t, dir, "run.json", `{"phase":"finalized"}`)
	cfg.BaseSchema = filepath.Join(tmp, "base.sql")
	cfg.Sources = nil
	cfg.SemanticSchemas = nil
	cfg.loadPhase = loadPhaseConfig{StartPattern: "LOAD_START", EndPattern: "LOAD_END"}
	if err := cfg.loadPhase.compile(); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "bench.log", "2026-09-06T12:00:00Z\tBENCHMARK_START\n2026-09-06T12:00:01Z\tLOAD_START\n2026-09-06T12:00:09Z\tLOAD_END\n2026-09-06T12:00:10Z\tBENCHMARK_END\n")
	return runner{options: options{db: filepath.Join(tmp, "analysis.duckdb"), results: results, duckdb: duckdb, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}, config: cfg}, dir
}

func profileSQL(t *testing.T, r runner, sql, want string) {
	t.Helper()
	got, err := r.duckdbOutput(nil, "-noheader", "-list", r.db, "-c", sql)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != want {
		t.Fatalf("%s: got %q, want %q", sql, got, want)
	}
}

func TestStandardProfilesDuckDBAndSchemaUpgrade(t *testing.T) {
	r, dir := profileTestRunner(t)
	f := &profile.Function{ID: 1, Name: "example/work"}
	l := &profile.Location{ID: 1, Line: []profile.Line{{Function: f}}}
	for _, fixture := range []struct {
		kind, metric, unit string
		value              int64
	}{
		{"fgprof", "time", "nanoseconds", 2_000_000_000},
		{"go-cpu", "cpu", "nanoseconds", 70_000_000},
		{"go-heap", "inuse_space", "bytes", 1024},
		{"go-allocs", "alloc_space", "bytes", 4096},
		{"go-goroutine", "goroutine", "count", 3},
	} {
		p := &profile.Profile{TimeNanos: time.Date(2026, 9, 6, 12, 0, 2, 0, time.UTC).UnixNano(), DurationNanos: int64(time.Second), SampleType: []*profile.ValueType{{Type: fixture.metric, Unit: fixture.unit}},
			Sample:   []*profile.Sample{{Location: []*profile.Location{l}, Value: []int64{fixture.value}}},
			Location: []*profile.Location{l}, Function: []*profile.Function{f}}
		var b bytes.Buffer
		if err := p.Write(&b); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "host-"+fixture.kind+".pprof"), b.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.sync(); err != nil {
		t.Fatal(err)
	}
	profileSQL(t, r, "select count(*), count(distinct profile_type) from pprof_metadata", "4|4")
	profileSQL(t, r, "select profile_type, sample_type, value_unit, flat_value from pprof_functions order by profile_type", "fgprof|time|seconds|2.0\ngo-cpu|cpu|seconds|0.07\ngo-goroutine|goroutine|count|3.0\ngo-heap|inuse_space|bytes|1024.0")
	profileSQL(t, r, "select count(*), sum(total_wall_seconds) from profile_metadata", "1|2.0")
	profileSQL(t, r, "select count(*) from pprof_samples join pprof_frames using (run_id, host, source, sample_type, sample_id)", "4")
	if err := r.duckdbCommand(nil, r.db, "-c", "update analysis_metadata set value='4' where key='import_schema_version'; drop table pprof_samples;"); err != nil {
		t.Fatal(err)
	}
	if err := r.sync(); err != nil {
		t.Fatal(err)
	}
	if err := r.sync(); err != nil {
		t.Fatal(err)
	}
	profileSQL(t, r, "select count(*) from pprof_samples", "4")
	profileSQL(t, r, "select count(*) from pprof_metadata", "4")
}

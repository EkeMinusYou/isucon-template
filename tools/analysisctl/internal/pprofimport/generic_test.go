package pprofimport

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/pprof/profile"
)

func TestAllProfileMetrics(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "20260906-120000")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := &profile.Function{ID: 1, Name: "example/work", Filename: "work.go"}
	l := &profile.Location{ID: 1, Line: []profile.Line{{Function: f, Line: 7}}}
	var paths []string
	for _, fixture := range []struct {
		kind   string
		types  []*profile.ValueType
		values []int64
	}{
		{"fgprof", []*profile.ValueType{{Type: "samples", Unit: "count"}, {Type: "time", Unit: "nanoseconds"}}, []int64{10, 2_000_000_000}},
		{"go-cpu", []*profile.ValueType{{Type: "samples", Unit: "count"}, {Type: "cpu", Unit: "nanoseconds"}}, []int64{7, 70_000_000}},
		{"go-heap", []*profile.ValueType{{Type: "alloc_objects", Unit: "count"}, {Type: "alloc_space", Unit: "bytes"}, {Type: "inuse_objects", Unit: "count"}, {Type: "inuse_space", Unit: "bytes"}}, []int64{8, 4096, 2, 1024}},
		{"go-allocs", []*profile.ValueType{{Type: "alloc_objects", Unit: "count"}, {Type: "alloc_space", Unit: "bytes"}, {Type: "inuse_objects", Unit: "count"}, {Type: "inuse_space", Unit: "bytes"}}, []int64{9, 8192, 3, 2048}},
		{"go-goroutine", []*profile.ValueType{{Type: "goroutine", Unit: "count"}}, []int64{4}},
	} {
		p := &profile.Profile{SampleType: fixture.types, Sample: []*profile.Sample{{Location: []*profile.Location{l}, Value: fixture.values}},
			Location: []*profile.Location{l}, Function: []*profile.Function{f}, TimeNanos: 1788663600000000000, DurationNanos: 120_000_000_000}
		var b bytes.Buffer
		if err := p.Write(&b); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(runDir, "host-with-dashes-"+fixture.kind+".pprof")
		if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	out := t.TempDir()
	if err := Run(out, paths, io.Discard); err != nil {
		t.Fatal(err)
	}
	rows := readRows(t, out, "pprof-metadata.rows")
	if len(rows) != 13 {
		t.Fatalf("metric count=%d, want 13", len(rows))
	}
	want := map[string]string{"go-cpu/cpu": "0.070000000000000007", "go-heap/inuse_space": "1024", "go-allocs/alloc_space": "8192", "go-goroutine/goroutine": "4", "fgprof/time": "2"}
	for _, r := range rows {
		if r["host"] != "host-with-dashes" {
			t.Fatalf("host=%q", r["host"])
		}
		k := r["profile_type"] + "/" + r["sample_type"]
		if v, ok := want[k]; ok && r["total_value"] != v {
			t.Fatalf("%s=%s, want %s", k, r["total_value"], v)
		}
		if r["sample_unit"] == "nanoseconds" && r["value_unit"] != "seconds" {
			t.Fatalf("units=%v", r)
		}
		if r["sample_unit"] != "nanoseconds" && r["value_unit"] != r["sample_unit"] {
			t.Fatalf("units=%v", r)
		}
	}
	legacy := readRows(t, out, "profile-metadata.rows")
	if len(legacy) != 1 || legacy[0]["total_wall_seconds"] != "2" {
		t.Fatalf("legacy=%v", legacy)
	}
	for _, name := range []string{"samples", "frames", "functions"} {
		if rows := readRows(t, out, "pprof-"+name+".rows"); len(rows) != 13 {
			t.Fatalf("%s rows=%d", name, len(rows))
		}
	}
}

func TestEmptyAndInvalidProfiles(t *testing.T) {
	out := t.TempDir()
	if err := Run(out, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if rows := readRows(t, out, "pprof-metadata.rows"); len(rows) != 0 {
		t.Fatal(rows)
	}
	dir := filepath.Join(t.TempDir(), "20260906-120000")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "host-go-cpu.pprof")
	p := &profile.Profile{SampleType: []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}}, DurationNanos: 120_000_000_000}
	var b bytes.Buffer
	if err := p.Write(&b); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run(out, []string{path}, io.Discard); err != nil {
		t.Fatal(err)
	}
	rows := readRows(t, out, "pprof-metadata.rows")
	if len(rows) != 1 || rows[0]["sample_count"] != "0" || rows[0]["total_value"] != "0" {
		t.Fatal(rows)
	}
	for _, data := range [][]byte{nil, []byte("HTTP 500 error"), b.Bytes()[:8]} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Run(out, []string{path}, io.Discard); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if _, _, err := metricValue(1, "widgets"); err == nil {
		t.Fatal("unknown unit accepted")
	}
}

func TestWriteTSVPropagatesRowError(t *testing.T) {
	want := errors.New("row failure")
	err := writeTSV(filepath.Join(t.TempDir(), "rows"), []string{"header"}, func(*csv.Writer) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
}

func readRows(t *testing.T, dir, name string) []map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	all, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var result []map[string]string
	for _, row := range all[1:] {
		m := make(map[string]string)
		for i, v := range row {
			m[all[0][i]] = v
		}
		result = append(result, m)
	}
	return result
}

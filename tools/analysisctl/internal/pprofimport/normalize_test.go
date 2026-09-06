package pprofimport

import (
	"os"
	"path/filepath"
	"testing"

	pprofprofile "github.com/google/pprof/profile"
)

func TestNormalizeProfileWallTimeAndEdges(t *testing.T) {
	leaf := &pprofprofile.Function{ID: 1, Name: "example/leaf", Filename: "leaf.go"}
	parent := &pprofprofile.Function{ID: 2, Name: "example/parent", Filename: "parent.go"}
	leafLocation := &pprofprofile.Location{ID: 1, Line: []pprofprofile.Line{{Function: leaf, Line: 10}}}
	parentLocation := &pprofprofile.Location{ID: 2, Line: []pprofprofile.Line{{Function: parent, Line: 20}}}
	profile := &pprofprofile.Profile{
		SampleType: []*pprofprofile.ValueType{{Type: "samples", Unit: "count"}, {Type: "time", Unit: "nanoseconds"}},
		Sample: []*pprofprofile.Sample{
			{Location: []*pprofprofile.Location{leafLocation, parentLocation}, Value: []int64{1, 2_000_000_000}},
			{Location: []*pprofprofile.Location{parentLocation}, Value: []int64{1, 1_000_000_000}},
		},
		Location:      []*pprofprofile.Location{leafLocation, parentLocation},
		Function:      []*pprofprofile.Function{leaf, parent},
		DurationNanos: 5_000_000_000,
	}

	runDir := filepath.Join(t.TempDir(), "20260902-154306")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runDir, "isucon-1-fgprof.pprof")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.Write(file); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := normalizeProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.metadata.totalValue != 3 || got.metadata.durationSeconds != 5 || got.metadata.sampleUnit != "nanoseconds" {
		t.Fatalf("metadata = %#v", got.metadata)
	}
	if len(got.samples) != 2 || len(got.frames) != 3 {
		t.Fatalf("samples/frames = %d/%d", len(got.samples), len(got.frames))
	}
	if len(got.edges) != 1 || got.edges[0].caller != "example/parent" || got.edges[0].callee != "example/leaf" || got.edges[0].value != 2 {
		t.Fatalf("edges = %#v", got.edges)
	}
	values := map[string]functionRow{}
	for _, row := range got.functions {
		values[row.function] = row
	}
	if values["example/leaf"].flatValue != 2 || values["example/leaf"].cumulative != 2 {
		t.Fatalf("leaf = %#v", values["example/leaf"])
	}
	if values["example/parent"].flatValue != 1 || values["example/parent"].cumulative != 3 {
		t.Fatalf("parent = %#v", values["example/parent"])
	}
}

func TestValueToSecondsRejectsCPUCountUnit(t *testing.T) {
	if _, err := valueToSeconds(1, "count"); err == nil {
		t.Fatal("count unit was accepted as wall time")
	}
}

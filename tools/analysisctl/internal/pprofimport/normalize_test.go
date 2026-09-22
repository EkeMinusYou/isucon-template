package pprofimport

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	pprofprofile "github.com/google/pprof/profile"
)

func TestImportProfileWallTimeAndEdges(t *testing.T) {
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

	out := t.TempDir()
	if err := Run(out, []string{path}, io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata := readRows(t, out, "profile-metadata.rows")
	if len(metadata) != 1 || metadata[0]["total_wall_seconds"] != "3" || metadata[0]["duration_seconds"] != "5" || metadata[0]["sample_unit"] != "nanoseconds" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if samples, frames := readRows(t, out, "profile-samples.rows"), readRows(t, out, "profile-frames.rows"); len(samples) != 2 || len(frames) != 3 {
		t.Fatalf("samples/frames = %d/%d", len(samples), len(frames))
	}
	edges := readRows(t, out, "pprof-edges.rows")
	var timeEdges []map[string]string
	for _, row := range edges {
		if row["sample_type"] == "time" {
			timeEdges = append(timeEdges, row)
		}
	}
	if len(timeEdges) != 1 || timeEdges[0]["caller"] != "example/parent" || timeEdges[0]["callee"] != "example/leaf" || timeEdges[0]["value"] != "2" {
		t.Fatalf("edges = %#v", timeEdges)
	}
	values := map[string]map[string]string{}
	for _, row := range readRows(t, out, "pprof-functions.rows") {
		if row["sample_type"] == "time" {
			values[row["function"]] = row
		}
	}
	if values["example/leaf"]["flat_value"] != "2" || values["example/leaf"]["cumulative_value"] != "2" || values["example/parent"]["flat_value"] != "1" || values["example/parent"]["cumulative_value"] != "3" {
		t.Fatalf("functions = %#v", values)
	}
}

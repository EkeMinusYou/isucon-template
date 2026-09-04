package main

import (
	"os"
	"path/filepath"
	"testing"

	pprofprofile "github.com/google/pprof/profile"
)

func TestSummarizeFgprof(t *testing.T) {
	leaf := &pprofprofile.Function{ID: 1, Name: "example/leaf"}
	parent := &pprofprofile.Function{ID: 2, Name: "example/parent"}
	leafLocation := &pprofprofile.Location{ID: 1, Line: []pprofprofile.Line{{Function: leaf}}}
	parentLocation := &pprofprofile.Location{ID: 2, Line: []pprofprofile.Line{{Function: parent}}}
	profile := &pprofprofile.Profile{
		SampleType: []*pprofprofile.ValueType{
			{Type: "samples", Unit: "count"},
			{Type: "time", Unit: "nanoseconds"},
		},
		Sample: []*pprofprofile.Sample{
			{Location: []*pprofprofile.Location{leafLocation, parentLocation}, Value: []int64{1, 10}},
			{Location: []*pprofprofile.Location{parentLocation}, Value: []int64{1, 20}},
		},
	}

	functions, total := summarizePprof(profile, 1)
	if total != 30 {
		t.Fatalf("total = %d, want 30", total)
	}
	if len(functions) != 2 {
		t.Fatalf("function count = %d, want 2", len(functions))
	}
	if functions[0] != (pprofFunctionStat{name: "example/parent", flat: 20, cum: 30}) {
		t.Fatalf("first function = %#v, want parent flat=20 cum=30", functions[0])
	}
	if functions[1] != (pprofFunctionStat{name: "example/leaf", flat: 10, cum: 10}) {
		t.Fatalf("second function = %#v, want leaf flat=10 cum=10", functions[1])
	}
}

func TestParseFgprofFile(t *testing.T) {
	leaf := &pprofprofile.Function{ID: 1, Name: "example/leaf"}
	location := &pprofprofile.Location{ID: 1, Line: []pprofprofile.Line{{Function: leaf}}}
	profile := &pprofprofile.Profile{
		SampleType:    []*pprofprofile.ValueType{{Type: "samples", Unit: "count"}, {Type: "time", Unit: "nanoseconds"}},
		Sample:        []*pprofprofile.Sample{{Location: []*pprofprofile.Location{location}, Value: []int64{1, 2_500_000}}},
		Location:      []*pprofprofile.Location{location},
		Function:      []*pprofprofile.Function{leaf},
		DurationNanos: 3_000_000_000,
	}

	path := filepath.Join(t.TempDir(), "isucon-1-fgprof.pprof")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.Write(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := parsePprofFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "isucon-1-fgprof.pprof" || got.SampleType != "time" || got.SampleUnit != "nanoseconds" {
		t.Fatalf("metadata = %#v", got)
	}
	if got.DurationSec != 3 || got.TotalMs != 2.5 {
		t.Fatalf("duration/total = %v/%v, want 3/2.5", got.DurationSec, got.TotalMs)
	}
	if len(got.Functions) != 1 || got.Functions[0].FlatMs != 2.5 || got.Functions[0].CumPct != 100 {
		t.Fatalf("functions = %#v", got.Functions)
	}
}

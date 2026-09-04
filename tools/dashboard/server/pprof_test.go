package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	runtimepprof "runtime/pprof"
	"testing"

	pprofprofile "github.com/google/pprof/profile"
)

func writeTestPprof(t *testing.T, path string, profile *pprofprofile.Profile) {
	t.Helper()
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
}

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
	writeTestPprof(t, path, profile)

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

func TestParseGoPprofFileUsesKindSpecificSampleType(t *testing.T) {
	leaf := &pprofprofile.Function{ID: 1, Name: "example/allocate"}
	location := &pprofprofile.Location{ID: 1, Line: []pprofprofile.Line{{Function: leaf}}}
	profile := &pprofprofile.Profile{
		SampleType: []*pprofprofile.ValueType{
			{Type: "alloc_space", Unit: "bytes"},
			{Type: "inuse_space", Unit: "bytes"},
		},
		Sample:   []*pprofprofile.Sample{{Location: []*pprofprofile.Location{location}, Value: []int64{8192, 2048}}},
		Location: []*pprofprofile.Location{location},
		Function: []*pprofprofile.Function{leaf},
	}

	path := filepath.Join(t.TempDir(), "isucon-1-go-heap.pprof")
	writeTestPprof(t, path, profile)
	kind, ok := findGoPprofKind("heap")
	if !ok {
		t.Fatal("heap kind was not found")
	}
	got, err := parseGoPprofFile(path, kind)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "heap" || got.SampleType != "inuse_space" || got.SampleUnit != "bytes" || got.Total != 2048 {
		t.Fatalf("profile metadata = %#v", got)
	}
	if len(got.Functions) != 1 || got.Functions[0].Flat != 2048 || got.Functions[0].CumPct != 100 {
		t.Fatalf("functions = %#v", got.Functions)
	}
}

func TestRuntimeGoProfileSampleTypes(t *testing.T) {
	expectedTypes := map[string]string{
		"heap":      "inuse_space",
		"allocs":    "alloc_space",
		"goroutine": "goroutine",
	}
	for kindName, expectedType := range expectedTypes {
		t.Run(kindName, func(t *testing.T) {
			var buf bytes.Buffer
			if err := runtimepprof.Lookup(kindName).WriteTo(&buf, 0); err != nil {
				t.Fatal(err)
			}
			profile, err := pprofprofile.ParseData(buf.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			kind, ok := findGoPprofKind(kindName)
			if !ok {
				t.Fatalf("%s kind was not found", kindName)
			}
			_, sampleType, err := selectPreferredPprofSampleType(profile, kind.preferredSampleTypes)
			if err != nil {
				t.Fatal(err)
			}
			if sampleType == nil || sampleType.Type != expectedType || sampleType.Unit == "" {
				t.Fatalf("sample type = %#v, want %s", sampleType, expectedType)
			}
		})
	}
}

func TestHandleGoPprofListsProfilesByKindAndHost(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "runs")
	runID := "20260905-120000"
	runDir := filepath.Join(runsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	function := &pprofprofile.Function{ID: 1, Name: "main.work"}
	location := &pprofprofile.Location{ID: 1, Line: []pprofprofile.Line{{Function: function}}}
	profile := &pprofprofile.Profile{
		SampleType: []*pprofprofile.ValueType{{Type: "samples", Unit: "count"}, {Type: "cpu", Unit: "nanoseconds"}},
		Sample:     []*pprofprofile.Sample{{Location: []*pprofprofile.Location{location}, Value: []int64{1, 1_000_000}}},
		Location:   []*pprofprofile.Location{location},
		Function:   []*pprofprofile.Function{function},
	}
	writeTestPprof(t, filepath.Join(runDir, "isucon-2-go-cpu.pprof"), profile)
	writeTestPprof(t, filepath.Join(runDir, "isucon-1-go-cpu.pprof"), profile)

	a := &app{runsDir: runsDir}
	req := httptest.NewRequest("GET", "/api/runs/"+runID+"/pprof", nil)
	req.SetPathValue("run_id", runID)
	recorder := httptest.NewRecorder()
	a.handleGoPprof(recorder, req)
	if recorder.Code != 200 {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var got goPprofResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Available || len(got.Profiles) != 2 {
		t.Fatalf("response = %#v", got)
	}
	if got.Profiles[0].Host != "isucon-1" || got.Profiles[1].Host != "isucon-2" || got.Profiles[0].SampleType != "cpu" {
		t.Fatalf("profiles = %#v", got.Profiles)
	}

	if _, _, ok := a.resolveGoPprofFile(runID, "../../cpu", "isucon-1"); ok {
		t.Fatal("invalid kind resolved successfully")
	}
	if _, _, ok := a.resolveGoPprofFile(runID, "cpu", "../isucon-1"); ok {
		t.Fatal("invalid host resolved successfully")
	}
}

package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestAccessLogSecondFormats(t *testing.T) {
	for _, tc := range []struct {
		line string
		want int64
		ok   bool
	}{
		{`{"time":"2026-09-06T04:41:18.125Z"}`, 1788669678, true},
		{`{"msec":"1788669678.999","time":"2000-01-01T00:00:00Z"}`, 1788669678, true},
		{`{"msec":"NaN"}`, 0, false},
		{`{"msec":"Inf"}`, 0, false},
		{`{"msec":1e30}`, 0, false},
		{`{"msec":-1}`, 0, false},
		{`{}`, 0, false},
	} {
		var rec accessLogLine
		if err := json.Unmarshal([]byte(tc.line), &rec); err != nil {
			t.Fatal(err)
		}
		got, ok := accessLogSecond(rec)
		if ok != tc.ok || ok && got != tc.want {
			t.Errorf("%s: got (%d,%v), want (%d,%v)", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

func TestTimelineReadsCompressedMsecLogs(t *testing.T) {
	runs := t.TempDir()
	runID := "20000101-000000"
	dir := filepath.Join(runs, runID, "raw")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	log := `{"msec":"1788669678.1","status":200,"response_time":0.01,"body_bytes":10}
{"msec":1788669678.9,"status":"500","response_time":0.03,"body_bytes":20}
{"msec":"1788669679.1","status":204,"response_time":0.02,"body_bytes":0}
{"msec":"broken","status":200}
`
	for name, data := range map[string]string{"access-host-1.log.zst": log, "access-host-2.log.zst": ""} {
		if err := os.WriteFile(filepath.Join(dir, name), encoder.EncodeAll([]byte(data), nil), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a := &app{runsDir: runs}
	r := httptest.NewRequest("GET", "/api/runs/"+runID+"/timeline", nil)
	r.SetPathValue("run_id", runID)
	w := httptest.NewRecorder()
	a.handleTimeline(w, r)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	var got timelineResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Available || len(got.Buckets) != 2 || got.Buckets[0].Requests != 2 || got.Buckets[0].Status5xx != 1 || got.Buckets[1].Requests != 1 {
		t.Fatalf("unexpected timeline: %+v", got)
	}
}

func TestTimelineBenchErrorsWithoutAccessLogs(t *testing.T) {
	if _, err := exec.LookPath("duckdb"); err != nil {
		t.Skip("duckdb is not installed")
	}
	runs := filepath.Join(t.TempDir(), "runs")
	runID := "20260913-201155"
	dir := filepath.Join(runs, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The benchmarker's own line format lives in tools/contest/analysis-schema/,
	// so this fixture only carries the wrapper markers every contest gets. Assert
	// parsed errors in a contest test next to the patterns that produce them.
	log := `2026-09-13T11:11:55Z BENCHMARK_START
2026-09-13T11:13:19Z benchmarker output in the contest's own format
2026-09-13T11:14:00Z BENCHMARK_END
`
	a := &app{runsDir: runs, analysisDB: filepath.Join(t.TempDir(), "analysis.duckdb")}
	for _, available := range []bool{false, true} {
		if available {
			if err := os.WriteFile(filepath.Join(dir, "bench.log"), []byte(log), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		raw, err := os.ReadFile("../../analysis/schema/bench-errors.sql")
		if err != nil {
			t.Fatal(err)
		}
		semantic, err := os.ReadFile("../../contest/analysis-schema/bench-errors-semantic.sql")
		if err != nil {
			t.Fatal(err)
		}
		sql := string(semantic)
		if available {
			sql = "set variable run_glob = '" + strings.ReplaceAll(dir, "'", "''") + "';\n" + string(raw) + "\ncreate or replace table bench_error_logs as select * from bench_error_logs_raw;\n" + sql
		}
		cmd := exec.Command("duckdb", a.analysisDB, "-c", sql)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		r := httptest.NewRequest("GET", "/api/runs/"+runID+"/timeline", nil)
		r.SetPathValue("run_id", runID)
		w := httptest.NewRecorder()
		a.handleTimeline(w, r)
		var got timelineResponse
		if w.Code != 200 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body)
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Available || got.BenchLogAvailable != available || got.BenchErrors == nil {
			t.Fatalf("unexpected response: %+v", got)
		}
		if available {
			// The placeholder patterns classify nothing, so the views stay empty
			// while the query path and the JSON contract are still exercised.
			if len(got.BenchErrors) != 0 || len(got.BenchErrorCounts) != 0 {
				t.Fatalf("unexpected parsed output: %+v %+v", got.BenchErrors, got.BenchErrorCounts)
			}
			_, _, events, _, err := a.readBenchErrors(context.Background(), runID)
			if err != nil || len(events) != 0 {
				t.Fatalf("unexpected warning events: %v, err=%v", events, err)
			}
		}
	}
}

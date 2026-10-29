package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeSeriesKeepsSparseValuesAndFiltersWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.tsv")
	body := "sample\ttimestamp\telapsed_ms\tmetric\tvalue\n" +
		"0\t2026-01-01T00:00:00Z\t0\tgo_goroutines\t10\n" +
		"2\t2026-01-01T00:00:02Z\t2000\tgo_goroutines\t7\n" +
		"1\t2026-01-01T00:00:01Z\t1000\tgo_goroutines\t0\n" +
		"1\t2026-01-01T00:00:01Z\t1000\tgo_heap_live_bytes\tNaN\n" +
		"1\t2026-01-01T00:00:01Z\t1000\tgo_process_started_unix_ms\t1\n" +
		"3\t2026-01-01T00:00:03Z\t3000\tgo_goroutines\t9\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	start, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:01Z")
	series, err := parseRuntimeSeries(path, &analysisWindow{StartedAt: start, EndedAt: start.Add(2 * time.Second)})
	if err != nil || len(series) != 2 {
		t.Fatalf("series=%+v err=%v", series, err)
	}
	if value, ok := series[0].Values["go_goroutines"]; !ok || value != 0 {
		t.Fatal("actual zero was dropped")
	}
	if _, ok := series[0].Values["go_heap_live_bytes"]; ok {
		t.Fatal("non-finite measurement was imported")
	}
	if series[0].ElapsedMs != 1000 || series[1].ElapsedMs != 2000 {
		t.Fatal(series)
	}
}

func TestMetricsAPIIncludesRuntimeOnlyHosts(t *testing.T) {
	root := t.TempDir()
	runID := "20260101-000000"
	dir := filepath.Join(root, runID)
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "sample\ttimestamp\telapsed_ms\tmetric\tvalue\n0\t2026-01-01T00:00:00Z\t0\tgo_goroutines\t7\n"
	for _, host := range []string{"app-b", "app-a"} {
		if err := os.WriteFile(filepath.Join(dir, host+"-runtime-metrics.tsv"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a := &app{runsDir: root}
	request := httptest.NewRequest("GET", "/api/runs/"+runID+"/metrics", nil)
	request.SetPathValue("run_id", runID)
	response := httptest.NewRecorder()
	a.handleMetrics(response, request)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var got metricsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RuntimeScope != "collection" || len(got.Runtimes) != 2 || got.Runtimes[0].Host != "app-a" || len(got.Hosts) != 0 {
		t.Fatalf("unexpected runtime response: %+v", got)
	}
	runs, err := a.listRuns()
	if err != nil || len(runs) != 1 || !runs[0].HasMetrics {
		t.Fatalf("runtime-only run not reported: %+v %v", runs, err)
	}
}

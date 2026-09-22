package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHandleMetricsUsesAnalysisWindow(t *testing.T) {
	if _, err := exec.LookPath("duckdb"); err != nil {
		t.Skip("duckdb is not installed")
	}

	runsDir := filepath.Join(t.TempDir(), "runs")
	runID := "20260917-000000"
	runDir := filepath.Join(runsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	proc := "sample\ttimestamp\telapsed_ms\tcpu_busy_pct\tmem_total_bytes\tmem_used_bytes\n" +
		"0\t2026-09-17T00:00:00Z\t0\t1\t100\t50\n" +
		"1\t2026-09-17T00:00:01Z\t1000\t2\t100\t50\n" +
		"2\t2026-09-17T00:00:02Z\t2000\t3\t100\t50\n" +
		"3\t2026-09-17T00:00:03Z\t3000\t4\t100\t50\n"
	service := "service\ttimestamp\telapsed_ms\tavailable\tcpu_pct\n" +
		"app.service\t2026-09-17T00:00:00Z\t0\t1\t10\n" +
		"app.service\t2026-09-17T00:00:01Z\t1000\t1\t20\n" +
		"app.service\t2026-09-17T00:00:02Z\t2000\t1\t30\n" +
		"app.service\t2026-09-17T00:00:03Z\t3000\t1\t40\n"
	if err := os.WriteFile(filepath.Join(runDir, "isucon-1-proc-metrics.tsv"), []byte(proc), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "isucon-1-service-metrics.tsv"), []byte(service), 0o600); err != nil {
		t.Fatal(err)
	}

	analysisDB := filepath.Join(t.TempDir(), "analysis.duckdb")
	sql := `create table analysis_windows (
 run_id varchar primary key,
 started_at timestamptz,
 ended_at timestamptz,
 status varchar
);
insert into analysis_windows values
 ('20260917-000000', timestamptz '2026-09-17 00:00:01+00', timestamptz '2026-09-17 00:00:03+00', 'ok');`
	cmd := exec.Command("duckdb", analysisDB, "-c", sql)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create analysis DB: %v: %s", err, output)
	}

	a := &app{runsDir: runsDir, analysisDB: analysisDB}
	req := httptest.NewRequest("GET", "/api/runs/"+runID+"/metrics", nil)
	req.SetPathValue("run_id", runID)
	recorder := httptest.NewRecorder()
	a.handleMetrics(recorder, req)
	if recorder.Code != 200 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}

	var got metricsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Hosts) != 1 || len(got.Hosts[0].Series) != 2 {
		t.Fatalf("unexpected host series: %+v", got.Hosts)
	}
	if got.Hosts[0].Series[0].ElapsedMs != 1000 || got.Hosts[0].Series[0].CPUBusyPct != 2 ||
		got.Hosts[0].Series[1].ElapsedMs != 2000 || got.Hosts[0].Series[1].CPUBusyPct != 3 {
		t.Fatalf("unexpected host points: %+v", got.Hosts[0].Series)
	}
	if len(got.Services) != 1 || len(got.Services[0].Series) != 2 {
		t.Fatalf("unexpected service series: %+v", got.Services)
	}
	if got.Services[0].Series[0].ElapsedMs != 1000 || got.Services[0].Series[0].CPUPct != 20 ||
		got.Services[0].Series[1].ElapsedMs != 2000 || got.Services[0].Series[1].CPUPct != 30 {
		t.Fatalf("unexpected service points: %+v", got.Services[0].Series)
	}
}

func TestHandleMetricsKeepsAllRowsWithoutAnalysisWindow(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "runs")
	runID := "20260917-000001"
	runDir := filepath.Join(runsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := "sample\ttimestamp\telapsed_ms\tcpu_busy_pct\n" +
		"0\t2026-09-17T00:00:00Z\t0\t1\n" +
		"1\t2026-09-17T00:00:01Z\t1000\t2\n"
	if err := os.WriteFile(filepath.Join(runDir, "isucon-1-proc-metrics.tsv"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	a := &app{runsDir: runsDir}
	req := httptest.NewRequest("GET", "/api/runs/"+runID+"/metrics", nil)
	req.SetPathValue("run_id", runID)
	recorder := httptest.NewRecorder()
	a.handleMetrics(recorder, req)
	if recorder.Code != 200 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}

	var got metricsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Hosts) != 1 || len(got.Hosts[0].Series) != 2 {
		t.Fatalf("unexpected fallback series: %+v", got.Hosts)
	}
}

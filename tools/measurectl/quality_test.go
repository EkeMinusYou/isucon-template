package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveLoadWindow(t *testing.T) {
	log := strings.Join([]string{
		"2026-09-02T02:43:19.037Z\tBENCHMARK_START",
		"2026-09-02T02:44:19.339Z\tBENCHMARK_END",
	}, "\n")
	window := resolveLoadWindow([]byte(log))
	if window.Status != "ok" || window.DurationMS != 60302 || window.Source != "bench.log" {
		t.Fatalf("window = %#v", window)
	}
}

func TestResolveLoadWindowRequiresBothMarkers(t *testing.T) {
	window := resolveLoadWindow([]byte("2026-09-02T02:43:19.037Z\tBENCHMARK_START\n"))
	if window.Status != "unavailable" || window.Reason == "" {
		t.Fatalf("window = %#v", window)
	}
}

func TestProcMetricsIsPeriodic(t *testing.T) {
	interval, ok := periodicArtifactInterval("isucon-1-proc-metrics.tsv")
	if !ok || interval != time.Second {
		t.Fatalf("interval = %v, ok = %t", interval, ok)
	}
}

func TestInspectPeriodicTSVMeasuresWindowCoverage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "isucon-1-proc-metrics.tsv")
	var body strings.Builder
	body.WriteString("sample\ttimestamp\telapsed_ms\tcpu_busy_pct\n")
	start := time.Date(2026, 9, 2, 2, 43, 19, 0, time.UTC)
	for index := 0; index < 60; index++ {
		body.WriteString(strings.Join([]string{
			string(rune('0' + index%10)),
			start.Add(time.Duration(index) * time.Second).Format(time.RFC3339Nano),
			"0",
			"50",
		}, "\t") + "\n")
	}
	if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	quality := inspectPeriodicTSV(path, LoadWindow{
		StartedAt: start.Format(time.RFC3339Nano),
		EndedAt:   start.Add(60 * time.Second).Format(time.RFC3339Nano),
		Status:    "ok",
	}, time.Second)
	if quality.Status != "valid" || quality.InWindowSamples != 60 || quality.ExpectedSamples != 60 || quality.WindowCoveragePct != 100 {
		t.Fatalf("quality = %#v", quality)
	}
}

func TestInspectPeriodicTSVRejectsLargeGap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysql-status.tsv")
	start := time.Date(2026, 9, 2, 2, 43, 19, 0, time.UTC)
	body := "sample\ttimestamp\telapsed_ms\tthreads_running\n" +
		"0\t" + start.Format(time.RFC3339Nano) + "\t0\t1\n" +
		"1\t" + start.Add(10*time.Second).Format(time.RFC3339Nano) + "\t10000\t1\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	quality := inspectPeriodicTSV(path, LoadWindow{
		StartedAt: start.Format(time.RFC3339Nano),
		EndedAt:   start.Add(11 * time.Second).Format(time.RFC3339Nano),
		Status:    "ok",
	}, time.Second)
	if quality.Status != "invalid" || !strings.Contains(quality.Reason, "maximum sample gap") {
		t.Fatalf("quality = %#v", quality)
	}
}

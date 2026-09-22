package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDetectTrafficWindowUsesFirstLowBucketAsEnd(t *testing.T) {
	collection := collectionWindow{
		StartedAt: time.Unix(100, 0).UTC(),
		EndedAt:   time.Unix(120, 0).UTC(),
	}
	buckets := map[int64]int64{
		105: 10,
		106: 12,
		107: 8,
		108: 6,
		109: 4,
		110: 3,
		111: 2,
	}

	start, end, ok, reason := detectTrafficWindow(collection, buckets)
	if !ok {
		t.Fatalf("detectTrafficWindow failed: %s", reason)
	}
	if want := time.Unix(105, 0).UTC(); !start.Equal(want) {
		t.Fatalf("start=%s, want %s", start, want)
	}
	if want := time.Unix(109, 0).UTC(); !end.Equal(want) {
		t.Fatalf("end=%s, want %s", end, want)
	}
}

func TestDetectTrafficWindowFallsBackWhenEndThresholdIsMissing(t *testing.T) {
	collection := collectionWindow{
		StartedAt: time.Unix(100, 0).UTC(),
		EndedAt:   time.Unix(112, 500_000_000).UTC(),
	}
	buckets := map[int64]int64{
		105: 10,
		106: 10,
		107: 7,
		108: 7,
		109: 7,
		110: 7,
		111: 7,
	}

	start, end, ok, reason := detectTrafficWindow(collection, buckets)
	if !ok {
		t.Fatalf("detectTrafficWindow failed: %s", reason)
	}
	if want := time.Unix(105, 0).UTC(); !start.Equal(want) {
		t.Fatalf("start=%s, want %s", start, want)
	}
	if !end.Equal(collection.EndedAt) {
		t.Fatalf("end=%s, want collection end %s", end, collection.EndedAt)
	}
	if !strings.Contains(reason, "collection end") {
		t.Fatalf("reason=%q does not describe the fallback", reason)
	}
}

func TestResolveAnalysisWindowCountsRequestStartTimes(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, "run-1")
	if err := os.MkdirAll(filepath.Join(runDir, "raw"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1_800_000_000, 0).UTC()
	end := start.Add(20 * time.Second)
	bench := start.Format(time.RFC3339Nano) + "\tBENCHMARK_START\n" + end.Format(time.RFC3339Nano) + "\tBENCHMARK_END\n"
	if err := os.WriteFile(filepath.Join(runDir, "bench.log"), []byte(bench), 0o600); err != nil {
		t.Fatal(err)
	}

	var lines []string
	appendRequests := func(sec int64, count int, responseTime float64) {
		for i := 0; i < count; i++ {
			msec := float64(sec) + 0.3 + float64(i)/1000.0
			lines = append(lines, `{"msec":"`+strconv.FormatFloat(msec, 'f', 3, 64)+`","response_time":`+strconv.FormatFloat(responseTime, 'f', 3, 64)+`}`)
		}
	}
	appendRequests(start.Unix()+5, 10, 0.2)
	appendRequests(start.Unix()+6, 10, 0.2)
	appendRequests(start.Unix()+7, 4, 0.2)
	appendRequests(start.Unix()+8, 3, 0.2)
	appendRequests(start.Unix()+9, 2, 0.2)
	if err := os.WriteFile(filepath.Join(runDir, "raw", "access-host.log"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	window := resolveAnalysisWindow(runDir)
	if window.Status != "ok" || window.Source != "traffic-threshold" {
		t.Fatalf("window=%+v", window)
	}
	if want := start.Add(5 * time.Second); !window.StartedAt.Equal(want) {
		t.Fatalf("start=%s, want %s", window.StartedAt, want)
	}
	if want := start.Add(7 * time.Second); !window.EndedAt.Equal(want) {
		t.Fatalf("end=%s, want %s", window.EndedAt, want)
	}
	if window.RequestCount != 20 || window.ActiveSeconds != 2 {
		t.Fatalf("request stats=%d/%d, want 20/2", window.RequestCount, window.ActiveSeconds)
	}
}

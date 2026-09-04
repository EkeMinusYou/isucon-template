package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBenchRunnerFinalizesAutomaticRunOnce(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	state := filepath.Join(tmp, "raw", "current-run-id")
	results := filepath.Join(tmp, "runs")
	callLog := filepath.Join(tmp, "task-calls.log")
	fakeTask := filepath.Join(tmp, "task")
	body := `#!/bin/sh
set -eu
case "$1" in
  before-bench)
    mkdir -p "$ISUCON_BENCH_RESULTS_DIR/20260901-120000" "$(dirname "$ISUCON_BENCH_RUN_STATE_FILE")"
    printf '%s\n' 20260901-120000 > "$ISUCON_BENCH_RUN_STATE_FILE"
    ;;
  after-bench)
    printf '%s\n' "$*" >> "$ISUCON_TEST_CALL_LOG"
    ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(fakeTask, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "tools/bench/run.sh", "auto", "--", "sh", "-c", "echo 'score: 10'; echo BENCHMARK_PASS")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"ISUCON_BENCH_RUN_STATE_FILE="+state,
		"ISUCON_BENCH_RESULTS_DIR="+results,
		"ISUCON_BENCH_COLLECT_FLAGS=-no-collectors",
		"ISUCON_TEST_CALL_LOG="+callLog,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bench runner: %v\n%s", err, output)
	}
	calls, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(calls), "after-bench"); got != 1 {
		t.Fatalf("after-bench calls = %d:\n%s", got, calls)
	}
	if !strings.Contains(string(calls), "MEASURECTL_COLLECT_FLAGS=-no-collectors") {
		t.Fatalf("collector flags were not preserved:\n%s", calls)
	}
	benchLog, err := os.ReadFile(filepath.Join(results, "20260901-120000", "bench.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"BENCHMARK_START", "BENCHMARK_END", "BENCHMARK_PASS"} {
		if !strings.Contains(string(benchLog), marker) {
			t.Errorf("bench.log does not contain %s:\n%s", marker, benchLog)
		}
	}
}

func TestBenchRunnerStopsWhenBeforeBenchFails(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	fakeTask := filepath.Join(tmp, "task")
	callLog := filepath.Join(tmp, "task-calls.log")
	benchmarkMarker := filepath.Join(tmp, "benchmark-ran")
	body := `#!/bin/sh
printf '%s\n' "$*" >> "$ISUCON_TEST_CALL_LOG"
[ "$1" != before-bench ] || exit 7
`
	if err := os.WriteFile(fakeTask, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "tools/bench/run.sh", "auto", "--", "sh", "-c", "touch \"$ISUCON_TEST_BENCHMARK_MARKER\"")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"ISUCON_BENCH_RUN_STATE_FILE="+filepath.Join(tmp, "raw", "current-run-id"),
		"ISUCON_BENCH_RESULTS_DIR="+filepath.Join(tmp, "runs"),
		"ISUCON_TEST_CALL_LOG="+callLog,
		"ISUCON_TEST_BENCHMARK_MARKER="+benchmarkMarker,
	)
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("bench runner succeeded after before-bench failure:\n%s", output)
	}
	if _, err := os.Stat(benchmarkMarker); !os.IsNotExist(err) {
		t.Fatalf("benchmark command ran after before-bench failure: %v", err)
	}
	calls, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(calls); got != "before-bench MEASURECTL_COLLECT_FLAGS=\n" {
		t.Fatalf("task calls = %q", got)
	}
}

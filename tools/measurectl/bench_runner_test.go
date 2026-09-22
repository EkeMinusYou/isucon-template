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
[ "$1" != --silent ] || shift
case "$1" in
  before-bench)
    mkdir -p "$ISUCON_BENCH_RESULTS_DIR/20260901-120000" "$(dirname "$ISUCON_BENCH_RUN_STATE_FILE")"
    printf '%s\n' 20260901-120000 > "$ISUCON_BENCH_RUN_STATE_FILE"
    ;;
  after-bench)
    printf '%s\n' "$*" >> "$ISUCON_TEST_CALL_LOG"
    ;;
  bench-timestamp)
    echo '2026-09-01T03:00:10.123456789Z'
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
		"ISUCON_BENCH_PROFILES_ENABLED=false",
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

func TestBenchClockFailureFinalizesWithoutLocalFallback(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"auto", "manual"} {
		for _, failAt := range []string{"1", "2"} {
			t.Run(mode+"/clock"+failAt, func(t *testing.T) {
				tmp := t.TempDir()
				fake := `#!/bin/sh
set -eu
[ "$1" != --silent ] || shift
echo "$1" >> "$ISUCON_TEST_DIR/calls"
case "$1" in
before-bench)
  mkdir -p "$ISUCON_BENCH_RESULTS_DIR/20260901-120000"
  echo 20260901-120000 > "$ISUCON_BENCH_RUN_STATE_FILE"
  ;;
bench-timestamp)
  n=1
  if test -f "$ISUCON_TEST_DIR/clock-first"; then n=2; fi
  touch "$ISUCON_TEST_DIR/clock-first"
  [ "$n" != "$ISUCON_TEST_FAIL_AT" ] || exit 23
  echo '2026-09-01T03:00:10.123456789Z'
  ;;
after-bench) ;;
*) exit 96 ;;
esac
`
				if err := os.WriteFile(filepath.Join(tmp, "task"), []byte(fake), 0755); err != nil {
					t.Fatal(err)
				}
				args := []string{"tools/bench/run.sh", mode}
				if mode == "auto" {
					args = append(args, "--", "sh", "-c", `touch "$ISUCON_TEST_DIR/load-ran"; echo BENCHMARK_PASS`)
				}
				cmd := exec.Command("sh", args...)
				cmd.Dir = root
				cmd.Stdin = strings.NewReader("12\ny\n")
				cmd.Env = append(os.Environ(), "PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
					"ISUCON_TEST_DIR="+tmp, "ISUCON_TEST_FAIL_AT="+failAt,
					"ISUCON_BENCH_RUN_STATE_FILE="+filepath.Join(tmp, "state"),
					"ISUCON_BENCH_RESULTS_DIR="+filepath.Join(tmp, "runs"), "ISUCON_BENCH_PROFILES_ENABLED=false")
				output, err := cmd.CombinedOutput()
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
					t.Fatalf("clock failure=%v output=%s", err, output)
				}
				calls, err := os.ReadFile(filepath.Join(tmp, "calls"))
				if err != nil || strings.Count(string(calls), "after-bench") != 1 {
					t.Fatalf("finalize calls=%s err=%v", calls, err)
				}
				log, _ := os.ReadFile(filepath.Join(tmp, "runs", "20260901-120000", "bench.log"))
				if strings.Contains(string(log), "BENCHMARK_END") || failAt == "1" && strings.Contains(string(log), "BENCHMARK_START") {
					t.Fatalf("failed clock produced a marker: %s", log)
				}
				if mode == "auto" && failAt == "1" {
					if _, err := os.Stat(filepath.Join(tmp, "load-ran")); !os.IsNotExist(err) {
						t.Fatal("benchmark ran without a start timestamp")
					}
				}
			})
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
	if got := string(calls); got != "before-bench MEASURECTL_COLLECT_FLAGS= PROFILES_ENABLED=true\n" {
		t.Fatalf("task calls = %q", got)
	}
}

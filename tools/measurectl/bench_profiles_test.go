package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBenchProfilesStartBeforeLoadAndFinishBeforeFinalize(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mode                                            string
		benchExit, profileExit, readyExit, stopExit, wantExit int
	}{
		{"success", "auto", 0, 0, 0, 0, 0},
		{"benchmark failure", "auto", 7, 0, 0, 0, 7},
		{"profile failure", "auto", 0, 9, 0, 0, 9},
		{"readiness failure", "auto", 0, 0, 6, 0, 6},
		{"stop failure", "auto", 0, 0, 0, 8, 8},
		{"manual", "manual", 0, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			fake := `#!/bin/sh
set -eu
[ "$1" != --silent ] || shift
run="$ISUCON_BENCH_RESULTS_DIR/20260901-120000"
echo "$1" >> "$ISUCON_TEST_DIR/calls"
case "$1" in
before-bench)
  mkdir -p "$run" "$(dirname "$ISUCON_BENCH_RUN_STATE_FILE")"
  echo 20260901-120000 > "$ISUCON_BENCH_RUN_STATE_FILE"
  ;;
profiles-collect)
  echo PROFILE_CAPTURE_STARTED
  touch "$ISUCON_TEST_DIR/sampling"
  i=0
  until test -f "$ISUCON_TEST_DIR/stop-requested"; do
    i=$((i+1)); [ "$i" -lt 400 ] || exit 99
    sleep 0.01
  done
  sleep 0.05
  touch "$ISUCON_TEST_DIR/profile-done"
  echo PROFILE_CAPTURE_COMPLETED
  exit "$ISUCON_TEST_PROFILE_EXIT"
  ;;
profiles-stop)
  echo PROFILE_STOP_REQUESTED
  if [ "$ISUCON_TEST_READY_EXIT" = 0 ]; then
    grep -q BENCHMARK_END "$run/bench.log" || exit 93
  fi
  touch "$ISUCON_TEST_DIR/stop-requested"
  exit "$ISUCON_TEST_STOP_EXIT"
  ;;
profiles-ready)
  i=0
  until test -f "$ISUCON_TEST_DIR/sampling"; do
    i=$((i+1)); [ "$i" -lt 200 ] || exit 98
    sleep 0.01
  done
  if [ "$ISUCON_TEST_READY_EXIT" = 0 ]; then
    if test -f "$ISUCON_TEST_DIR/ready-first"; then
      test -f "$ISUCON_TEST_DIR/settled" || exit 94
    else
      touch "$ISUCON_TEST_DIR/ready-first"
      (sleep 0.9; touch "$ISUCON_TEST_DIR/settled") >/dev/null 2>&1 &
    fi
  fi
  exit "$ISUCON_TEST_READY_EXIT"
  ;;
after-bench)
  test -f "$ISUCON_TEST_DIR/profile-done" || exit 97
  ;;
bench-timestamp)
  if test -f "$ISUCON_TEST_DIR/clock-first"; then
    echo '2026-09-01T03:01:10.123456789Z'
  else
    touch "$ISUCON_TEST_DIR/clock-first"
    echo '2026-09-01T03:00:00.123456789Z'
  fi
  ;;
*) exit 96 ;;
esac
`
			if err := os.WriteFile(filepath.Join(tmp, "task"), []byte(fake), 0o755); err != nil {
				t.Fatal(err)
			}
			args := []string{"tools/bench/run.sh", tc.mode}
			if tc.mode == "auto" {
				// This is a local stub, never the contest benchmark executable.
				args = append(args, "--", "sh", "-c", fmt.Sprintf("test -f \"$ISUCON_TEST_DIR/sampling\" || exit 95; touch \"$ISUCON_TEST_DIR/load-ran\"; echo BENCHMARK_PASS; exit %d", tc.benchExit))
			}
			cmd := exec.Command("sh", args...)
			cmd.Dir = root
			cmd.Stdin = strings.NewReader("\n\n12\ny\n")
			var manualInput chan error
			if tc.mode == "manual" {
				reader, writer := io.Pipe()
				cmd.Stdin = reader
				defer reader.Close()
				manualInput = make(chan error, 1)
				go func() {
					defer writer.Close()
					if _, err := io.WriteString(writer, "\n"); err != nil {
						manualInput <- err
						return
					}
					deadline := time.Now().Add(5 * time.Second)
					for {
						if _, err := os.Stat(filepath.Join(tmp, "stop-requested")); err == nil {
							break
						}
						if time.Now().After(deadline) {
							manualInput <- fmt.Errorf("stop notification waited for result input")
							return
						}
						time.Sleep(5 * time.Millisecond)
					}
					_, err := io.WriteString(writer, "\n12\ny\n")
					manualInput <- err
				}()
			}
			cmd.Env = append(os.Environ(), "PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
				"ISUCON_TEST_DIR="+tmp, "ISUCON_BENCH_RESULTS_DIR="+filepath.Join(tmp, "runs"),
				"ISUCON_BENCH_RUN_STATE_FILE="+filepath.Join(tmp, "raw", "current-run-id"),
				"ISUCON_BENCH_PROFILES_ENABLED=true", "ISUCON_BENCH_COLLECT_FLAGS=",
				fmt.Sprintf("ISUCON_TEST_PROFILE_EXIT=%d", tc.profileExit), fmt.Sprintf("ISUCON_TEST_READY_EXIT=%d", tc.readyExit), fmt.Sprintf("ISUCON_TEST_STOP_EXIT=%d", tc.stopExit))
			out, err := cmd.CombinedOutput()
			if manualInput != nil {
				if err := <-manualInput; err != nil {
					t.Fatal(err)
				}
			}
			code := 0
			if err != nil {
				if e, ok := err.(*exec.ExitError); ok {
					code = e.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if code != tc.wantExit {
				t.Fatalf("exit=%d want=%d: %s", code, tc.wantExit, out)
			}
			calls, err := os.ReadFile(filepath.Join(tmp, "calls"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(calls), "after-bench") != 1 {
				t.Fatalf("calls=%s", calls)
			}
			if strings.Count(string(calls), "profiles-stop") != 1 {
				t.Fatalf("stop notification must happen once: %s", calls)
			}
			profileLog, err := os.ReadFile(filepath.Join(tmp, "runs", "20260901-120000", "profile-collection.log"))
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"PROFILE_CAPTURE_STARTED", "PROFILE_STOP_REQUESTED", "PROFILE_CAPTURE_COMPLETED"} {
				if !strings.Contains(string(profileLog), marker) {
					t.Fatalf("concurrent profile output lost %s: %s", marker, profileLog)
				}
			}
			if tc.readyExit == 0 && strings.Count(string(calls), "profiles-ready") != 2 {
				t.Fatalf("readiness was not checked again after the lead-in: %s", calls)
			}
			if tc.readyExit == 0 {
				log, err := os.ReadFile(filepath.Join(tmp, "runs", "20260901-120000", "bench.log"))
				if err != nil {
					t.Fatal(err)
				}
				for _, marker := range []string{"2026-09-01T03:00:00.123456789Z\tBENCHMARK_START", "2026-09-01T03:01:10.123456789Z\tBENCHMARK_END"} {
					if !strings.Contains(string(log), marker) {
						t.Fatalf("server clock marker missing: %s", log)
					}
				}
			}
			if tc.readyExit != 0 {
				if _, err := os.Stat(filepath.Join(tmp, "load-ran")); !os.IsNotExist(err) {
					t.Fatal("load ran before readiness")
				}
			}
		})
	}
}

func TestProfileTasksResolveConfiguredDuration(t *testing.T) {
	task, err := exec.LookPath("task")
	if err != nil {
		t.Skip("task is not installed")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(state, []byte("20000101-000000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"profiles-collect", "profiles-ready", "profiles-stop"} {
		// Task dry mode expands variables without running collectors or SSH.
		cmd := exec.Command(task, "--dry", name, "RUN_STATE_FILE="+state, "PROFILE_SECONDS=210")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v: %s", name, err, out)
		}
		for _, want := range []string{"/debug/fgprof?seconds=210", "-var profile_seconds=210", "-var profile_timeout_seconds=225", "-var profile_stop_url=http://127.0.0.1:6060/debug/profiles/stop"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("%s is missing %q: %s", name, want, out)
			}
		}
	}
}

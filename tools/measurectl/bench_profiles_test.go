package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBenchProfilesStartBeforeLoadAndFinishBeforeFinalize(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mode                                  string
		benchExit, profileExit, readyExit, wantExit int
	}{
		{"success", "auto", 0, 0, 0, 0},
		{"benchmark failure", "auto", 7, 0, 0, 7},
		{"profile failure", "auto", 0, 9, 0, 9},
		{"readiness failure", "auto", 0, 0, 6, 6},
		{"manual", "manual", 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			fake := `#!/bin/sh
set -eu
run="$ISUCON_BENCH_RESULTS_DIR/20260901-120000"
echo "$1" >> "$ISUCON_TEST_DIR/calls"
case "$1" in
before-bench)
  mkdir -p "$run" "$(dirname "$ISUCON_BENCH_RUN_STATE_FILE")"
  echo 20260901-120000 > "$ISUCON_BENCH_RUN_STATE_FILE"
  ;;
profiles-collect)
  touch "$ISUCON_TEST_DIR/sampling"
  if [ "$ISUCON_TEST_READY_EXIT" = 0 ]; then
    i=0
    until test -f "$run/bench.log" && grep -q BENCHMARK_END "$run/bench.log"; do
      i=$((i+1)); [ "$i" -lt 200 ] || exit 99
      sleep 0.01
    done
  fi
  sleep 0.05
  touch "$ISUCON_TEST_DIR/profile-done"
  exit "$ISUCON_TEST_PROFILE_EXIT"
  ;;
profiles-ready)
  i=0
  until test -f "$ISUCON_TEST_DIR/sampling"; do
    i=$((i+1)); [ "$i" -lt 200 ] || exit 98
    sleep 0.01
  done
  exit "$ISUCON_TEST_READY_EXIT"
  ;;
after-bench)
  test -f "$ISUCON_TEST_DIR/profile-done" || exit 97
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
			cmd.Stdin = strings.NewReader("12\ny\n")
			cmd.Env = append(os.Environ(), "PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
				"ISUCON_TEST_DIR="+tmp, "ISUCON_BENCH_RESULTS_DIR="+filepath.Join(tmp, "runs"),
				"ISUCON_BENCH_RUN_STATE_FILE="+filepath.Join(tmp, "raw", "current-run-id"),
				"ISUCON_BENCH_PROFILES_ENABLED=true", "ISUCON_BENCH_COLLECT_FLAGS=",
				fmt.Sprintf("ISUCON_TEST_PROFILE_EXIT=%d", tc.profileExit), fmt.Sprintf("ISUCON_TEST_READY_EXIT=%d", tc.readyExit))
			out, err := cmd.CombinedOutput()
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
	for _, name := range []string{"profiles-collect", "profiles-ready"} {
		// Task dry mode expands variables without running collectors or SSH.
		cmd := exec.Command(task, "--dry", name, "RUN_STATE_FILE="+state, "PROFILE_SECONDS=210")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v: %s", name, err, out)
		}
		for _, want := range []string{"/debug/fgprof?seconds=210", "-var profile_seconds=210", "-var profile_timeout_seconds=225"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("%s is missing %q: %s", name, want, out)
			}
		}
	}
}

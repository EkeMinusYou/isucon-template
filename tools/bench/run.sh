#!/bin/sh

# Owns the benchmark log window and guarantees exactly one finalize attempt.
set -eu

if [ "${1:-}" != -- ] || [ "$#" -lt 2 ]; then
  echo 'usage: run.sh -- benchmark command [args...]' >&2
  exit 2
fi
shift

state_file=${ISUCON_BENCH_RUN_STATE_FILE:?ISUCON_BENCH_RUN_STATE_FILE is required}
results_dir=${ISUCON_BENCH_RESULTS_DIR:?ISUCON_BENCH_RESULTS_DIR is required}
collect_flags=${ISUCON_BENCH_COLLECT_FLAGS:-}
profiles_enabled=${ISUCON_BENCH_PROFILES_ENABLED:-true}
case "$profiles_enabled" in true|false) ;; *) echo 'profiles enabled must be true or false' >&2; exit 2 ;; esac
finalized=0
bench_output=''
profile_pid=''
profile_stop_requested=0
profile_stop_status=0

task before-bench MEASURECTL_COLLECT_FLAGS="$collect_flags" PROFILES_ENABLED="$profiles_enabled"
active_run_id=$(sh tools/bench/active-run-id.sh "$state_file")
run_dir=$results_dir/$active_run_id
# Portal adapters save the original result alongside the benchmark log.
ISUCON_BENCH_ARTIFACT_DIR=$run_dir
export ISUCON_BENCH_ARTIFACT_DIR

stop_profiles() {
  [ -n "$profile_pid" ] && [ "$profile_stop_requested" -eq 0 ] || return 0
  profile_stop_requested=1
  task profiles-stop >> "$run_dir/profile-collection.log" 2>&1 || profile_stop_status=$?
  if [ "$profile_stop_status" -ne 0 ]; then
    echo "profile stop notification failed (exit $profile_stop_status); waiting for capture upper bound; see $run_dir/profile-collection.log" >&2
  fi
}

finalize() {
  [ "$finalized" -eq 0 ] || return 0
  finalized=1
  # Complete captures through their HTTP responses, including interrupted runs.
  trap '' INT HUP TERM
  if [ -n "$bench_output" ] && [ -f "$bench_output" ]; then
    tee -a "$run_dir/bench.log" < "$bench_output"
    rm -f "$bench_output"
  fi
  stop_profiles
  profile_status=0
  if [ -n "$profile_pid" ]; then
    wait "$profile_pid" || profile_status=$?
    if [ "$profile_status" -ne 0 ]; then
      echo "profile collection failed (exit $profile_status); see $run_dir/profile-collection.log" >&2
    fi
  fi
  finalize_status=0
  task after-bench MEASURECTL_COLLECT_FLAGS="$collect_flags" || finalize_status=$?
  [ "$profile_status" -eq 0 ] || return "$profile_status"
  [ "$profile_stop_status" -eq 0 ] || return "$profile_stop_status"
  return "$finalize_status"
}

trap finalize EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM

if [ "$profiles_enabled" = true ]; then
  : > "$run_dir/profile-collection.log"
  # Both collection and stop notification append so neither overwrites output.
  task profiles-collect >> "$run_dir/profile-collection.log" 2>&1 &
  profile_pid=$!
  # Do not launch a benchmark until every app is sampling for this RUN.
  task profiles-ready
  # Leave a lead-in for the asynchronous CPU profile writer and small host
  # server clock offsets. Artifact validation still requires full coverage.
  sleep 1
  task profiles-ready
fi

# Profiles and access logs use server clocks, not the workstation clock.
# Assign separately so a failed clock read stops before emitting a marker.
started_at=$(task --silent bench-timestamp)
bench_output=$run_dir/.bench-output
printf '%s\tBENCHMARK_START\n' "$started_at" | tee "$run_dir/bench.log"
bench_status=0
"$@" >"$bench_output" 2>&1 || bench_status=$?
tee -a "$run_dir/bench.log" < "$bench_output"
rm -f "$bench_output"
bench_output=''
ended_at=$(task --silent bench-timestamp)
printf '%s\tBENCHMARK_END\n' "$ended_at" | tee -a "$run_dir/bench.log"
stop_profiles
[ "$bench_status" -eq 0 ] || printf 'BENCHMARK_FAIL\texit_status=%s\n' "$bench_status" | tee -a "$run_dir/bench.log"

finalize_status=0
finalize || finalize_status=$?
trap - EXIT HUP INT TERM
[ "$bench_status" -eq 0 ] || exit "$bench_status"
exit "$finalize_status"

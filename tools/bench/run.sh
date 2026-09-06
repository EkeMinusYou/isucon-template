#!/bin/sh

# Owns the benchmark log window and guarantees exactly one finalize attempt.
set -eu

mode=${1:-}
if [ "$mode" != auto ] && [ "$mode" != manual ]; then
  echo 'usage: run.sh auto|manual [-- benchmark command...]' >&2
  exit 2
fi
shift
if [ "${1:-}" = -- ]; then
  shift
fi
if [ "$mode" = auto ] && [ "$#" -eq 0 ]; then
  echo 'auto mode requires a benchmark command after --' >&2
  exit 2
fi

state_file=${ISUCON_BENCH_RUN_STATE_FILE:?ISUCON_BENCH_RUN_STATE_FILE is required}
results_dir=${ISUCON_BENCH_RESULTS_DIR:?ISUCON_BENCH_RESULTS_DIR is required}
collect_flags=${ISUCON_BENCH_COLLECT_FLAGS:-}
profiles_enabled=${ISUCON_BENCH_PROFILES_ENABLED:-false}
case "$profiles_enabled" in true|false) ;; *) echo 'profiles enabled must be true or false' >&2; exit 2 ;; esac
score=''
finalized=0
bench_output=''
profile_pid=''

task before-bench MEASURECTL_COLLECT_FLAGS="$collect_flags" PROFILES_ENABLED="$profiles_enabled"
active_run_id=$(sh tools/bench/active-run-id.sh "$state_file")
run_dir=$results_dir/$active_run_id

finalize() {
  [ "$finalized" -eq 0 ] || return 0
  finalized=1
  # Once finalization begins, preserve the bounded captures even on interruption.
  trap '' INT HUP TERM
  if [ -n "$bench_output" ] && [ -f "$bench_output" ]; then
    tee -a "$run_dir/bench.log" < "$bench_output"
    rm -f "$bench_output"
  fi
  profile_status=0
  if [ -n "$profile_pid" ]; then
    wait "$profile_pid" || profile_status=$?
    if [ "$profile_status" -ne 0 ]; then
      echo "profile collection failed (exit $profile_status); see $run_dir/profile-collection.log" >&2
    fi
  fi
  finalize_status=0
  task after-bench SCORE="$score" MEASURECTL_COLLECT_FLAGS="$collect_flags" || finalize_status=$?
  [ "$profile_status" -eq 0 ] || return "$profile_status"
  return "$finalize_status"
}

trap finalize EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM

if [ "$profiles_enabled" = true ]; then
  task profiles-collect > "$run_dir/profile-collection.log" 2>&1 &
  profile_pid=$!
  # Do not launch a benchmark until every app is sampling for this RUN.
  task profiles-ready
fi

if [ "$mode" = manual ]; then
  printf '%s\tBENCHMARK_START\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" > "$run_dir/bench.log"
  echo '別の端末やポータルから、手動でベンチを実行してください。'
  printf '完了後にスコアを入力してください（空欄可）: '
  read -r score || score=''
  printf '整合性チェックまで成功した場合は y を入力してください: '
  read -r passed || passed=''
  printf '%s\tBENCHMARK_END\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" >> "$run_dir/bench.log"
  case "$passed" in
    y|Y|yes|YES) echo 'BENCHMARK_PASS' >> "$run_dir/bench.log" ;;
    *) echo 'BENCHMARK_FAIL' >> "$run_dir/bench.log" ;;
  esac
  finalize_status=0
  finalize || finalize_status=$?
  trap - EXIT HUP INT TERM
  exit "$finalize_status"
fi

bench_output=$run_dir/.bench-output
printf '%s\tBENCHMARK_START\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" | tee "$run_dir/bench.log"
bench_status=0
"$@" >"$bench_output" 2>&1 || bench_status=$?
tee -a "$run_dir/bench.log" < "$bench_output"
rm -f "$bench_output"
bench_output=''
printf '%s\tBENCHMARK_END\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" | tee -a "$run_dir/bench.log"
[ "$bench_status" -eq 0 ] || printf 'BENCHMARK_FAIL\texit_status=%s\n' "$bench_status" | tee -a "$run_dir/bench.log"

finalize_status=0
finalize || finalize_status=$?
trap - EXIT HUP INT TERM
[ "$bench_status" -eq 0 ] || exit "$bench_status"
exit "$finalize_status"

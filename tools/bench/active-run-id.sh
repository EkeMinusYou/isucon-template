#!/bin/sh
# 進行中の RUN_ID を検証して標準出力へ返す。
#
# before-bench が RUN_ID を採番して raw/current-run-id に記録し、after-bench と
# 各 collector (proc / mysql metrics, fgprof) がこれを読んで同じ
# runs/<RUN_ID>/ へ成果物を集める。Taskfile の vars から
#   ACTIVE_RUN_ID:
#     sh: sh tools/bench/active-run-id.sh '{{.RUN_STATE_FILE}}'
# の形で呼ぶ。
#
# 使い方: active-run-id.sh [state_file]
set -eu

state_file="${1:-raw/current-run-id}"

if [ ! -f "$state_file" ]; then
  echo "active RUN_ID not found; run 'task before-bench' first" >&2
  exit 1
fi

run_id=$(sed -n '1p' "$state_file")

if ! printf '%s\n' "$run_id" | grep -Eq '^[0-9]{8}-[0-9]{6}$'; then
  echo "invalid RUN_ID in $state_file: $run_id" >&2
  exit 1
fi

printf '%s\n' "$run_id"

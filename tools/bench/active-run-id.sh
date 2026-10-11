#!/bin/sh
# 進行中の RUN_ID を検証して標準出力へ返す。
#
# The state contains the RUN ID on line 1 and its canonical worktree path on
# line 2. Reject a different worktree before returning the ID to any caller.
# Taskfile invokes this helper through dynamic variables:
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
owner=$(sed -n '2p' "$state_file")

if ! printf '%s\n' "$run_id" | grep -Eq '^[0-9]{8}-[0-9]{6}$'; then
  echo "invalid RUN_ID in $state_file: $run_id" >&2
  exit 1
fi

if [ -z "$owner" ] || [ "$(awk 'END { print NR }' "$state_file")" -ne 2 ]; then
  echo "missing or invalid worktree owner in $state_file" >&2
  exit 1
fi
current=$(git rev-parse --show-toplevel)
current=$(cd "$current" && pwd -P)
if [ "$owner" != "$current" ]; then
  echo "RUN $run_id belongs to worktree $owner; run this task there" >&2
  exit 1
fi

printf '%s\n' "$run_id"

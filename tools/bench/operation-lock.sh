#!/bin/sh

# Serializes server-changing operations and benchmark runs across every
# worktree of this repository. The kernel releases the lock when the holder
# exits, so a crashed operation never leaves a stale lock behind. Commands run
# under the lock inherit ISUCON_OPERATION_LOCK_HELD, so nested task calls from
# the holder pass through instead of deadlocking.
set -eu

if [ "$#" -eq 0 ]; then
  echo 'usage: operation-lock.sh command [args...]' >&2
  exit 2
fi
if [ -n "${ISUCON_OPERATION_LOCK_HELD:-}" ]; then
  exec "$@"
fi

lock=$(git rev-parse --path-format=absolute --git-common-dir)/isucon-operation.lock

if [ -n "${ISUCON_OPERATION_LOCK_ACQUIRED:-}" ]; then
  printf '%s\n' "$ISUCON_OPERATION_LOCK_ACQUIRED" > "$lock.holder"
  unset ISUCON_OPERATION_LOCK_ACQUIRED
  status=0
  ISUCON_OPERATION_LOCK_HELD=1 "$@" || status=$?
  rm -f "$lock.holder"
  exit "$status"
fi

wait=${OPERATION_LOCK_WAIT:-0}
holder="since $(date '+%Y-%m-%d %H:%M:%S') pid $$ in $(git rev-parse --show-toplevel): $*"
self=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
status=0
if command -v flock > /dev/null 2>&1; then
  flock -w "$wait" -E 75 "$lock" env ISUCON_OPERATION_LOCK_ACQUIRED="$holder" sh "$self" "$@" || status=$?
else
  lockf -k -s -t "$wait" "$lock" env ISUCON_OPERATION_LOCK_ACQUIRED="$holder" sh "$self" "$@" || status=$?
fi
if [ "$status" -eq 75 ]; then
  echo "operation lock is held $(cat "$lock.holder" 2> /dev/null || echo by another process)" >&2
  echo 'wait for it to finish, or set OPERATION_LOCK_WAIT=<seconds> to wait; never delete the lock by hand' >&2
fi
exit "$status"

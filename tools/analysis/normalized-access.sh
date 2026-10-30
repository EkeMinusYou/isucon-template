#!/usr/bin/env bash
# Feed alp canonical labels without changing retained raw logs.
set -euo pipefail
config=$1
shift
zstdcat "$@" | tools/user-transition-metrics/user-transition-metrics -config "$config" -normalize-access-log

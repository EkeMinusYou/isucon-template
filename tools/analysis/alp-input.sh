#!/usr/bin/env bash
# All access files in a digester invocation belong to the same RUN.
set -euo pipefail
run_dir=$(dirname "$(dirname "$1")")
exec tools/analysis/normalized-access.sh "$run_dir/raw/api-routes.json" "$@"

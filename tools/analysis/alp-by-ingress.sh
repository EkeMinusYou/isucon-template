#!/usr/bin/env bash
# Aggregate each saved Go HTTP access log independently and retain its ingress host.
set -euo pipefail

if (($# < 2)); then
    echo "usage: $0 ROUTE_CONFIG access-<host>.log[.zst] ..." >&2
    exit 2
fi

for command in zstdcat alp jq; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "$command command not found in PATH" >&2
        exit 1
    fi
done

config=$1
shift

printf '%s\n' $'ingress_host\tcount\tmethod\turi\t2xx\t3xx\t4xx\t5xx\tmax\tavg\tsum\tp90\tp99\tsum_body\tavg_body'

for input in "$@"; do
    base=${input##*/}
    base=${base%.zst}
    if [[ $base != access-*.log ]]; then
        echo "cannot derive ingress host from access log path: $input" >&2
        exit 1
    fi
    host=${base#access-}
    host=${host%.log}
    tools/analysis/normalized-access.sh "$config" "$input" \
        | alp json --config alp.yml --decode-uri --query-string --format json \
        | jq -r --arg host "$host" '
            if length == 0 then empty
            else .[1:][] | ([$host] + .) | @tsv
            end
        '
done

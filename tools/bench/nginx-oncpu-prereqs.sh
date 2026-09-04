#!/usr/bin/env bash

set -euo pipefail

action=${1:-}
host=${2:-unknown}
probe_url=${3:-http://127.0.0.1/}
state=/tmp/isucon-nginx-oncpu-prereqs

install_prerequisites() {
	if [ -e "$state" ]; then
		echo "$state already exists; run deploy-nginx-oncpu-prereqs-cleanup first" >&2
		exit 1
	fi
	mkdir -m 700 "$state"

	tool_pkg="linux-tools-$(uname -r)"
	sudo apt-get -s install "$tool_pkg" | awk '$1 == "Inst" { print $2 }' | sort -u > "$state/candidates"
	if [ ! -s "$state/candidates" ]; then
		echo "no install candidates for $tool_pkg" >&2
		exit 1
	fi

	while IFS= read -r package; do
		if dpkg-query -W -f='${Status}' "$package" 2>/dev/null | grep -qx "install ok installed"; then
			printf '%s\n' "$package"
		fi
	done < "$state/candidates" | sort -u > "$state/baseline"

	sudo DEBIAN_FRONTEND=noninteractive apt-get install -y "$tool_pkg"
	comm -23 "$state/candidates" "$state/baseline" > "$state/added"
	command -v perf
	sudo perf version
	echo "[$host] temporary packages added: $(tr '\n' ' ' < "$state/added")"
}

cleanup_prerequisites() {
	test -s "$state/added"
	set --
	while IFS= read -r package; do
		case "$package" in
			linux-tools-*|linux-*-tools-*) set -- "$@" "$package" ;;
			*) echo "refusing unexpected package: $package" >&2; exit 1 ;;
		esac
	done < "$state/added"

	if [ "$#" -gt 0 ]; then
		sudo DEBIAN_FRONTEND=noninteractive apt-get remove -y -- "$@"
	fi
	while IFS= read -r package; do
		if dpkg-query -W -f='${Status}' "$package" 2>/dev/null | grep -qx "install ok installed"; then
			echo "temporary package remains installed: $package" >&2
			exit 1
		fi
	done < "$state/added"

	rm -f "$state/candidates" "$state/baseline" "$state/added"
	rmdir "$state"
	echo "[$host] temporary perf packages removed"
}

check_profiler() {
	test -s "$state/added"
	load_pid=""
	cleanup() {
		if [ -n "$load_pid" ]; then
			kill "$load_pid" 2>/dev/null || true
		fi
		sudo rm -f "$state/preflight.data"
		rm -f "$state/profiler" "$state/preflight.folded" "$state/preflight.json" \
			"$state/preflight.stdout" "$state/preflight.stderr"
	}
	trap cleanup EXIT HUP INT TERM

	(
		i=0
		while [ "$i" -lt 200 ]; do
			curl -ksS --max-time 1 "$probe_url" >/dev/null || true
			i=$((i + 1))
		done
	) &
	load_pid=$!

	"$state/profiler" -duration 2s -frequency 49 -max-workers 2 -max-samples 196 \
		-max-output-bytes 8388608 -call-graph dwarf,8192 \
		-raw-output "$state/preflight.data" -folded-output "$state/preflight.folded" \
		-metadata-output "$state/preflight.json" >"$state/preflight.stdout" 2>"$state/preflight.stderr"
	wait "$load_pid" || true
	load_pid=""

	test ! -s "$state/preflight.stderr"
	grep -Eq '"valid":[[:space:]]*true' "$state/preflight.json"
	samples=$(grep -oE '"sample_count":[[:space:]]*[0-9]+' "$state/preflight.json" | grep -oE '[0-9]+')
	unknown=$(grep -oE '"unknown_sample_count":[[:space:]]*[0-9]+' "$state/preflight.json" | grep -oE '[0-9]+')
	test "$samples" -gt 0
	echo "[$host] profiler preflight: samples=$samples stacks_with_unknown=$unknown"
	sort -k2,2nr "$state/preflight.folded" | sed -n '1,10p'
}

case "$action" in
	install) install_prerequisites ;;
	cleanup) cleanup_prerequisites ;;
	check) check_profiler ;;
	*) echo "usage: $0 install|cleanup|check HOST [PROBE_URL]" >&2; exit 2 ;;
esac

#!/usr/bin/env bash
# Report what this contest repository should give back to the template repository.
#
# Usage: diff.sh [<template-dir>]   DETAIL=1 also prints the unified diff of each change.
#
# Only the [template] pathspecs of tools/template/paths.txt are compared. Paths
# in [contest] and [exclude] are removed from broad [template] matches, while a
# file path explicitly listed in [template] remains an exception. The
# `# >>> contest values >>>` regions are dropped from both sides first, so
# per-contest values never appear as backport candidates.
set -uo pipefail

template_dir=${1:-../isucon-template}
if [ ! -f "$template_dir/Taskfile.yml" ]; then
  echo "template-diff: not a template checkout: $template_dir" >&2
  exit 2
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Lines of one [section] of the manifest, comments and blanks removed.
section() { sed -n "/^\[$1\]\$/,/^\[/p" tools/template/paths.txt | sed '/^\[/d; /^ *#/d; /^ *$/d'; }

set -f
template_spec=$(section template)
contest_spec=$(section contest)
exclude_spec=$(section exclude)

git ls-files --cached --others --exclude-standard -- $template_spec | sort -u > "$tmp/template-all"
# [contest] and [exclude] both narrow a broad [template] directory.
git ls-files --cached --others --exclude-standard -- $contest_spec $exclude_spec | sort -u > "$tmp/excluded"
comm -23 "$tmp/template-all" "$tmp/excluded" > "$tmp/template"

# An exact file path listed in [template] is an exception to the two above.
section template | while IFS= read -r path; do
  if [ -f "$path" ] && grep -Fxq "$path" "$tmp/excluded"; then
    echo "$path"
  fi
done >> "$tmp/template"
sort -u "$tmp/template" -o "$tmp/template"

git ls-files --cached --others --exclude-standard -- $template_spec $contest_spec $exclude_spec | sort -u > "$tmp/declared"

# The same selection on the template side, so files deleted here are reported.
git -C "$template_dir" ls-files -- $template_spec | sort -u > "$tmp/theirs-all"
git -C "$template_dir" ls-files -- $contest_spec $exclude_spec | sort -u > "$tmp/theirs-excluded"
comm -23 "$tmp/theirs-all" "$tmp/theirs-excluded" > "$tmp/theirs"
section template | while IFS= read -r path; do
  if grep -Fxq "$path" "$tmp/theirs-excluded"; then
    echo "$path"
  fi
done >> "$tmp/theirs"
sort -u "$tmp/theirs" -o "$tmp/theirs"
set +f
git ls-files --cached --others --exclude-standard | sort -u | comm -23 - "$tmp/declared" > "$tmp/unclassified"
comm -13 "$tmp/template" "$tmp/theirs" > "$tmp/removed"

strip_regions() {
  awk '/# >>> contest values >>>/ { skip = 1; print "  # <contest values omitted>"; next }
       /# <<< contest values <<</ { skip = 0; next }
       !skip' "$1"
}

: > "$tmp/added"; : > "$tmp/modified"
while IFS= read -r path; do
  if [ ! -e "$template_dir/$path" ]; then
    echo "$path" >> "$tmp/added"
  elif ! diff -q <(strip_regions "$path") <(strip_regions "$template_dir/$path") >/dev/null; then
    echo "$path" >> "$tmp/modified"
  fi
done < "$tmp/template"

grep -v -e '\.md$' -e '^tools/template/' "$tmp/template" | xargs grep -n '# contest:' > "$tmp/markers" 2>/dev/null

report() { # <title> <file> <prefix>
  [ -s "$2" ] || return 0
  printf '\n== %s (%d) ==\n' "$1" "$(wc -l < "$2" | tr -d ' ')"
  sed "s|^|$3|" "$2"
}

report 'templateに無い（新規にbackportする）' "$tmp/added" 'A  '
report '内容が異なる（差分をbackportする）' "$tmp/modified" 'M  '
report 'templateにのみある（こちらで削除済み）' "$tmp/removed" 'D  '
report '汎用ファイルに残る競技値（backport時に手で戻す）' "$tmp/markers" ''
report '未分類（tools/template/paths.txtへ追記する）' "$tmp/unclassified" '?  '

if [ -n "${DETAIL:-}" ]; then
  while IFS= read -r path; do
    printf '\n--- %s\n' "$path"
    diff -u <(strip_regions "$template_dir/$path") <(strip_regions "$path") | tail -n +3
  done < "$tmp/modified"
fi

printf '\ntemplate: %s\n' "$template_dir"

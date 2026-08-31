#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
snapshot="$root/docs/public-api.txt"
temporary=$(mktemp "${TMPDIR:-/tmp}/tempestkeep-api.XXXXXX")
trap 'rm -f "$temporary"' EXIT

packages=(api collect config model store)
for package in "${packages[@]}"; do
  if [[ "$package" != "${packages[0]}" ]]; then
    printf '\n' >>"$temporary"
  fi
  printf '## pkg/tempest/%s\n\n' "$package" >>"$temporary"
  (
    cd "$root"
    go doc -all "./pkg/tempest/$package"
  ) | awk '
    /^package / || /^(CONSTANTS|VARIABLES|FUNCTIONS|TYPES)$/ ||
    /^(const|var|type|func) / || /^\t/ || /^\)$/ || /^}$/ { print }
  ' >>"$temporary"
done

if [[ ${1:-} == "--update" ]]; then
  mv "$temporary" "$snapshot"
  trap - EXIT
  exit 0
fi

diff -u "$snapshot" "$temporary"

#!/usr/bin/env bash
# Reports the coverage of a Go cover profile and fails when it is below the
# required minimum.
#
# Usage: scripts/coverage.sh <profile> <minimum-percent> [exclude-pattern]
#
# The profile written by `go test -coverpkg=./...` contains every block once
# per test binary. `go tool cover` merges those duplicates by taking the
# highest count, so a block counts as covered when any test binary covered it.
set -euo pipefail

profile=${1:-coverage.out}
minimum=${2:-80}
exclude=${3:-}

if [ ! -f "$profile" ]; then
    echo "coverage profile $profile not found" >&2
    exit 1
fi

# Generated code and the wiring entry point are not worth covering.
if [ -n "$exclude" ]; then
    filtered="${profile%.out}.filtered.out"
    grep -v "$exclude" "$profile" >"$filtered"
    profile=$filtered
fi

total=$(go tool cover -func="$profile" | awk '/^total:/ { gsub("%", "", $3); print $3 }')

if [ -z "$total" ]; then
    echo "could not read the total coverage from $profile" >&2
    exit 1
fi

echo ""
echo "Total coverage: ${total}% (minimum ${minimum}%)"

awk -v total="$total" -v minimum="$minimum" 'BEGIN {
    if (total + 0 < minimum + 0) {
        printf "coverage %.2f%% is below the required %.2f%%\n", total, minimum
        exit 1
    }
    printf "coverage %.2f%% meets the required %.2f%%\n", total, minimum
}'

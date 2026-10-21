#!/bin/bash
# ci/parse_changelog.sh
# Extracts changes from the most recent release section of CHANGELOG.md
# and formats them for ArtifactHub annotations in Helm Chart.yaml.
#
# Usage:
#   ./ci/parse_changelog.sh [path/to/CHANGELOG.md] [path/to/Chart.yaml]
#
# If Chart.yaml is provided, it updates the file in-place using yq.
# Otherwise, it outputs the formatted YAML list to stdout.

set -euo pipefail

changelog_file="${1:-CHANGELOG.md}"
chart_file="${2:-}"

if [ ! -f "$changelog_file" ]; then
  echo "Error: Changelog file not found: $changelog_file" >&2
  exit 1
fi

# Find line of first version header (e.g. ## v0.20.0 (...))
header_line_num=$(grep -nE '^## v[0-9]+\.[0-9]+\.[0-9]+' "$changelog_file" | head -1 | cut -d: -f1)

if [ -z "$header_line_num" ]; then
  echo "Error: No version header found in $changelog_file" >&2
  exit 1
fi

total_lines=$(wc -l < "$changelog_file")

# Find the next version header line to delimit the section
next_header_line_num=$(grep -nE '^## v[0-9]+\.[0-9]+\.[0-9]+' "$changelog_file" | sed -n '2p' | cut -d: -f1)
if [ -z "$next_header_line_num" ]; then
  next_header_line_num=$((total_lines + 1))
fi

tmp_changes=$(mktemp)
trap 'rm -f "$tmp_changes"' EXIT

# Extract bullet items, strip GH issue links and trailing spaces, escape quotes
sed -n "${header_line_num},$((next_header_line_num - 1))p" "$changelog_file" | \
awk '
  /^\*/ {
    line = $0
    sub(/^\* /, "", line)
    sub(/\s*\[\[GH-[0-9]+\]\(.*\)\]\s*$/, "", line)
    sub(/[ \t\r\n]+$/, "", line)
    gsub(/"/, "\\\"", line)
    print "- kind: changed"
    printf "  description: \"%s\"\n", line
  }
' > "$tmp_changes"

if [ ! -s "$tmp_changes" ]; then
  echo "Warning: No bullet points found in latest changelog section" >&2
fi

if [ -n "$chart_file" ]; then
  if [ ! -f "$chart_file" ]; then
    echo "Error: Chart file not found: $chart_file" >&2
    exit 1
  fi
  yq eval --inplace '.annotations."artifacthub.io/changes" = load_str("'"$tmp_changes"'")' "$chart_file"
  echo "Successfully updated artifacthub.io/changes in $chart_file"
else
  cat "$tmp_changes"
fi

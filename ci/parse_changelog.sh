#!/bin/bash
# ci/parse_changelog.sh
# Extracts changes from the most recent release section of CHANGELOG.md (and optionally sk8l-ui changelog)
# and formats them for ArtifactHub annotations in Helm Chart.yaml.
#
# Usage:
#   ./ci/parse_changelog.sh [path/to/CHANGELOG.md] [path/to/Chart.yaml] [path/to/ui_changelog.md] [ui_version]
#   ./ci/parse_changelog.sh --raw [path/to/changelog.md] [target_version]
#
# If Chart.yaml is provided, it updates the file in-place using yq.
# Otherwise, it outputs the formatted YAML list to stdout.

set -euo pipefail

# Helper to find line numbers delimiting a release section
get_version_bounds() {
  local file="$1"
  local target_ver="${2:-}"
  local clean_ver="${target_ver#v}"
  local header_line_num=""

  if [ -n "$clean_ver" ]; then
    header_line_num=$(grep -nE "^## v${clean_ver}([ (]|$)" "$file" | head -1 | cut -d: -f1 || true)
  fi

  if [ -z "$header_line_num" ]; then
    header_line_num=$(grep -nE '^## v[0-9]+\.[0-9]+\.[0-9]+' "$file" | head -1 | cut -d: -f1 || true)
  fi

  if [ -z "$header_line_num" ]; then
    return 1
  fi

  local total_lines
  total_lines=$(wc -l < "$file")

  local relative_next
  relative_next=$(tail -n "+$((header_line_num + 1))" "$file" | grep -nE '^## v[0-9]+\.[0-9]+\.[0-9]+' | head -1 | cut -d: -f1 || true)

  local end_line
  if [ -n "$relative_next" ]; then
    end_line=$((header_line_num + relative_next - 1))
  else
    end_line=$total_lines
  fi

  echo "${header_line_num} ${end_line}"
}

# Extracts raw markdown content of a version section (excluding header)
extract_raw_section() {
  local file="$1"
  local target_ver="${2:-}"

  if [ ! -f "$file" ]; then
    echo "Error: File not found: $file" >&2
    return 1
  fi

  local bounds
  if ! bounds=$(get_version_bounds "$file" "$target_ver"); then
    echo "Warning: Version header not found in $file" >&2
    return 0
  fi

  local start_line end_line
  start_line=$(echo "$bounds" | awk '{print $1}')
  end_line=$(echo "$bounds" | awk '{print $2}')

  sed -n "$((start_line + 1)),${end_line}p" "$file" | awk '
    NF { found=1 }
    found { lines[NR] = $0 }
    END {
      last = NR
      while (last > 0 && lines[last] ~ /^[ \t\r\n]*$/) {
        last--
      }
      for (i = 1; i <= last; i++) {
        if (i in lines) print lines[i]
      }
    }
  '
}

# Parses bullet points from a version section into ArtifactHub changes YAML list
parse_changes_yaml() {
  local file="$1"
  local target_ver="${2:-}"
  local is_ui="${3:-0}"

  if [ ! -f "$file" ] || [ ! -s "$file" ]; then
    return 0
  fi

  local bounds
  if ! bounds=$(get_version_bounds "$file" "$target_ver"); then
    echo "Warning: Version header not found in $file" >&2
    return 0
  fi

  local start_line end_line
  start_line=$(echo "$bounds" | awk '{print $1}')
  end_line=$(echo "$bounds" | awk '{print $2}')

  sed -n "$((start_line + 1)),${end_line}p" "$file" | awk -v is_ui="$is_ui" '
    BEGIN {
      current_kind = "changed"
    }

    /^[A-Z][A-Z ]+:/ {
      sec = $0
      sub(/:.*/, "", sec)
      if (sec == "FEATURES") {
        current_kind = "added"
      } else if (sec == "BUG FIXES") {
        current_kind = "fixed"
      } else if (sec == "SECURITY") {
        current_kind = "security"
      } else if (sec == "DEPRECATIONS") {
        current_kind = "deprecated"
      } else if (sec == "REMOVALS") {
        current_kind = "removed"
      } else {
        current_kind = "changed"
      }
    }

    /^\*/ {
      line = $0
      sub(/^\*[ \t]*/, "", line)
      sub(/[ \t]*\[\[GH-[0-9]+\]\(.*\)\][ \t]*$/, "", line)
      sub(/[ \t\r\n]+$/, "", line)
      gsub(/"/, "\\\"", line)

      if (is_ui == 1) {
        if (line ~ /^(ui:|ui\/)/) {
          # already prefixed with ui
        } else if (sub(/^(js\/vue|js):[ \t]*/, "", line)) {
          line = "ui: " line
        } else if (line ~ /^[a-zA-Z0-9_-]+(\/[a-zA-Z0-9_-]+)*:/) {
          line = "ui/" line
        } else {
          line = "ui: " line
        }
      }

      print "- kind: " current_kind
      printf "  description: \"%s\"\n", line
    }
  '
}

# Handle --raw mode
if [ "${1:-}" = "--raw" ]; then
  extract_raw_section "${2:-}" "${3:-}"
  exit 0
fi

changelog_file="${1:-CHANGELOG.md}"
chart_file="${2:-}"
ui_changelog_file="${3:-}"
ui_version="${4:-}"

if [ ! -f "$changelog_file" ]; then
  echo "Error: Changelog file not found: $changelog_file" >&2
  exit 1
fi

tmp_changes=$(mktemp)
trap 'rm -f "$tmp_changes"' EXIT

# 1. Parse API changelog
parse_changes_yaml "$changelog_file" "" 0 >> "$tmp_changes"

# 2. Parse UI changelog if provided
if [ -n "$ui_changelog_file" ] && [ -f "$ui_changelog_file" ]; then
  parse_changes_yaml "$ui_changelog_file" "$ui_version" 1 >> "$tmp_changes"
fi

if [ ! -s "$tmp_changes" ]; then
  echo "Warning: No bullet points found in changelog sections" >&2
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

#!/bin/bash
# ci/clean_changelog.sh
# Cleans multi-line commit boilerplate from Dependabot release notes
# before writing to CHANGELOG.md, preserving the single-line format:
#   * ci/dependabot: <title> [[GH-<PR_NUM>](<url>)]
#
# Usage:
#   cat notes.md | ./ci/clean_changelog.sh
#   ./ci/clean_changelog.sh notes.md

set -euo pipefail

input_file="${1:-/dev/stdin}"

awk '
BEGIN {
  in_dep = 0
  dep_title = ""
}

# Match the start of a dependabot entry
/^\* (ci\/)?dependabot:/ {
  # If already on a single line with GH link, output directly
  if (match($0, /\[\[GH-[0-9]+\]\([^)]+\)\]/)) {
    print $0
    in_dep = 0
    next
  }
  in_dep = 1
  dep_title = $0
  next
}

in_dep {
  # Search for the issue/PR link in the trailing metadata of the dependabot entry
  if (match($0, /\[\[GH-[0-9]+\]\([^)]+\)\]/)) {
    gh_link = substr($0, RSTART, RLENGTH)
    print dep_title " " gh_link
    in_dep = 0
    next
  }
  # If a new bullet or section header begins before finding a link, flush title
  if ($0 ~ /^\* / || $0 ~ /^[A-Z]{3,}:/) {
    print dep_title
    in_dep = 0
    # Fall through to print the new line normally
  } else {
    # Skip intermediate commit noise (Bumps..., Release notes, updated-dependencies, Signed-off-by, etc.)
    next
  }
}

{
  print $0
}

END {
  if (in_dep) {
    print dep_title
  }
}
' "$input_file"

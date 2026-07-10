#!/bin/sh
set -eu

find . -name '*.go' -not -path './dist/*' -exec sh -c '
  status=0
  for file do
    lines=$(wc -l <"$file")
    if [ "$lines" -gt 800 ]; then
      echo "$file: $lines lines (limit 800)" >&2
      status=1
    fi
  done
  exit "$status"
' sh {} +

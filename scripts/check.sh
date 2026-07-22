#!/bin/sh
set -eu

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  echo "$unformatted" >&2
  exit 1
fi

scripts/check-go-file-lines.sh
scripts/check-architecture.sh
go test ./...
go vet ./...
go build ./...
go mod verify
go mod tidy
git diff --exit-code -- go.mod go.sum

if command -v govulncheck >/dev/null 2>&1; then
  govulncheck ./...
else
  echo "govulncheck unavailable; vulnerability scan skipped" >&2
fi

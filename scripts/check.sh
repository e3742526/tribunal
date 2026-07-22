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
# tidy -diff checks without mutating the tree and needs no git checkout.
go mod tidy -diff

if command -v govulncheck >/dev/null 2>&1; then
  govulncheck ./...
elif [ "${CI:-}" = "true" ]; then
  echo "govulncheck is required in CI; install golang.org/x/vuln/cmd/govulncheck" >&2
  exit 1
else
  echo "govulncheck unavailable; vulnerability scan skipped" >&2
fi

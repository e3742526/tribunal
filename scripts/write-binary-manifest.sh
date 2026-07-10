#!/bin/sh
set -eu

binary=$1
directory=$(dirname "$binary")
name=$(basename "$binary")
manifest="$binary.sha256"

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$directory" && sha256sum "$name") >"$manifest"
elif command -v shasum >/dev/null 2>&1; then
  (cd "$directory" && shasum -a 256 "$name") >"$manifest"
else
  echo "no SHA-256 tool available" >&2
  exit 1
fi

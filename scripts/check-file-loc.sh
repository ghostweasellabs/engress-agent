#!/usr/bin/env bash
# Enforces the 400-line soft target / 600-line hard cap on non-generated,
# non-test .go files (design spec §10). Generated code (*.pb.go) and
# _test.go files are exempt.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOFT_LIMIT=400
HARD_LIMIT=600
FAIL=0

while IFS= read -r -d '' file; do
  case "$file" in
    *_test.go|*.pb.go|*_grpc.pb.go) continue ;;
  esac
  lines="$(wc -l < "$file")"
  if (( lines > HARD_LIMIT )); then
    echo "FAIL $file: $lines lines (hard cap $HARD_LIMIT)"
    FAIL=1
  elif (( lines > SOFT_LIMIT )); then
    echo "WARN $file: $lines lines (soft target $SOFT_LIMIT)"
  fi
done < <(find "$ROOT" -type f -name '*.go' -not -path '*/vendor/*' -print0)

exit "$FAIL"

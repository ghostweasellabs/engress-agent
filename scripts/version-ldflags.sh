#!/usr/bin/env bash
# Emit -ldflags for engress-sdk/observability.Version and .Commit.
# Usage: go build -ldflags "$(scripts/version-ldflags.sh)" ./cmd/...
set -euo pipefail

SHA="$(git rev-parse --short HEAD)"
printf '%s' "-X github.com/ghostweasellabs/engress-sdk/observability.Version=${SHA} -X github.com/ghostweasellabs/engress-sdk/observability.Commit=${SHA}"

#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  build)
    mkdir -p bin
    go build -o bin/sirens-discord-ops ./cmd/sirens-discord-ops
    go build -o bin/sirens-echo ./cmd/sirens-echo
    go build -o bin/sirens-echo-eval ./cmd/sirens-echo-eval
    ;;
  format)
    mapfile -d '' go_files < <(find cmd internal -type f -name '*.go' -print0)
    gofmt -w "${go_files[@]}"
    ;;
  *)
    echo "unknown Ward action: ${1:-}" >&2
    exit 2
    ;;
esac

#!/usr/bin/env bash
set -euo pipefail

resolve_aos_provider() {
  local provider=/opt/substrate-seed/coilyco-flight-deck__agentic-os.git
  if [ ! -e "${provider}" ]; then
    provider=../../coilyco-flight-deck/agentic-os
  fi
  printf '%s\n' "${provider}"
}

with_community_bundle() {
  local bundle_path
  local scratch
  local provider
  local status
  scratch=$(mktemp -d)
  provider=$(resolve_aos_provider)
  scripts/build-community-bundle.sh "${provider}" "${scratch}/community-bundle"
  bundle_path="${scratch}/community-bundle"
  if command -v cygpath >/dev/null 2>&1; then
    bundle_path=$(cygpath -m "${bundle_path}")
  fi
  if SIRENS_ECHO_AOS_BUNDLE="${bundle_path}" "$@"; then
    status=0
  else
    status=$?
  fi
  rm -rf -- "${scratch}"
  return "${status}"
}

case "${1:-}" in
  build)
    mkdir -p bin
    go build -o bin/sirens-echo ./cmd/sirens-echo
    go build -o bin/sirens-echo-bundle-check ./cmd/sirens-echo-bundle-check
    go build -o bin/sirens-echo-eval ./cmd/sirens-echo-eval
    ;;
  bundle-check)
    with_community_bundle go run ./cmd/sirens-echo-bundle-check
    ;;
  run-echo)
    with_community_bundle go run ./cmd/sirens-echo
    ;;
  eval-echo)
    with_community_bundle go run ./cmd/sirens-echo-eval
    ;;
  format)
    while IFS= read -r -d '' go_file; do
      gofmt -w "${go_file}"
    done < <(find cmd internal -type f -name '*.go' -print0)
    ;;
  *)
    echo "unknown Ward action: ${1:-}" >&2
    exit 2
    ;;
esac

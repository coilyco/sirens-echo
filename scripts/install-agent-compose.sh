#!/usr/bin/env bash
# Pinned agent-compose and its roster for the role compose check and the image, vendored
# from agentic-os scripts/ci/install-agent-compose.sh. Bump version and all three sums
# together, from the release's SHA256SUMS.
set -euo pipefail

version=2.218.0
roster_sha256=e5dbd908bcd97f51d172431f511c0e4217a1301d21a5ad7f72d8470495b71723
base=${AGENT_COMPOSE_BASE:-https://github.com/coilyco/agent-compose/releases/download/v${version}}
dest=${AGENT_COMPOSE_DEST:-${RUNNER_TEMP:?RUNNER_TEMP is not set}/agent-compose}

case "$(uname -m)" in
  x86_64 | amd64)
    arch=amd64
    binary_sha256=1d820c56dd26107c49725ec13bc930d4eafc4e8ada9edd7c21d906ca170fd6fc
    ;;
  aarch64 | arm64)
    arch=arm64
    binary_sha256=cbc1ac13f026bcb8e2ec7d80875b5427d06fd0762562e574570246e50e317598
    ;;
  *)
    echo "the pinned agent-compose has no linux build for $(uname -m)" >&2
    exit 1
    ;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fetch() {
  curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "$base/$1" -o "$work/$1"
  echo "$2  $work/$1" | sha256sum -c - >/dev/null || {
    echo "$1 does not match the pinned checksum" >&2
    exit 1
  }
}

fetch "agent-compose-linux-${arch}" "$binary_sha256"
fetch agent-compose-roster.tar.gz "$roster_sha256"

# The binary finds its roster at ../share/agent-compose/roster from its own directory.
mkdir -p "$dest/bin" "$dest/share/agent-compose"
install -m 0755 "$work/agent-compose-linux-${arch}" "$dest/bin/agent-compose"
tar -xzf "$work/agent-compose-roster.tar.gz" -C "$dest/share/agent-compose"
echo "$dest/bin" >> "${GITHUB_PATH:-/dev/null}"
"$dest/bin/agent-compose" version

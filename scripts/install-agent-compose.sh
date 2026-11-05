#!/usr/bin/env bash
# Pinned agent-compose and roster, vendored from agentic-os scripts/ci. Bump the version
# and all three sums together. The release is private: docs/sirens-echo-compose.md.
set -euo pipefail

version=2.220.0
tag=agent-compose-v${version}
release_repo=coilyco/treehouse-servers
roster_sha256=f9c7bc0f593939b81829e42e407f2f2d54f1a617ab10770ebd95fc993d4d55d2
dest=${AGENT_COMPOSE_DEST:-${RUNNER_TEMP:?RUNNER_TEMP is not set}/agent-compose}

case "$(uname -m)" in
  x86_64 | amd64)
    arch=amd64
    binary_sha256=b084826ba3a1fe27b342b66b01f68ef49b4977902f21a9164c74ff30542c670f
    ;;
  aarch64 | arm64)
    arch=arm64
    binary_sha256=f95237d2a3be0a31d31136da6a07e14d6113dd603f82e7f6e152d616da566808
    ;;
  *)
    echo "the pinned agent-compose has no linux build for $(uname -m)" >&2
    exit 1
    ;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

token=${AGENT_COMPOSE_TOKEN:-}
if [ -z "$token" ] && [ -n "${AGENT_COMPOSE_TOKEN_FILE:-}" ]; then
  token=$(tr -d '[:space:]' <"$AGENT_COMPOSE_TOKEN_FILE")
fi
if [ -z "${AGENT_COMPOSE_BASE:-}" ] && [ -z "$token" ]; then
  echo "set AGENT_COMPOSE_BASE, AGENT_COMPOSE_TOKEN_FILE or AGENT_COMPOSE_TOKEN: ${tag} is private" >&2
  exit 1
fi

# The token rides in curl's header argument inside this function and nowhere else, so it
# is in no environment this script exports and in no layer.
fetch() {
  if [ -n "${AGENT_COMPOSE_BASE:-}" ]; then
    cp "${AGENT_COMPOSE_BASE#file://}/$1" "$work/$1"
  else
    local api=https://api.github.com/repos/${release_repo} id
    id=$(curl --retry 5 --retry-all-errors --retry-delay 2 -fsS \
      -H "Authorization: Bearer ${token}" -H "Accept: application/vnd.github+json" \
      "${api}/releases/tags/${tag}" | jq -r --arg n "$1" '.assets[] | select(.name == $n) | .id' | head -n1)
    [ -n "$id" ] || { echo "release ${tag} has no asset $1" >&2; exit 1; }
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      -H "Authorization: Bearer ${token}" -H "Accept: application/octet-stream" \
      "${api}/releases/assets/${id}" -o "$work/$1"
  fi
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

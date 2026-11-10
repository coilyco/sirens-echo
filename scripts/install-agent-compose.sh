#!/usr/bin/env bash
# Pinned agent-compose and roster from the private treehouse-servers release (COI-3302).
# Bump the version and all three sums together, from the release's SHA256SUMS.
set -euo pipefail

version=2.226.0
tag=agent-compose-v${version}
release_repo=coilyco/treehouse-servers
roster_sha256=6a03a603896b25f1c88d47220db2cd505fd0430a8ff34ae9d36d9aafbb6f415c
dest=${AGENT_COMPOSE_DEST:-${RUNNER_TEMP:?RUNNER_TEMP is not set}/agent-compose}

case "$(uname -m)" in
  x86_64 | amd64)
    arch=amd64
    binary_sha256=640abb082ab67e9a170c5839acb5c1b2495e21f5c60d37d3f323734b64480e35
    ;;
  aarch64 | arm64)
    arch=arm64
    binary_sha256=d4b06bf2be802dc150e40db5c796d4a961dda75bd385be9682da49c7abc4919e
    ;;
  *)
    echo "the pinned agent-compose has no linux build for $(uname -m)" >&2
    exit 1
    ;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# AGENT_COMPOSE_BASE reads from a directory instead, for a test that has no release.
token=${AGENT_COMPOSE_TOKEN:-}
if [ -z "$token" ] && [ -n "${AGENT_COMPOSE_TOKEN_FILE:-}" ]; then
  token=$(tr -d '[:space:]' <"$AGENT_COMPOSE_TOKEN_FILE")
fi
if [ -z "${AGENT_COMPOSE_BASE:-}" ] && [ -z "$token" ]; then
  echo "set AGENT_COMPOSE_TOKEN or AGENT_COMPOSE_TOKEN_FILE: ${tag} is a private release" >&2
  exit 1
fi

# The token rides in curl's header argument here and nowhere else, so it is in no
# exported environment and no image layer. curl drops it on the redirect to storage.
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

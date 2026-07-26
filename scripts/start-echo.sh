#!/usr/bin/bash
# start-echo.sh - ExecStart for the independently deployable Sirens Echo agent.

set -euo pipefail

if [ -x /home/linuxbrew/.linuxbrew/bin/brew ]; then
  eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
fi

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_DIR}"

git fetch --quiet origin main
git checkout --quiet main
git merge --ff-only --quiet origin/main

mkdir -p bin
go build -o bin/sirens-echo ./cmd/sirens-echo

fetch_ssm() {
  local name="$1"
  local value
  value="$(aguard ops aws ssm get-parameter \
    --name "${name}" \
    --with-decryption \
    --query Parameter.Value \
    --output text)"
  if [ -z "${value}" ]; then
    echo "FATAL: SSM fetch for ${name} returned empty" >&2
    exit 1
  fi
  printf '%s' "${value}"
}

DISCORD_TOKEN="$(fetch_ssm /sirens-echo/discord-bot-token)"
DISCORD_CHANNEL_ID="$(fetch_ssm /discord/channel/bots)"
FORGEJO_TOKEN="$(fetch_ssm /sirens-echo/forgejo-issue-token)"
AGENT_PROXY_MODEL="$(fetch_ssm /sirens-echo/agent-proxy-model)"
export DISCORD_TOKEN DISCORD_CHANNEL_ID FORGEJO_TOKEN AGENT_PROXY_MODEL

exec "${REPO_DIR}/bin/sirens-echo"

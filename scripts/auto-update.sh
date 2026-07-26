#!/usr/bin/bash
# auto-update.sh - ExecStart for sirens-discord-ops-update.service (its .timer
# drives it). Restarts the main service when origin/main moves. See docs/deploy.md.

set -euo pipefail

# Linuxbrew supplies go and git on PATH for the systemd context.
if [ -x /home/linuxbrew/.linuxbrew/bin/brew ]; then
  eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
fi

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_DIR}"

# set -e makes a failed fetch abort loud (non-zero, journal-visible) rather
# than silently skip the deploy check.
git fetch --quiet origin main

local_rev="$(git rev-parse main)"
remote_rev="$(git rev-parse origin/main)"

if [ "${local_rev}" = "${remote_rev}" ]; then
  echo "up to date at ${local_rev:0:12}. Nothing to deploy"
  exit 0
fi

# The native updater owns only the host-local admin bot.
echo "origin/main moved ${local_rev:0:12} -> ${remote_rev:0:12}. Restarting admin service"
sudo systemctl restart sirens-discord-ops

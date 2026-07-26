#!/usr/bin/bash
# install.sh - one-time idempotent bootstrap of sirens-discord-ops on kai-server.
# Run as `kai` from the checkout. See README "Production deploy" for the workflow.

set -euo pipefail

if [ -x /home/linuxbrew/.linuxbrew/bin/brew ]; then
  eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
fi

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "==> sudoers"
sudo install -m 0440 -o root -g root \
  "${REPO_DIR}/sudoers/kai-sirens-discord-ops" \
  /etc/sudoers.d/kai-sirens-discord-ops
sudo visudo -cf /etc/sudoers.d/kai-sirens-discord-ops

echo "==> systemd unit files"
sudo install -m 0644 "${REPO_DIR}/systemd/sirens-discord-ops.service" /etc/systemd/system/
sudo install -m 0644 "${REPO_DIR}/systemd/sirens-discord-ops-update.service" /etc/systemd/system/
sudo install -m 0644 "${REPO_DIR}/systemd/sirens-discord-ops-update.timer" /etc/systemd/system/

echo "==> daemon-reload + enable --now"
sudo systemctl daemon-reload
sudo systemctl enable --now sirens-discord-ops.service
# The auto-updater is the .timer (enable that), not the oneshot .service it
# drives. enable --now arms the 5-minute cadence and runs the first poll.
sudo systemctl enable --now sirens-discord-ops-update.timer

echo
echo "==> status"
sudo systemctl --no-pager status sirens-discord-ops.service | head -10 || true
sudo systemctl --no-pager list-timers sirens-discord-ops-update.timer || true
echo
echo "Verify with:"
echo "  sudo journalctl -u sirens-discord-ops -n 20 --no-pager -f"
echo "  sudo journalctl -u sirens-discord-ops-update -n 20 --no-pager"

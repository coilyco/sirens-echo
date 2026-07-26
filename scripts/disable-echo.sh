#!/usr/bin/bash
# disable-echo.sh - immediate rollback for the Sirens Echo Discord agent.

set -euo pipefail

sudo systemctl disable --now sirens-echo.service
sudo systemctl --no-pager status sirens-echo.service || true

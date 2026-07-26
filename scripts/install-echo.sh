#!/usr/bin/bash
# install-echo.sh - explicitly opt into the independently managed Echo unit.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SIRENS_ECHO_ENABLE=1 bash "${REPO_DIR}/scripts/install.sh"

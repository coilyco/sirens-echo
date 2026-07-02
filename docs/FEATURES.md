# Features

Baseline of `sirens-discord-ops`. Thin Discord-button passthrough to `coily` with audit logging. Out-of-scope (mod install, backups, log tailing, web UI, etc.) called out in the README.

## Discord bot core

- Role-gated admin commands via `ADMIN_ROLE_ID`. Ephemeral "not authorized" for non-admins.
- Bot lifecycle opens a Discord session and blocks until SIGTERM.

## Control panel UI

- Pinned control panel per game, posted on startup. v1 ships eco only.
- Message Components V2 with semantic button colors. Auto-migrates legacy V1 panels.
- In-place editing on subsequent startups; no duplicate panels.

## Button interaction flow

- Custom IDs in `sdo:<game>:<verb>` format.
- Two-step confirmation for destructive verbs (restart, stop, start). Ephemeral confirm/cancel prompts.
- Deferred interaction responses (past Discord's 3s window).

## Game server operations

- Eco verbs: restart, status, stop, start. Registry is extensible.
- Per-game verb lists + coily prefix mapping (`coily gaming eco restart`).
- All verbs shell out to coily. 5-minute timeout per invocation.
- One goroutine per invocation. No in-process concurrency protection (coily is the safety net).

## Audit logging

- Dedicated audit channel logs button press start + completion with actor mention.
- Full coily stdout/stderr captured. Tail-truncated past Discord's 2000-char limit with a journalctl pointer.
- Exit code captured per subprocess.

## Configuration

- Required env: Discord token, admin/audit channel IDs, admin role ID. Optional `COILY_BIN`.
- Prod secrets injected via SSM in `start.sh`. No env files on disk.

## Systemd

- `sirens-discord-ops.service` runs as `kai`. Graceful SIGTERM shutdown with context cancellation. Restart-on-failure: 5s backoff, 5-burst-per-60s limit. `network-online.target` dep.
- `sirens-discord-ops-update.timer` + `-update.service` run `scripts/auto-update.sh` every 5 minutes (oneshot) - the auto-updater.

## Deployment

- Continuous: `sirens-discord-ops-update.timer` fires `scripts/auto-update.sh` every 5 minutes. It polls `origin/main` and restarts the main service only when the checkout is behind, so `git push` deploys on its own.
- Immediate: `sudo systemctl restart sirens-discord-ops` on kai-server skips the timer wait.
- `start.sh` fast-forwards `main` to `origin/main` via `git merge --ff-only`, runs an incremental `go build`, refetches SSM at every start (credential rotation without re-running install), execs the binary. The SSM fetch is fail-loud: an empty/failed credential pull aborts the start rather than booting with an empty token.
- One-time `install.sh` places systemd units (main + updater) + sudoers config.
- See [deploy.md](deploy.md) for the unit layout, the auto-updater, and the fail-loud SSM fetch.

## Sudoers

- NOPASSWD for manual restart + coily diagnostic commands. Explicit systemctl argument matching. Covers both `/bin/systemctl` and `/usr/bin/systemctl`.

## Dev loop

- Makefile: `build`, `vet`, `test`, `tidy`, `run`. `make run` against Sirens Echo test server.
- Pre-commit: trailing whitespace, EOF, YAML, large-file/merge-conflict, line endings, `go mod tidy`, `go vet`, trufflehog. Commit-msg hook requires same-repo issue close.

## See also

- [README.md](../README.md) - human-facing intro.
- [AGENTS.md](../AGENTS.md) - agent-facing operating rules.
- [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).

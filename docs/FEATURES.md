# Features

Living inventory of the Sirens Discord deployment. The repository ships the
game-server admin control panel and the independently managed Sirens Echo
community agent.

## Sirens Echo community agent

- Separate `sirens-echo` Discord identity and systemd service. The existing
  admin bot token and process remain independent.
- Source-controlled agent definition pins the `siren community host` seat,
  `community` role, `ornith:35b` model, `#bots` boundary, recent-history
  budget, and repository-local skill roots.
- Mention-or-reply invocation inside the exact configured `#bots` channel.
  DMs, other channels, bots, self messages, and duplicate Gateway deliveries
  are rejected.
- Agent Proxy OpenAI-compatible client with a two-minute request ceiling,
  one-at-a-time dispatch, and Ward role, harness, repository, and request
  correlation.
- Strict JSON response parser and grounding checks reject malformed output,
  invented channels, overlong replies, unsupported issue types, and
  first-person action claims.
- Discord replies use soft message references with all mentions disabled,
  including the replied member.
- Repo-local skillpack under
  `.agents/skills/sirens-echo-community/`, loaded deterministically with its
  approved reference Markdown. No automatic memory.
- Unanswered questions and explicit corrections create or reuse sanitized
  ordinary Forgejo issues in this repository. The write client supports no
  labels or other tracker actions.
- Offline regression suite plus a non-mutating live Agent Proxy gate for the
  unknown-answer and explicit-correction cases.
- Explicit opt-in `sirens-echo.service` installer and independent disable
  script. See [sirens-echo.md](sirens-echo.md).

## Discord bot core

- Role-gated admin commands via `ADMIN_ROLE_ID`. Ephemeral "not authorized" for non-admins.
- Bot lifecycle opens a Discord session and blocks until SIGTERM.

## Control panel UI

- Pinned control panel per game, posted on startup. v1 ships eco only.
- Message Components V2 with semantic button colors. Auto-migrates legacy V1 panels.
- In-place editing on subsequent startups with no duplicate panels.

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
- Echo secrets injected independently from the Sirens Echo token, `#bots`
  channel, and repository-scoped Forgejo issue token SSM parameters.

## Systemd

- `sirens-discord-ops.service` runs as `kai`. Graceful SIGTERM shutdown with context cancellation. Restart-on-failure: 5s backoff, 5-burst-per-60s limit. `network-online.target` dep.
- `sirens-echo.service` runs the community agent independently with the same
  restart and graceful-shutdown behavior. The base installer places the unit
  but does not enable it.
- `sirens-discord-ops-update.timer` + `-update.service` run `scripts/auto-update.sh` every 5 minutes (oneshot) - the auto-updater.

## Deployment

- Continuous: `sirens-discord-ops-update.timer` fires `scripts/auto-update.sh` every 5 minutes. It polls `origin/main`, restarts the admin service on a new commit, and restarts Echo second only when her unit is enabled.
- Immediate: `sudo systemctl restart sirens-discord-ops` on kai-server skips the timer wait.
- `start.sh` fast-forwards `main` to `origin/main` via `git merge --ff-only`, runs an incremental `go build`, refetches SSM at every start (credential rotation without re-running install), execs the binary. The SSM fetch is fail-loud: an empty/failed credential pull aborts the start rather than booting with an empty token.
- One-time `install.sh` places admin, Echo, updater, and sudoers files.
  `install-echo.sh` performs the explicit Echo enablement.
- See [deploy.md](deploy.md) for the unit layout, the auto-updater, and the fail-loud SSM fetch.

## Sudoers

- NOPASSWD for manual restart + coily diagnostic commands. Explicit systemctl argument matching. Covers both `/bin/systemctl` and `/usr/bin/systemctl`.

## Dev loop

- Makefile and Ward verbs build and test all three binaries. `run` starts the
  admin bot, `run-echo` starts Sirens Echo, and `eval-echo` exercises her
  non-mutating Agent Proxy gate.
- Pre-commit: trailing whitespace, EOF, YAML, large-file/merge-conflict, line endings, `go mod tidy`, `go vet`, trufflehog. Commit-msg hook requires same-repo issue close.
- Forgejo CI: push to `main` and pull requests run `make build`, `make vet`, `make test`, plus `pre-commit run --all-files` in the moving :release dev-base image.

## See also

- [README.md](../README.md) - human-facing intro.
- [AGENTS.md](../AGENTS.md) - agent-facing operating rules.
- [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).

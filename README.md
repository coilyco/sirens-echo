# sirens-discord-ops

Discord-native admin control panel for the Sirens game servers. v1 wires up
eco; the per-game registration shape leaves room for factorio and friends.

## What it does

The bot pins one message per game in `#admin-control` with one button per
verb. Pressing a button shells out to `coily gaming <game> <verb>` on
kai-server and posts the start + completion lines to `#admin-audit`,
verbatim coily output included. Coily is the API. The bot has no awareness
of how coily implements those verbs.

v1 ships with eco only:

- Restart
- Status
- Stop
- Start

## Auth

Every button press checks the actor's role membership against
`ADMIN_ROLE_ID`. No per-verb gating in v1. Non-admins get an ephemeral
"not authorized" and no audit-channel write.

## Concurrency

If two admins press Restart at once, both invocations run. Coily's
underlying behavior is the safety net. Documented as known-acceptable.

## Configuration

Required env vars live in SSM under `/sirens-discord-ops/`: `DISCORD_TOKEN`, `ADMIN_CHANNEL_ID`, `AUDIT_CHANNEL_ID`, `ADMIN_ROLE_ID`. Optional `COILY_BIN` overrides the coily binary path (defaults to PATH lookup). `scripts/start.sh` fetches them at exec time.

## Local development

Point at Sirens Echo (the test server), export the four env vars, and run:

```sh
make run
```

The bot will post a pinned control panel in the configured admin channel on
first connect. Subsequent runs reuse and edit the existing pinned message
in place, so the verb list stays current without duplicate panels.

## Production deploy

Native systemd unit on kai-server (not k3s), runs as `kai`. `ExecStart` points at `scripts/start.sh`: fetch origin/main, fast-forward, rebuild in place, pull SSM env at exec time, exec the binary. The SSM fetch fails loud - an empty or failed credential pull aborts start.sh at the source rather than booting the bot with an empty token.

## Continuous deploy

A companion timer makes deploys automatic: `git push` is enough. `sirens-discord-ops-update.timer` fires `scripts/auto-update.sh` every 5 minutes; the script polls `origin/main` and, when the checkout is behind, runs `sudo systemctl restart sirens-discord-ops` so start.sh fast-forwards, rebuilds, and re-execs. For an immediate deploy, skip the wait: `ssh kai@kai-server sudo systemctl restart sirens-discord-ops`.

One-time bootstrap on kai-server: clone, then `bash scripts/install.sh`. The script drops units (main service + updater service/timer) + sudoers, daemon-reloads, enables the service and the timer. Re-run only when units/sudoers change.

## Adding a new game

Edit `internal/bot/games.go`, append a `Game{}` entry, redeploy. The bot will post a new
pinned panel for the new game on next start. Channels and admin role stay
the same. Per-game verb lists keep new games from being locked into eco's
exact four-verb mold.

## Out of scope (v1)

Anything outside `coily gaming eco {restart,status,stop,start}`: mod
install/uninstall, save backup/restore, log tailing, in-game player count,
version pinning, whitelist edits. Anything beyond eco. Concurrency
protection. Tighter coily integration (in-process call instead of
`exec.Command`). Web UI, mobile, REST API, observability dashboards.

## See also

- [AGENTS.md](AGENTS.md) - agent-facing operating rules.
- [docs/FEATURES.md](docs/FEATURES.md) - inventory of what ships today.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).

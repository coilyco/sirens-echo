# sirens-discord-ops

Source-controlled Discord deployment for the Sirens community. The repository
ships the existing game-server admin panel and the independently managed
Sirens Echo community agent.

## Admin control panel

The admin bot pins one message per game in `#admin-control` with one button per
verb. Pressing a button shells out to `coily gaming <game> <verb>` on
kai-server and posts the start + completion lines to `#admin-audit`,
verbatim coily output included. Coily is the API. The bot has no awareness
of how coily implements those verbs.

v1 ships with eco only:

- Restart
- Status
- Stop
- Start

Every button press checks the actor's role membership against
`ADMIN_ROLE_ID`. Non-admins get an ephemeral refusal and no audit-channel
write.

## Sirens Echo

Sirens Echo is the automated she/her community host selected from the
agent-compose `siren community host` seat and `community` role.

Her deployed definition, `ornith:35b` model pin, skillpack, and evaluation
cases are versioned in this repository. Agent Proxy supplies inference
transport. It does not own Sirens Echo's identity or community knowledge.

The first rollout is deliberately narrow:

- Only an exact `#bots` channel match can invoke her.
- A member must mention Sirens Echo or reply to one of her messages.
- Bot-authored messages, DMs, other channels, and duplicate Gateway events are
  ignored.
- Replies carry an empty `allowed_mentions` policy and never ping the replied
  member.
- The model receives at most 12 preceding messages from `#bots`.
- Unanswered questions and explicit corrections become sanitized ordinary
  Forgejo issues in this repository. No labels are added.
- No moderation, direct messages, reactions, edits, deletes, announcements,
  role changes, or account actions exist in the runtime.

The initial skillpack lives at
`.agents/skills/sirens-echo-community/`. Reviewed issue fixes update that
skillpack and add a regression conversation. Sirens Echo keeps no automatic
memory.

## Concurrency

If two admins press Restart at once, both invocations run. Coily's
underlying behavior is the safety net. Documented as known-acceptable.

## Configuration

The admin service reads `DISCORD_TOKEN`, `ADMIN_CHANNEL_ID`,
`AUDIT_CHANNEL_ID`, and `ADMIN_ROLE_ID` from SSM under
`/sirens-discord-ops/`. Optional `COILY_BIN` overrides the coily binary path.

The Echo service reads:

- `/sirens-echo/discord-bot-token`
- `/discord/channel/bots`
- `/sirens-echo/forgejo-issue-token`

The Forgejo token should be dedicated to ordinary issue reads and creates in
this repository. `AGENT_PROXY_URL` can override the source default
`http://ser8:8080`. Start scripts keep resolved values in process memory and
write no env file.

## Local development

Run repository commands through Ward:

```sh
ward exec build
ward exec test
ward exec run
ward exec run-echo
```

The non-mutating Echo deployment gate uses the production prompt and Agent
Proxy path without sending Discord messages or creating issues:

```sh
ward exec eval-echo
```

## Production deploy

Both bots run as native systemd units on kai-server. The admin service remains
enabled by the base installer. The Echo service is installed but disabled
until an operator runs `scripts/install-echo.sh` after satisfying the
credential, model, and Discord-intent prerequisites in
[`docs/sirens-echo.md`](docs/sirens-echo.md).

## Continuous deploy

The update timer polls `origin/main` every five minutes. It restarts the admin
service on a new commit and then restarts Sirens Echo only when her separate
unit is enabled. The admin start path performs the shared checkout
fast-forward before Echo rebuilds.

`scripts/disable-echo.sh` is the immediate Echo rollback. It leaves the admin
bot, repository, skillpack, and credentials untouched.

## Adding a new game

Edit `internal/bot/games.go`, append a `Game{}` entry, redeploy. The bot will post a new
pinned panel for the new game on next start. Channels and admin role stay
the same. Per-game verb lists keep new games from being locked into eco's
exact four-verb mold.

## Out of scope

Anything outside `coily gaming eco {restart,status,stop,start}`: mod
install/uninstall, save backup/restore, log tailing, in-game player count,
version pinning, whitelist edits. Anything beyond eco. Concurrency
protection. Tighter coily integration (in-process call instead of
`exec.Command`). Web UI and mobile UI.

For Sirens Echo: ambient channel participation, DMs, moderation, member
accounts, automatic memory, autonomous skill edits, and any Discord action
other than a no-ping reply inside `#bots`.

## See also

- [AGENTS.md](AGENTS.md) - agent-facing operating rules.
- [docs/FEATURES.md](docs/FEATURES.md) - inventory of what ships today.
- [docs/sirens-echo.md](docs/sirens-echo.md) - agent flow, feedback loop,
  evaluation, rollout, and rollback.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).

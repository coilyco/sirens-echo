# Features

Living inventory of the Sirens Discord deployment.

## Sirens Echo community agent

- Separate `sirens-echo` Discord identity, binary, and systemd service
- Source-controlled seat, role, `ornith:35b` model, `#bots` boundary,
  history budget, skill roots, and evaluation cases
- Mention-or-reply invocation with exact channel, guild, author, and duplicate
  gates
- Agent Proxy client with two-minute timeout and serialized dispatch
- Strict JSON parser and grounding checks for channels and action claims
- Soft-reference replies with every Discord mention disabled
- Repo-local `sirens-echo-community` skill and approved knowledge references
- No automatic memory or autonomous skill edits
- Sanitized ordinary Forgejo issues for unanswered questions and corrections
  with exact-title reuse and no labels
- Offline regression suite and non-mutating live Agent Proxy gate
- Explicit enable script and independent disable rollback

See [sirens-echo.md](sirens-echo.md) and
[sirens-echo-rollout.md](sirens-echo-rollout.md).

## Admin bot core

- Role-gated admin commands with ephemeral refusal for non-admins
- Discord session blocks until graceful SIGTERM shutdown
- Pinned Components V2 panel per game, updated without duplicates
- Custom IDs in `sdo:<game>:<verb>` format
- Two-step confirmation for restart, stop, and start
- Deferred interaction responses for long-running operations

## Game operations

- Eco restart, status, stop, and start through `coily gaming eco`
- Extensible per-game verb registry
- Five-minute subprocess timeout
- Combined output and exit code in the audit channel
- Tail truncation at Discord's message limit with journal pointer

## Configuration and deployment

- Admin secrets injected from `/sirens-discord-ops/`
- Echo token, `#bots` identifier, and Forgejo token injected separately
- No env files on disk
- Independent `sirens-discord-ops.service` and `sirens-echo.service`
- Shared five-minute update timer restarts enabled services on a new main
  commit
- Start paths fast-forward main, build incrementally, and refetch SSM values
- Sudoers grants exact service lifecycle commands only

See [deploy.md](deploy.md).

## Development gates

- Ward verbs for build, format, vet, test, tidy, run, Echo run, Echo
  evaluation, and full pre-commit
- Forgejo CI builds, vets, tests, and runs pre-commit
- Pre-commit covers repository structure, skills, links, Go modules, Go vet,
  comments, and secret scanning

## Deliberate exclusions

- The admin bot has no game-operation implementation beyond Coily
- Sirens Echo has no moderation, DM, account, role, announcement, edit,
  delete, reaction, or ambient-channel surface
- Neither bot owns a web or mobile UI

## See also

- [README.md](../README.md) - human-facing introduction
- [AGENTS.md](../AGENTS.md) - agent-facing rules
- [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted commands

Cross-reference convention from [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).

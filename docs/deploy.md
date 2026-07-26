# Deploy and auto-update

How `sirens-discord-ops` ships to kai-server, and the two reliability guards
added after the 2026-05-29 reboot incident.

## The four units

- `sirens-discord-ops.service` - the bot. `ExecStart` runs `scripts/start.sh`,
  which fast-forwards `main` to `origin/main`, rebuilds in place, fetches the
  SSM env, and execs the binary.
- `sirens-echo.service` - the independently managed community agent.
  `ExecStart` runs `scripts/start-echo.sh`, which fetches the Sirens Echo
  Discord token, the `#bots` channel, and the Forgejo issue token without
  writing an env file.
- `sirens-discord-ops-update.service` - a `oneshot` that runs
  `scripts/auto-update.sh`. Not enabled directly.
- `sirens-discord-ops-update.timer` - fires the update service every 5 minutes.
  This is the unit `install.sh` enables.

`scripts/install.sh` installs all four plus the sudoers drop-in, enables the
admin bot and timer, and leaves Echo disabled. `scripts/install-echo.sh`
performs the explicit Echo opt-in after the operator satisfies the
prerequisites in [sirens-echo.md](sirens-echo.md).

## Continuous deploy

`git push` is the deploy. Within 5 minutes the timer fires `auto-update.sh`,
which `git fetch`es `origin/main`, compares it to the deployed `main`, and
restarts the admin service only when the checkout is behind. The admin
`start.sh` performs the shared fast-forward. The updater then restarts Echo
only when `sirens-echo.service` is enabled, so Echo builds from the updated
checkout.

For an immediate deploy, skip the timer: `ssh kai@kai-server sudo systemctl
restart sirens-discord-ops`.

The restart call omits the `.service` suffix on purpose: sudoers strict-matches
arguments, and the NOPASSWD rule is `systemctl restart sirens-discord-ops`.

## Echo disable path

`scripts/disable-echo.sh` disables and stops `sirens-echo.service`. It does
not change the admin bot, repository checkout, SSM parameters, or skillpack.
The operator can inspect and repair the failed boundary before explicitly
re-enabling Echo.

## Why the auto-updater regressed

The updater service was hand-installed on kai-server and `auto-update.sh` was
never committed, so a fresh checkout had the unit's `ExecStart` pointing at a
missing script (`No such file or directory`, every 5 minutes). Committing the
script and both units here closes that gap: the repo now carries the updater
`install.sh` installs, so the deployed state matches `origin/main`.

## Fail-loud SSM fetch

The original `start.sh` fetched each credential with
`export VAR="$(coily ... )"`. When the coily verb path broke, the command
substitution failed but `export` still returned 0, so `set -e` never tripped -
`DISCORD_TOKEN` resolved to empty and the bot died downstream on
`invalid header field value for "Authorization"`, masking the real cause.

`fetch_ssm` in `start.sh` fixes this with two independent guards:

- The assignment is split from the declaration (`local value` then
  `value="$(...)"`). A combined `local value="$(...)"` would take the exit
  status of `local` (always 0) and mask a failed fetch. Split, a non-zero
  fetch trips `set -e`.
- An explicit `-z` check catches an empty-but-successful fetch (a present but
  blank SSM parameter) and exits 1 with a named error.

Either guard alone leaves a hole. Together a broken or empty fetch fails at the
source instead of booting the bot with an empty credential.

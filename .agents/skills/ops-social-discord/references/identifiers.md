# Sirens Discord identifiers

Use one exact SSM parameter at the point of need. Do not load the `/discord/`
tree into the shell environment. All values are `SecureString` and remain
opaque even when the value is a non-secret identifier.

## Guild

* `/discord/server-id` - Sirens guild identifier.
* `/discord/server-name` - Sirens guild display name.

## Durable channel aliases

* `/discord/channel/bots` - isolated Sirens Echo channel.
* `/discord/channel/crafting-feed` - Eco crafting activity.
* `/discord/channel/cycle-current` - active Eco cycle channel. The operator
  updates this alias when a cycle changes.
* `/discord/channel/eco-configs` - long-lived Eco configuration summary.
* `/discord/channel/election-feed` - Eco election activity.
* `/discord/channel/general` - general community conversation.
* `/discord/channel/general-public` - public general conversation.
* `/discord/channel/map-display` - Eco map display.
* `/discord/channel/player-status-feed` - Eco player status.
* `/discord/channel/server-ad` - Eco server advertisement.
* `/discord/channel/server-info-display` - Eco server information.
* `/discord/channel/server-status-feed` - Eco server status.
* `/discord/channel/suggestions` - suggestions conversation.
* `/discord/channel/suggestions-forum` - suggestions forum.
* `/discord/channel/trade-feed` - Eco trade activity.
* `/discord/channel/work-party-display` - Eco work-party display.

Alias names describe consumer purpose and do not always repeat the current
Discord channel name. If the needed channel has no exact parameter, resolve it
through `list-guild-channel` and report the missing durable cache path without
copying the identifier. Active threads always use live discovery.

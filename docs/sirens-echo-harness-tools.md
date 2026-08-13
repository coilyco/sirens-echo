# Harness tools

Almost every tool Echo offers comes from an MCP server the deployment rostered.
A harness tool is the exception: the harness itself answers the call. There is
one today.

## `harness__refresh_tools`

A tool listing is held for an hour. That is a long time to be wrong about your
own tools, and the thing best placed to notice is the model that just failed to
find one it expected. This tool lets it say so.

It marks every rostered server for re-reading. It dials nothing, so twenty calls
in one turn cost one listing rather than twenty handshakes, and it needs no
bound of its own for that reason.

## The listing lands on the next turn

The refresh does not change the tools the calling turn is holding. Its result
says so in as many words, because a model that read otherwise would tell a
member a tool is available before it can see one, which is the over-claiming
failure in [issue 211](https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/211).

That is also why the tool's description carries the bound rather than leaving
the model to discover it.

## It is named like a server's tool on purpose

The name is built by the same `server__tool` rule, with `harness` as the server.
So a rostered server called `harness` publishing `refresh_tools` collides, and a
collision is fatal exactly as a roster collision is. Whichever tool lost a
quieter contest would disappear without anything saying so.

## An empty roster is offered nothing

An empty roster is a valid no-tool capability boundary. Offering a refresh when
there is nothing to refresh would put a tool in the one configuration meant to
have none, which is a capability claim with no capability behind it.

## See also

See [the MCP roster](sirens-echo-mcp-roster.md) for what holds a listing and for
how long, and [runtime MCP tools](sirens-echo-tools.md) for how a tool call is
bounded and reported.

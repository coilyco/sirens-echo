# Runtime MCP tools

Each source-controlled definition owns its MCP roster and optional issue tracker, and **no server name
or endpoint appears here**. Echo's carries a private tracker MCP: **Echo sends no credential to it**,
the separate workload holds the token. The CoilyCo definition reaches that same tracker MCP and
names no issue tracker, **so a write happens because the model chose a tool**: general-purpose
describes topic scope, not mutation authority.

## Tool loop

When a definition configures MCP servers, the harness opens every session, lists each published tool,
description, and input schema, exposes each model name as `<server>__<tool>`, sends those schemas with
the Agent Proxy request, executes valid model-requested tools, renders each result as text, appends the
calls and results, then continues. **An empty roster skips discovery and sends no tools.** Final
content can be an OpenAI-compatible string, text-part array, or text object.

The harness rejects empty, colliding, or overlong model-facing tool names; calls missing an identifier
or function name; calls to tools absent from the roster; arguments that are not JSON objects; a
response with neither tool calls nor content; and more than six rounds. **A
server that fails to connect or list contributes no tools and the turn goes on with the rest, named to
the model so it reports the gap.** Only an unreachable roster stops the turn, a name collision stays
fatal, an invocation failure ends the turn with the tool-failure notice, and an MCP error result is
grounded data the model self-corrects from **once**: an identical failed call is replayed, not re-made
(#943).

The in-process fixture proves schema discovery, a tool-call-only first response, continuation, alternate
content forms, and that **a tool which never answers fails on the call bound rather than the
turn's**. The live Echo evaluation requires an `eco__get_eco_server_status` call and no writes.

## Harness tools

Almost every tool Echo offers comes from a rostered MCP server. **A harness tool is the exception: the
harness itself answers the call.** A tool listing is held for an hour, and **the thing best
placed to notice it is wrong is the model that failed to find a tool it expected**, so
`harness__refresh_tools` lets it say so. It marks every rostered
server for re-reading and dials nothing, so twenty calls cost one listing.

## The read_skill tool

**A skill's references left the prompt**, read on demand (#859). `SKILL.md` stays inline as the index,
and **a reference is fetchable unless it declares `inline: always`**, which the refusal-shaping ones do.

## The calculate tool

**Every number Echo produced herself was predicted rather than calculated** (#916), fine for "roughly a
third" and not for a total or a unit price. `calculate` evaluates `+ - * / ^`, parentheses and a
trailing `%`, and **nothing else**: the grammar has no identifiers and no calls, so no expression
reaches anything. **Exact rather than floating point**, so `0.1 + 0.2` is `0.3`, and a result that is
not a decimal is rounded **and says so**, the exact fraction beside it. The expression is echoed with
the answer. In process rather than a twelfth roster server, and registered unconditionally.

## The fetch tool

A read-only HTTPS GET. **The fetching is easy, the allowlist is the feature.** It runs in the cluster,
so unbounded it reaches the tailnet, other services' internals, and cloud metadata, and **the model is
precisely the component an attacker gets to talk to**. `SIRENS_ECHO_FETCH_HOSTS` is a comma-separated
allowlist, and **empty offers no tool at all**: no schema in the prompt, nothing to be talked into.

* **A wildcard covers subdomains and nothing else.** `*.mozilla.com` matches `www.mozilla.com` and
  `a.b.mozilla.com`, **not** `mozilla.com`, which is a separate entry. It is not a suffix test: the
  leading dot is part of the comparison, so `mozilla.com.evil.example` is refused, and a pattern with a
  misplaced or missing star matches nothing rather than everything.
* **Exact host match otherwise**, not a suffix, because `host.evil.example` is a different host a
  suffix check accepts and registering that domain costs an attacker nothing. **HTTPS only.**
* **Private addresses refused at dial time**, not by reading the URL, since an allowlisted hostname can
  resolve to an internal address. Loopback, private ranges, link-local, unspecified, and CGNAT all go.
  **CGNAT is named separately because Go's `IsPrivate` is RFC1918 only**, `100.64.0.0/10` being the
  tailnet.
* **Redirects refused**: a redirect is a destination the allowlist never saw.
* **A page over the cap is marked, not silently cut.** The read takes one byte past the limit, and an
  oversize body returns what it got plus a truncation line, seam repaired to a rune boundary.
* **A media URL is described, not decoded**: a non-text content type returns its type and length
  rather than bytes nobody can read, by shape not by a list (#1029).

**GET only.** A fetched page is untrusted text entering the prompt, and **the allowlist bounds where it
comes from, saying nothing about what it says**.

## What happens to a large result

One tool result is capped at 8 KiB before it re-enters the prompt, so a round of parallel calls cannot
inflate the context past the model's budget. The full result stays on the executed-tool record for
telemetry and grounding, and only the capped copy reaches the model.

When a result is trimmed and the deployment mounts a scratchpad, the runtime writes the whole result
there and appends a line naming the file and the true byte count, so the model can `scratch_read` it.
**The save goes through the `scratch_write` tool rather than the filesystem**, so path confinement, the
per-file limit, the per-requester quota, and attribution all apply as for a model-requested write, and
saves are numbered per turn so a second call to the same tool cannot overwrite the first. **Every
failure falls back rather than failing the turn**: no scratchpad, a result over the per-file limit, or
a partition at quota each leave a trimmed result carrying the truncation marker. The file name is built from the tool's own name with everything outside letters,
digits, hyphen, and underscore removed, under a single `tool-output` directory, because **a tool name is
server-supplied and can never reach the filesystem as a path**.

**A spent tool budget answers rather than discards.** The tools are withdrawn once the last round's
results are in and one further call asks for an answer from what was gathered, instructed to say plainly
what could not be determined and to claim no result no tool returned. Withdrawing after the results
land rather than on the next request costs no extra model call and keeps the six-round ceiling true. If
the outer model-call budget is also spent, the turn ends with the rounds-spent notice.

## Public repository inventory

`list_public_repos` lists an organization's public repositories with description, URL, language, and
last update. `SIRENS_ECHO_REPO_INVENTORY_URL` and `..._ORG` switch it on, **either empty offering no
tool at all**.

**The read is unauthenticated**, with no `Authorization` header anywhere in `repoinventory.go` and a
test asserting none is sent. **That is the guarantee rather than a visibility filter**: an
unauthenticated request cannot see a private repository at all, and one arriving anyway is dropped.

The inventory comes from the forge API rather than a mounted tree, so it reports what the organization
has rather than what a mount contains. The listing is capped, entries sort by name, and the dialer is
the fetch tool's.

`read_public_file` takes owner, repo, path, and an optional ref, with the same absence of a credential.
**A path climbing out of the repository is refused before any request is made, and each segment is
escaped separately so one carrying a slash cannot forge one.** Output is capped and says so when it
cuts. **Neither tool writes.**

# The capability report

One line at boot saying what this process can and cannot do. `startup.capabilities`
carries a name and a boolean for each optional surface, and nothing else.

## Why it exists

The harness is built so that absence is inert rather than broken: an unset
variable offers no tools instead of tools that fail. That is the right shape,
and it has one cost. **A capability nobody switched on is indistinguishable,
from inside the process, from one that was never built.**

Three were found in one day, each by accident, each by reading this repository
and then the deploy repository to see whether a variable was set: the content
gate, job-store durability, and the scratchpad on the Echo lane. All three are
built. None of them were on. Nothing anywhere said so.

## What it reports

Discord ingress, the MCP roster, the access policy, content classes, job-store
durability, the scratchpad, and the issue tracker.

A name and a boolean. **No path, endpoint, or configured value reaches it**, so
it stays inside the telemetry contract without an exception. A test asserts
that, because a report of what is configured is exactly the shape that invites
someone to log the value next.

## Where it fires

First thing in `Run`, before the job runner starts and before the gateway
opens. A process that dies during startup has already said what it was
configured to be, which is the case where the answer is most wanted.

## The set is closed

A test counts the reported surfaces against the optional fields the config
loader reads, and a second one reads `config.go` to check the two agree. Adding
an optional surface and forgetting the report fails the suite rather than
shipping a capability nobody can see.

## What it does not do

It turns nothing on and changes no behaviour. Whether a capability should be
configured is a deployment decision, and for two of the three above it is
Kai's. This only makes the state legible.

## See also

- [configuration](sirens-echo-config.md) - the variables themselves.
- [observability](sirens-echo-observability.md) - what else a turn reports.

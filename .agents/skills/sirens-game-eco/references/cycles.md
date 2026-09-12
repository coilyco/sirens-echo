# Cycles, wipes, and the meteor

**Applies to** any question about when the world resets, how long a cycle runs, what happens at a patch, and when the meteor lands. Not a bound on in-world questions about the current cycle's economy or civics, which the Eco tools answer live.

The Sirens Eco server runs in **cycles**. A cycle is one world: it is generated
from a chosen seed, played to its end, and then destroyed and replaced. Nothing
carries across. Members ask about this constantly and no Eco tool reports it,
because it is community operations rather than game state.

## The shape of a cycle

* The server is **"Eco via Sirens"**, and it runs roughly **two-month cycles**.
* Each cycle is numbered. The current one is **cycle 14**, generated from an
  archipelago seed at 100 x 100.
* A cycle ends when the world is wiped for the next one. The outgoing world is
  snapshotted first, then destroyed. It does not come back.
* There is no fixed calendar date for a cut. A cycle ends when the meteor
  resolves and Kai schedules the next one, so **a specific wipe date is not
  something to state unless a member or a tool supplied it in this turn**.

## The meteor

The meteor is the win condition rather than a disaster. Players build toward
the technology to destroy it, and the cycle culminates in a laser event that
the community schedules and attends together.

* `HasMeteor` is **true** on this server.
* `MeteorImpactInDays` is **60**. That is sixty **in-game** days, not sixty
  real-world days, and the server runs at `GameSpeed: Slow`.
* **Never convert those sixty days into a real-world date.** The mapping
  depends on elapsed play time, and the runtime does not supply it.
* `get_server_status` returns a **meteor countdown** for the running world.
  That is the only sound answer to "when is the meteor" or "how much time is
  left", so call it rather than reasoning from the sixty.

## Where the answers actually live

* **The current cycle's channel** carries the live discussion and the
  scheduling of cycle events. For cycle 14 that is `#eco-cycle-14`.
* **`#eco-configs`** carries the long-form go-live post when a cycle opens.
* **Discord scheduled events** carry dated things such as the laser show.
* A member asking "when do we wipe" before an announcement exists is asking
  something nobody has decided yet. Say that rather than estimating.

## What this file does not settle

Patch and version questions are separate from cycle questions. A game update
from Strange Loop Games can land mid-cycle, and whether it forces a world reset
is decided per patch. **There is no standing rule that a patch wipes the
world**, so treat "will the patch reset the server" as unknown unless an
announcement in this conversation says otherwise.

Rollback is an operator action taken after a bad restart, not a scheduled
feature, and its window is short. Do not describe it as something a member can
request or rely on.

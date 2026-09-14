---
description: This server's settings, and why it has no wipe cadence.
---

# How this server is set up

**Applies to** any Enshrouded rules, rate, or difficulty question asked about
the Sirens server. Not a bound on live world state, which no tool reaches, and
not a general Enshrouded tutorial, which the wiki pages in
[links.md](links.md) answer.

Every value here was read from the server's own `enshrouded_server.json` on
2026-09-13, and Kai confirmed the same day that this is the community server
rather than a private world. Nothing here is live state. It changes when Kai
edits the config, so **prefer this file over the wiki for a local value, and
say plainly when the specific number is not one this file carries**.

## The headline: this server runs Default

The preset is **Default**, and nearly every individual factor sits at **1**.
That is the answer to most rate questions, and it is worth saying out loud
rather than hedging: player health, mana, stamina, body heat, and diving time,
enemy and boss damage, health, and stamina, enemy perception, threat, combat
XP, mining XP, exploration and quest XP, mining damage, plant growth, resource
drop stacks, workshop production, food buff duration, perk cost, and the shroud
timer are all at the default multiplier.

So **a member reading the wiki's Default-preset behaviour is reading this
server correctly** for all of the above. This file earns its place on the
short list of exceptions below rather than by contradicting the wiki wholesale.

## What a member will actually notice

* **Text chat and voice chat are both off.** `enableTextChat` and
  `enableVoiceChat` are **false**. There is no in-game chat at all, so
  coordination happens in Discord. A member asking why nobody answers them in
  game is hitting this rather than being ignored.
* **Sixteen slots.** `slotCount` is **16**.
* **Days are 30 minutes, nights are 12.** `dayTimeDuration` is 1800 seconds and
  `nightTimeDuration` is 720 seconds.
* **You cannot starve to death.** `enableStarvingDebuff` is **false**. Hunger
  still turns to starving after 600 seconds, and the debuff that would follow
  is switched off.
* **Death keeps your materials.** `tombstoneMode` is
  **AddBackpackMaterials**.
* **Weapon upgrade recycling returns half.** `perkUpgradeRecyclingFactor` is
  **0.5**, the one factor on the whole config that is not 1.
* **Durability and glider turbulence are on.** `enableDurability` and
  `enableGliderTurbulences` are both **true**.
* **Enemies are not pacified.** `pacifyAllEnemies` is **false**, and enemy
  amount and aggro pool are both **Normal**.
* **Weather, fishing difficulty, and curse chance are all Normal.**
* **Taming a startled animal loses some progress**, not all of it
  (`tamingStartleRepercussion` is LoseSomeProgress).

## Who can do what

Four groups, each with its own password. **Only Admin can kick or ban.**

* **Admin** - kick and ban, inventories, edit world, edit and extend base.
* **Friend** - inventories, edit world, edit and extend base. No kick or ban.
* **Guest** - edit world only. No inventory access, and cannot edit or extend
  a base.
* **Visitor** - none of the above. Look, do not touch.

No account is banned.

## There is no cycle and no wipe cadence

**This world is not on a cycle.** Kai confirmed on 2026-09-13 that the wipe
before it was a one-off rather than a schedule, so a question about the next
wipe is answered by saying this world runs no cadence. Do not borrow another
game's cycle language for it, and do not infer a rhythm from the single reset
below.

The current world was generated on **2026-09-12**, after a one-time wipe of
what came before it. That is history rather than a pattern, and naming a next
wipe date would be inventing one.

## What this file does not carry

The server name is still the stock **Enshrouded Server** with no tags set, so
it is not a branded listing a member can search for by community name. How a
member joins, and where the server ultimately lives, are operations questions
rather than settings questions, and this file does not answer them.

Anything above that is not listed here follows the Default preset, so answer it
from the wiki's World Settings page linked in [links.md](links.md), and say
plainly when the exact number is not something this reference carries.

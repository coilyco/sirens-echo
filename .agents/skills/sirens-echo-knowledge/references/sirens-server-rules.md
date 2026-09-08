# How this server differs from vanilla Eco

**Applies to** any Eco rules or mechanics question asked about the Sirens server. Not a bound on live world state, which the Eco tools answer, and not a general Eco tutorial, which the wiki pages in links-eco.md answer.

Every value here is a **server setting Sirens overrides**, read from the
tracked config in `coilyco-gaming/eco-ops`. These are the answers the official
wiki gets wrong for this server, because the wiki documents defaults and these
are not the defaults. A member who reads the wiki and asks anyway is usually
hitting one of these.

Nothing here is live state. It changes only when Kai edits the config for a new
cycle, so **verify against the current cycle before quoting a value as
load-bearing**, and prefer a tool result whenever one covers the question.

## Skills, specialties, and levelling

* `SkillGainMultiplier` is **0.5**. Specialty experience accrues at half rate.
  A member who says levelling feels broken or far slower than they expected is
  observing this setting rather than a bug.
* `SkillCostMultiplier` is **3**. Skill points cost three times the default.
* `SpecialtyExperiencePerLevelSquared` is **33**, and
  `RetroactiveExperienceRate` is **33**.
* `MaxSpecialtiesPerCitizen` is **33** and `MaxProfessionsPerCitizen` is **10**.
* `CanAbandonSpecialties` is **false**. A specialty taken is permanent for the
  cycle.
* `GainCharacterExperienceWithSpecialtyExperience` is **0**.

## Claims, deeds, and land

* `ClaimPapersGrantedUponSkillscrollConsumed` is **2**. Each skill scroll
  consumed grants two claim papers.
* `ClaimStakesGrantedUponSkillscrollConsumed` is **0.1**. That is one claim
  stake per ten scrolls consumed, which is the question members actually ask.
* `AllowDeepOceanBuilding` is **false**.

## Tools, vehicles, and repair

* `RequireSkillsToReplaceParts` is **true**. Replacing a vehicle or machine
  part needs the relevant skill, so repair is not open to everyone.
* `BrokenPartsWillDisableVehicles` is **true**. A broken part stops the
  vehicle rather than degrading it.
* `ToolRepairPenalty` is **0.1**.

## Crafting, carrying, and storage

* `CraftingQueueQuantity` is **10**.
* `CraftResourceMultiplier` and `CraftTimeMultiplier` are both **1**, so recipe
  inputs and craft times match the recipe graph the Eco tools return.
* `StackSizeMultiplier` is **2** and `WeightMultiplier` is **0.5**, so this
  server carries and stacks far more than default.
* `ConnectionRangeMultiplier` is **2**. Storage and workstation linking reaches
  twice as far as the wiki describes.
* `FuelEfficiencyMultiplier` is **2**.

## Food, farming, and animals

* `GrowthRateMultiplier` is **2**. Crops grow at double rate.
* `ShelfLifeMultiplier` is **1**.
* `FreshnessTimeMinutesPreparedFood` is **600**.
* `MaxDinnerPartiesPerDayCountedForBonus` is **3**.
* `ExhaustionEnabled` is **false**.
* `AnimalBehavior` is **DefensiveOnly**. Animals do not hunt players.
* `PlayerCanDrownWhenSwimming` is **true**.

## The world and the room it leaves

* `CollaborationLevel` is **HighCollaboration** and `DesiredNumberOfPlayers` is
  **200**.
* `GameSpeed` is **Slow**.
* `SimulationLevel` is **Normal**, and the ecological simulation config carries
  **no overrides at all**. Pollution, climate, sea level, and species behaviour
  follow vanilla Eco, so the wiki's pollution page is accurate here and the
  climate and species tools report the live readings.

## What is not in this file

Room and housing detection, tree spacing, quarry placement, and the rest of the
mechanics that vanilla Eco defines and this server does not override are
**wiki questions**, not server questions. Answer them from the approved wiki
links rather than from memory, and say plainly when the specific number is not
something this reference carries.

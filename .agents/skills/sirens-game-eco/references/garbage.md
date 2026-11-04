# Garbage, sorting, and recycling

**Applies to** any question about trash, garbage, waste, scrap, sorting,
recycling, or the pollution garbage causes. Eco 0.14 added this system, and
cycle 15 is the first cycle on this server to run it.

**The tools do not cover it yet.** The recipe data behind the Eco tools predates
0.14, so a recipe lookup shows no garbage output. Answer the mechanics from this
file, and say plainly that a recipe's exact garbage amount is not in the tools.
Live amounts, such as who dropped how much garbage, still come from a tool.

## Where garbage comes from

* **Crafting makes garbage.** Every craft leaves Mixed Garbage, sized from what
  its ingredients would yield if recycled, scaled by the server's craft garbage
  ratio. This server runs the default, 0.08. Some recipes add their own process
  waste on top.
* **Thrown-away items become garbage.** An item dropped as garbage sits in a
  garbage bag, and the bag decays into Mixed Garbage cubes.
* **Fresh waste waits before it pollutes.** New garbage has a grace period
  before its pollution starts, which is the window to move it into storage.

## The items

* **Mixed Garbage** is unsorted. It pollutes according to what is inside it.
* **Garbage** is the worthless remainder at the end of the chain. It pollutes
  soil, and burying it does nothing. Only proper garbage storage holds it back.
* **A member asking about "trash" means Garbage or Mixed Garbage.** Stores list
  them under those two names, so look them up by those names.
* Sorting produces five intermediate streams: **Mixed Biowaste**, **Mixed Metal
  Scrap**, **Mixed Construction Scrap**, **Mixed Industrial Waste**, and **Mixed
  Residuals**.

## Sorting

Sorting is the **Recycling** specialty, under the Scientist profession. Every
sorter needs Recycling level 1. A sorter takes one input and splits it. Most
outputs need a **catalyst** loaded in the sorter, and the catalyst is used up
as it works.

* **Garbage Sorter.** Mixed Garbage into the five intermediate streams.
* **Biowaste Sorter.** Mixed Biowaste into food scrap, wood scrap, and textiles.
  Food scrap uses Dirt as its catalyst, and wood scrap uses a Mesh Filter.
* **Metal Scrap Sorter.** Mixed Metal Scrap into iron, copper, and gold scrap.
  Iron uses a **Magnet**, and copper and gold use **Sulfuric Acid**.
* **Construction Scrap Sorter.** Into ceramic scrap, glass scrap, and stone
  rubble, each using a Mesh Filter.
* **Industrial Waste Sorter.** Into plastic scrap, electronic scrap, and chemical
  waste. Plastic uses an Electrostatic Filter, electronic scrap a Mesh Filter,
  and chemical waste a Carbon Filter.
* **Residual Waste Sorter.** Mixed Residuals into compost, bio residue, tailings,
  wet tailings, chemical pollutant, and Garbage.

The first-stage catalysts are a Mesh Filter for biowaste and construction scrap,
a Magnet for metal scrap, and an Electrostatic Filter for industrial waste.

**Sorting is lossy.** What a sorter fails to recover degrades. Lost food scrap
becomes compost, lost wood scrap and textiles become bio residue, and most other
losses become Garbage.

## Pollution

* **Unsorted garbage pollutes by what it contains.** Biowaste mostly does not.
  Metal, ceramic, glass, and rubble feed soil pollution. Plastic, electronics,
  and chemical waste feed chemical pollution. Residuals and tailings feed
  heavy-mineral pollution.
* **Sorting removes pollution.** Once a material is sorted out it stops adding
  to the mix. Residuals are the exception and keep polluting until stored.
* **Storage holds pollution back.** Dedicated garbage storage cuts the pollution
  of what it holds by 90, 95, or 99 percent, depending on its tier.
* **A house garbage bin is not containment.** It has a waste shield that
  recharges while the bin sits empty, so it stays clean only if a garbage truck
  empties it regularly.
* Garbage trucks with a garbage collector attachment drain bins and dumpsters
  they park beside.

## This server

* The craft garbage ratio is the default, 0.08.
* The StorageMore Green Trash Bin, a server mod, accepts garbage on this server.

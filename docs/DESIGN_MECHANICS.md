# Durst World — Game Mechanics Design

> The plan for turning Durst World from a **place** into a **game**. Read
> [`GAME.md`](GAME.md) first for what exists today, and
> [`STYLE_GUIDE.md`](STYLE_GUIDE.md) before touching visuals. This doc is the
> direction; [`ROADMAP.md`](ROADMAP.md) tracks what's shipped.

## Where we are

Durst World is a beautiful, deterministic, social **place**: infinite terrain,
biomes, fog-of-war, villages and stone cities, 14 resources, foraging, hats,
riddle-gates, presentations. What it lacks is a **loop** — a reason to return
tomorrow, something to work toward, a way your actions change the world.

Two facts decide the next chapter:

1. **14 resources already exist** (berries, herbs, mushrooms, shells, crystals,
   nuggets, grain, **cut stone, timber, fish**, geodes, relics, spores, amber)
   — a crafting economy with nothing to craft yet.
2. **The villages and stone cities are empty stages** — gorgeous, walkable,
   purposeless. They want to become places you settle and build.

## Direction (decided)

**Genre: Cozy frontier.** Gather → craft → build → automate → trade. No combat,
no death, no danger. This extends the world's current cozy/social tone and fits
**every** design pillar — most importantly "persist between visits, not during",
which turns offline automation into a feature rather than a problem.

Survival/combat (wildlife, weapons, enemies) is **parked**, not cancelled — see
"Parked: light danger" below. The cozy systems are designed so it *can* layer on
later, but nothing built now depends on it.

**Flavor: corporate × medieval.** Deadpan satire of the world's existing tension
— Durst HQ and "Demo Center" sitting next to cathedrals and palisades. The
smelter is the *Ingot Synergy Furnace*; a building plot is a *Workspace*; the
mill posts *quarterly yields*. This makes naming a **generator**, not a chore,
and carries all the flavor the game needs without a plot.

## The core loop

```
        gather ──► craft ──► build ──► automate ──► trade
          ▲                                            │
          └────────────── come back tomorrow ◄─────────┘
```

### 1. Crafting — the bridge from "I have stuff" to "I can do things"

- A **recipe table** (pure data) + a **Crafting (Self-Service)** workbench panel.
  The inventory panel and the deck-authoring UI are the templates to copy.
- Recipes consume existing resources: `timber → planks`,
  `planks + stone → wall block`, `nugget + amber → lamp`,
  `herb + mushroom → salve`, `grain → flour`, `nugget → ingot`.
- Outputs are new inventory items (placeable parts, machines, consumables).

### 2. Building — the one architectural change

Let players **place objects into the world** (fences, walls, planters, signs,
workbenches, machines). Placed objects are **stored mutable state** — this is
the single thing that steps outside "every cell is a pure function of the seed".

**It's an established pattern here, not a new risk.** The guestbook, the
player-authored decks, and the co-op gate pool already live in SQLite on top of
the deterministic world. Building is the same shape:

- A `placements` table keyed by `(x, y)` → object kind, owner, state.
- Overlaid on the stateless terrain at render time (terrain stays a pure
  function of the seed; placements are a sparse layer on top).
- The **empty villages become claimable Workspaces** — plots you build on.

This is what makes the world *yours* and is the strongest reason to return.

### 3. Machines / automation — the sleeper hit

This is where "persist between visits, not during" becomes the headline feature:

- A **mill** turns grain → flour, a **sawmill** turns timber → planks, a
  **smelter** turns nuggets → ingots — slowly, on a **wall clock**, **while you
  are logged off.** You come back to a full hopper.
- **Kraftwerk** is already a "machine hall" — promote it from decoration to the
  real production interface.
- State per machine: input buffer, output buffer, `last_tick` wall-clock. On
  load, fast-forward elapsed time → no server-side simulation needed while
  empty. Deterministic given inputs + elapsed time; no per-tick RNG.

### 4. Trade — the multiplayer payoff

Once people craft and automate, let them **gift/trade** items, or stock a
**market stall** in the villages (which already render market stalls). This is
what makes it an *MMO* and not single-player in a shared room. Lands after the
single-player loop proves out.

## Naming language (corporate × medieval)

Don't write a dictionary — apply the voice and things name themselves:

| Thing | Cozy-frontier name | Corporate × medieval name |
| --- | --- | --- |
| Workbench | Workbench | **Crafting (Self-Service)** |
| Building plot | Homestead plot | **Workspace** |
| Smelter | Smelter | **Ingot Synergy Furnace** |
| Mill output | Flour | **Q3 Flour Yield** |
| Market stall | Stall | **Durst Group Concession** |
| Storage chest | Chest | **Cold Storage (Asset Locker)** |

Story stays near zero. The lore-gates (Whispering/Sunken gates, the Vault,
ancient relics) carry the only narrative flavor needed: theme, not plot.

## First milestone — the smallest thing that becomes a *game*

A vertical slice proving the whole loop end to end:

1. **Recipe table + workbench panel** — craft 3–4 things from existing
   resources. (Data table + a panel modeled on the inventory/deck UI.)
2. **Placements layer** — the SQLite-backed "place an object at `(x, y)`"
   system. The one architectural piece; everything later rides on it.
3. **One machine** — a mill or sawmill that converts a resource over wall-clock
   time, **offline**, and lets you collect the output. Proves the idle loop.

Slice in one sentence: *gather → craft a machine → place it → it produces while
you're gone → come back and collect.* Everything after — trade, settlement
claims, building tiers, wildlife — layers onto these three.

## Pillar check

| Pillar | Cozy frontier? |
| --- | --- |
| Works from any terminal | ✅ panels + placed tiles render in both clients |
| SSH username is identity | ✅ placements/machines owned by username |
| Shared, real-time, eventually consistent | ✅ placements fan out as events like chat |
| Persist between visits, not during | ✅ **machines are built on exactly this** |
| Deterministic & offline | ⚠️ terrain stays pure-seed; **placements are a stored sparse layer** — same precedent as guestbook/decks/gate-pool. Machines fast-forward by elapsed time, no per-tick RNG. |

## Parked: light danger (the optional later layer)

If we ever want "animals / weapons / enemies" without breaking determinism or
the cozy tone:

- **Huntable wildlife** placed as a pure function of `(seed, x, y, time-window)`
  — deer in forest, rabbits in grass (fish already exist). No AI, no drift.
- Hunting needs a **tool** (a knife/bow crafted from the recipe table) and
  yields **meat + hide** → food that buffs gather speed, hide for better gear.
- This delivers "animals" and a reason for "weapons/tools" with **no combat, no
  enemies, no mob AI**. True enemies and PvP stay behind a future genre decision.

Designed so it slots onto the cozy loop (it's just more resources + recipes),
never a prerequisite for it.

> **Update:** the genre decision has been made for *light* danger — weapons, a
> shared player/creature damage model, and **consensual, zone-gated PvP** (safe
> hub + settlements, live PvP only in the open Wilds, cozy knock-out with no item
> loss). See [`WEAPON_PLAN.md`](WEAPON_PLAN.md). It is gated on the wildlife
> branch landing in `main` first, since it builds on the creature HP model.

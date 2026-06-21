# Milestone 1 — Implementation Plan (Craft · Place · Automate)

> The first vertical slice that turns Durst World from a place into a game, from
> [`DESIGN_MECHANICS.md`](DESIGN_MECHANICS.md). Scope: **gather → craft a machine
> → place it → it produces offline → collect.** Grounded in the real packages
> (`internal/{store,world,game,areas/wilds}`). The five production sprites
> (`PropWorkbench/Sawmill/Mill/Furnace/Chest`) already exist — see
> `internal/game/tileset.go`, previewed by `cmd/mechpreview`.

## What already exists to build on

| Need | Precedent in the codebase |
| --- | --- |
| Spend/earn items | `store.AddItem/SpendItem/LoadInventory`, `Ctx.Inventory` |
| Per-player stored facts on a stateless world | fog `Save/LoadDiscovery`, `MarkCollected` |
| **Shared** stored facts placed in the world | the co-op gate pool: `world.OfferToGate`, `store.Save/LoadGateWorld` |
| Drawing props in both renderers | `propArt` (HD) + a `Ch` glyph (glyph client) |
| Sampling a window of the stateless world each frame | `internal/areas/wilds` samples `worldgen` per frame |
| On-frame panels | `DrawInventoryPanel`, `DrawCharPanel` in `hd_ui.go` |

The placements layer is the co-op gate pattern generalized: a small table of
stored facts overlaid on the deterministic terrain. Nothing here is novel to the
architecture — it's an established shape applied to a new feature.

## Step 1 — Recipe table + crafting (no world changes yet)

Pure data + a panel. Smallest first win; touches only `internal/game`.

- **`internal/game/recipes.go`** — a static catalog, the twin of `inventory.go`:
  ```go
  type Recipe struct {
      ID, Name, Blurb string         // "wall_block", "Wall Block", flavor line
      In   []Ingredient              // {ItemID, N}
      Out  ItemID; OutN int          // most yield an item…
      Prop TileProp                  // …machines/structures yield a placeable
  }
  ```
  IDs stable (they key nothing persisted yet, but will). Voice: corporate ×
  medieval ("Crafting (Self-Service)").
- **`Craftable(r, inv) int`** — how many you can make from an inventory map;
  `Craft(ctx, r)` spends via `store.SpendItem` + `AddItem` (or sets a placeable
  in the pack). Reuses the exact inventory plumbing foraging already uses.
- **UI in both renderers** (golden rule #1):
  - HD: `DrawCraftPanel(img, ctx, sel)` in `hd_ui.go`, modeled on
    `DrawInventoryPanel`; opened by a key (e.g. `e` at a workbench, or a menu
    entry). Layout is already prototyped in `cmd/uipreview`.
  - Glyph: a panel like the lobby guestbook / presentation editor.
- **Tests:** `Craftable` math, `Craft` spends exactly the inputs and yields the
  output, insufficient-inputs is a no-op. (`recipes_test.go`.)

**Done when:** standing anywhere, you can open Crafting and turn `2 Timber` into
`Planks`, inventory updating live. No placement yet — pure inventory→inventory.

## Step 2 — The placements layer (the one architectural piece)

A sparse, stored overlay on the stateless Wilds. This is the co-op gate
generalized from one global pool to many owned, positioned objects.

- **Store** (mirror the gate-world methods):
  ```go
  AddPlacement(p Placement)                  // owner, x, y, kind, createdUnix, state json
  RemovePlacement(x, y int)
  LoadPlacements(minX,minY,maxX,maxY int) []Placement
  ```
  New `placements` table keyed by `(x,y)`; add no-ops to `noopStore` so the game
  still **plays without memory** (pillar #4). `state` is a JSON blob so machines
  (Step 3) need no schema change.
- **World** holds the live set so it fans out like presence:
  `world.Place/Unplace` mutate under the one mutex and broadcast a
  `PlacementEvent`; sessions apply it the way they apply chat/joins. Load the
  area's placements into the world on startup (like `LoadGateWorld`).
- **Render overlay** in `internal/areas/wilds`: when sampling the player window
  each frame, after `worldgen.At`, stamp any placement at `(wx,wy)` onto the tile
  (`t.Prop, t.PropHex = …`), exactly like `hdpreview_test.go` already overlays
  loot. Terrain stays a pure function of the seed; placements are the only
  mutable layer — and they're sparse, so the lookup is a map hit per tile.
- **Build mode** (prototyped in `cmd/uipreview`): `b` enters placement; a ghost
  follows the cursor (green/red via a walkability + occupancy check); `e` spends
  the recipe's materials and calls `world.Place`; `r` rotates where it matters.
  Block placing on water/peaks/occupied/other areas; restrict to open ground.
- **Tests:** place→load round-trips; an occupied or blocked cell rejects;
  placements survive a reload; two sessions both see a new placement
  (broadcast). Determinism test: terrain unchanged with placements absent.

**Done when:** you craft a Fence/Workbench, place it in the Wilds, walk away and
back, and it's still there — and another player sees it too.

## Step 3 — One offline machine (proves the idle loop)

A machine is a placement whose `state` JSON carries buffers + a wall-clock.

- **Model** (`internal/game/machine.go`):
  ```go
  type Machine struct {
      Recipe   string  // what it converts (e.g. timber→planks)
      In, Out  int     // buffer counts
      LastTick int64   // unix seconds, last settled
  }
  ```
  `Settle(m, now)` is **pure**: advance `floor((now-LastTick)/period)` cycles,
  bounded by input available and output cap, consuming `In`, producing `Out`,
  set `LastTick`. No per-tick simulation, no RNG — so it's deterministic and
  costs nothing while you're gone. Called lazily on open/collect/load.
- **Interaction:** `e` at a machine opens `DrawMachinePanel` (prototyped in
  `cmd/uipreview`): `Settle` first, show input/output meters, the **"while you
  were away"** delta, `e` collect (Out→pack via `AddItem`), `f` refuel (pack→In
  via `SpendItem`). Persist the new `state` through `AddPlacement`.
- **Promote Kraftwerk** from decoration to the worked example: a couple of these
  machines on the production floor (one hand-built area, no worldgen needed) —
  the lowest-risk place to ship machines before they're loose in the Wilds.
- **Tests:** `Settle` is the core — elapsed time → exact output, respects input
  exhaustion and output cap, idempotent within a period, and **deterministic**
  (same inputs+elapsed → same result). This is the test that guards the pillar.

**Done when:** you load a Furnace with nuggets, disconnect, reconnect later, and
it greets you with ingots it smelted while you were offline.

## Sequencing & risk

1. **Step 1** ships alone (no world/store risk) — immediate playable win.
2. **Step 2** is the architectural commit; review the determinism + broadcast
   tests carefully, it's the foundation everything later rides on.
3. **Step 3** is small once Step 2 exists (a machine is just a placement with a
   richer `state` and a `Settle`).

Everything after this milestone — trade/Concessions, settlement claims, building
tiers, the parked wildlife layer — layers onto these three with no new
architecture.

## Pillar guardrails (must stay green)

- **Both renderers** for every panel and prop (golden rule #1).
- **Plays without memory:** all new store methods have `noopStore` no-ops.
- **Deterministic & offline:** terrain stays pure-seed; placements are the only
  stored mutable layer (precedent: guestbook, decks, gate pool); machines
  fast-forward by elapsed wall-clock with **no per-tick randomness**.
- **Broadcaster never blocks:** placement events fan out on the existing
  buffered-channel path; never add synchronous cross-session work.

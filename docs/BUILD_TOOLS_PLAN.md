# Build Tools Plan — palette UI, clearing tools, terrain regrowth

> The build loop's next chapter, from the brainstorm. Two threads: **a better
> build interface** (the current blind `r`-cycle becomes a palette + hotbar), and
> **clearing tools** that let you fell trees and break hill boulders — found,
> crafted, and unlocked — turning cleared ground into a stored overlay that
> **regrows when abandoned**. Grounded in the real packages; like claims and the
> Concession it adds no new storage *architecture* (clearing rides the same sparse
> overlay precedent). Voice stays corporate × medieval.

## Decisions (locked)

| Question | Choice |
| --- | --- |
| Build interface | **Palette panel + number-key hotbar** (keep the ghost cursor for placement) |
| Tool acquisition | **Found head → craft** (a rare head unlocks the workbench recipe) |
| Clearable | **Trees + hill boulders** (mountain peaks stay permanent) |
| Cleared land | **Regrows when the owner is long absent** (reuses the claim lapse clock) |

## Step A — the palette interface ✅

Pure UI, no new mechanics. Build mode stays non-modal: movement still steers the
ghost, so the palette is an always-on **reference HUD** while building, not a
focus-stealing screen. Shipped: `game.BuildPalette`/`PaletteHotkey` +
`DrawBuildPanel` (HD) + the glyph `buildPanel`, the `BuildViewer` interface, and
`1`–`9` hotkey selection. The palette footer carries the claim hint / block
reason ("can't build: trees in the way"), and the HD client suppresses the
centered action prompt while building so the two don't collide.

- **`game.BuildPalette(inv)`** groups `Placeables` into **Structures · Machines ·
  Trade** (a **Tools** group joins in Step C), each entry annotated with whether
  the pack can afford it and how many it could build.
- **`game.DrawBuildPanel(img, ctx, sel, reason)`** (HD) and a glyph `buildPanel()`
  render the grouped list: sprite/glyph, name, cost, **dimmed when unaffordable**,
  a `[1]`–`[9]` hotbar badge on the first nine, the selected row highlighted with
  its blurb, and — when the ghost is on a bad cell — an honest **block reason**
  ("trees in the way", "Anna's land", "occupied").
- **Selection:** number keys `1`–`9` jump straight to a placeable; `r` / `[` `]`
  still cycle. Arrows stay on the ghost (no key conflict, no modality).
- Placement is unchanged (`e` place, `x` remove/release, `b` done).

Both renderers, mirroring `DrawCraftPanel`. The area exposes a tiny `BuildViewer`
interface (`BuildPanel() (sel, reason, show)`) the HD client reads, the same way
it reads `HDMinimapper`.

## Step B — cleared-cells overlay + regrowth ✅

A sparse stored overlay on the pure-seed terrain (the third rider on the
placements/claims precedent). A cleared cell overrides forest/boulder → grass/
ground: walkable, buildable, rendered as a clearing. Each carries an owner + a
last-touch clock and **regrows** (reverts to seed terrain) once untouched past a
~2-week lease — the same wall-clock lapse claims use, so the woods reclaim ghost
towns. Gated by `BuildRight`, so you only clear where you may build. Shipped:
`world.{ClearCell,ClearedAt,ClearedOverlapping,TouchCleared,Regrow}` + persistence;
`game.{ClearGround,IsCleared,ClearedActive,ActiveClearedSet,TouchCleared}`; the
wilds `walkableAt`/`canBuildAt`/`sample` consult the overlay and the body tends
its own clearing on the move. (The player-facing clear *action* — a tool over a
tree — is Step C; Step B is the overlay it writes to.)

## Step C — tools: found heads, recipes, the clear action

- New finds: a **flint Axe-head** (Wilds forage, rare) and an **iron Pick-head**
  (caves, rare). Finding one **unlocks** its recipe (head + Timber → the tool),
  crafted at a workbench — exploration *and* the craft table, like wearables.
- Owning a tool adds it to the palette's **Tools** group; selecting it turns the
  ghost into a clear cursor: **Axe** over a tree → "✂ fell (Timber)", **Pick**
  over a hill boulder → "⛏ break (Cut Stone)", writing the Step-B overlay.
- Mountain peaks are never clearable.

## Step D — docs

Fold the shipped result into `ROADMAP.md` Phase 9.

## Pillar guardrails

- **Deterministic & offline:** terrain stays pure-seed; the cleared overlay is the
  stored mutable layer (precedent: placements, claims), regrown by elapsed wall-
  clock, no per-tick RNG.
- **Plays without memory:** cleared-cell store methods get `noopStore` no-ops.
- **Both renderers:** the palette and the clear cursor render in HD and glyph.
- **Cozy / no grief:** clearing is `BuildRight`-gated (your land / open frontier
  only) and bounded, and the world heals itself over time.

# Claims Plan — Land Tenure & Protected Building

> How players come to **own** a patch of the world and build on it safely, built
> on the cozy-frontier foundation ([`DESIGN_MECHANICS.md`](DESIGN_MECHANICS.md),
> [`ROADMAP.md`](ROADMAP.md) Phase 9). Grounded in the real packages
> (`internal/{worldgen,world,game,areas/wilds}`). Like the trade Concession, this
> adds **no new storage architecture**: a claim is a row on the existing
> placements layer, and protection is one predicate over placements you already
> store. Voice stays corporate × medieval — a deed is a **Workspace Charter**.

## The problem

Today there is **object** ownership (you own the stall you built; only you can
demolish it) but no **land** tenure. Anyone can build anywhere that's discovered,
walkable and unoccupied — including hemming in someone else's machines. For a
cozy game that's a griefing surface, and the design's strongest return hook
("makes the world *yours*") needs a bounded, owned space to return to.

## Two tiers, one predicate

Protection comes in two flavours, both resolved by a single function consulted in
build mode and demolition:

```go
// BuildRight reports whether `who` may place or remove a structure at (x,y):
//   - inside a live claim or another player's wilds buffer → only that owner
//   - otherwise (open, unclaimed, lapsed) → anyone
// Movement is never affected — you can always walk anywhere.
func BuildRight(w *World, who string, x, y int) (ok bool, owner string)
```

| | **Town plots** | **Open Wilds** |
| --- | --- | --- |
| Tenure | Formal deed (a Workspace Charter) over a pre-drawn settlement building | Organic: a small buffer around your own structures |
| How you get it | Claim a settlement plot (scarce, finite) | Just build (infinite frontier) |
| Bounds | The plot's parcel (footprint + a margin) | ~3-tile radius of each owned structure |
| Identity | Named ("Anna's Workspace, Brixen") | Unnamed |
| Lapse | Fades after the owner is absent `leasePeriod` | Same |
| Movement | Never blocked | Never blocked |

The two are the *same idea* — an owned anchor projects a build-zone — so they
share `BuildRight` and the same wall-clock lapse rule. Town plots add scarcity, a
name, and a village around you; the wilds tier adds grief-free homesteading with
no deed, no UI, no scarcity (the world is infinite, so a buffer never starves the
next builder — they step a few tiles over).

## Architecture

### Claims are placements

A claim is a `world.Placement` with `Kind == "charter"`, owned by the claimant's
username, persisted and broadcast like any structure. Its opaque `State` is:

```go
type Claim struct {
    PlotID        string  // worldgen plot id this charter deeds (town claims)
    MinX, MinY    int     // parcel bbox, snapshotted at claim time …
    MaxX, MaxY    int     // … so the claim stays self-describing if worldgen shifts
    LastTouchUnix int64   // refreshed while the owner is present; drives lapse
}
```

Snapshotting the parcel bbox into the charter (rather than recomputing from
worldgen every time) mirrors how fog-of-war stores chunk bitmasks: the generator
is the *source* of plots, but a stored claim carries its own bounds so a later
worldgen tweak can't silently move someone's deed.

### Plots come from worldgen (step 1)

Settlements already segment buildings (`lBuildAnchor` + `footprint`). Expose them
as stable, pure-seed parcels:

```go
type Plot struct {
    ID         string // "<settlement-hex>:<ax>,<ay>" — stable across runs
    Settlement string // settlement identity, for grouping / naming
    Town       bool   // a stone city vs a timber village
    Kind       string // "Cottage", "Townhouse", "Church", …
    AX, AY     int    // anchor (north-west corner) world coords
    W, H       int    // footprint size
}

func (g *Generator) PlotAt(x, y int) (Plot, bool)
```

Buildings are solid (you can't stand *on* one), so you claim a plot by standing
**beside** it — the game layer reuses the `stationAdjacent` ring scan that already
opens machines and stalls. The buildable parcel is the building's footprint
expanded by a margin, clipped to the settlement and to walkable ground; that
margin policy lives in the game layer, not worldgen (worldgen reports structure;
the game decides build rules).

### Lapse: the lazy wall-clock settle

No background simulation — the same pattern as `Machine.Settle`. A charter's
`LastTouchUnix` is refreshed whenever the owner stands in or builds on the plot.
On any access (someone walking in, another player trying to claim, the HUD naming
it) a pure check `now - LastTouchUnix > leasePeriod` decides if the claim has
lapsed; a lapsed claim is treated as released and the plot becomes re-deedable.
Deterministic, costs nothing while nobody looks, no per-tick RNG. `leasePeriod`
starts at ~7 real days, one tunable constant. The wilds buffer lapses the same
way off its structures' owner activity.

### The claim index

`world` keeps an in-memory index (claim → bbox, owner) rebuilt on `EventPlaced`,
so `BuildRight` is a handful of bbox tests per ghost cell — the same per-tile
cost as the existing placement lookup. The wilds buffer is derived from the
placement set already in memory (foreign-owned structure within K tiles), no
extra storage.

## Pillar guardrails (must stay green)

- **Deterministic & offline:** terrain *and* plots stay pure-seed; the charter is
  the sole stored mutable layer (precedent: guestbook, decks, gate pool, stalls).
- **Plays without memory:** charter rows ride the existing `placements` table, so
  `noopStore` already no-ops them.
- **Both renderers:** the charter is a prop (like the stall) and the "whose plot"
  HUD line renders in HD and glyph alike.
- **Broadcaster never blocks:** claim/release fan out as `EventPlaced` on the
  existing buffered path.
- **Cozy:** claims gate *building*, never *movement* — no one is ever walled out.

## Sequencing

1. **`worldgen.PlotAt`** + tests — pure, self-contained, no game changes (safest
   to land first). Determinism, footprint coverage, anchor recovery from any body
   cell, `ok=false` off-building.
2. **Claim model + Workspace Charter placeable + the `world` claim index +
   `BuildRight` + `settle` (lapse).** Tests: a claim reserves its plot, a second
   claimant fails, a lapsed claim is re-deedable, the owner's presence refreshes
   the lease, and the wilds buffer rejects a foreign builder but not the owner.
3. **Wire `BuildRight` into `canBuildAt` / `Demolish`**, claim & release in the
   build flow, and the HUD "Anna's Workspace, Brixen" line (both clients).
4. **Docs** — fold the shipped result into `ROADMAP.md` Phase 9.

Everything after — co-owned plots, settlement-wide perks, decay tuning — layers
onto these with no new architecture.

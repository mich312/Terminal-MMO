# Wildlife Plan — Living Things in the Wilds

> **Status:** ✅ Phase 1 shipped — the live-creature registry + the wildlife
> stepper, an MVP slice of biome fauna rendered in both clients, and the
> observe/compendium loop. ✅ Phase 2 shipped — hunting: species HP + drop
> tables feeding the inventory, an `f`-to-strike action settled atomically so
> two hunters can't double-loot one kill (no PvP, no player damage). ⬜ Phase 3
> (taming / companions) remains, building on the same `Owner` field + registry.

> How wildlife lands on the cozy-frontier foundation
> ([`DESIGN_MECHANICS.md`](DESIGN_MECHANICS.md),
> [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md)), reusing the placement,
> event-broker, and forage patterns already proven by
> [`TRADE_PLAN.md`](TRADE_PLAN.md). Voice is cozy × naturalist.

## The shape: a herd the server keeps alive, not a function of the seed

Everything in the Wilds today is either **deterministic** (terrain, items, hats —
a pure function of `(seed, x, y)` in `internal/worldgen` and
`internal/areas/wilds/items.go`) or **player-driven** (placements, chat, trade).
There is no autonomous anything: the design note in `wilds.go` literally reads
*"No AI/NPCs. The world is deterministic and offline."*

Wildlife breaks that on purpose, but minimally. A deer that wanders, startles,
and flees **cannot** be a pure function of position — its state changes over time
and in response to players. So creatures become the first thing the **server owns
as live state** and broadcasts, exactly like player movement.

The trick that keeps this cheap and on-pillar: creatures are **deterministically
*seeded* but live-*simulated*.** Where a herd *can* exist is a hash of
`(seed, biome, region)` — so two sessions see fauna in the same places and the
world still feels authored. But once spawned, each creature is a real entity the
world steps on the tick and fans out to subscribers. Spawning is lazy (only near
online players) and despawn is aggressive (no players nearby → gone), so the live
population is bounded by who's actually online, not by the size of an infinite map.

## The one new architectural piece: a creature registry on the World

Players already live in `World.players map[string]*Player` under one mutex
(`internal/world/world.go:71`), snapshotted out via `PlayersInArea`. Creatures
get the same treatment — a sibling map, the same mutex, the same snapshot
discipline (nobody mutates shared creatures outside the world lock).

```go
// internal/world/creature.go (new)
type Creature struct {
    ID     string   // stable instance id, e.g. "deer-3f2a"
    Kind   string   // species id -> looked up in a species table
    Area   string   // area id (MVP: only "wilds")
    X, Y   int
    Facing Dir      // reuses the existing 8-way Dir
    State  string   // "graze" | "wander" | "flee" | "tamed" (opaque to world)
    HP     int      // 0 until Phase 2; world stays mechanics-agnostic
    Owner  string   // "" until Phase 3 (tamed companion's player)
    Seed   int64    // per-instance RNG seed for deterministic-ish wandering
}
```

Add to `World`, guarded by the existing mutex:

```go
creatures map[string]*Creature   // id -> creature (live, server-owned)
```

And a small surface mirroring what players already have:

- `CreaturesInArea(area string) []Creature` — snapshot for rendering.
- `SpawnCreature(c Creature)` / `DespawnCreature(id string)` — broadcast on change.
- `MutateCreature(id string, fn func(*Creature) bool) bool` — atomic
  read-modify-write under the lock, the same primitive `MutatePlacement` gave
  trade. This is how the tick steps a creature and how Phase 2 applies damage
  without races.

The world stays **schema-agnostic**: it never interprets `State`, `Kind`, or
`HP` — the simulation in the wilds area does.

## New events

Add to `internal/world/events.go` alongside `EventMoved` / `EventTick`:

```go
EventCreatureSpawned
EventCreatureMoved
EventCreatureDespawned
```

Fanned out with the existing `broadcastToArea` / `deliver` machinery — same
non-blocking, drop-oldest delivery players already use, so a slow session just
gets eventual consistency on the herd. No new transport.

## Who drives the simulation: one ticker, not every session

Today the only tick-driven mutation is the portal pulse. Each session's
bubbletea model reacts to `EventTick`, but **only the server should step
creatures** — if every session simulated, N sessions would fight over one herd.

So add a single **wildlife stepper** owned by the server, riding the existing
2 Hz `tickLoop` in `world.go` (or a dedicated goroutine in
`cmd/durstworld/main.go` that subscribes to ticks). Once per tick it:

1. **Census** online players per region (cheap; players are already in memory).
2. **Spawn** lazily: for each populated region whose deterministic fauna budget
   isn't met and that has a player within ~2 screens, roll seeded spawns at
   biome-appropriate, walkable tiles (reuse `worldgen` biome lookup +
   `Walker.CanStep`-style collision).
3. **Step** each live creature via a tiny behavior function (below).
4. **Despawn** any creature with no player within range for a few ticks.

Stepping at 2 Hz is intentionally slow and legible — animals drift a tile every
second or two, not every frame. The `EventTick.Frame` counter already exists for
animation phase.

## Behavior: a 30-line state machine, no pathfinding

MVP behavior is deliberately tiny and reads from a per-species table:

```go
// internal/areas/wilds/fauna.go (new) — species definitions
type Species struct {
    Kind      string
    Glyph     rune          // glyph client
    Color     string        // hex
    Sprite    int           // HD avatar/critter bitmap index
    Biomes    []Biome       // where it spawns
    Skittish  int           // flee radius in tiles (0 = placid)
    Speed     int           // move every N ticks
    // Phase 2+: MaxHP, Drops []ItemDrop, Tameable bool, Bait string
}
```

Per-tick per-creature logic:

- **flee**: if a player is within `Skittish`, set `State=flee`, step one tile
  directly away (along the existing 8-way `Dir`), update `Facing`.
- **wander**: otherwise, on its `Speed` cadence, step to a random adjacent
  walkable tile using the creature's own `Seed` (so motion is reproducible and
  desyncs heal).
- **graze**: occasionally idle in place (just a facing flip) for flavor.

Collision reuses the same walkability check the `Walker` uses, against
`worldgen` terrain and `World.placements` — creatures don't walk through walls,
water (land animals), or buildings.

### MVP species (the slice)

A handful, one or two per common biome, all ambient + observable:

| Kind     | Biome(s)        | Glyph | Behavior      |
|----------|-----------------|-------|---------------|
| Rabbit   | Grass, Savanna  | `r`   | very skittish |
| Deer     | Forest, Grass   | `d`   | skittish      |
| Fox      | Forest, Hill    | `f`   | wary          |
| Bird     | any land        | `v`   | flits, placid |
| Fish     | Water (shallow) | `~`   | placid        |

Sprites: glyph client gets a colored letter via the existing
`stampSprite` path; HD client gets small critter bitmaps added next to the
avatar bitmaps (`internal/game/avatar_sprites.go`). If an HD sprite is missing,
fall back to a glyph tile so the renderer never blocks on art.

## Rendering: one new stamp pass, players still on top

Creatures draw in the existing pipeline in `internal/game/render.go`. Add a
`stampCreatures(grid, creatures)` pass that runs **before** `stampPlayers` so
players always render on top of fauna sharing a tile. It honors the same
fog-of-war / lighting the wilds already applies — a deer in unexplored fog isn't
revealed; one in your sight radius glows normally. The HD loop
(`cmd/durstworld/hd.go`) gets the parallel treatment in its delta-frame encoder.

## Interaction (MVP): observe + a clean hook for more

The wilds area already routes `e` for forage/gates. Extend its key handler: when
standing adjacent to a creature, `e` **observes** it — a short toast ("A deer
watches you, then bolts.") and a first-sighting entry in a lightweight
**compendium** persisted per player (reuse the inventory/unlock persistence
pattern in `internal/store`). This ships value immediately and proves the
adjacency-targeting code that Phases 2–3 need.

## Persistence

Live creatures are **ephemeral** (like chat and presence) — they respawn from the
deterministic budget, so the registry need not survive a restart. What *does*
persist, in `internal/store`:

- The player's **compendium** (species sighted).
- Phase 3: **tamed companions** (a creature with `Owner` set), so your pet is
  still yours next session.

---

## Phase 2 — Hunting (the first combat in the game) ✅ shipped

A genuine new system, kept minimal and non-griefy (it's a cozy world). As built:

- Species carry `MaxHP` and a `Drops []Drop` table (item id + count range +
  optional chance) feeding the existing `inventory.go` catalog — new hunting
  spoils `meat`/`hide`/`pelt`/`feather` (fish drop the existing `fish`),
  grouped under a new "Hunting spoils" compendium section with their own icons.
- A **strike action on `f`** (kept separate from `e`-observe, so peaceful play
  stays the default and hunting is deliberate; `f` is wired through both the
  glyph and HD clients). It calls `World.MutateCreature` to decrement `HP`
  **atomically**: the blow that brings HP to 0 is the one that despawns the
  animal and rolls `RollDrops` into the hunter's pack — so two hunters striking
  one creature can never both claim the kill (`TestMutateCreatureAtomicKill`).
- **No new events.** A struck animal flips to `flee` state and the existing
  poll/tick redraw shows it bolt or vanish — consistent with Phase 1's
  no-fan-out rendering. Discrete hit events can come later if needed.
- **No PvP, no player damage, no aggressive mobs.** Predators that fight back
  are a later flag on `Species`.

## Phase 3 — Taming & companions

Builds directly on the registry + ownership field:

- `Species.Tameable` + `Bait` (an item id). Feeding bait near a wary creature
  rolls a tame chance; success sets `Owner` and `State=tamed`.
- **Follow behavior**: a tamed creature's step targets its owner's tile (greedy
  step toward, still no real pathfinding), and it despawns/reattaches with the
  owner across areas. This is the one place a creature persists.
- Companions are cosmetic-first (a fox trotting behind you), with room later for
  utility (a pack animal that extends inventory, a hound that flushes game).

---

## Build order (Phase 1, the shippable slice)

1. `internal/world/creature.go` + registry/methods/events on `World`
   (`world.go`, `events.go`) with `MutateCreature`. **Foundations first.**
2. Wildlife stepper goroutine (server-side) with census → spawn → step →
   despawn, riding the existing tick.
3. `internal/areas/wilds/fauna.go` — `Species` table + behavior fn + deterministic
   spawn budget; MVP species above.
4. `stampCreatures` in `render.go` (glyph) and the HD parallel in `hd.go`;
   critter sprites with glyph fallback.
5. `e`-to-observe + per-player compendium persistence (`store`).
6. Tests: deterministic spawn placement, behavior stepping, flee-from-player,
   despawn-when-empty, and the atomic `MutateCreature` race (mirroring the
   stall-transaction test from the trade work).

## Risks & guardrails

- **Population blowup on an infinite map** → spawning is gated on online players
  and a per-region budget; despawn is aggressive. The live set tracks players,
  not world size.
- **N-session simulation fights** → exactly one server-side stepper owns
  mutation; sessions only render snapshots.
- **Event flood at 2 Hz** → reuse drop-oldest delivery; only broadcast on actual
  position/state change, not every tick.
- **Breaking the "deterministic & offline" feel** → fauna *placement* stays
  seeded so the world reads as authored; only motion is live, and only near
  players who are there to see it.

---

## Performance impact analysis

The honest summary: **Phase 1 adds a single low-frequency goroutine and a few
small per-frame reads, and deliberately adds zero new network fan-out.** The
costs are bounded by the number of *online players*, not by the (infinite) map,
and they sit comfortably inside the headroom the existing 2 Hz tick already
assumes.

### Baseline (what runs today)

- One global `tickLoop` at **2 Hz** broadcasts `EventTick` to every glyph
  subscriber (`world.go:293`).
- Each glyph session redraws on tick; the HD client *polls* world state at its
  own frame cadence and is excluded from the tick/move stream
  (`MarkPoller`, `world.go:359`).
- **One mutex** guards all world state (`world.go:71`). Every read
  (`PlayersInArea`) and write (`Move`) takes it briefly.

### What Phase 1 adds

**1. The stepper — one goroutine, O(creatures) per 500 ms.**
Per tick it does a player census, a bounded spawn pass, one step per creature,
and a despawn sweep. Each creature step is a handful of cheap pure-noise
`gen.At`/`Walkable` calls plus one short locked `MutateCreature`. With the
population capped (see knobs), this is on the order of *tens of short critical
sections per half-second* — negligible CPU, and it never touches disk (live
creatures are ephemeral).

**2. Lock traffic — the one thing to watch.**
The stepper contends with rendering and movement on the single world mutex.
Mitigations already baked in: snapshot-then-act, and critical sections that do
nothing but a map read/write. At MVP caps this is far below contention that
would show up as input lag. **Tuning lever:** if a future profile shows lock
churn, collapse the per-creature `MutateCreature` calls into one
`StepCreatures` batch under a single lock — turning *C* lock cycles per tick
into one.

**3. Rendering — O(1) per tile, one extra snapshot per frame.**
`sample()` gains one `CreaturesInArea` call (a locked snapshot + slice alloc)
and an O(visible-tiles) overlay using an O(1) position map. For the glyph
client that's 2×/sec; for HD it's per polled frame. The only real cost is the
per-frame slice allocation from `CreaturesInArea` — bounded by the cap, and
skippable with an early `if len(creatures)==0` guard when none are near.

**4. Network — unchanged. This is the big win.**
We render creatures by **reading live state in the existing redraw**, not by
broadcasting moves. The naive design — an `EventCreatureMoved` per creature per
tick, fanned to every subscriber in the area — would multiply event-channel
traffic by *C × subscribers* and risk evicting chat/trade events from the
64-deep buffers. By piggybacking on the tick (glyph) and poll (HD) loops that
already run, Phase 1 adds **no new events and no new fan-out.** (Events return
only in Phase 2, for discrete one-shot interactions like a strike — rare, not
per-tick.)

**5. Memory — trivial and bounded.** Each `Creature` is a small flat struct;
the cap puts a hard ceiling on the live set, and nothing persists.

### Scaling

With *P* online players and *C ≈ min(k·P, maxTotal)* creatures: per-tick CPU is
**O(C)**, per-frame render overhead is **O(visible tiles + C)**, network is
**O(0)** beyond today. Because spawning is gated on nearby players and despawn
is aggressive, an empty region costs nothing and a crowded one is capped — the
infinite map never inflates the bill.

### Tuning knobs (all constants in `internal/wildlife`)

`maxPerPlayer`, `maxTotal` (population ceiling) · `MoveEvery` per species
(stagger cadence so not all creatures step on the same tick) · spawn/despawn
radii · the optional `len==0` render guard · the optional batched
`StepCreatures` lock. Start conservative; raise `maxTotal` only after a profile
under real concurrency.

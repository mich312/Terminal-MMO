# Community Plan — Shared Builds & the Async-Social Loop

> **Status:** ⬜ Proposed. The next chapter after the cozy-frontier loop
> ([`DESIGN_MECHANICS.md`](DESIGN_MECHANICS.md)), wildlife
> ([`WILDLIFE_PLAN.md`](WILDLIFE_PLAN.md)) and arms
> ([`WEAPON_PLAN.md`](WEAPON_PLAN.md)). It adds **no new genre** — it closes the
> loop the others opened. Read [`GAME.md`](GAME.md) for what exists and
> [`STYLE_GUIDE.md`](STYLE_GUIDE.md) before touching visuals. Voice is
> corporate × medieval.

## Why this, and why now

Two facts decide it:

1. **The economy has no sink.** You gather → craft → automate → trade and then…
   accumulate in your own pack. `DESIGN_MECHANICS.md` set out to give the world
   "a reason to return tomorrow"; the loop it built dead-ends in private
   hoarding. Nothing you make changes the shared world, and trade only pays off
   when a buyer happens to be online.
2. **The audience is one company — the room is usually empty.** For a social
   game with a small player pool, the existential risk isn't "not enough
   mechanics," it's "I logged in and no one's here." None of the shipped systems
   answer it: combat, wildlife and the arcade all want a crowd.

This plan targets both with one feature. It leans on the two mechanics that
already work when you're **alone** — offline machines and async Concession
trade — and gives them a **shared destination**: a communal build the whole
company raises together, asynchronously, that visibly changes the world and
leaves the trace of other people all over it.

It is the co-op **Sunken Gate** — the one social pattern the game already proved
(everyone pools offerings until a gate opens for all) — generalized from a
one-shot into an **ongoing, staged, named** project.

## The shape: a community project is shared world state

A **Community Project** is a large, owner-less structure on the Wilds that the
whole player base builds together over time:

- It sits at a **deterministic, pure-seed site** near the hub (like the sealed
  gates, placed in a forced clearing so it's always reachable and identical for
  everyone). Only its *built-up state* is stored — the site itself is seeded.
- It advances through **phases** (foundation → frame → roof → fit-out), each a
  small bill of existing resources (e.g. *50 Timber*, *30 Cut Stone*,
  *20 Planks*). Phase costs come straight from the 14 resources already in the
  game and the recipe outputs — no new item types required.
- You **contribute** from your pack at the site (`e`, or a panel to pick the
  resource and amount). Contributing **spends** the goods permanently — this is
  the sink. Rewards are *shared unlocks*, never your goods back, so it's a true
  drain on the economy, not a wash.
- Progress is **global and visible**: one bar everyone sees, the structure
  **grows in the world** as phases complete (a more-built sprite each phase),
  and a **milestone reward unlocks for everyone** — a new recipe, a new
  buildable, access to a new room, or a cosmetic.
- It is **named after its builders.** A persisted ledger records who gave what;
  the finished structure carries a plaque ("Raised by anna, markus, …"). Your
  contribution is a permanent, public mark in a shared place — the strongest
  reason to come back.

### The flagship: *The All-Hands Hall*

The first project — corporate × medieval deadpan, a great hall the company
convenes in that is also an all-hands meeting nobody can leave. The system is
generic; naming follows the same generator as everything else:

| Thing | Cozy-frontier name | Corporate × medieval name |
| --- | --- | --- |
| The shared build | Community project | **Capital Works** |
| The flagship structure | Town hall | **The All-Hands Hall** |
| A contribution | Donation | **Q3 Capital Contribution** |
| The builders' plaque | Founders' board | **The Cap Table** |
| A phase | Stage | **Milestone (per the Roadmap)** |

## The second half: "while you were away"

A shared build only fights the empty room if you can *see* it moved. So the
companion feature — cheap, because the data already exists:

- On join, compute a **digest since your last visit** (`last_seen` is already
  refreshed by `RecordDisconnect`): what the company contributed, which phases
  completed, what colleagues built, placed or sold near you. Surface it as a
  dismissable entry panel/toast — *"While you were away: the Hall reached its
  roof; markus laid 40 timber; your Sawmill filled."*
- It's mostly a **read + format** over the existing **event log** (`LogEvent`)
  plus the new project ledger and the machine/stall state you already settle on
  load. The "while you were away" machine delta (`Settle`) is the precedent —
  this generalizes it from your own machines to the whole frontier.
- When the log is quiet it says so gracefully (*"The frontier is still."*),
  never a blank panel.

Together: you always arrive to **evidence other people exist** and that the
world advanced — even at 3am with nobody else connected.

## Architecture — it rides the layers you already have

Nothing here is a new architectural risk. A project is the **gate pool with
more structure**, and it reuses every primitive the shared-mutable layers
already established:

| Need | Existing precedent to copy |
| --- | --- |
| Shared, persisted project state | the co-op gate pool — `World.OfferToGate`/`GateFixed`, persisted via `store.SaveGateWorld`/`LoadGateWorld` (`gates_world`) |
| Atomic concurrent contribution | `World.MutatePlacement` (the 60-buyers-at-20-stock stall test) and the `artifact` first-write-wins registry |
| A small shared set loaded at boot | placements / claims / cleared / artifacts (`LoadPlacements`, `LoadClaims`, …) |
| Multi-tile structure that grows | settlements' bottom-anchored `drawBuilding` / `buildingArt`, swapped per phase |
| Contribute panel in both clients | the Concession buyer panel + `OfferDraft` composer |
| Fan-out on change | `EventPlaced`-style broadcast via `broadcastToArea`/drop-oldest delivery |
| Per-player digest | the `events` table (`LogEvent`) + machine `Settle` "while you were away" |

New surface, kept minimal:

```go
// internal/world/project.go (new) — sibling to the gate pool, one mutex
type Project struct {
    ID         string         // "all-hands-hall"
    Phase      int            // current stage
    Pool       map[string]int // resource id -> amount banked toward this phase
    Done       bool
}
// Atomic contribute, mirroring OfferToGate/MutatePlacement:
func (w *World) ContributeToProject(id, item string, n int) (Project, bool)
```

- **Store:** add `SaveProject`/`LoadProjects` and a `project_contributions`
  ledger (player, item, count) — direct copies of the `SaveGateWorld` shape.
  Add one events reader for the digest (`EventsSince(unix int64)`), the only
  genuinely new query.
- **Events:** `EventProjectContributed` / `EventProjectAdvanced`, fanned out
  with the existing machinery. Discrete and rare (a contribution, a phase flip),
  never per-tick — no fan-out pressure.
- **Rewards:** a phase-complete hook flips a shared unlock (start with: a new
  recipe + the plaque). Persisted globally like the gate's fixed flag; gating a
  recipe on it reuses the same "is this unlocked?" predicate shape as hats.
- **Determinism:** the project *site* stays pure-seed; its *state* is one more
  sparse stored layer, the same documented exception as the gate pool,
  placements, claims and cleared cells. Terrain never drifts.

## Pillar check

| Pillar | Community projects? |
| --- | --- |
| Works from any terminal | ✅ contribute panel + multi-tile sprite render in both clients (settlement-building + Concession-panel precedents) |
| SSH username is identity | ✅ contributions are credited and persisted by username; the plaque names them |
| Shared, real-time, eventually consistent | ✅ project state fans out as discrete events like the gate pool; the digest covers what you missed offline |
| Persist between visits, not during | ✅ **a communal build is the purest expression of this** — it *is* the between-visits artifact; the digest is built on it |
| Seeded terrain; bounded live layer | ✅ the site is pure-seed; project state is one more sparse stored layer, same precedent as the gate pool / placements / claims |

## First milestone — the shippable slice

The smallest thing that makes the loop *land somewhere*:

1. **`internal/world/project.go`** — the shared project struct, `LoadProjects`
   at boot, and `ContributeToProject` settled atomically (a test fires N
   concurrent contributors and asserts the pool is exact, never over- or
   double-counted — mirroring the stall-transaction test). **Foundations first.**
2. **One project, `all-hands-hall`**, on a pure-seed clearing near the hub, with
   3 phases costed from existing resources.
3. **Site rendering** — a bottom-anchored multi-tile sprite that swaps per phase
   (reuse `drawBuilding`/`buildingArt`), in both clients; `e`-to-contribute plus
   a contribute panel modeled on the Concession buyer panel.
4. **One milestone reward** — phase completion unlocks a new recipe and writes
   the builders' plaque; both persisted and shared.
5. **"While you were away" digest** — `EventsSince(last_seen)` summarized into
   the entry panel (project moves first, then nearby builds/sales).
6. **Tests** — atomic concurrent contribution, persistence/resume across
   restart, phase advance + reward unlock, deterministic site placement and
   reachability, and a graceful empty digest.

Slice in one sentence: *the company pools its crafted goods into a hall that
rises in the Wilds while people come and go, and everyone who logs in sees how
far it got and who built it.*

## Risks & guardrails

- **Stalls forever with too few players** → keep phase costs small and
  achievable solo over a few sessions; let **machine output** and passive yields
  feed contributions, so the build advances even on a quiet week. Tune costs
  conservative; raise only after watching real use.
- **Griefing** → contribution is **additive-only and capped per phase**; there's
  no subtract, no sabotage, and the structure is owner-less, so no claim
  conflict. The worst case is over-eagerness, which the per-phase cap absorbs.
- **Determinism feel** → the *site* is seeded and identical for everyone; only
  the build state is stored, the established exception. Documented in GAME.md's
  pillar 5.
- **Lock / fan-out pressure** → contributions and phase flips are discrete and
  rare (not per-tick); they reuse drop-oldest delivery. The digest is one query
  at join, off the hot path.
- **Reward design creep** → ship with the cheapest meaningful reward (a recipe +
  the plaque). Bigger payoffs (a new area, cosmetics, regional projects) are
  backlog, not the slice.

## Future tasks (backlog)

Deferred-by-design extensions, roughly by value-to-effort.

- [ ] **Rotating / regional projects** — once one works, projects per settlement
  so different groups rally different builds.
- [ ] **A project that opens a new area** — the ultimate reward: the finished
  Hall is itself a portal into a community-only room.
- [ ] **Seasonal goals** — projects tied to the day/night clock or the real
  calendar (a harvest drive), giving a cadence to return for.
- [ ] **Richer digest** — a scrollable "frontier ledger" beyond the entry toast;
  per-colleague activity, your own contribution history.
- [ ] **Contribution from machines/stalls directly** — designate a machine's
  output or a stall's till to auto-feed a project while you're away.
- [ ] **Leaderboard / Cap Table view** — who's built the most, shown in-world on
  the plaque, surfaced via `/founders` or the Tab menu.
</content>

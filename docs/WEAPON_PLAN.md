# Weapon Plan — Arms, the Hunt, and Honourable Combat

> **Status:** ✅ Phases 1–3 shipped (wildlife having landed in `main`, #26).
> Players now have HP and an atomic damage path (`MutatePlayer`/`Strike`/
> `Respawn` + auto-respawn on the tick); a craftable weapon catalog
> (knife/spear/bow + arrows) wielded by ownership; and one unified `f` strike
> that hunts animals anywhere and, **only out in the open Wilds** (the hub and
> claimed homesteads stay sanctuaries), turns on other players — a cozy
> knock-out with no item loss, reviving at Durst HQ. A bottom-line HP bar and
> downed banner show in both clients via the shared `Hint`.
>
> **Phase 4 polish shipped:** victim-side feedback (you're told who hit you and
> with what, even though you didn't act), a red on-hit screen-rim flash in HD
> (`pixel.TintEdges`/`DrawHurtFlash`), post-respawn immunity (`RespawnImmunity`,
> no spawn-camping) and per-weapon strike cooldowns, a discoverable "f — strike
> <name>" prompt, bespoke compendium icons for all four arms, and the
> **`/wield`** (choose your arm, incl. `auto`/`fists`) and **`/pvp`** (am I safe
> here?) commands, sharing one `PvPAllowedAt` truth with the strike action.
>
> **Arsenal expansion shipped:** the roster now spans three classes
> (`Weapon.Found`/`Weapon.Unique`):
> - **Craftable:** Flint Knife, Spear, Hunter's Bow (+Arrows), and the new
>   **Cast Blade** (from ingots).
> - **Found (hidden):** the **Sling** (flings gathered stones, ranged) and the
>   **Bone Dagger** — turned up by exploring the biome scatter, no recipe.
> - **Legends (unique, one per world):** **Durstbane** (a 6-damage blade) and
>   **Skypiercer** (a long-range legendary bow). Each hides at a deterministic,
>   biome-themed spot far from the hub, glows so a distant shimmer draws you in,
>   and is claimed **once** — the world's `artifact` registry settles a finders'
>   race atomically (first-write-wins, persisted in the `artifacts` table). Once
>   claimed it never reappears; it's obtainable only by trade thereafter (it's a
>   normal pack item, so the stall composer and direct trade already move it).
>   `/legends` lists who holds each one, or rumors where an unclaimed one lies.
> Adds a `Legendary` rarity tag. Tests cover the registry race, persistence,
> placement standability, and the once-per-world claim.
>
> **Weapon-in-hand sprites shipped:** your wielded arm now draws on the HD
> avatar — `world.Player.Weapon` carries it (synced each frame from the
> renderer, broadcast like style/accessory), `AvatarBitmap` overlays a per-shape
> weapon sprite (blade / short blade / polearm / bow / sling, with glowing
> variants for the two legends) the same way a hat overlays, and `spritePixel`
> gained steel/haft/glow codes. As with hats, this is HD-only — the glyph client
> draws each player as a single name token, so there's no sprite to hang a
> weapon on there.
>
> **Special abilities shipped:** weapons now carry abilities, not just stats —
> the **Spear** knocks its target back a tile (creatures move directly; players
> get an `EventPlayerShoved` their own client applies, respecting position
> authority); the **Cast Blade** and **Durstbane** *cleave* every foe around the
> target; **Skypiercer** *pierces* every foe along its line; and the **Bone
> Dagger** lands a *backstab* bonus from behind. The strike resolves a target
> set (one for a plain blow, many for cleave/pierce) and a per-target damage with
> the backstab bonus. Tests cover all four.
>
> **Arms compendium section shipped:** weapons (and arrows) now group under a
> dedicated "Arms" heading in the codex via a new `Arms` item source, instead of
> being scattered across Forage/Crafted.
>
> **Companion defense shipped:** a tamed pet now stands up for you. `Strike`
> records who hurt a player (`Player.LastHurtBy`); the wildlife sim then has the
> owner's companion break from heel, pursue that attacker, and bite on a cadence
> (credited to the owner, gentle damage scaled by the animal's size) — only out
> in the open Wilds, never in the hub or on a claim, and it stands down once the
> threat leaves or is downed. The whole weapon system is now feature-complete
> against this plan.

> How arms land on the cozy-frontier foundation
> ([`DESIGN_MECHANICS.md`](DESIGN_MECHANICS.md),
> [`WILDLIFE_PLAN.md`](WILDLIFE_PLAN.md)), reusing the creature HP model, the
> recipe/inventory plumbing, and the area/event machinery already proven by
> wildlife and trade. Voice is corporate × medieval.

## The decision this plan acts on

`DESIGN_MECHANICS.md` parks "animals / weapons / enemies / PvP" behind a *future
genre decision*, and explicitly says hunting can land "with **no combat, no
enemies, no mob AI** … True enemies and PvP stay behind a future genre
decision." **That decision is now made:** we are unparking *light* danger —
weapons, a shared damage model, and **consensual, zone-gated PvP** — while
keeping the cozy spine intact. The guardrails below are what keep "danger" light.

Three product calls frame everything that follows:

1. **Foundation — wait for the merge.** Build on `main` *after* wildlife lands,
   so one damage model serves both creatures and players. No parallel combat code.
2. **PvP is zone-based.** The hub, settlements, the lobby, and all non-Wilds
   areas are **safe** — strikes against players are refused there. PvP is live
   only in the open **Wilds** (and later, optionally, the caves). You always
   know whether you can be hit.
3. **Defeat is a cozy knock-out.** A downed player is stunned briefly and
   **respawns at the hub with their pack intact** — no item loss, no death
   penalty. Combat is sport and predator-defense, not punishment.

## The shape: one strike, two targets, gated by zone

Today's wildlife `hunt()` already is a combat action — it just only accepts
animals. Weapons generalize it: **the same strike resolves against whatever is
adjacent in the facing direction** — a creature *or* a player — through one
damage path. What differs is *gating*, not mechanics:

| Target | Where it works | Resolved by |
| --- | --- | --- |
| Wild creature | anywhere creatures spawn (Wilds) | `MutateCreature` (exists) |
| Player | **PvP zones only** (Wilds) | `MutatePlayer` (new, mirrors it) |
| Tamed companion | PvP zones only; cozy knock-out, returns later | `MutateCreature` |

A weapon is the multiplier on that strike: bare hands deal 1, a knife more, a
bow lets you strike at range. The animal HP/`Drops` tables from wildlife are
untouched; we add an HP model to players and a damage value to the strike.

## Piece 1 — a damage model on the Player

Players today are pure appearance + position
(`internal/world/world.go` `Player`: `Name, Area, X, Y, Color, Facing, Style,
Accessory, LastMoved`). Combat needs them to take and recover damage, mirroring
the creature fields wildlife already added:

```go
// internal/world/world.go — added to Player
HP      int       // current health; MaxHP when full
MaxHP   int       // cap (starts at a flat 10; gear/food can lift it later)
DownedUntil time.Time // > now ⇒ knocked out, immune, awaiting respawn
LastHurt    time.Time // gates passive regen and the "in combat" UI flag
```

HP is **live session state, never persisted** — you always reconnect at full
health (respawn-on-login is the cozy default; the store stays combat-free). The
world stays schema-agnostic exactly as it does for creatures: it stores and
guards the fields under the one mutex; the *game* layer owns what they mean.

New world surface, the twin of `MutateCreature`:

```go
// internal/world/player.go (new), guarded by w.mu
func (w *World) MutatePlayer(name string, fn func(*Player) bool) bool
```

This is the race-safe primitive that makes a strike atomic — two attackers on
one victim can't drive HP below zero twice or both claim the knock-out, the same
discipline that stops double-loot in `hunt()`.

## Piece 2 — weapons as wieldable items

Weapons reuse the **tool-wield pattern already shipped for `axe`/`pick`**
(`internal/game/placeable.go`, `recipes.go`): you *craft* the item, and owning
it means it's "wielded" — no separate equip step needed for MVP. We add a small
static catalog beside `Species` and `Recipe`:

```go
// internal/game/weapon.go (new)
type Weapon struct {
    Item     string // inventory item id (also the recipe output)
    Name     string
    Damage   int    // HP removed per strike
    Reach    int    // 1 = melee (adjacent); >1 = ranged (line-of-tiles)
    Cooldown int    // ticks between strikes (throttles spam / griefing)
    Ammo     string // "" for melee; an item id consumed per shot (e.g. arrow)
}

var weapons = []Weapon{
    // bare hands are the implicit Damage:1, Reach:1, no entry needed
    {Item: "knife", Name: "Flint Knife", Damage: 2, Reach: 1, Cooldown: 1},
    {Item: "spear", Name: "Spear",       Damage: 3, Reach: 1, Cooldown: 2},
    {Item: "bow",   Name: "Hunter's Bow", Damage: 2, Reach: 4, Cooldown: 2, Ammo: "arrow"},
}
```

Crafted through the existing recipe table — gated, like the axe, by a found head
or a refined input so the recipe "unlocks" when you turn the materials up:

```go
// internal/game/recipes.go — appended
{ID: "knife", Name: "Flint Knife", In: []Ingredient{{"flint",1},{"wood",1}}, Out: "knife", OutN: 1},
{ID: "spear", Name: "Spear",       In: []Ingredient{{"flint",1},{"wood",3}}, Out: "spear", OutN: 1},
{ID: "bow",   Name: "Hunter's Bow",In: []Ingredient{{"wood",2},{"hide",1},{"sinew",1}}, Out: "bow", OutN: 1},
{ID: "arrow", Name: "Arrows",      In: []Ingredient{{"flint",1},{"feather",1}}, Out: "arrow", OutN: 4},
```

`hide`/`feather` already drop from wildlife; `flint`/`sinew` slot into the forage
/drop tables (a one-line addition each). **The currently-wielded weapon is the
best one in the pack** (highest `Damage`, ranged before melee on a tie) — zero
new UI to choose; matches how `axe`/`pick` "just work" when owned. A `/wield`
command and an inventory toggle can refine this later.

Each weapon needs an `item_icons.go` entry + an `inventory.go` catalog entry
(the doc/HUD/compendium read it) — the same two touch-points every existing item
has.

## Piece 3 — the unified strike action

Generalize wildlife's `f`-to-hunt into one **strike** the Wilds area dispatches
(`internal/areas/wilds/wilds.go`, where `hunt()` already lives):

```
on 'f':
  w := bestWeapon(inventory)               // hands if none
  if w.Ammo != "" && inventory[w.Ammo]==0: toast "out of arrows"; return
  target := firstTargetInFacing(w.Reach)   // scan up to Reach tiles ahead
  switch target {
  case creature:  strikeCreature(target, w.Damage)   // = today's hunt(), parameterized
  case player:
        if !pvpAllowed(area): toast "this is a peaceful place"; return
        if target.downed:     toast "they're already down"; return
        if onCooldown():      return
        strikePlayer(target, w.Damage)
  default:        toast "nothing within reach"
  }
  if w.Ammo != "": spend one ammo
```

`strikePlayer` is `MutatePlayer` decrementing HP; at `HP<=0` it sets
`DownedUntil = now + downedDuration`, fires `EventPlayerDowned`, and schedules a
respawn (HP refilled, teleport to the hub spawn) when the timer lapses — handled
by the same 2 Hz tick loop that already steps creatures and the portal pulse. A
non-lethal hit fires `EventPlayerDamaged` so the victim's client reacts even
though it didn't act.

`strikeCreature` is the existing `hunt()` body with `cc.HP -= w.Damage` instead
of `cc.HP--` — a one-line change; drops/taming/compendium logic is untouched.

### Zone gating — where `pvpAllowed` comes from

Areas already self-identify by id (`lobby`, `wilds`, `cave`, `kraftwerk`, …). Add
a single boolean to the area surface (`internal/game/area.go`), defaulting
**false** (safe), and flip it true only for the Wilds:

```go
// Area interface gains:
PvP() bool   // may players damage each other here? default false (safe)
```

`pvpAllowed` also respects **settlement claims** inside the Wilds — a claimed
plot (`internal/world/claims.go`, already shared state) is a sanctuary, so your
homestead is safe even in the wild. This reuses the claim lookup Phase 9 added;
no new state. The result: hub, lobby, presentation wing, demo center, caves
(MVP), and any claimed land are all no-strike zones, enforced server-side in one
predicate — never trust the client.

## Piece 4 — making combat visible (both clients)

New events on `internal/world/events.go`, fanned out by the existing
`broadcastToArea`/`deliver` machinery (non-blocking, drop-oldest — a slow
session just gets eventual consistency, same as everything else):

```go
EventPlayerDamaged  // Target took a hit; Detail = attacker, X/Y for a hit-spark
EventPlayerDowned   // Target was knocked out
EventPlayerRespawn  // Target is back at the hub at full HP
```

UI work, mirrored in both renderers (the cozy contract: glyph + HD never drift):

- **Health bar** in the HUD — a small heart/pip row (HD) and an ASCII `HP ▓▓▓░░`
  (glyph). Only shown when hurt or in a PvP zone, so peaceful play stays quiet,
  matching the "deliberately quiet HUD" the README describes.
- **Wielded-weapon glyph** beside the HP, when one is in the pack.
- **Strike feedback** — a brief hit-spark at the target tile + a toast
  (`"you strike Markus — knocked out!"` / `"Anna's spear catches you"`), through
  the existing `setToast` path and the raster spark used for other one-shot FX.
- **Downed state** — the avatar drawn prone/greyed until respawn; a centered
  "Knocked out — back at the hub in 3…" banner for the downed player.
- **Controls** — `Controls()` (`internal/game/controls.go`) gains
  `{"f", "strike / hunt what you're facing"}` under **Act**, so it's
  discoverable in the `?` overlay of both clients. (`f` already means hunt on the
  wildlife branch — this just broadens its description.)

## Companions & balance

- **Companion combat (cozy):** a tamed pet (wildlife Phase 3, `Owner` set) can be
  caught in a PvP zone; it's *knocked out*, not killed — it stops following,
  then re-attaches when its owner returns to the Wilds (the reattach path
  already exists). Pets don't auto-fight at MVP; "companion defends you" is a
  parked stretch.
- **Anti-grief guardrails** (what keeps danger *light*):
  - Safe zones cover every social space; PvP is opt-in by *walking into the
    wild*, and your claimed land stays safe.
  - **Respawn immunity** — a few seconds invulnerable + can't strike after
    respawn, so no spawn-camping.
  - **Weapon cooldowns** throttle strike spam.
  - No item loss on defeat, so there's nothing to farm by ganking.
  - A `/pvp` status line tells you plainly whether you can be hit right now.

## What this touches (grounded in real files)

| Concern | File(s) |
| --- | --- |
| Player HP fields + `MutatePlayer` | `internal/world/world.go`, `internal/world/player.go` (new) |
| Combat events | `internal/world/events.go` |
| Respawn / downed timers on the tick | `internal/world/world.go` (tick loop), `cmd/durstworld/main.go` |
| Weapon catalog | `internal/game/weapon.go` (new), `internal/game/inventory.go`, `internal/game/item_icons.go` |
| Weapon recipes + new mats | `internal/game/recipes.go` |
| Unified strike + zone gate | `internal/areas/wilds/wilds.go` (extend `hunt()`), `internal/game/area.go` (`PvP()`) |
| Claim-sanctuary check | `internal/world/claims.go` (read existing claims) |
| HUD: HP bar, weapon, downed banner | `internal/game/hd_ui.go` (HD), glyph HUD, `internal/game/render.go` |
| Controls + help | `internal/game/controls.go` |

## Phased rollout (each phase shippable, tested, on-tone)

- **Phase 0 — merge gate.** Land `claude/wildlife-game-plan-1qbybr` in `main`,
  reconcile with Phase 9, branch the weapon work from the result. *(Blocking.)*
- **Phase 1 — the damage model.** Player `HP`/`MaxHP`/downed fields,
  `MutatePlayer`, passive regen, respawn-at-hub on the tick. No weapons yet:
  prove HP + knock-out + respawn with a debug strike. HUD HP bar in both clients.
  *Tests:* `MutatePlayer` atomicity, HP floor at 0, respawn restores full HP and
  hub position, downed immunity window.
- **Phase 2 — weapons & the unified strike.** `Weapon` catalog + recipes,
  `bestWeapon`, generalize `hunt()` into `strike` over creatures (damage scales
  with weapon), ranged reach + ammo. *Tests:* damage application, reach scan,
  ammo spend, best-weapon selection, drop tables unchanged.
- **Phase 3 — zone-based PvP.** `Area.PvP()`, claim-sanctuary check,
  `strikePlayer`, combat events, victim-side UI (spark, toast, downed banner),
  anti-grief guardrails. *Tests:* strike refused in safe zones + claimed land,
  allowed in open Wilds, downed→respawn cycle, no item loss, respawn immunity.
- **Phase 4 — polish.** `/wield` + `/pvp` commands, hit-spark FX tuning,
  compendium "Arms" section, optional companion-defends stretch, balance pass on
  HP/damage/cooldowns.

## Non-goals (keeping it light)

No mob AI / hostile enemies, no permadeath, no full-loot, no global PvP, no
HP persistence, no levels/stats grind. Weapons are a multiplier on one legible
strike; danger is a place you choose to walk into, not a state forced on you.

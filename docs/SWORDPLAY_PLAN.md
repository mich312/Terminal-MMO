# Swordplay Plan — A Witcher-Style Action Camera and Duels Worth Watching

> **Status:** ✅ Phases 1–2 shipped. The over-the-shoulder action camera is
> the browser client's default view (`V` drops to the top-down overview and
> back; pointer-lock mouse-look, camera-relative WASD, `face:N` aiming); every player is now an articulated, procedurally-animated
> rig with their wielded weapon in hand; and the fencer's verb set is live —
> fast strike (click), strong strike (hold, `F`), dodge with server-side
> i-frames (Space), guard/parry/riposte (hold right mouse) with the attacker
> stagger, and lock-on (`Q`). Swings broadcast via `EventPlayerActed`, whiffs
> included, so duels read from every client. All referee rulings
> (`world.StrikeOutcome`) resolve under the world mutex; protocol bumped to v2.
> Phases 3–4 (blocking parity in terminal HUDs, `/duel`, the Fight Pit, sound)
> remain open.
>
> How the browser client grows a third-person action mode — your character a
> real 3D figure in front of the camera, Witcher-style, with fast and strong
> attacks, dodges, parries and target lock — on top of the PvP sword combat the
> weapon system already shipped. Grounded in
> [`WEAPON_PLAN.md`](WEAPON_PLAN.md) (shipped, feature-complete) and
> [`BROWSER.md`](BROWSER.md) (the top-down 3D client).

## The one-sentence finding from reading the code

**The sword fight already exists — server-side, tested, and shipped. What's
missing is the camera, the body, and the *choreography*.**

The audit that led here:

| "The Witcher, but multiplayer, with PvP sword duels" needs… | Already in the codebase? |
| --- | --- |
| A shared, live 3D world | ✅ `internal/web` — three.js client, instanced geometry, day/night, shadows, fog |
| Multiplayer presence, chat, trade | ✅ `internal/world` — one mutex, event fan-out, proven by three clients |
| Player HP, damage, knock-out, respawn | ✅ `world.Player.HP/MaxHP/DownedUntil`, `MutatePlayer`/`Strike`/`Respawn` (`internal/world/player.go`) |
| Swords | ✅ `internal/game/weapon.go` — Flint Knife, Spear, Cast Blade, Bone Dagger, the legendary **Durstbane**; damage/reach/cooldown per arm |
| Special moves | ✅ cleave, pierce, backstab, knockback — shipped weapon abilities |
| PvP rules | ✅ zone-gated (`PvPAllowedAt`): open Wilds only, hub/claims are sanctuaries; respawn immunity; no item loss |
| The strike action | ✅ `internal/areas/wilds/wilds.go` `strike()` — facing-based, reach-aware, cooldown-throttled, server-authoritative |
| An immunity window on the victim | ✅ `World.Immune` (respawn protection) — the hook dodge i-frames extend |
| Forced movement with position authority | ✅ `World.Shove` + `EventPlayerShoved` — the hook a parry-stagger reuses |
| Combat visibility | ✅ `EventPlayerDamaged/Downed/Respawn/Shoved` broadcast to every client |
| A third-person action camera | ❌ the browser camera is a fixed 52° top-down 3/4 follow-cam (`scene.js`) |
| A hero who *looks* like one | ❌ the avatar is a capsule, a head and a nose-wedge (`actors.js`) — no limbs, no sword in hand, no animations |
| Fast/strong attacks, dodge, parry, lock-on | ❌ one strike verb on a cooldown; no defensive verbs at all |

So this plan is **not** "build a sword-fighting game". It is "give the existing
sword-fighting game an action-game presentation and a fencer's verb set" —
exactly the move the browser client itself made ("a third renderer, not a
second game"). The action mode is **a fourth camera, not a fourth world.** SSH
players, top-down players and shoulder-cam players all stand in the same
Wilds; a duel can be fought between a browser swashbuckler and a
glyph-terminal archer, and the server neither knows nor cares.

That framing is also the anti-cheat story: reach, cooldown, damage, dodge
windows, parry timing, zone-gating and the knock-out all resolve under the
world mutex. A hacked client can render itself a longer sword; it cannot swing
one, and it cannot dodge a blow the server says landed.

## The camera: over the shoulder, hero in frame

`V` toggles between views (mnemonic: *view*; `V` is unbound today).

- **Top-down** stays the default and stays canonical — the better client for
  building, trading, and reading a village.
- **Action camera** locks the pointer, drops behind and slightly above your
  character — offset a little to the right so the hero frames the left third
  of the screen, Witcher/Souls-standard — at roughly 4–6 world units back,
  wheel-zoomable. Mouse orbits the camera freely around the character.

The load-bearing Witcher convention: **the character does not face the
camera's way — they face the way they *move*** (until lock-on, below). Push
`W` and the hero turns and runs away from the camera; pull `S` and they run
toward it. The camera is a chase-cam with lag (`scene.js` already lags the
top-down follow target for exactly this weight-feel), the character is a
creature in the world, and you watch them fight rather than looking out of
their skull. This sidesteps first-person's biggest costs — no viewmodel arms,
no motion discomfort — and it makes *other people's* fights just as watchable
as your own, which matters in a multiplayer world with spectators.

Everything else in the client — tile cache, instanced geometry, DOM panels,
chat, palette — is untouched. Fog distances are world-unit math and work at
any camera angle; the Wilds' discovery circle becomes a wall of darkness you
walk toward. `scene.js` grows a `mode` and a second update path, nothing more.

### Movement and facing on the grid

Movement in action mode is camera-relative: `WASD` picks a direction relative
to the camera yaw, the client quantizes it to the world's 8-way grid and sends
the same movement keys the terminal clients send, at the same server-floored
10 steps/s. The character model turns smoothly toward its actual movement
(the `actors.js` turn-the-short-way-round easing, already shipped).

Facing is the one place the wire grows: strikes resolve along `world.Dir`,
which today only changes as a side effect of moving (`world.go`:
`p.Facing = Facing8(dx, dy)`). Lock-on and stationary swings need **face
without stepping**:

- Protocol: `{t:"key", key:"face:3"}` (0–7, `world.Dir` order), riding the
  existing `CmdKey` channel; one new case in `session.go`'s whitelist,
  rate-limited like movement.
- World: `SetFacing(name, dir)` under the mutex, broadcast like a move so
  every client turns your avatar.

The world stays a tile grid — that's load-bearing (collision, chat proximity,
strike reach, stateless worldgen). What makes it feel continuous is already
half-built: actors glide 110ms/step with ease-out, and the camera rides the
*interpolated* position. The grid's coarseness is real — footwork is 8-way and
stepwise — so the fight's finesse has to live in the *verbs and their timing*,
which is the next section, and in the dodge, which is the burst movement the
grid lacks.

## The verb set: fencing, not spam

The Witcher's melee is legible because every exchange is built from a small
set of readable verbs. Here is that set, each mapped to what the server
actually does. The design rule throughout: **the client animates immediately,
the server resolves authoritatively** — same as movement today (client paces,
server floors it).

### Fast attack — left mouse

The shipped strike, as-is: wielded weapon's damage, per-weapon cooldown,
reach, abilities. A quick diagonal slash animation, alternating left/right on
consecutive swings so a flurry reads as a combo rather than a loop. Combo
chaining is **pure presentation** — the server just sees strikes arriving at
cooldown pace; the client strings the animations together seamlessly when they
come inside a chain window.

### Strong attack — hold left mouse (or right mouse when not guarding)

The first new server-side combat verb, and a small one: a strike variant.

- Wire: the strike key gains a kind — `f` stays the fast attack, `F` (shift,
  matching the run convention) is the strong one.
- Server: `Weapon` gains nothing; the strike applies a global multiplier —
  ~1.8× damage, ~2.5× cooldown — so every weapon keeps its identity and the
  balance surface stays two numbers. A strong attack **breaks a guard**
  (below), which is what makes it a real choice rather than a bigger number.
- Client: a wind-up (the hold), an overhead or spinning heavy swing, a longer
  recovery. The wind-up is visible to the opponent via the swing broadcast
  (below) — telegraphing is the point.

### Dodge — spacebar + a direction

The Witcher's sidestep-hop, and the burst movement the 10/s grid lacks.

- Server: a new action — hop **two tiles** in the pressed direction if both
  are walkable (else one, else nothing), with a ~300ms **i-frame window**
  (`DodgeUntil` on the player; `World.Strike` already consults an immunity
  check for respawn protection — `Immune` grows one more clause) and a
  ~800ms cooldown so it's a read, not a hum. Under the mutex, broadcast as a
  normal move plus a flag so clients play a roll rather than a glide.
- Client: a dive-roll animation covering the two-tile glide, camera briefly
  loosening its lag so the hop reads as explosive.
- Why server-side: i-frames are a combat outcome. A client-side dodge would
  be a cosmetic that desynced from the authoritative hit resolution — the one
  sin this architecture never commits.

### Guard and parry — hold right mouse

The mechanic that turns strike-trading into fencing.

- **Guard:** while held, a `Guarding` flag (+`GuardStart` timestamp) on the
  player; `Strike` halves incoming damage. The character raises their blade —
  a stance visible to the attacker, so guard vs. guard-break becomes a
  mind-game.
- **Parry:** a blow that lands within ~250ms of `GuardStart` is parried — no
  damage, a steel-on-steel spark, and the **attacker staggers**: shoved back
  one tile (reusing `World.Shove` + `EventPlayerShoved`, position authority
  already solved) and their strike cooldown restarted. The parrier gets a
  ~1s **riposte window** during which their next strike carries a bonus —
  the backstab-bonus plumbing (`weaponDamage` in `wilds.go`) shows exactly
  where such a conditional multiplier lives.
- **Guard break:** a *strong* attack against a standing guard staggers the
  *defender* instead. Rock-paper-scissors closes: fast beats nothing, guard
  beats fast, strong beats guard, dodge beats strong.
- Timing note: 250ms parry windows over a WebSocket are honest because both
  timestamps (guard start, blow landing) are taken **server-side** under the
  mutex — latency shifts your inputs, but never the referee's clock.

### Target lock — middle mouse / `Q`

Almost entirely client-side. Lock snaps to the nearest player or creature in
front of the camera; the camera keeps both fighters in frame (the classic
lock-on framing), the character now **strafes** — facing the target while
WASD circles around them — and the client keeps the server's facing pointed
at the target via `face:N` as either fighter moves. Strikes while locked
always face the right tile, which on an 8-way grid is the difference between
combat feeling precise and feeling like a slot machine. A lock indicator
floats over the target; lock breaks on distance, knock-out, or re-press.

### What the opponent sees

Two gaps make today's fights illegible, both worth fixing for the top-down
view too:

1. **Swings are invisible unless they land.** Only damage events broadcast; a
   whiff shows nothing. Add `EventPlayerStruck` (attacker, facing, kind:
   fast/strong/parried) fired by `strike()` regardless of outcome; clients
   play the matching animation on the attacker's avatar. Small event, big
   consequence: you can now watch a duel, bait a whiff, read a wind-up, and
   time your dodge — the entire skill loop runs through this one event.
2. **Weapon-in-hand.** `actors.js` ignores `Actor.Weapon` (HD sprites got
   this; the browser didn't). Every avatar gets a held-weapon mesh per class —
   blade, short blade, polearm, bow, sling; glowing for the legends — so an
   armed stranger approaching you in the Wilds *looks* armed.

## The hero: a body worth putting in frame

"The player is a 3D model in front of the camera" is a renderer demand: the
capsule-and-head avatar that reads perfectly from 52° above does not hold up
as a protagonist. The upgrade stays inside the client's own idiom —
primitives, no asset files, no skeletal-animation formats, `go:embed`-friendly:

- **An articulated figure** built from the same primitive vocabulary as
  `props.js`: torso, head, two arms (upper/fore, a hand holding the weapon),
  two legs, all parented into a hierarchy so joints can rotate. Still low-poly,
  still tinted by the player's chosen color, hats still attach. Maybe a cloak
  quad riding the existing wind shader, because a duel at dusk deserves one.
- **Procedural animation, not clips:** an idle sway, a walk/run cycle driven
  by the actor's existing glide progress (feet planted on the grid steps),
  the two swing arcs, guard stance, dodge roll, hit flinch, and the knockout
  crumple (upgrading today's whole-body tip-over). Each is a handful of
  sine/ease curves on joint rotations — the same school as the wind shader:
  cheap, deterministic, tunable in code.
- **One rig for everyone.** The same articulated body serves top-down and
  action mode, every player, both cameras — no second avatar system. Top-down
  players get better-looking fights for free.

## Making it Witcher-shaped (the brainstorm shelf)

Ordered by value-for-effort; each stands alone.

- **Duels anywhere: `/duel <name>`.** Consensual PvP outside the Wilds: both
  parties agree (reuse the trade-request handshake in `world/trade.go`), a
  countdown, first knock-out wins; `PvPAllowedAt` gains an active-duel clause.
  Honourable combat in the office lobby — corporate × medieval at its best.
- **The Fight Pit.** A `worldgen.Landmark` arena in the Wilds — a ring of
  standing stones, always-on PvP inside, spectator berm around it. A normal
  self-registered area with `PvP() = true`. Later: a leaderboard off the
  SQLite event log (knock-outs are already events), `/champions`.
- **Sound.** The client has no audio at all. Even four sounds — whoosh, the
  steel *clang* of a parry, a landed thud, the knock-out bell — would double
  the fight. WebAudio, synthesized or tiny embedded samples.
- **Weapon-ability theatrics.** Cleave (Cast Blade, Durstbane) becomes a
  spinning sweep, pierce (Skypiercer) a drawn-and-loosed power shot, the
  spear's knockback a visible lunge — the abilities are shipped; they've just
  never been *seen*.
- **Signs, Durst-flavored.** The Witcher's one-button spells could someday
  map to found artifacts (a ward = timed guard, a push = the shove plumbing).
  Parked: new balance surface, and the cozy contract says danger stays light.
- **Stamina, adrenaline, oils, mutagens, gear stats.** All parked, same
  reason. The verb set above is the skill ceiling; depth can wait for demand.
- **First-person as a third view.** `V` could cycle top-down → shoulder → FP
  later; the shoulder cam builds everything FP would need except viewmodel
  arms. Bonus if it ever lands: a native browser rendering for the Doom
  cabinet, closing the one gap `BROWSER.md` admits to.

## What this touches (grounded in real files)

| Concern | File(s) | Size of change |
| --- | --- | --- |
| Shoulder camera, pointer lock, orbit, lock-on framing | `internal/web/static/js/scene.js` | the big one, client-only |
| Camera-relative movement, `V`, mouse buttons, dodge/guard keys | `internal/web/static/js/input.js` | moderate |
| Articulated avatar + procedural animation set | `internal/web/static/js/actors.js` (+ new `rig.js`) | the other big one, client-only |
| Held-weapon meshes per class | `internal/web/static/js/props.js` | small |
| Lock indicator, hurt vignette, KO sequence | `internal/web/static/js/ui.js` + CSS | small |
| `face:N`, `dodge:N`, guard up/down, strong-strike key | `internal/web/protocol.go`, `internal/web/session.go` | tiny |
| `SetFacing`, `Dodge` (+i-frames in `Immune`), `Guarding`/parry/riposte in `Strike` | `internal/world/world.go`, `internal/world/player.go` | small but the heart of it |
| Strong-attack multiplier, stagger via `Shove`, `EventPlayerStruck` | `internal/areas/wilds/wilds.go` (`strike()`), `internal/world/events.go` | small |
| Guard/parry surfaced in terminal HUDs (parity) | `internal/game/hd_ui.go`, glyph HUD, `internal/game/controls.go` | small |
| `/duel` (Phase 3) | chat commands, `internal/game/weapon.go` (`PvPAllowedAt`) | moderate |
| Fight Pit (Phase 3) | `internal/worldgen` landmark + new area package | isolated |

The shape: the two big efforts (camera, rig) are pure client JavaScript. The
Go changes are small, sit exactly where the shipped combat already lives, and
— because the verbs are *world* verbs, not browser verbs — terminal players
get dodge, guard, parry and strong attacks too. The cozy contract holds:
glyph, HD and browser never drift.

## Phased rollout (each phase shippable, tested, on-tone)

- **Phase 1 — the camera and the body.** `V` toggle, pointer-lock orbit
  chase-cam, camera-relative movement, `face:N` + `SetFacing`, the articulated
  rig with idle/walk/run and the existing fast attack animated, held weapons
  on every avatar, `EventPlayerStruck` for visible swings. Playable, watchable
  PvP at the end of this phase — with one attack verb.
  *Tests:* `SetFacing` under the mutex + broadcast; session whitelist accepts
  `face:0..7`, rejects junk; struck event fires on whiff and hit; weapon-mesh
  mapping covers the catalog (mirror `TestEveryPropHasAShape`).
- **Phase 2 — the fencer's verbs.** Strong attack (multiplier, guard-break),
  dodge with server i-frames, guard/parry/riposte with the stagger, lock-on,
  hit flinch + KO crumple, the rock-paper-scissors complete.
  *Tests:* dodge i-frames block a strike mid-window and not after; parry
  window judged on server clocks; riposte bonus applies once; guard halves;
  strong breaks guard; dodge cooldown enforced; all under `MutatePlayer`
  atomicity.
- **Phase 3 — the duel as an institution.** `/duel` handshake, the Fight Pit,
  spectator framing, leaderboard.
  *Tests:* duel offer/accept/timeout mirrors the trade tests; `PvPAllowedAt`
  honors an active duel and the Pit.
- **Phase 4 — polish.** Sound, animation tuning, ability theatrics,
  `/champions`, and whatever the first hundred duels teach.

## Non-goals (keeping the cozy spine)

- **No sub-tile physics.** The grid stays authoritative for position,
  collision and reach; dodge is a grid hop, not a physics impulse. The whole
  reason this plan is small.
- **No client-authoritative combat.** Every verb resolves under the world
  mutex; the client animates hopefully and the server referees.
- **No browser-only gameplay.** Every verb is a world verb the terminal
  clients also get; the browser only *presents* it better.
- **No hostile mobs, no death penalty, no full-loot** — unchanged from
  `WEAPON_PLAN.md`. Defeat stays a knock-out and a walk home.
- **No stat/gear grind.** Skill expression lives in the verb timing, not in
  numbers earned elsewhere.

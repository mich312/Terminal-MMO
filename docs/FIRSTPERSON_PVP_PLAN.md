# First-Person Plan — Durst World Through Your Own Eyes

> **Status:** 📋 Brainstorm / design. Nothing here is built yet.
>
> How the browser client grows a first-person mode — walk the Wilds at eye
> level, Skyrim-style, and fight the PvP sword duels the weapon system already
> supports — without forking the game. Grounded in
> [`WEAPON_PLAN.md`](WEAPON_PLAN.md) (shipped, feature-complete) and
> [`BROWSER.md`](BROWSER.md) (the top-down 3D client).

## The one-sentence finding from reading the code

**The sword fight already exists — server-side, tested, and shipped. What's
missing is a camera.**

The audit that led here:

| "Skyrim, but multiplayer, with PvP sword fights" needs… | Already in the codebase? |
| --- | --- |
| A shared, live 3D world | ✅ `internal/web` — three.js client, instanced geometry, day/night, shadows, fog |
| Multiplayer presence, chat, trade | ✅ `internal/world` — one mutex, event fan-out, proven by three clients |
| Player HP, damage, knock-out, respawn | ✅ `world.Player.HP/MaxHP/DownedUntil`, `MutatePlayer`/`Strike`/`Respawn` (`internal/world/player.go`) |
| Swords | ✅ `internal/game/weapon.go` — Flint Knife, Spear, Cast Blade, Bone Dagger, the legendary **Durstbane**; damage/reach/cooldown per arm |
| Special moves | ✅ cleave, pierce, backstab, knockback — shipped weapon abilities |
| PvP rules | ✅ zone-gated (`PvPAllowedAt`): open Wilds only, hub/claims are sanctuaries; respawn immunity; no item loss |
| The strike action | ✅ `internal/areas/wilds/wilds.go` `strike()` — facing-based, reach-aware, cooldown-throttled, server-authoritative |
| Combat visibility | ✅ `EventPlayerDamaged/Downed/Respawn/Shoved` broadcast to every client |
| A first-person view | ❌ the browser camera is a fixed 52° top-down 3/4 follow-cam (`scene.js`) |
| A sword you can *see yourself swing* | ❌ weapons draw on the HD sprite avatar only; the 3D actor doesn't carry one |
| Mouse-look, click-to-swing | ❌ input is keyboard-grid only |

So this plan is **not** "build a sword-fighting game". It is "give the existing
sword-fighting game a first-person renderer" — exactly the move the browser
client itself made ("a third renderer, not a second game"). First person is a
**fourth camera, not a fourth world.** SSH players, top-down players and
first-person players all stand in the same Wilds; a duel can be fought between
a first-person browser knight and a glyph-terminal archer, and the server
neither knows nor cares.

That framing is also the anti-cheat story: the client only ever sends the keys
it already sends. Reach, cooldown, damage, zone-gating and the knock-out all
resolve under the world mutex, exactly as they do today. A hacked client can
render itself a longer sword; it cannot swing one.

## The camera is a mode, not a fork

`V` toggles between the two views (mnemonic: *view*; `V` is unbound today).

- **Top-down** stays the default and stays canonical — it's the better client
  for building, trading, and reading a village.
- **First-person** locks the pointer (browser Pointer Lock API), drops the
  camera to eye height (~0.85 world units, the avatar head's Y in
  `actors.js`), and hides your own body meshes. `Esc` (or `V`) returns.

Everything else in the client — the tile cache, instanced geometry, the DOM
panels, chat, the palette — is untouched. `scene.js`'s `WorldScene` grows a
`mode` and a second `update` path; the fog/lighting pipeline works unchanged
because fog distances are already computed from world units, not from the
camera angle. The discovery circle that reads as a fog ring from above becomes
a *wall of darkness you walk into* at eye level — free atmosphere, no new code.

### Look and facing

Mouse X spins a continuous camera yaw; mouse Y pitches within ±60°. But the
*world's* facing is 8-way (`world.Dir`), and strikes resolve along it. The
bridge: the client quantizes camera yaw to the nearest of the 8 facings and
tells the server when it changes. That needs the one genuinely new input in
this plan — **face without stepping** — because today facing only changes as a
side effect of movement (`world.go`: `p.Facing = Facing8(dx, dy)`).

- Protocol: `{t:"key", key:"face:3"}` (0–7, `world.Dir` order). It rides the
  existing `CmdKey` channel and the session whitelist in `session.go` gains one
  case, rate-limited like movement.
- World: a `SetFacing(name string, d Dir)` under the mutex, broadcast like any
  move so other clients turn your avatar. (Bonus: the top-down client and HD
  could later use it for a "turn in place" key; SSH clients are unaffected.)

Movement becomes camera-relative in FP: `W` steps toward the quantized look
direction, `A`/`D` sidestep, `S` backsteps — all mapped client-side onto the
same 8-way grid steps the server already floors to 10/s. No wire change; the
`input.js` mapping table just consults the camera before picking which key to
send. Diagonal facings make strafing feel right at no server cost.

### The grid, at eye level

The world stays a tile grid — that's load-bearing (collision, chat proximity,
strike reach, the stateless worldgen). What makes it *feel* continuous:

- The FP camera rides the **interpolated** self-actor position (`actors.js`
  already glides 110ms/step with ease-out) — the same trick that makes
  top-down movement read as walking, doubled in importance here.
- Head-bob synced to the existing walk-bob phase; a small FOV kick (+4°) while
  running (Shift), the classic sprint cue.
- Camera yaw is never snapped to the 8-way grid — only the *reported facing*
  is quantized. You look wherever you like; the grid only matters when you
  swing.

Sub-tile continuous movement is explicitly **parked** (see Non-goals). If it
ever happens it's a world-level project for all clients, not an FP feature.

## The sword in your hand

### Viewmodel (your own arms)

A first-person rig — right hand + wielded weapon — built from the same
primitive-geometry idiom as `props.js`, attached to the camera, drawn last.
The weapon shown is `Actor.Weapon`, which the wire already carries per player.
Per-class silhouettes matching the HD sprite vocabulary: blade, short blade,
polearm, bow (+ nocked arrow), sling; glowing variants for Durstbane and
Skypiercer, because a legendary should light your own hands.

- **Click (or `F`) = strike.** Left mouse sends the exact `f` the terminal
  players press. The client plays a ~250ms swing arc immediately (responsive),
  the server resolves the hit (authoritative). Per-weapon cooldown is mirrored
  client-side only to pace the *animation*; the server keeps enforcing the real
  one.
- **Bow:** click draws (a short hold animation), release sends `f`. Same key
  on the wire; the hold is presentation. Out-of-ammo toast already exists.
- A **crosshair dot** appears only in FP, and only warms to a reticle when the
  server's existing strike prompt (`"f — strike <name>"`) says a target is in
  reach — the prompt is already in the `Scene` frame, so target confirmation
  costs zero new wire.

### Other people's swords

Two gaps make fights hard to *read*, both worth fixing for the top-down view
too:

1. **Weapon-in-hand on the 3D avatar.** `actors.js` ignores `Actor.Weapon`
   today (HD sprites got this; the browser didn't). Add a small held-weapon
   mesh per class at the avatar's side. Now an armed stranger approaching you
   in the Wilds *looks* armed — which is half the PvP tension.
2. **Swings are invisible unless they land.** Only damage events broadcast; a
   whiffed swing shows nothing. Add `EventPlayerStruck` (attacker, facing)
   fired by `strike()` regardless of outcome; clients play the swing animation
   on the attacker's avatar. Small event, big legibility: you can now *watch*
   a duel, bait a whiff, and time your counter.

### Feeling the hit

All triggered by data already on the wire (`Scene.Hurt`, the combat events),
so nothing here needs the server:

- **Taking a hit:** red vignette pulse (the HD client's `DrawHurtFlash`,
  reborn as a screen-space effect), 150ms camera shake, and a **directional
  damage indicator** — the event names your attacker, whose position the actor
  list knows, so an arc shows *where it came from*. Skyrim-standard, and it
  matters: in FP you can be hit from behind.
- **Landing a hit:** tiny hit-stop (~60ms freeze of the swing), hit-spark at
  the target.
- **Knock-out, first person:** the camera drops to the ground and tips
  sideways, world desaturates, the existing "back at the hub in 3…" banner
  counts down, then fade-to-black into the hub respawn. The cozy defeat,
  staged like a cinematic.
- **Victory:** the winner's existing toast, plus the loser's body tip-over
  (already shipped in `actors.js`) seen from eye level.

## Making it Skyrim-shaped (the brainstorm shelf)

Ordered roughly by value-for-effort; everything below Phase 2 is optional
seasoning, and each item stands alone.

- **Blocking / parry.** Hold right mouse to raise your blade: a `guard`
  state on the player (world field + one event), server halves incoming
  damage while raised; a block within ~200ms of the blow is a *parry* that
  briefly staggers the attacker (reuse the shove/knockback plumbing). This is
  the one mechanic that turns strike-trading into *fencing* — footwork, reach,
  timing — and it's the first server-side combat change worth making.
- **Duels anywhere: `/duel <name>`.** Consensual PvP outside the Wilds:
  both parties agree (reuse the trade-request handshake pattern in
  `world/trade.go`), a countdown, first knock-out wins, no zone change needed
  — `PvPAllowedAt` gains an "active duel" clause. Honourable combat in the
  office lobby, corporate × medieval at its best.
- **The Fight Pit.** A `worldgen.Landmark` arena out in the Wilds — a ring of
  standing stones, always-on PvP inside, spectator berm around it. Areas
  self-register (`docs/AREAS.md`), so this is a normal area with a `PvP() =
  true`. Later: a leaderboard from the SQLite event log (knock-outs are
  already events), `/champions`.
- **Sound.** There is no audio anywhere in the client today. Even three
  sounds — swing whoosh, blade clash, the knock-out thump — would double the
  fight's impact. WebAudio, synthesized or tiny embedded samples
  (`go:embed` keeps the no-CDN property).
- **First-person everywhere.** FP isn't combat-only: walking the cathedral,
  the lobby, or a presentation stage at eye level is worth the toggle on its
  own. The minigame boards stay top-down (the client can force the mode per
  area) — except **Doom**, where the FP camera finally gives the raycaster
  cabinet a native browser rendering, closing the one known gap in
  `BROWSER.md`.
- **Stamina, lock-on, power attacks, armor.** Classic melee-game depth, all
  parked: each adds server state and balance surface, and the cozy contract
  (`DESIGN_MECHANICS.md`) says danger stays light. Revisit only if dueling
  becomes a scene.

## What this touches (grounded in real files)

| Concern | File(s) | Size of change |
| --- | --- | --- |
| FP camera mode, pointer lock, head-bob | `internal/web/static/js/scene.js` | the big one, client-only |
| Camera-relative movement, `V`, mouse buttons | `internal/web/static/js/input.js` | moderate |
| Viewmodel arms + weapon meshes | `internal/web/static/js/props.js` (new builders), new `viewmodel.js` | moderate, client-only |
| Weapon-in-hand + swing anim on avatars | `internal/web/static/js/actors.js` | small |
| Crosshair, vignette, damage arc, KO fade | `internal/web/static/js/ui.js` + CSS | small |
| `face:N` input + whitelist | `internal/web/protocol.go`, `internal/web/session.go` | tiny |
| `SetFacing` + broadcast | `internal/world/world.go` | tiny |
| `EventPlayerStruck` | `internal/world/events.go`, `internal/areas/wilds/wilds.go` (`strike()`) | tiny |
| Blocking/parry (Phase 3) | `internal/world/player.go`, `internal/areas/wilds/wilds.go`, both terminal HUDs | moderate, cross-client |
| `/duel` (Phase 3) | `internal/game/chat commands`, `internal/game/weapon.go` (`PvPAllowedAt`) | moderate |
| Fight Pit (Phase 3) | `internal/worldgen` landmark + new area package | isolated |

Note the shape: **Phases 1–2 are almost entirely client-side JavaScript.** The
Go server changes are two tiny additions (`face:`, `EventPlayerStruck`) that
benefit every client and break none — the SSH clients ignore both.

## Phased rollout (each phase shippable, tested, on-tone)

- **Phase 1 — the fourth camera.** `V` toggle, pointer lock, eye-height camera
  riding the interpolated self-actor, camera-relative WASD, `face:N` +
  `SetFacing`, hidden self-body, crosshair, click-to-strike wired to `f`.
  Playable PvP duel in FP at the end of this phase — with invisible swings.
  *Tests:* `SetFacing` under the mutex + broadcast; session whitelist accepts
  `face:0..7`, rejects junk; facing quantization table mirrors `world.Dir`.
- **Phase 2 — the sword made visible.** Viewmodel + swing animation, weapon-in-
  hand on all avatars, `EventPlayerStruck`, hurt vignette + directional
  indicator, hit-stop, the first-person knock-out sequence, bow draw-and-loose.
  *Tests:* struck event fires on whiff and hit; weapon-mesh class mapping
  covers the full catalog (mirror `TestEveryPropHasAShape`'s pattern).
- **Phase 3 — the duel.** Blocking + parry, `/duel`, the Fight Pit landmark
  and area. *Tests:* guard halves damage server-side; parry staggers; duel
  handshake (offer/accept/timeout) mirrors the trade tests; `PvPAllowedAt`
  honors an active duel and the Pit.
- **Phase 4 — polish.** Sound, FOV/bob tuning, spectator niceties,
  leaderboard, Doom-cabinet FP rendering, `/champions`.

## Non-goals (keeping the cozy spine)

- **No sub-tile physics.** The grid stays authoritative for position,
  collision and reach. FP is presentation over the same simulation — the whole
  reason this plan is small.
- **No FP-only gameplay.** Anything a first-person player can *do*, a terminal
  player can do. FP may feel better; it must never be required or advantaged
  beyond presentation.
- **No damage rebalance by default.** Same HP, weapons, cooldowns and zones in
  every view.
- **No hostile mobs, no death penalty, no full-loot** — unchanged from
  `WEAPON_PLAN.md`. Defeat stays a knock-out and a walk home.
- **No second protocol.** FP speaks the existing scene/key wire plus the two
  tiny additions above.

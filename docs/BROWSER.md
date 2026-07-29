# The Browser Client (top-down 3D)

> How the third renderer works, and why it's shaped the way it is. Read
> [`GAME.md`](GAME.md) first for what the world *is*; this is about how a browser
> draws it.

## The one-liner

`internal/web` serves Durst World to a browser: a WebSocket session that drives
the same `game.Area` objects the SSH clients do, and a WebGL client that renders
the scene as a tilted 3/4 top-down 3D world. It is a **third renderer**, not a
second game — a browser player and an `ssh` player stand in the same Wilds, see
each other move, chat, and trade.

```sh
go run ./cmd/durstworld
# then open http://localhost:8080 — and, at the same time:
ssh -p 2222 markus@localhost
```

## Why this was small

The hard part was already done. Rendering was never coupled to the terminal:

- `internal/world` owns shared state and fans out events. It knows nothing about
  how anyone draws.
- `game.HDViewer.HDView(vw, vh)` returns a **semantic tile window** — per tile a
  kind, a ground texture, a hex color, one of ~93 props, a prop color and
  walkability, plus the window's absolute origin. Every walkable area implements
  it (via `game.Walker`).

That last point is what makes 3D cheap: the tile stream is already *colored and
semantic*, so the browser can build geometry from meaning rather than from
pixels. **The pixel-art tileset is never sent and never needed.** A biome
recolor, a new item, a new species — all of it reaches the browser for free.

So the browser client needs to answer only one question the terminal ones
don't: *what shape is a cathedral?*

## The shape vocabulary

`internal/web/shapes.go` maps every `game.TileProp` to a **builder**, a style, a
footprint in tiles, a height, and how much it glows at night:

```go
game.PropBldCathedral: {BuildBuilding, "cathedral", 3.0, 4.0, 4.6, 0.1, 0},
game.PropFir:          {BuildTree, "conifer", 1.0, 1.0, 2.8, 0, 0.15},
```

There are twelve builders (`tree`, `clump`, `rock`, `box`, `building`, `fence`,
`flat`, `glow`, `portal`, `item`, `creature`, `chess`); the style picks the
variant inside one. The table lives in Go, next to the props, and is **shipped
to the client in the hello message** — the browser never hardcodes a prop id, so
the two cannot drift.

A prop added later and not listed renders as a plain marker box in its own
color. That's the intended failure mode: visibly unfinished, never broken.
`TestEveryPropHasAShape` fails the build if one is missing.

## The wire format

Tiles are addressed by **absolute world coordinate**, not by position in the
camera window. This is the central decision and everything follows from it:

- Walking a step costs one row of tiles, because the ground you already hold
  stays held. (`TestSceneStepSendsOnlyTheNewEdge` asserts a step is ~8 tiles,
  not a screenful.)
- The 3D scene keeps its geometry as you walk instead of rebuilding it.
- Standing still costs **nothing at all** — an unchanged frame is not sent.

Colors are interned into a session-global, append-only palette: a hex is sent
once and referenced by index forever after. Tiles outside a retain radius are
explicitly dropped, so crossing an infinite overworld doesn't grow either side's
scene without bound.

A change of area resets the client's whole cache — every coordinate now means
something else.

## The session loop

`internal/web/session.go` is deliberately the same shape as
`cmd/durstworld/hd.go`'s `runHD`: join the world, build an area, funnel keys
into it, watch for `game.Transition`, pump world events, drive `game.Ticker`
areas off the wall clock. That loop is the contract every client honors —
reimplementing it differently is how two clients start disagreeing about the
world.

Differences, all of them forced by the medium:

| | HD (sixel) | Browser |
| --- | --- | --- |
| Frame | rasterized pixels, delta-encoded | JSON scene, delta-encoded |
| Panels | drawn into the pixel frame by hand | DOM, from server-sent data |
| Key release | inferred (SSH has no key-up) | real `keyup` events |
| Movement pacing | client guesses, server unaware | client paces, server floors it |
| Identity | SSH username | typed name, kept in `localStorage` |

The panels are where the browser is genuinely simpler. HD has no glyph layer, so
most of `internal/game/hd_ui.go` exists to rasterize the compendium, the trade
table and the crafting bench. The browser gets the same data — from the same
functions (`game.Compendium`, `game.Controls`, `game.TradeViewFor`, …) — and
lets HTML lay it out.

## Rendering

`internal/web/static/js/`:

| File | What it does |
| --- | --- |
| `main.js` | boot, frame loop, message dispatch |
| `net.js` | the socket, with backoff reconnect |
| `scene.js` | cameras (top-down and the `V` action cam), sun, sky, fog |
| `field.js` | ground/walls/props, all instanced |
| `props.js` | the twelve geometry builders |
| `actors.js` | players and creatures, interpolated |
| `rig.js` | the hero-rig skeleton contract and verb state machine |
| `rig_character.js` | the armored duelist body (pauldrons, tabard, cloak) |
| `rig_weapons.js` | the arms catalog, legends glowing (see weapon.go) |
| `rig_animation.js` | procedural combat motion: swings, dodge, guard, trail |
| `ui.js` | the DOM overlays and panels |
| `input.js` | keyboard, and the action mode's mouse combat |

The action camera and the swordplay verbs it drives (fast/strong strikes,
dodge, guard/parry, lock-on) are the subject of
[`SWORDPLAY_PLAN.md`](SWORDPLAY_PLAN.md); the server referees every verb, the
client only animates hopefully. Protocol v2 added the combat FX list on the
scene, guard state on actors, the held-weapon vocabulary in the hello, and the
`face:`/`dodge:`/`guard:` client commands.

Everything is drawn with `InstancedMesh`, pooled per material — a screenful of
forest is ~55 draw calls (including the shadow pass), not a thousand. Wind is a
few lines injected into the vertex shader (`addWind`), so 300 swaying trees cost
no per-frame CPU.

Three things carry over directly from the terminal renderers:

- **Day/night.** `ui.Ambient` already computes a sky tint and strength for the
  hour; here it drives the sun's color and intensity, the sky gradient, the
  environment light and the fog, instead of a per-pixel wash.
- **Radial light.** `game.HDLighter` (the Wilds' discovery circle, a cave's
  lantern) becomes distance fog, so darkness is something you walk a hole in.
- **Fog of war.** The Wilds already colors unexplored ground `#0B0E13`. The
  browser draws exactly that, so the discovery boundary looks the same in both
  clients.

### Lighting

- **ACES filmic tone mapping.** Highlights roll off instead of clipping, which
  is most of the difference between "3D shapes" and "a rendered scene".
- **Shadows** (soft PCF, 2048²). The shadow camera is a tight ortho box that
  follows the player, which buys far more resolution than a bigger map would.
  Ground receives but never casts; foliage casts nothing (double-sided quads
  with faked normals self-shadow into mush).
- **PBR materials keyed off the surface the server already sends.** `TileTex`
  says grass, brick, metal or water per tile — which is exactly the information
  roughness and metalness need. Four material classes cover fourteen textures,
  at the cost of no new data.
- **Warm key, cool fill.** A warm sun against a blue sky bounce gives a surface
  a lit side and a shaded side that differ in *hue*, not just brightness.
- **Gradient sky, doubling as the environment map.** The dome is PMREM-filtered
  into `scene.environment` (re-baked only when the horizon color moves, since
  the bake costs milliseconds and the sky changes over minutes), so PBR
  materials reflect the actual sky.
- **`Ambient.Night`.** The wire carries `strength` normalized to 0…1 by the
  server. `strength` alone is unusable for lighting — its top end is whatever
  the day cycle's darkest entry happens to be, a number only the server's table
  knows — and a client guessing at the range lit deep night like a bright
  afternoon.
- **Avatars stay lit after dark.** `internal/ui/atmosphere.go` says players are
  "left untouched so avatars stay readable at night"; the 3D equivalent is a
  floor of self-illumination on bodies (half as much for creatures).

### Variation

Identical, grid-aligned copies are the loudest signal that a world was
generated. Props whose `Shape.Jitter` is non-zero get a random facing, a few
percent of scale, and a slight shift in tone — hashed from the tile's own
coordinates, so a tree is the same tree for every player and across reconnects.
Ground tiles get ±3% brightness for the same reason.

Only things that *grew* are jittered. A cathedral squared to the street is not
a mistake to be corrected, so buildings, fences, bridges and portals are exempt
— which is why the flag lives in the Go shape table rather than being inferred
in the client.

Movement interpolation is the one thing the browser adds. The world is a grid
and always was — the server says "anna is now on tile (12, 7)". A terminal has
to snap; here actors glide over ~110ms, so the same 10-steps-per-second reads as
walking rather than teleporting.

### three.js

Vendored at `internal/web/static/vendor/three.module.min.js` (r160, MIT, license
alongside) and served by `go:embed`, so the binary stays self-contained and the
game works on an air-gapped network — the same property the SSH server has. Only
the core module is vendored: no `OrbitControls`, no `BufferGeometryUtils`.

## Security notes

- Player-authored text — chat lines, presentation decks — is only ever written
  with `textContent`, and the markdown renderer emits **DOM nodes**, never an
  HTML string. There is no path by which a deck can inject markup into another
  player's page.
- `CleanName` is the only gate on browser identity: 1–16 letters, digits, `-`
  or `_`. The world's `Join` then deduplicates it exactly as it does for two SSH
  clients claiming one username.
- The client may only send keys the areas already act on; anything else is
  dropped rather than forwarded into an area.
- WebSocket origins are same-origin by default (`WEB_ORIGINS` to widen).
- The tile window is clamped server-side, so one client can't ask for a 200×200
  scene.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `WEB_PORT` | `8080` | browser client port; `off` disables it entirely |
| `WEB_ORIGINS` | *(same-origin)* | comma-separated allowed origins, or `*` |
| `PORT` | `2222` | the SSH server, unchanged |

## Known gaps

- **Doom** (the arcade raycaster) draws via `game.HDFramer`, which paints into a
  pixel buffer. The browser has no equivalent yet, so the cabinet renders its
  (empty) tile window rather than the first-person view. Every other area,
  including all the other minigames, renders as a 3D board.
- The build palette's hotkeys (`1`–`9`) aren't bound in the browser yet; the
  palette itself works via `r`/`e`/`x`.
- No touch controls — the browser client assumes a keyboard.

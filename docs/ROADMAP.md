# Durst World — Graphics, Generative World & Commands Roadmap

Direction agreed with the team. Decisions locked:

- **Graphics:** truecolor + half-block "pixel" rendering, auto-detecting the
  client and downsampling to 256/16 colors.
- **Generative world:** a chunked, camera-scrolled infinite overworld.
- **AI/NPCs:** none — the world stays deterministic and fully offline.
- **Order:** rendering foundation first, then chat commands, then worldgen.

## Phase 0 — Rendering foundation ✅ (this milestone)

- Per-session, auto-detecting renderer (`bubbletea.MakeRenderer` →
  `ui.NewTheme`); truecolor palette with automatic 256/16 fallback.
- `ui.Theme` style set bound to one renderer; threaded through `game.Ctx`.
  Back-compat globals (`ui.WallStyle`, …) alias the process `ui.Default`.
- Half-block pixel layer (`Theme.HalfBlock`) + gradient/shimmer helpers
  (`Theme.Shimmer`, `Theme.VGradient`, `ui.Blend`).
- Camera/viewport (`game.Camera`, `CameraOn`, `RenderViewport`,
  `Walker.RenderViewport`) — renders a window around the player so maps can be
  larger than the screen. Prerequisite for the chunked overworld.
- Cinematic intro: animated DURST WORLD wordmark, then a smooth camera pan
  straight down onto the live play field.

## Phase 1 — Fancy graphics polish

- ✅ Real-time day/night tint cycle (`ui.Ambient`): tiles blend toward a
  time-of-day color; player glyphs stay full-bright. Tick now carries a
  monotonic `Frame` for animation.
- ✅ Animated tiles (`TileAnim`): glyph + color cycle off the frame counter.
  Kraftwerk machines pulse, coolant flows, lamps flicker; lobby plants breathe.
- ✅ Radial lighting (`Light` / `Walker.RenderLit`): tiles fade to shadow past
  the player's reach. Kraftwerk is now a dim machine hall.
- ✅ Biome palettes & climate: domain-warped elevation/moisture for organic
  biome edges, plus a temperature field (colder with altitude) that adds
  snowfields, snow-capped peaks, warm-dry savanna and wet-low swamp to the
  original water/sand/grass/forest/hill/mountain set. Each climate biome has
  its own hand-authored 6×6 HD ground texture (snow sparkle, dry savanna
  flecks, swamp mud blotches).
- ✅ Fog-of-war discovery: the Wilds starts hidden and reveals a circle around
  the player as they walk (bright sight radius + dimmed explored memory + fog
  beyond). Drives both the glyph and HD renderers and the minimap. Persisted to
  the store as 8×8 chunk bitmasks (full chunks pack to 8 bytes, frontier chunks
  keep exact bits), alongside the player's position — so the map and where you
  stand survive disconnects and re-entry.
- ⬜ Particles / weather layer.
- ⬜ Directional / two-cell avatars with a drop shadow.

## Phase 2 — Chunked infinite overworld ✅

- ✅ `internal/worldgen`: seeded, **stateless** generation — every cell is a
  pure function of `(seed, x, y)` (multi-octave value noise for elevation &
  moisture, hash-scatter for props). No stored chunks, so the world is
  infinite and identical for every session on the same seed.
- ✅ Biomes: deep/shallow water, sand, grass (flowers, tufts), forest
  (blocking trees), hills (boulders), mountain peaks. Water flows via the
  animated-tile system; day/night tint applies on top.
- ✅ `internal/areas/wilds`: keeps the player's absolute world coordinates and
  samples a player-centered window each frame, rendered through the existing
  camera/tile path. Players share one coordinate space, so presence and
  proximity chat just work.
- ✅ A home gate (`⌂`) at the origin returns to the lobby, with a compass hint
  so players are never stranded; a forced clearing guarantees a sane spawn.
  Reached from the new lobby portal `◈ The Wilds`.
- ⬜ Admin `/regen <seed>` to reroll (lands with Phase 3 commands).

## Phase 3 — Chat commands ✅

- ✅ Command registry + parser (`internal/game/commands.go`): chat starting
  with `/` routes to a handler instead of `World.Chat`. Aliases supported.
- ✅ Core commands: `/help [cmd]`, `/who`, `/where`, `/me`, `/w` (aliases
  `/whisper /tell /msg`), `/roll [NdM]`, `/color [0-21]`, `/goto <area>`,
  `/clear`. Multi-line output (`/help`, `/who`) uses a dismissable info panel.
- ✅ New world primitives: `Emote` (proximity), `Whisper` (one recipient,
  sender echoes locally), `SetColor`.
- ⬜ Admin `/regen <seed>` and `/tp`: deferred — the game has no auth (the SSH
  username is the identity, no password), so there's no admin gate yet. Needs
  an ownership/allowlist concept first, and `/regen` needs the Wilds seed to
  become shared mutable state with safe re-spawning.

## Phase 4 — Detailed avatars, Wilds hub, controls ✅

- ✅ Cell-grid renderer: the map composites into a `[]rcell` grid (glyph + fg +
  optional bg) so transparent multi-cell sprites overlay terrain correctly —
  ANSI-string overlay couldn't do partial transparency.
- ✅ Multi-tile players: a 2×2 collision footprint drawn as a larger
  Claude-inspired half-block sprite (square pixels — terminal cells aren't),
  with a "you" chevron and a name initial. Defined in `internal/game/sprite.go`.
- ✅ The Wilds is the spawn hub. Landmark portals near the origin (`⌂` HQ, `P`
  Presentation, `K` Kraftwerk, `D` Demo) lead to the hand-built areas; Durst HQ
  stays enterable. Each landmark sits in a forced clearing.
- ✅ Controls: 8-way movement (WASD/arrows + Y U B N diagonals), run on
  Shift/uppercase (2 tiles), and a minimap toggle (`m`) in the Wilds.
- ✅ Portal triggering reworked for multi-tile bodies: proximity-based with an
  "armed" latch so you can't bounce back through the portal you arrived from
  (a 2×2 body can't always stand on a wall-embedded portal tile).

## Phase 5 — Items & inventory ✅

- ✅ Collectibles scattered through the Wilds: a sparse, deterministic,
  biome-appropriate roll (`internal/areas/wilds/items.go`) places `◆` items —
  berries/mushrooms in forest, shells on sand, crystals in snow, nuggets in
  hills. They render as glinting gems (HD) over the biome ground and only show
  once you've discovered the ground they sit on.
- ✅ Pickup with `e` (works in both the glyph and HD clients): harvests the item
  under the 2×2 body into the player's pack, marks the cell collected so it's
  gone for that player, and toasts the find. `/inventory` (`/i`) lists the haul.
- ✅ Persistence: per-item counts and harvested cells survive disconnects
  (store `inventory` + `collected` tables), loaded into `Ctx.Inventory` at join.
- ✅ Wearable hats as gated loot: the 5 accessories are now **found**, not free —
  each `♚` hat hides in a themed biome (crown in hills, halo in snow, …). Pick
  one up (`e`) to unlock and equip it; ownership persists (`hats` table) and
  `/avatar` only lets you wear what you've found. New players start hatless.
- ✅ Interactive character panel (`/character`): a live avatar preview over
  cycleable Style / Color / Hat fields (↑↓ pick, ←→ change), persisting each
  change. Hat cycling is limited to unlocked hats.

## Phase 6 — UI in HD (the default client) ✅

Rule: HD is the default renderer, so all UI is built into the pixel frame, not
just the glyph client. HD has no glyph layer, so the interface is rasterized
straight onto the RGBA frame with basicfont (ASCII).

- ✅ HD overlay layer (`internal/game/hd_ui.go`, on `pixel.DrawPanel`/`Shade`):
  a bottom **status/hint bar** (area + contextual hint + control legend) and
  transient **pickup toasts** (toast moved to a wall-clock so it works without
  the glyph tick).
- ✅ HD **character panel** (`c`) and **inventory panel** (`i`), reachable by
  single keys since HD has no command line; arrows navigate/edit, `q` closes.
  The avatar customization is shared with the glyph panel
  (`game.CycleAvatarField`) so both clients stay in sync.
- ✅ HD **chat**: world events are formatted (`game.HDChatLine`) and drawn as a
  fading log above the HUD with per-speaker colors; `Enter` opens a text input
  (`/me`, `/w`, `/goto` plus plain proximity chat). Events are now handled in
  the HD select loop instead of being drained, so HD players see joins, chat,
  emotes and whispers — full UI parity with the glyph client.

## Phase 7 — Sealed gates ✅

Optional, riddle/offering-gated doors out past the hub (the four landmark
doors stay open). Two kinds, to exercise both solo and social play:

- ✅ **Personal gates** (the Whispering Gate → The Grove): each player repairs
  it themselves — say the riddle's answer in chat at the gate, or press `e` to
  offer the required item. Per-player state (`Ctx.FixedGates`, `gates_personal`).
- ✅ **Co-op gates** (the Sunken Gate → The Vault): a shared community effort —
  anyone presses `e` to contribute an item into a pool; when it fills, the gate
  opens for everyone. Global state lives in the shared `World`
  (`OfferToGate`/`GateFixed`), persisted (`gates_world`) and live across sessions.
- ✅ Sealed gates render as a dull broken arch (`PropSealed`) that becomes a
  glowing portal once repaired; the status/HUD hint shows the riddle or the
  pool progress. Works in both clients (chat answer + `e` offer). Destinations
  are `game.FlavorArea` reward rooms; worldgen places the gates on extended
  trails with clearings, so they're always reachable.

## Phase 8 — Settlements (villages & hamlets) ✅

Structures beyond the lone homestead: deterministic settlements scattered
through the Wilds, still a pure function of `(seed, x, y)` — nothing is stored.

- ✅ A macro-grid (`internal/worldgen/settlement.go`) hashes each cell to decide
  whether it hosts a settlement, its centre, tier and organic outline. The hub
  is kept clear; only temperate lowland is settled, and footprints clip cleanly
  against water and peaks.
- ✅ **Organic, not rectangular.** A settlement's edge is a wobbly radial curve
  `R(θ) = R0·(1 + Σ aₖ·sin(kθ+φₖ))` — a lopsided "incorrectly shaped circle".
  Each cell decides inside/edge/outside in O(1) from its angle and distance, so
  there's no boundary tracing or flood fill in the hot path.
- ✅ **Villages** (fenced) and **hamlets** (open): a central well, jittered house
  plots, radial dirt roads that breach the fence ring to form gateways, and a
  plowed **field** wedge for villages. New props (`PropWell`, `PropFence`) and a
  furrowed ground texture (`TexField`); houses now cluster here, with only the
  odd remote cabin left scattered in the open Wilds.
- ✅ Tests assert determinism, hub protection, and that a fenced village's
  interior is always reachable from outside (the roads really do gate the wall).

## Parked polish

- ✅ Real-pixel renderer (kitty graphics / sixel): shipped as the **default**
  renderer — a plain `ssh` serves HD (sixel/kitty, auto-detected from TERM),
  `ssh -t … glyph` opts back into the half-block client. Flat + delta,
  event-driven, with the half-block renderer as the fallback. Background and
  measurements: [`docs/pixel-renderer.md`](pixel-renderer.md).
- Particles / weather layer.
- Directional facing for avatars (sprite mirrors with movement).
- Mark landmarks on the minimap distinctly.
- Admin `/regen`, `/tp` (needs an auth/ownership concept).

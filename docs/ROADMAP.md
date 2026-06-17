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
  `/whisper /tell /msg`), `/roll [NdM]`, `/color [0-7]`, `/goto <area>`,
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

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
- ⬜ Biome palettes (will land with the overworld).
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

## Phase 3 — Chat commands

- Command registry + parser: chat starting with `/` routes to a handler
  instead of `World.Chat`.
- Core: `/help`, `/who`, `/where`, `/me`, `/w <name>`, `/goto <area>`,
  `/roll`, `/color`. Admin: `/regen`, `/tp`.
- New world methods: `Whisper` (one subscriber) and `Announce` (global).

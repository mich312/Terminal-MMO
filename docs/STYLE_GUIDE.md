# Durst World — Style Guide

> The art direction and code conventions for Durst World. Read this before
> touching anything visual or adding an area, so new work looks and behaves like
> what's already there. For *what the game is*, read [`GAME.md`](GAME.md).

## Golden rules

1. **Every visual feature must work in both renderers.** HD (real pixels) is the
   default; the glyph (half-block) client is the universal fallback. They share
   the same scene data and must agree. If you add a thing the player can see,
   wire it into *both* paths.
2. **Color is truecolor-first, defined in one place.** All hex lives in
   `internal/palette`. `internal/ui` builds lipgloss styles from it. Never
   hard-code a hex literal in an area or a renderer — pull it from the palette
   or the theme.
3. **One renderer per session.** Each SSH session gets its own
   `*lipgloss.Renderer` (via `bubbletea.MakeRenderer`) wrapped in a `ui.Theme`,
   threaded through `game.Ctx`. The renderer auto-detects the terminal and
   downsamples 24-bit hex to 256/16 color. Don't reach for process globals in
   session code — use the session's `Theme`.
4. **Retro on a 6-pixel grid.** All pixel art is authored at 6 art-pixels per
   tile. Keep it that way; it's what makes the look cohere.
5. **Persist between visits, never mid-session for cosmetics.** Live look is
   computed each frame (light, tint, animation); only durable facts hit SQLite.

## The palette

The single source is `internal/palette/palette.go` — a leaf package (no imports)
so both `ui` and `markdown` can share it without an import cycle. It is
deliberately **restrained**: deep slate, near-white, the Durst blue→cyan accent
ramp, one warn amber, plus panel/code backgrounds and a link blue.

| Token | Hex | Use |
| --- | --- | --- |
| `Accent` | `#2E8BFF` | Durst blue — borders, portals, highlights |
| `Accent2` | `#7DF0FF` | cyan tip of the accent ramp |
| `Bright` | `#F5F7FA` | near-white text/title |
| `Text` | `#C2CBD6` | body text |
| `Dim` | `#6B7480` | walls, decor |
| `Faint` | `#333A45` | floor dots |
| `Warn` | `#FFB454` | warnings |
| `PortalA/B` | `#2E8BFF` / `#7DF0FF` | portal pulse phases |
| `BarBg / BarText` | `#1B2027` / `#C2CBD6` | status bar |
| `Toast` | `#8A93A0` | join/leave one-liners |
| `PanelBg / CodeBg` | `#11151B` / `#171C28` | panels, code blocks |
| `Link` | `#6FB7FF` | markdown links |

**Adding a color:** add it to `palette`, alias it in `ui` (`Hex*` consts),
and bind it into `ui.Theme` if it's a reusable style. Don't scatter literals.

### Avatar colors

Player colors are a separate set of **22 readable truecolor hues** in
`internal/ui/theme.go` (`avatarColors`), starting with "claude clay" `#D97757`.
A player's default color is a deterministic hash of their name
(`ui.AvatarColor`); `/color [0-21]` picks by index. Keep new hues readable on the
dark slate background and distinct from their neighbors.

## The lipgloss theme

`ui.Theme` (built by `ui.NewTheme`) is the full style set bound to one renderer.
Use its named styles (`Title`, `Status`, `Panel`, `Wall`, `Floor`, `Object`,
`PortalA/B`, `ChatName`, `Toast`, `Dim/Faint/Bright/Accent/Warn`) rather than
constructing styles ad hoc. For per-cell computed colors (lighting, day/night,
animation) use the helpers: `Fg`, `FgBold`, `FgBg` (packs two half-block
pixels into one cell), `Wrap`, `Bar`, `Screen`. `ui.Blend(a, b, t)` mixes two
hex colors in CIE-Lab space.

`ui.Default` is the process-wide theme for code with no session (tests,
init-time globals). The old `WallStyle`-style globals are thin aliases to it —
**don't add new ones**; thread a session `Theme` instead.

## Pixel art conventions (HD renderer)

All sprites are hand-authored ASCII grids decoded into colors per renderer. Two
families: **ground textures** and **props/structures**, plus **avatars**.

### Density: 6 pixels per tile

Tiles are **6×6 art-pixels**. The avatar is **12×12** and spans 2×2 tiles → 6
art-pixels per tile, so everything shares one density. The HD renderer
nearest-neighbor upscales art to the on-screen tile size, so edges stay crisp;
the glyph renderer downsamples the same data to the half-block grid.

### Ground textures (`groundArt` in `tileset.go`)

Shade patterns colored *live* by each cell's biome color (so day/night tint and
lighting still apply). Codes:

| Code | Meaning |
| --- | --- |
| `B` / `' '` | base (the cell's own color) |
| `L` | light (blended toward white by `GroundLightMix`) |
| `D` | dark (blended toward shadow by `GroundDarkMix`) |

Rules:
- **Keep edge pixels at base** so same-type tiles join seamlessly — marks go in
  the interior.
- Provide **multiple variants** per texture; one is picked per tile by a
  coordinate hash to break up obvious repetition (for water, the variants are
  animation *frames* instead).
- Give each biome a distinct *grain*: open grass is sparse dapples; forest floor
  is busy leaf litter; savanna is flat horizontal `DD` dashes; snow is bright
  with sparse `L` glints; swamp is clumped `D` mud with the odd `L` algae sheen.

### Props & structures (`propArt`, `treeArt`, signature canopies, `portalArt`)

Props overlay the ground. The richer prop palette codes:

| Code | Meaning |
| --- | --- |
| `P` | prop body (the prop's color) |
| `p` | prop shade (toward shadow by `PropShadeMix`) |
| `o` | outline |
| `L` / `W` | highlight / bright glint |
| `D` | dark shade |
| `G` | animated glow (pulses off the frame counter) |
| `T` | tree trunk (fixed wood `#6B4A2B`) |
| `.` | transparent |

Conventions:
- **Loot reads as special, not terrain.** Gems are mostly their *own* color with
  a single white glint `W` (not all-white sparkle, which looked like snow); hats
  glint so they read as wearable loot.
- **Sealed vs open.** A sealed gate (`PropSealed`) is a dull cracked stone arch
  with a dark empty centre; once repaired it becomes the glowing `portalArt`
  swirl. The contrast is the whole point — keep it.
- **Trees overhang.** `treeArt` and the signature biome canopies (acacia, palm,
  fir, crag) are taller than one tile and drawn back-to-front so a stand reads
  as a continuous canopy, not a grid of identical stamps. The crag uses no `p`
  so the rim-dither leaves it solid — hard rock, not feathered foliage.
- **Portals are portals, not houses.** `portalArt` is a freestanding 2×2 gate:
  a ring `R` in the destination's color around a swirling animated field `@`.

### Avatars (`avatar.go`, `sprite.go`)

12×12 sprites with **8-way facing**, a 2-frame walk cycle, a few body styles and
optional accessories. Codes: `B` body, `L`/`D` light/dark shade, `E` eye,
`W` eye highlight, `m` mouth, `H`/`h` accessory, `.`/`' '` transparent. Only
three views are authored — **front, back, side** — and the side view (facing
right) is **mirrored** for the leftward facings; the other four diagonals reuse
front/back/side. The same data drives both renderers.

The glyph renderer **can't** fit a face in one tile, so there each player is a
single colored token (their name initial, reversed onto their body color) with a
`▾` chevron over your own head. Keep that fallback in mind: anything you encode
in the 12×12 sprite needs a sensible single-token degradation.

## Named art styles (palette recolor)

`internal/game/style.go` defines `Style` — the HD art direction (sprite sets,
portal art, shade-mix factors, vignette, and a `ui.Palette`). The shipped look
is `DefaultStyle()` (full color, no recolor). `StyleByName` adds two alternates
that **share the sprite sets** and differ only in a final whole-frame recolor
pass (`ui.Palette.Map`):

- **gameboy** — maps every pixel to the nearest of the 4-tone DMG green ramp by
  luminance.
- **neon** — pushes saturation and lifts lightness for a synthwave glow.

If you add a style, follow this pattern: reuse the sprites, express the look as a
`func(colorful.Color) colorful.Color` recolor, not a new tileset.

## Lighting & atmosphere

Computed per frame, never persisted:

- **Day/night tint** (`ui.Ambient`): tiles blend toward a time-of-day color;
  player glyphs stay full-bright. The tick carries a monotonic `Frame` for
  animation.
- **Animated tiles** (`TileAnim`): glyph + color cycle off the frame counter
  (machines pulse, coolant flows, lamps flicker, plants breathe). The `G` glow
  code in sprites is the prop-level version.
- **Radial light** (`Light` / `Walker.RenderLit`): tiles fade to shadow past the
  player's reach. Only *luminous* loot (crystals, mushrooms) glows at night;
  the campfire warms and flickers.
- **Fog-of-war**: bright sight radius + dimmed explored memory + fog beyond.

Keep these as render-time passes layered on the same scene data — that's what
keeps the two renderers in agreement and the world deterministic.

## HD UI conventions

HD has no glyph layer, so the interface is rasterized straight onto the RGBA
frame with `basicfont` (ASCII) via `internal/game/hd_ui.go` on
`pixel.DrawPanel`/`Shade`:

- A bottom **status/hint bar**: area + contextual hint + control legend.
- Transient **pickup toasts** (on a wall clock, so they work without the glyph
  tick).
- Single-key **panels** (HD has no command line): `c` character editor, `i`
  inventory, `Enter` chat input. Arrows navigate/edit, `q` closes.
- A fading **chat log** above the HUD with per-speaker colors.

Avatar customization is **shared** between the HD and glyph panels
(`game.CycleAvatarField`) so both clients stay in sync — don't fork the logic.

## HD frame budget (don't regress this)

The HD renderer is bounded on purpose (see `docs/pixel-renderer.md`):
**flat shading + delta + event-driven**, ~10–12 fps, ~16 KB/frame ceiling, ~0
when idle.

- **Flat tiles compress; smooth gradients don't** (~10–15× heavier). Default to
  flat. Sixel dithering must be *off* for flat art (its noise bloats a flat
  frame ~10×).
- **Delta is the big lever, and only when the camera is still.** Send only the
  changed region. Panning is 100% dirty — avoid gratuitous camera motion.
- **CPU is per-session.** Rasterize + encode runs for every connected player.
  Don't add per-frame work that scales with players carelessly.

## Code conventions

- **Go ≥ 1.24, no CGO, single static binary.** Keep it that way (the SQLite
  driver in use is pure-Go for this reason).
- **Areas are isolated, self-registering packages.** One package per area under
  `internal/areas/`; register in `init()` via `game.Register` (or
  `game.RegisterFlavor` for a simple room) and import the package for its side
  effect in `cmd/durstworld/main.go`.
- **Shared state goes through `world.World`** (one mutex, pub/sub events). Never
  let sessions touch each other directly; the broadcaster must never block.
- **Markdown is GFM via the `internal/markdown` package** and renders in both
  clients — text in glyph mode, bitmap font in HD. Don't special-case one.
- **Test what matters.** The suite covers world state (presence, proximity chat,
  emotes, whispers, color, slide sharing, non-blocking broadcast), map geometry,
  the cinematic intro, the terrain generator (determinism, biomes),
  chat-command routing, and store degradation. New mechanics should come with
  tests in the same spirit — especially anything that must be *deterministic*.
- **Comments explain art intent.** The tileset/sprite files document *why* a
  pattern looks the way it does (seamless edges, canopy overhang, loot vs
  terrain). Keep that habit — future sessions edit art by reading those notes.
</content>

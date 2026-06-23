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

## Phase 8 — Settlements (medieval villages) ✅

Structures beyond the lone homestead: deterministic villages scattered through
the Wilds, still a pure function of `(seed, x, y)` — nothing is stored.

- ✅ **Whole-layout generation, cached.** Unlike the terrain (decided per cell),
  a village's plan is generated as a whole into a small local grid from its
  macro-cell hash and cached (`internal/worldgen/settlement.go`); `At()` then
  looks the cell up. Generating the plan at once is what lets villages have
  multi-tile buildings, lanes that wrap housing, a wall with real corners, and
  roads that leave through the gates — none of which a per-cell decision could.
- ✅ **Medieval nucleated plan:** a central well + a stone **church** with a
  steeple, a main road bending through (in one gate, out the other) plus a loop
  lane and branch lanes so housing clusters in 2D; **varied multi-tile
  buildings** (cottage 1×1, house 2×2, longhouse 3×2, barn, church 2×3) front
  the streets, densest at the core; an **irregular palisade** (a per-sector
  vertex polygon, Bresenham-walked) with autotiled rail/corner sprites and gates
  where roads cross it; and **fields** along the approach.
- ✅ **Fits the land:** only temperate lowland is settled, the hub stays clear,
  and footprints clip against water/peaks so a village meets its lake or hillside
  instead of paving over it; cleared ground keeps the underlying biome look.
- ✅ **Connecting roads:** each village links toward its nearest neighbour;
  distant ones dwindle to a faded trail.
- ✅ Renderer support for bottom-anchored multi-tile buildings (`drawBuilding`,
  procedural `buildingArt`), oriented palisade sprites, and a furrowed `TexField`.
- ✅ Tests assert determinism, hub protection, and that a village's centre is
  always reachable on foot from outside the wall (the roads really do gate it).
- ✅ Lived-in detail: planted yards (bushes, flowers, tufts, orchard trees) and
  kitchen gardens between the houses, a duck pond by the green in some villages,
  per-building roof/colour variety, and **harvestable grain** standing in the
  fields and gardens — a new collectible that hooks straight into the existing
  pickup + inventory (`e` to harvest, persists like other forage).
- ✅ Outlying worksites that follow the nearest natural biome edge: a **stone
  quarry** cut into nearby rock (hills *or* mountain), a **lumber camp** clearing
  bitten into the forest, and a **fishing hut** with a **jetty** at the shore.
  Each only appears where that biome lies within reach; the worksite's own ground
  is carved into the real biome (rock / trees / water), with the hut on the
  village side and a path back through a gate. Each worksite is harvestable
  like the fields — **stone** at the quarry, **timber** at the lumber camp,
  **fish** off the jetty (`e` to gather, persists in the inventory).
- ✅ Distant reach: a village's layout extends well past its core, so a worksite
  can sit at a biome edge ~40 tiles out and be linked by a long resource road —
  a meadow village still reaches its far-off forest or lake.
- ✅ **Tangled-medieval stone cities** (~1 in 6 settlements): a much larger,
  denser tier (roughly double a village's span). Winding lanes radiate from the
  market to the gates, tied by wobbly ring roads and alleys — a connected,
  **walkable** web (a test floods from the centre and asserts it reaches a gate).
  Buildings pack the core solid on cobbled ground, behind a grey **stone curtain
  wall** with **towers** at corners and gates, around a market square anchored by
  a great **cathedral** and a castle **keep**. Crucially the footprint is **built
  to the terrain**: it's the patch of contiguous open lowland around the centre,
  so a city fills a plain, wraps a lake and stops at forest or hills — the wall
  conforms to that shape rather than being a disc. Villages keep their organic
  loop-and-lane layout.
- ✅ Every settlement draws its **own size** — a continuum of core reach from a
  small hamlet up to a large city (at/above a threshold it becomes a stone city,
  below it a timber village), so there's a full range rather than two fixed
  tiers. Settlements sit **far apart** (macro cell widened to 168 → typically
  200–490 tiles between neighbours), and their built-up outlines are
  **lobed/irregular** (harmonic boundaries) so neither villages nor cities ring a
  tidy circle.
- ✅ City interiors gained structure: a **citadel** — a castle keep inside its own
  small rectangular **inner wall** with corner towers and a gate, a fortified
  contrast to the organic streets — plus **secondary squares** scattered through
  the blocks alongside the central cathedral + market. Cities now read as having
  **districts**: a wealthy core of tall multi-storey **townhouses**, ordinary
  houses on the middling streets, and cottages + **warehouses** on the poorer
  outskirts; a **market hall** flanks the square opposite the cathedral.
- ✅ Settlement walls are **traced along the built-up edge** (a boundary trace of
  the hole-filled footprint) rather than an angular-sector polygon, so they're
  jagged and can follow concave bays around woods and lakes instead of rounding
  off; gates are punched where lanes meet the wall.
- ✅ City street-life pass (no NPCs, just structures and light):
  - **Cobbled streets** (pale) read distinctly from the darker packed ground
    between buildings, and city houses **terrace** into solid blocks (shared
    walls) instead of standing apart.
  - **Night braziers** flank every gate and dot the squares and citadel bailey,
    casting warm flickering pools of firelight after dark.
  - **Market stalls** (striped awnings) cluster on the plaza; a **smithy** whose
    forge-mouth glows warm day and night, and a **tavern** with cosy lamplit
    windows, front the central square (placed by an outward ring search so a lane
    can't crowd them out).
  - **Roof material variety by district**: golden thatch on cottages, barns and
    the smithy; red clay tile and blue-grey slate on the wealthy townhouses,
    market halls and the tavern; lead/slate on the grand churches and the keep —
    so a tiled-roof core reads against thatched outskirts.
  - **Plank bridges** span water a city straddles: the enclosed water counts as
    interior (the wall wraps it), and a bridge is laid where a street meets the
    bank, straight across to walkable ground on the far side (never a stub — a
    test asserts every bridge joins two walkable banks).

## Phase 9 — Cozy frontier: craft, build, automate (in progress)

The turn from a *place* into a *game* — see [`DESIGN_MECHANICS.md`](DESIGN_MECHANICS.md)
and [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md). Corporate × medieval voice.

- ✅ **Crafting** (`internal/game/recipes.go`): a static recipe catalog (Planks,
  Flour, Gold Ingot, Field Salve, Wrought Lamp) and pure `Craftable`/`Craft`
  that spend forage and yield refined goods through the existing inventory +
  store plumbing. A `DrawCraftPanel` in HD (open with `k` or the Tab menu) and a
  matching glyph panel via `/craft`; both list live craftable counts.
- ✅ **Production sprites**: `PropWorkbench/Sawmill/Mill/Furnace/Chest` as 6×6
  art in `tileset.go`, the furnace wired into the night light emitters.
- ✅ **Placements layer** (the one architectural piece): a SQLite `placements`
  table + a shared `world` set (`Place`/`Unplace`/`PlacementAt`, `EventPlaced`),
  overlaid on the stateless Wilds at render time — the co-op gate generalized
  from one pool to many owned, positioned objects. Terrain stays pure-seed;
  placements are the only stored mutable layer, and they make built walls solid
  (movement collision now consults placements, not just the generator).
- ✅ **Build mode** (`b` in the Wilds): a placeable catalog (Fence, Workbench,
  Cold Storage, Lamppost) with material costs; a green/red ghost follows the
  cursor, `r` cycles, `e` spends and places. Works in both clients.
- ✅ **Offline machines** (`internal/game/machine.go`): a machine is a placement
  whose JSON state carries input/output buffers + a wall-clock; a pure `Settle`
  fast-forwards elapsed time (no per-tick RNG), so a Sawmill/Mill/Furnace
  produces while you're logged off. Stand beside one and press `e` to open its
  panel — input/output meters, a "while you were away" delta, and Collect /
  Refuel. Built like any structure; the furnace glows as it works. Both clients.
- ✅ **Trade — the Durst Group Concession** (`internal/game/stall.go`,
  [`TRADE_PLAN.md`](TRADE_PLAN.md)): asynchronous vending stalls on the placements
  layer — build one, `/sell 10 plank for 6 stone` to post an offer (stocked from
  your pack), and anyone can press `e` to buy whenever, online or not; the owner
  `/collect`s the till later. The one racy op (concurrent buyers) is settled
  atomically by a new `world.MutatePlacement` primitive (a test fires 60 buyers
  at 20 stock and asserts exactly 20 sell, never oversold). Buyer panel in both
  clients.
- ✅ **Build/trade polish:** `x` in build mode **demolishes** your own structure
  and **returns whatever it holds** (a stall's stock + till, a machine's buffers)
  to your pack — owner-gated, so nobody tears down someone else's Workspace; a
  placed **workbench opens crafting** when you press `e` beside it (the build →
  craft loop made literal); and a stall owner can `x` **remove a mislisted
  offer**, refunding its stock.
- ✅ **In-panel offer authoring:** the Concession panel now composes a new offer
  in place (`n` at your stall) — pick the give/ask items and per-sale counts with
  the arrows, watch a live "stocks N (M sales)" readout, and `e` to post. It's the
  UI twin of `/sell` (which still works), routed through the same `AddOffer`, and
  it's the only way to post in HD, which has no command line. A shared composer
  (`OfferDraft` + `CycleOfferField`/`PostDraft` in `internal/game/offer_compose.go`)
  drives both clients identically, the way the character panel shares its cycling.
- ✅ **Settlement claims & protected building** (the land-tenure layer — see
  [`CLAIMS_PLAN.md`](CLAIMS_PLAN.md)). Two tiers resolved by one `game.BuildRight`
  predicate: **deed a settlement building** (a Workspace Charter) for a named,
  bounded parcel only you may build on, or — in the open Wilds — your own
  structures project a small **proximity buffer** others can't build inside.
  Worldgen exposes the pre-drawn buildings as stable, pure-seed plots
  (`worldgen.PlotAt`); a claim is shared world state persisted like the gate pool
  (not a placement), arbitrated atomically so two claimants can't both win, and it
  **lapses after the owner is absent ~7 days** (a wall-clock check, no per-tick
  sim) so plots recirculate. In build mode `e` over a building deeds it and `x`
  releases your own; the glyph HUD names the parcel ("Anna's Workspace, Brixen")
  and both clients toast on entry. Movement is never gated — claims protect
  building, never walking.
- ✅ **Build palette + clearing tools** (see [`BUILD_TOOLS_PLAN.md`](BUILD_TOOLS_PLAN.md)).
  The blind `r`-cycle build picker became a **palette + 1-9 hotbar**: the catalog
  grouped (Structures · Machines · Trade · Tools), each row with its cost, a live
  afford count (dimmed when short), the selected blurb, and an honest block reason
  ("can't build: trees in the way") — both renderers, non-modal. **Clearing tools**
  let you reshape the land: a rare **Axe-head** / **Pick-head** found in the world
  unlocks crafting an **Axe** / **Pickaxe** (head + Timber); wielding one in build
  mode fells a tree (→ Timber) or breaks a hill boulder (→ Cut Stone), with
  mountain peaks left permanent. Cleared cells are a third sparse stored overlay on
  the pure-seed terrain (after placements and claims) — walkable, buildable,
  `BuildRight`-gated — and **regrow after the owner is absent ~2 weeks**, so the
  woods reclaim ghost towns.
- ⬜ The parked wildlife layer comes after.

## Phase 10 — Community projects & the async-social loop (proposed)

The cozy-frontier loop (Phase 9) and the danger layers (wildlife, arms) gave the
world *things to do* but no **sink** and no answer to the empty room — the core
risk for a one-company MMO. This phase closes the loop: a **communal build** the
whole player base raises together over time (the co-op Sunken Gate generalized
into an ongoing, staged, named project), plus a **"while you were away"** digest
so you always arrive to evidence the world — and your colleagues — moved. It
rides the existing shared-state layers (gate pool, placements, `MutatePlacement`,
the event log); no new genre. Full design, pillar check and shippable slice:
[`COMMUNITY_PLAN.md`](COMMUNITY_PLAN.md).

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

# Durst World — What This Game Is

> Orientation for future sessions. Read this first to understand what you're
> working with; read [`STYLE_GUIDE.md`](STYLE_GUIDE.md) before touching the
> visuals; read [`ROADMAP.md`](ROADMAP.md) for what's done and what's next.

## The one-liner

Durst World is a small, persistent, multiplayer terminal "world" for Durst
Group employees, played **entirely over SSH**. There is no client to install:
you `ssh` in and your SSH username *is* your character. People walk around a
shared, generative overworld, bump into each other in real time, chat (only
those standing near you hear it), forage items, and step through portals into
hand-built areas — a lobby, a presentation hall, a machine room, a demo center,
and riddle-gated secret rooms.

```
ssh -p 2222 yourname@durstworld.example.com
```

No password, no account, no AI/NPCs. The world is deterministic and offline.

## Design pillars (the things that must stay true)

1. **It works from any terminal.** The pitch is "ssh from anything and it
   works." This is the hardest constraint and it shapes every rendering
   decision — see "Two renderers" below.
2. **Your SSH username is your identity.** No auth, no passwords. The username
   you connect with is your name in the world. (This is also why there's no
   admin tier yet — there's nothing to authenticate against.)
3. **Shared, real-time, eventually-consistent.** Everyone is in one live world.
   Presence and chat fan out through events; the broadcaster never blocks (it
   drops the oldest event for a slow session rather than stalling everyone).
4. **Persist between visits, not during.** Live state is in memory. SQLite is
   the memory *between* visits (where you stood, what you've explored, your
   pack, your hats, guestbook, decks). Chat is deliberately ephemeral.
5. **Deterministic & offline.** The overworld is a pure function of a seed — no
   stored chunks, no server-side randomness that drifts between sessions, no
   network calls, no NPC AI.

## Two renderers (important context)

There are **two clients for the same live world**, and they see each other:

- **HD renderer (the default).** On a sixel- or kitty-capable terminal
  (Windows Terminal 1.22+, kitty, ghostty, iTerm2, WezTerm) a plain `ssh`
  connection streams **real pixels** — hand-authored pixel art upscaled and
  sent as sixel/kitty image escapes with delta updates (only the changed region
  each frame). It lives **outside** bubbletea on purpose (image escapes are
  out-of-band; bubbletea's `View()` string loop would clobber them). Because HD
  has no glyph layer, **all UI is rasterized into the pixel frame** (status bar,
  toasts, chat log, character/inventory panels).
- **Glyph renderer (the fallback / classic client).** A bubbletea/lipgloss TUI
  built from half-block "pixels" and colored glyphs. Universal — runs in tmux,
  Terminal.app, PuTTY, anywhere. Opt in with `ssh -t … glyph`.

Both render the *same scene data*; they differ only in how they paint it. When
you add a feature, it must work in **both**. See
[`docs/pixel-renderer.md`](pixel-renderer.md) for the measurements and the
rationale behind the HD renderer's "flat shading + delta + event-driven" shape.

## The world & its areas

You spawn in **the Wilds**, the open-air hub, and reach everything else through
portals.

### The Wilds (generative overworld)

- **Infinite & stateless.** `internal/worldgen` generates every cell as a pure
  function of `(seed, x, y)` — multi-octave value noise for elevation/moisture,
  a temperature field that cools with altitude, hash-scatter for props and
  items. Nothing is stored; the same seed yields the same world for everyone.
- **Climate biomes.** Deep/shallow water, sand, grass, forest (blocking trees),
  hills (boulders), mountain peaks, plus climate variants: snowfields,
  snow-capped peaks, dry savanna, wet swamp. Each biome has its own
  hand-authored ground texture and signature flora (acacia, palm, fir, crag).
- **Fog-of-war discovery.** The map starts hidden. Walking reveals a circle
  around you (bright sight radius), which then stays visible but dimmed
  (explored memory); everything beyond is fog. Drives both renderers and the
  minimap (`m`). Persisted as 8×8 chunk bitmasks alongside your position.
- **Collectibles (`◆`).** Sparse, deterministic, biome-appropriate loot —
  berries/mushrooms in forest, shells on sand, crystals in snow, nuggets in
  hills, plus worksite harvests (grain, stone, timber, fish) and cave finds
  (geodes, relics, glowspore, amber). Stand on one and press `e` to forage it
  into your pack. Per-item counts and harvested cells persist.
- **Compendium (`i`, or `/compendium` · `/codex`).** The in-game codex of every
  collectible and wearable: what each find is, where it turns up, how rare it is,
  and what it can do (the gate it repairs, the wearable it unlocks and that
  wearable's power). Items you've found show a count; the rest stay dimmed but
  still described, so it doubles as a checklist. The panel scrolls. It's built
  straight from the item catalog, so both clients show the same thing.
- **Wearable hats (`♚`).** Five accessories are *found, not free* — each hides
  in a themed biome (a crown in the hills, a halo in the snow…). Pick one up to
  unlock and equip it; you can only wear hats you've found.
- **Sealed gates.** Broken arches out past the hub that lead to reward rooms:
  - *Whispering Gate → The Grove* — **personal**: say the riddle's answer in
    chat at the gate, or press `e` to offer the required item. Each player
    repairs their own.
  - *Sunken Gate → The Vault* — **co-op**: everyone presses `e` to pool
    offerings; when the pool fills, the gate opens for the whole community.
- **Trading.** Stand next to another player and `/trade <name>`; they `/accept`,
  and a table opens for both. Each lays items from their pack (←→ pick, `+`/`-`
  stage), and once **both** press `r` (ready) the swap commits atomically — items
  move in both inventories and persist. Changing an offer clears both ready
  flags, and walking off or disconnecting cancels the table. Negotiation lives in
  the shared world; each session applies its own half, so it works even with
  persistence off.

### Hand-built areas (reached from Wilds landmarks)

| Landmark | Area | What it is |
| --- | --- | --- |
| `⌂` | **Durst HQ / Lobby** | The original ASCII lobby. Sign the guestbook (`e`). |
| `P` | **Presentation Wing** | A concourse of stages for player-authored talks. |
| `K` | **Kraftwerk** | A dim machine hall (animated machines, coolant, lamps). |
| `D` | **Demo Center** | A showcase room. |
| `A` | **Arcade** | A neon hall of cabinets, each a portal into a minigame (Sokoban, a torch-lit Maze, Snake). Out west past Kraftwerk; also off the lobby. See [`AREAS.md`](AREAS.md). |
| — | **The Grove / The Vault** | Reward rooms behind the sealed gates. |

Every area's exit door returns to **the Wilds** (beside the landmark you used),
so the open world is the hub you always come back to — not the lobby.

### Presentation Wing (the one area with deep mechanics)

A growing concourse of stages (capacity 8). Walk to the `＋` booth and press
`e` to author a **markdown deck** in-world (type or paste it; `---` separates
slides). It becomes a new stage with a big screen. Decks are GitHub-flavored
Markdown (headings, bold/italic/strike, lists incl. task lists, tables,
blockquotes, links, fenced code with chroma syntax highlighting) and render in
*both* clients — as text in glyph mode, drawn with a bitmap font in HD. Everyone
in a stage sees the same slide; the owner drives it with `n`/`p` from the `▟`
lectern and can re-edit or retire it (`x`, then `x` to confirm). Decks are owned
by your SSH username and saved to SQLite, so talks survive a restart (only the
live slide index resets).

## Controls (quick reference)

| Key | Action |
| --- | --- |
| WASD / arrows | move (walk into a `◊`/`◈` portal to enter it) |
| Y U B N | move diagonally (↖ ↗ ↙ ↘) |
| Shift + move | run (two tiles per step) |
| m | toggle the minimap (Wilds) |
| Enter | chat — heard within 8 tiles |
| e | pick up `◆` · sign guestbook · author/edit a deck (`＋` booth / lectern) |
| k | open the crafting bench (refine forage into goods); `/craft` in glyph |
| b | build mode in the Wilds — place structures (move ghost, `r` next, `e` place) |
| n / p | next/previous slide while presenting |
| Tab | open the menu — compendium, character, who, help (HD) · who's online (glyph) |
| c | character editor (HD); `i` compendium (HD) |
| ? | help — every key and chat command, in one panel |
| q / Ctrl+C | quit (press twice) |

### Chat commands

A chat line starting with `/` runs a command instead of talking. `/help` lists
them.

| Command | Action |
| --- | --- |
| `/who` · `/where` | who's online; your area & coordinates |
| `/me <action>` | emote to those nearby |
| `/w <name> <msg>` | private message (aliases `/whisper /tell /msg`) |
| `/roll [NdM]` | roll dice for everyone nearby |
| `/color [0-21]` | change avatar color |
| `/goto <area>` | teleport to an area |
| `/character` · `/compendium` (`/i` · `/codex`) | open the avatar editor / the items & wearables codex |
| `/clear` · `/help` | clear your log; list commands |

## Persistence model

SQLite (WAL) at `./data/durstworld.db`, behind the small `internal/store`
interface. It remembers, per the design, only *between* visits:

- Your last position and your fog-of-war discovery (8×8 chunk bitmasks).
- Your inventory counts and which collectible cells you've harvested.
- Which hats you've unlocked.
- Personal gate repairs and the shared co-op gate pool.
- Visit counts, the guestbook, the event log, and player-authored decks.

If the DB is unwritable the game logs a warning and **plays on without memory** —
persistence is a nicety, never a hard dependency.

## Architecture at a glance

```
cmd/durstworld/            wiring: wish SSH server, middleware, signals, HD loop
internal/world/            shared in-memory state + pub/sub events (one mutex)
internal/store/            SQLite persistence behind a small interface
internal/game/             root bubbletea model, Area interface, tilemap, Walker,
                           sprites, tileset, both render paths
internal/areas/...         one package per area (self-registering via init())
internal/worldgen/         stateless seeded terrain generator
internal/pixel/            HD frame writer: kitty/sixel encoders + delta loop
internal/ui/               the shared visual language (styles, theme, palette)
internal/palette/          the single source of the truecolor hex palette
internal/markdown/         GFM → styled spans for presentation decks
```

**Sessions never touch each other directly.** Every change goes through the
`world`, which fans events out to per-session buffered channels (oldest dropped
when full). Each session's bubbletea model pulls events with a blocking
`tea.Cmd` and re-issues it after every event; the HD loop handles events in its
own select loop.

## Adding a new area (the common task)

Areas are isolated, self-registering packages implementing the `game.Area`
interface. For a simple walkable room with a portal back out, you don't even
implement the interface — hand a map, spawn point, and panel text to
`game.RegisterFlavor` (see `internal/areas/kraftwerk`). For an area with its own
interaction, embed `game.Walker` and implement `Area` yourself (see the lobby's
guestbook or the Presentation Wing's slides). The README's "Adding a new area"
section is the step-by-step; this is the pointer to it.

## How to run it locally

Requires Go ≥ 1.24. Single static binary, no CGO.

```sh
go run ./cmd/durstworld
ssh -p 2222 you@localhost            # HD (default; sixel/kitty auto-detected)
ssh -t -p 2222 you@localhost glyph   # classic half-block glyph client
go test ./...                        # the test suite
```

`PORT` changes the listen port (default `2222`). The host key lands in
`./.ssh/host_key` and the DB in `./data/durstworld.db`, both on first run.
</content>
</invoke>

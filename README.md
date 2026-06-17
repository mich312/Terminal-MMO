# Durst World

A small multiplayer terminal world for Durst Group employees, played over
SSH. Walk around the ASCII lobby of Durst HQ, bump into colleagues in real
time, chat (proximity-based — gossip stays local), sign the guestbook, and
step through portals into the Presentation Wing, Kraftwerk, Demo Center and
the (coming-soon) Arcade.

```
ssh -p 2222 yourname@durstworld.example.com
```

No password, no account — your SSH username is your name in the world.

## Run locally

Requires Go ≥ 1.24. Single static binary, no CGO.

```sh
go run ./cmd/durstworld
# in two other terminals:
ssh -p 2222 anna@localhost
ssh -p 2222 markus@localhost
```

- `PORT` env var changes the listen port (default `2222`).
- `./.ssh/host_key` — persistent Ed25519 host key, generated on first run.
- `./data/durstworld.db` — SQLite (WAL) for visits, guestbook, the event log
  and player-authored presentation decks. Created with schema on first run;
  delete it to start fresh. If the file is unwritable the game logs a warning
  and plays on without memory.

### HD mode (real-pixel renderer, default)

On a sixel- or kitty-capable terminal (e.g. Windows Terminal 1.22+, kitty,
ghostty, iTerm2, WezTerm) you walk the Wilds rendered as actual pixels instead
of half-block glyphs. It's the **default** — a plain connection serves it:

```sh
ssh -p 2222 you@localhost
```

A plain interactive `ssh` allocates a PTY for free, so no `-t` is needed. To
opt back into the classic glyph (bubbletea) client, pass a command — which does
need `-t` to get a PTY:

```sh
ssh -t -p 2222 you@localhost glyph   # classic half-block client
```

WASD or arrow keys to move, `Y U B N` for diagonals, Shift/uppercase to run,
`q` to quit. It shares the live world, so you and glyph-client players see each
other. It bypasses bubbletea and streams sixel with
delta updates (only the changed region each frame). Background and rationale:
[`docs/pixel-renderer.md`](docs/pixel-renderer.md).

### Controls

| Key | Action |
| --- | --- |
| WASD / arrows | move (walk into a `◊`/`◈` portal to enter it) |
| Y U B N | move diagonally (↖ ↗ ↙ ↘) |
| Shift + move | run (two tiles per step) |
| m | toggle the minimap (in the Wilds) |
| Enter | chat — heard within 8 tiles of where you stand |
| e | pick up a `◆` item in the Wilds · sign the guestbook · author a presentation (at the `＋` booth) · edit your deck (at the lectern) |
| n / p | next/previous slide while presenting from your lectern |
| Tab | who's online |
| q / Ctrl+C | quit (press twice) |

You spawn in **the Wilds**, the open-air hub. Walk to the landmark doors near
spawn — `⌂` Durst HQ (the lobby), `P` Presentation, `K` Kraftwerk, `D` Demo
Center — to enter each area. Players are multi-tile half-block avatars in their
own color, drawn over a 2×2 footprint.

The overworld starts **hidden** — only a circle of terrain around you is lit;
the rest is fog. Walking uncovers new ground, which then stays visible (dimmed)
on screen and fills in on the minimap (`m`), so the world is something you
explore rather than see all at once. Climate-driven biomes — forest, savanna,
snowfields and snow-capped peaks, wetlands, sand, hills — give each direction a
distinct look, each with its own HD pixel-art ground texture. Scattered through
them are `◆` **collectibles** — berries and mushrooms in the woods, shells on
the beach, crystals in the snow — that you forage by standing on one and
pressing `e`; `/inventory` (`/i`) shows your haul. Rarer still are wearable
`♚` **hats**, each hidden in a themed biome (a crown in the hills, a halo in
the snow…): find one and you unlock and wear it. `/character` opens an
interactive panel to preview your avatar and cycle body, color and the hats
you've earned with the arrow keys. Discovery, position, pack and unlocked hats
all persist across sessions.

### Presentation Wing

The Presentation Wing is a concourse of stages that grows as people add talks.
Walk to the `＋` booth and press `e` to author a **markdown deck** in world
(type or paste it; `---` separates slides) — it becomes a new stage with a big
screen. Decks are GitHub-flavored Markdown: headings, **bold**/_italic_/
~~strike~~, lists (incl. task lists), tables, blockquotes, links, and fenced
code blocks with syntax highlighting (via chroma). They render both in the
glyph client and — drawn as pixels with a bitmap font — in HD mode. Everyone standing in a stage sees the same slide; the deck's owner
drives it with `n`/`p` from the `▟` lectern and can re-edit with `e`. Decks are
owned by their author (your SSH username) and saved to SQLite, so your talks
come back after a restart — only the live slide index resets.

The wing holds a fixed number of stages (8). When it's full the `＋` booth says
so; a presenter has to retire one of their own talks (`x`, then `x` again to
confirm, at their lectern) before a new one can be added.

Minimum terminal size is 80×24.

### Chat commands

In the chat prompt (Enter), a line starting with `/` runs a command instead of
talking. `/help` lists them in a panel:

| Command | Action |
| --- | --- |
| `/who` · `/where` | who's online; your area & coordinates |
| `/me <action>` | emote to those nearby |
| `/w <name> <message>` | private message (aliases `/whisper /tell /msg`) |
| `/roll [NdM]` | roll dice for everyone nearby (e.g. `/roll 2d6`) |
| `/color [0-7]` | change your avatar color |
| `/goto <area>` | teleport to an area |
| `/clear` · `/help` | clear your log; list commands |

## Tests

```sh
go test ./...
```

Covers world state (presence, proximity chat, emotes, whispers, color, slide
sharing, non-blocking broadcast), map geometry, the cinematic intro, the
terrain generator (determinism, biomes), chat-command routing, and store
degradation.

## Deploy

### Docker

```sh
docker build -t durstworld .
docker run -d --name durstworld \
  -p 2222:2222 \
  -v "$PWD/.ssh:/app/.ssh" \
  -v "$PWD/data:/app/data" \
  durstworld
```

The two mounts keep the host key (so clients don't see scary
known-hosts warnings after a redeploy) and the SQLite DB.

### systemd

```ini
[Unit]
Description=Durst World SSH MUD
After=network.target

[Service]
User=durstworld
WorkingDirectory=/opt/durstworld
ExecStart=/opt/durstworld/durstworld
Environment=PORT=2222
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Build with `make build` (or `go build -o durstworld ./cmd/durstworld`) and
copy the binary to `/opt/durstworld/`. The host key and DB live next to it
in `.ssh/` and `data/`.

## Adding a new area (or mini-game)

Areas are isolated packages that implement a four-method interface — the
Arcade stub (`internal/areas/stub`) is the minimal template.

1. Create `internal/areas/yourarea/` and implement `game.Area`:

   ```go
   type Area interface {
       Name() string
       Init(player *world.Player) tea.Cmd
       Update(msg tea.Msg) (Area, tea.Cmd) // return game.Transition{To: "lobby"} to leave
       View(width, height int) string
   }
   ```

   For a simple room — a walkable map with a portal back out and a
   descriptive side panel — you don't implement the interface at all: hand
   the map (a string slice plus a rune legend), spawn point and panel text
   to `game.RegisterFlavor` and you're done. `internal/areas/kraftwerk` is
   the worked example.

   For anything with its own interaction (the lobby's guestbook, the
   Presentation Wing's slides), embed `game.Walker` and implement `Area`
   yourself. `Walker.HandleCommon` gives you movement, wall collision,
   portal triggering and the portal pulse for free.

2. Self-register in `init()`:

   ```go
   func init() {
       game.Register("yourarea", "Your Area", func(ctx *game.Ctx) game.Area {
           return &area{...}
       })
   }
   ```

3. Import the package for its side effect in `cmd/durstworld/main.go`.

4. Point a portal at it: add a tile to the lobby map and a legend entry
   `{Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "yourarea",
   Label: "Your Area"}`.

Optional extras: implement `game.Hinter` for a contextual status-bar hint,
`game.InputCapturer` to grab all keys while a panel is open, and use
`world.World` for any shared state that everyone in the area must agree on
(the Presentation Wing's player-authored decks are the worked example).

## Architecture

```
cmd/durstworld/            wiring: wish server, middleware, signals
internal/world/            shared in-memory state + pub/sub events (one mutex)
internal/store/            SQLite persistence behind a small interface
internal/game/             root bubbletea model, Area interface, tilemap, Walker
internal/areas/...         one package per area
internal/ui/               the only place colors and styles are defined
```

Sessions never touch each other directly: every change goes through the
world, which fans events out to per-session buffered channels (oldest
dropped when full — presence is eventually consistent, the broadcaster
never blocks). Each session's bubbletea model pulls events with a blocking
`tea.Cmd` and re-issues it after every event.

Live state is memory; SQLite is only the memory *between* visits (visit
counts, guestbook, event log). Chat is deliberately ephemeral.

### Rendering

Color is truecolor-first. Each SSH session gets its own `*lipgloss.Renderer`
(`bubbletea.MakeRenderer`) wrapped in a `ui.Theme`, threaded through
`game.Ctx`; the renderer auto-detects the client's terminal and downsamples
24-bit hex to 256- or 16-color as needed. `ui.Default` serves code with no
session (tests, init globals), and the old `ui.WallStyle`-style globals are
thin aliases to it.

Maps render through a camera (`game.CameraOn` + `RenderViewport`), so a map
may be larger than the screen. Tiles can animate (`TileAnim`), carry their own
biome color, and fade with a radial `Light`; a real-time day/night tint
(`ui.Ambient`) washes over everything. `ui` also provides a half-block "pixel"
layer (`Theme.HalfBlock`) and gradient/shimmer helpers, used by the cinematic
intro that pans the camera from the DURST WORLD title onto the play field.

### The Wilds (generative overworld)

`internal/worldgen` is a **stateless** terrain generator: every cell is a pure
function of `(seed, x, y)`, so the overworld is infinite and identical for
every session on the same seed — no chunks are stored. `internal/areas/wilds`
keeps the player's absolute world position and samples a player-centered
window each frame, rendered through the camera. A home gate (`⌂`) at the
origin returns to the lobby. Reach it from the lobby's `◈ The Wilds` portal.

See `docs/ROADMAP.md` for what's next (particles, directional avatars, and the
chat-command layer).

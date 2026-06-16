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
- `./data/durstworld.db` — SQLite (WAL) for visits, guestbook and the event
  log. Created with schema on first run; delete it to start fresh. If the
  file is unwritable the game logs a warning and plays on without memory.

### Controls

| Key | Action |
| --- | --- |
| WASD / arrows | move (walk into a `◊`/`◈` portal to enter it) |
| Y U B N | move diagonally (↖ ↗ ↙ ↘) |
| Shift + move | run (two tiles per step) |
| m | toggle the minimap (in the Wilds) |
| Enter | chat — heard within 8 tiles of where you stand |
| e | sign the guestbook (next to the lobby reception desk) |
| n / p | next/previous slide while standing on a `▣` presenter tile |
| Tab | who's online |
| q / Ctrl+C | quit (press twice) |

You spawn in **the Wilds**, the open-air hub. Walk to the landmark doors near
spawn — `⌂` Durst HQ (the lobby), `P` Presentation, `K` Kraftwerk, `D` Demo
Center — to enter each area. Players are multi-tile half-block avatars in their
own color, drawn over a 2×2 footprint.

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

   For a walkable map, embed `game.Walker` and define the map as a string
   slice plus a rune legend (see `internal/areas/kraftwerk` for the
   smallest example). `Walker.HandleCommon` gives you movement, wall
   collision, portal triggering and the portal pulse for free.

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
(the Presentation Wing's slide indices are the worked example).

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

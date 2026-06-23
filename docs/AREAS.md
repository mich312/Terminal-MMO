# Areas, portals & minigames — how it all links

Every place you can stand is an **Area** (`internal/game/area.go`). Areas are
isolated packages that self-register an id in `init()` and are imported for that
side effect in `cmd/durstworld/main.go`. You travel between them by stepping on
a **portal tile** whose `Portal` field names a destination area id; the area's
`Update` then returns a `game.Transition{To: id}` and the root model swaps in the
destination (building it from the registry with `game.NewArea`).

## The travel graph

```
                          ssh in → spawn
                               │
                     ┌─────────▼──────────┐
                     │      THE WILDS      │   the open-world hub you always
                     │  generative over-   │   return to (internal/areas/wilds)
                     │  world; landmarks   │
                     │  render as portals  │
                     └─┬───┬────┬────┬───┬──┘
   worldgen.Landmarks  │   │    │    │   │
      ⌂(0,0)    P(16,0) K(-16,0) D(0,12) A(-22,0)
        │         │      │       │       │
   ┌────▼───┐ ┌──▼──┐ ┌──▼──┐ ┌──▼─┐ ┌───▼────────────────────────┐
   │ LOBBY  │ │ PRES│ │KRAFT│ │DEMO│ │           ARCADE           │
   │  (HQ)  │ │ WING│ │WERK │ │CTR │ │     neon hall, cabinets     │
   └─┬──────┘ └──┬──┘ └──┬──┘ └─┬──┘ └─┬──┬──┬──┬──┬──┬──┬─────────┘
 '4' │ guestbook │'0'    │'0'   │'0'   │S M N T P / B Z G (cabinets)
     │           │       │      │      ▼ ▼ ▼ ▼ ▼   ▼ ▼ ▼
     └───────────┴───────┴──────┘   each cabinet is a portal into a game:
        every hall's '0' door        Sokoban Maze Snake Tetris Pong
        → "wilds" (return to hub)     Breakout Bomberman 2048 Chess Doom — each has a
                                      door/key back to the Arcade, and the
                                      Arcade's ◈ door → the Wilds.
```

Games split two ways: **keypress** (Sokoban, Maze) advance only on input;
**real-time** (Snake, Tetris, Pong, Breakout, Bomberman) implement `game.Ticker`
and run on the wall clock. The board games (Tetris, Pong, Breakout, 2048, Chess)
also implement
`game.AvatarHider` — the player isn't a token on the grid, so the camera frames
the board and no "you" avatar is drawn.

## The four kinds of link, in code

| Link | Where | How |
| --- | --- | --- |
| **World → Arcade** | `worldgen.Landmarks` (`worldgen.go`) | `{-22,0,"arcade",…}` — a portal *placed in the overworld*; the hub trail is extended + a glade added so it's reachable on any seed. (Also `lobby.go` `'4'`.) |
| **Arcade → games** | `arcade.go` legend | cabinet tiles `'S'→"sokoban"`, `'M'→"maze"`, `'N'→"snake"`; `'X'→"wilds"` is the way out. |
| **Game → Arcade** | each game | a door tile (`maze.go`) or a `Transition{To:"arcade"}` on a key/step (`sokoban.go`, `snake.go`). |
| **Hall → Wilds** | `kraftwerk/presentation/democenter` `'0'` | exit doors point to `"wilds"`, not the lobby. |

## How a transition actually happens

1. An area's `Update` (or a movement step) returns `game.Transition{To: id}`.
2. **Glyph client:** `root.go` `updateArea` catches it → `finishTransition()` sets
   `ctx.From = <area you left>`, then `NewArea(id)` + `Init`.
   **HD client:** `hd.go` `applyMove` catches it → `enterHD(ctx, from, id)`, which
   sets `ctx.From` the same way.
3. `ctx.From` is what powers **return-to-Wilds**: `wilds.go` `landmarkReturn()`
   finds the `Landmark` whose `Portal == ctx.From` and surfaces you on a walkable
   cell *beside that door* — so leaving any area drops you back in the open world
   where you went in, never at HQ.

## Two renderers, one Area

Areas return a `View(w,h) string` for the **glyph** client and a tile window
(`HDViewer.HDView`, free when you embed `game.Walker`) for the **HD pixel**
client. Walls/floors/props in the tilemap drive both. Optional area interfaces:
`Hinter`/`Prompter` (status hints), `Toaster` (transient messages),
`HDLighter` (a torch circle, used by Maze), `Ticker` (a real-time clock — see
below), `AvatarHider` (board games suppress the "you" avatar and frame the board
with the camera), and `HDFramer` (an area paints the HD pixel frame itself — the
Doom raycaster, which is neither tilemap nor board).

## Minigames: keypress vs. real-time

The HD client forwards **only key events** to an area's `Update` and ignores any
`tea.Cmd` it returns, so an area cannot schedule its own clock there.

- **Keypress games** (Sokoban, Maze) advance only when you press a key, so they
  need nothing special and play identically in both clients.
- **Real-time games** (Snake, Tetris, Pong, Breakout, Bomberman) implement
  `game.Ticker`: both clients drive `GameTick()` off a wall-clock cadence
  (`TickInterval()`), the HD loop from its frame ticker and the glyph client from
  a `tea.Tick` loop. This is the shared foundation for anything that moves on its
  own.

## Adding a game

1. New package under `internal/areas/`, embed `game.Walker`, implement `Area`
   (and `Ticker` if it's real-time).
2. `game.Register("yourgame", "Your Game", …)` in `init()`.
3. Import it for the side effect in `cmd/durstworld/main.go`.
4. Add a cabinet tile to `internal/areas/arcade` pointing `Portal: "yourgame"`
   (the `c` cabinet is the next free slot), and a door back: `Portal: "arcade"`.

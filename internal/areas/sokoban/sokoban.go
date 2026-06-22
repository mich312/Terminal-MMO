// Package sokoban is the arcade's crate-pushing puzzle: shove every crate onto
// a marked pad to clear a level, then on to the next. It is a keypress-driven
// minigame (no clock of its own — the HD client only forwards key events to an
// area, never a tick), so it plays identically in the glyph and HD clients.
//
// It embeds game.Walker for the shared tilemap/HD machinery and the player's
// position, but drives movement itself (a step may push a crate), rebuilding the
// render tilemap from the puzzle state after every move. A door tile returns to
// the Arcade.
package sokoban

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

func init() {
	game.Register("sokoban", "Sokoban", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "sokoban"}}
	})
}

// levels are hand-built and hand-verified solvable. Sokoban notation: '#' wall,
// ' ' floor, '@' player start, '$' crate, '.' goal pad, '*' crate already on a
// pad, '+' player on a pad, 'X' the door back to the Arcade.
var levels = []string{
	strings.Join([]string{
		"#######",
		"#     #",
		"#@$ . #",
		"#     #",
		"###X###",
	}, "\n"),
	strings.Join([]string{
		"#########",
		"#       #",
		"#@$  .  #",
		"#       #",
		"# $   . #",
		"#   X   #",
		"#########",
	}, "\n"),
	strings.Join([]string{
		"#######",
		"#  .  #",
		"#  $  #",
		"# @   #",
		"#  $  #",
		"#  .  #",
		"##X####",
	}, "\n"),
}

type pt = [2]int

type area struct {
	game.Walker
	idx     int         // current level
	walls   map[pt]bool // immovable wall cells
	goals   map[pt]bool // target pads
	boxes   map[pt]bool // crate positions (the mutable state)
	exit    pt          // the door back to the Arcade
	w, h    int         // level bounds
	moves    int         // steps taken this level
	pushes   int         // crates shoved this level
	cleared  int         // levels solved this visit
	finished bool        // the final level is solved (so we stop re-counting it)

	toast      string
	toastUntil time.Time
}

func (a *area) Name() string { return "Sokoban" }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.idx = 0
	a.cleared = 0
	a.load(a.idx)
	return nil
}

// load parses level i into the puzzle state and announces our position.
func (a *area) load(i int) {
	a.walls = map[pt]bool{}
	a.goals = map[pt]bool{}
	a.boxes = map[pt]bool{}
	a.moves, a.pushes = 0, 0
	a.finished = false
	rows := strings.Split(levels[i], "\n")
	a.h = len(rows)
	a.w = 0
	for y, row := range rows {
		if n := len([]rune(row)); n > a.w {
			a.w = n
		}
		for x, ch := range []rune(row) {
			c := pt{x, y}
			switch ch {
			case '#':
				a.walls[c] = true
			case '$':
				a.boxes[c] = true
			case '*':
				a.boxes[c], a.goals[c] = true, true
			case '.':
				a.goals[c] = true
			case '+':
				a.goals[c] = true
				a.X, a.Y = x, y
			case '@':
				a.X, a.Y = x, y
			case 'X':
				a.exit = c
			}
		}
	}
	a.rebuild()
	a.Ctx.World.EnterArea(a.Ctx.Name, a.AreaID, a.X, a.Y, a.Name())
}

// rebuild regenerates the render tilemap from the current puzzle state. The
// player avatar is drawn separately by the renderer from world position, so it
// is not stamped here.
func (a *area) rebuild() {
	tiles := make([][]game.Tile, a.h)
	for y := 0; y < a.h; y++ {
		row := make([]game.Tile, a.w)
		for x := 0; x < a.w; x++ {
			row[x] = a.tileAt(pt{x, y})
		}
		tiles[y] = row
	}
	a.Map = &game.TileMap{W: a.w, H: a.h, Tiles: tiles}
}

func (a *area) tileAt(c pt) game.Tile {
	switch {
	case c == a.exit:
		return game.Tile{Kind: game.TilePortal, Ch: '◊', Walkable: true,
			Portal: "arcade", Label: "Arcade", Color: ui.HexAccent}
	case a.walls[c]:
		return game.Tile{Kind: game.TileWall, Ch: '█', Tex: game.TexBrick, Ground: "#3A3550"}
	case a.boxes[c] && a.goals[c]:
		// A crate home on its pad — green, to read as "done".
		return game.Tile{Kind: game.TileDecor, Ch: '▣', Walkable: false,
			Tex: game.TexFloor, Ground: "#243024", Prop: game.PropCrate, PropHex: "#7BD88F"}
	case a.boxes[c]:
		return game.Tile{Kind: game.TileDecor, Ch: '▢', Walkable: false,
			Tex: game.TexFloor, Ground: "#2A2433", Prop: game.PropCrate, PropHex: "#C9A24B"}
	case a.goals[c]:
		// An empty target pad — a glowing inlay in the floor.
		return game.Tile{Kind: game.TileFloor, Ch: '◇', Walkable: true,
			Tex: game.TexFloor, Ground: "#3A3320", Prop: game.PropGemGlow, PropHex: "#FFD166"}
	default:
		return game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true,
			Tex: game.TexFloor, Ground: "#221E2B"}
	}
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	switch msg := msg.(type) {
	case game.WorldEventMsg:
		if ev := world.Event(msg); ev.Type == world.EventTick {
			a.Pulse, a.Frame = ev.Pulse, int(ev.Frame)
		}
		return a, nil
	case tea.KeyMsg:
		if msg.String() == "r" { // restart the current level
			a.load(a.idx)
			a.setToast("level reset")
			return a, nil
		}
		if dx, dy, _, ok := game.MoveKey(msg.String()); ok {
			return a.step(dx, dy)
		}
	}
	return a, nil
}

// step attempts a single move in (dx,dy): into floor it walks, into a crate it
// pushes (if the cell beyond is clear), into a wall it stops. Stepping onto the
// door returns to the Arcade.
func (a *area) step(dx, dy int) (game.Area, tea.Cmd) {
	n := pt{a.X + dx, a.Y + dy}
	if a.walls[n] {
		return a, nil
	}
	if a.boxes[n] {
		beyond := pt{n[0] + dx, n[1] + dy}
		if a.walls[beyond] || a.boxes[beyond] {
			return a, nil // crate is wedged
		}
		delete(a.boxes, n)
		a.boxes[beyond] = true
		a.pushes++
	}
	a.X, a.Y = n[0], n[1]
	a.moves++
	a.Ctx.World.Move(a.Ctx.Name, a.X, a.Y)
	a.rebuild()

	if n == a.exit {
		return game.Transition{To: "arcade"}, nil
	}
	if a.solved() && !a.finished {
		if a.idx+1 < len(levels) {
			a.cleared++
			a.idx++
			a.load(a.idx)
			a.setToast(fmt.Sprintf("solved! level %d of %d", a.idx+1, len(levels)))
		} else {
			a.cleared++
			a.finished = true
			a.setToast("🏆 all puzzles solved! step on the door to leave")
		}
	}
	return a, nil
}

// solved reports whether every goal pad currently holds a crate.
func (a *area) solved() bool {
	for g := range a.goals {
		if !a.boxes[g] {
			return false
		}
	}
	return len(a.goals) > 0
}

func (a *area) remaining() int {
	n := 0
	for g := range a.goals {
		if !a.boxes[g] {
			n++
		}
	}
	return n
}

func (a *area) setToast(s string) {
	a.toast = s
	a.toastUntil = time.Now().Add(4 * time.Second)
}

// Toast implements game.Toaster for the HD client.
func (a *area) Toast() (string, bool) {
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		return a.toast, true
	}
	return "", false
}

func (a *area) Hint() string {
	return fmt.Sprintf("level %d/%d · %d pads left · WASD/↑↓←→ push · r reset · door ◊ leaves",
		a.idx+1, len(levels), a.remaining())
}

// Prompt implements game.Prompter for the HD bottom-of-screen action line.
func (a *area) Prompt() (string, bool) {
	if a.X == a.exit[0] && a.Y == a.exit[1] {
		return "step out to the Arcade", true
	}
	return fmt.Sprintf("push crates onto the ◇ pads · %d left · r reset", a.remaining()), true
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	rows := []string{
		th.PanelTitle.Render("📦 Sokoban"), "",
		th.ChatText.Render("Shove every crate onto a"),
		th.ChatText.Render("glowing ◇ pad."), "",
		th.Dim.Render("Level   ") + th.Bright.Render(fmt.Sprintf("%d / %d", a.idx+1, len(levels))),
		th.Dim.Render("Pads    ") + th.Accent.Render(fmt.Sprintf("%d left", a.remaining())),
		th.Dim.Render("Moves   ") + th.ChatText.Render(fmt.Sprintf("%d", a.moves)),
		th.Dim.Render("Pushes  ") + th.ChatText.Render(fmt.Sprintf("%d", a.pushes)), "",
		th.Dim.Render("WASD / arrows  push"),
		th.Dim.Render("r              reset"),
		th.Dim.Render("◊ door         leave"),
	}
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		rows = append(rows, "", th.Accent.Render(a.toast))
	}
	panel := th.Panel.Width(30).Render(strings.Join(rows, "\n"))

	const gap = 3
	mapW := width - lipgloss.Width(panel) - gap
	if mapW < 24 {
		mapW = 24
	}
	mapView := a.RenderViewport(mapW, height)
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", mapView)
}

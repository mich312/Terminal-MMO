// Package tetris is the arcade's falling-blocks classic: rotate and slot the
// tetrominoes to clear lines. It's a game.Ticker (gravity runs on the wall
// clock) and a board game (camera-framed, no avatar). Left/right move, up
// rotates, down soft-drops; 'r' restarts, 'x' leaves.
package tetris

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

const (
	playW = 10 // well width in cells
	playH = 20 // well height in cells
	tileW = playW + 2
	tileH = playH + 1 // + bottom wall
)

type pt = [2]int

// shapes[piece][rotation] is the four cells of a tetromino in a 4×4 box (y down).
var shapes = [7][4][4]pt{
	{ // I
		{{0, 1}, {1, 1}, {2, 1}, {3, 1}}, {{2, 0}, {2, 1}, {2, 2}, {2, 3}},
		{{0, 2}, {1, 2}, {2, 2}, {3, 2}}, {{1, 0}, {1, 1}, {1, 2}, {1, 3}},
	},
	{ // O
		{{1, 0}, {2, 0}, {1, 1}, {2, 1}}, {{1, 0}, {2, 0}, {1, 1}, {2, 1}},
		{{1, 0}, {2, 0}, {1, 1}, {2, 1}}, {{1, 0}, {2, 0}, {1, 1}, {2, 1}},
	},
	{ // T
		{{1, 0}, {0, 1}, {1, 1}, {2, 1}}, {{1, 0}, {1, 1}, {2, 1}, {1, 2}},
		{{0, 1}, {1, 1}, {2, 1}, {1, 2}}, {{1, 0}, {0, 1}, {1, 1}, {1, 2}},
	},
	{ // S
		{{1, 0}, {2, 0}, {0, 1}, {1, 1}}, {{1, 0}, {1, 1}, {2, 1}, {2, 2}},
		{{1, 1}, {2, 1}, {0, 2}, {1, 2}}, {{0, 0}, {0, 1}, {1, 1}, {1, 2}},
	},
	{ // Z
		{{0, 0}, {1, 0}, {1, 1}, {2, 1}}, {{2, 0}, {1, 1}, {2, 1}, {1, 2}},
		{{0, 1}, {1, 1}, {1, 2}, {2, 2}}, {{1, 0}, {0, 1}, {1, 1}, {0, 2}},
	},
	{ // J
		{{0, 0}, {0, 1}, {1, 1}, {2, 1}}, {{1, 0}, {2, 0}, {1, 1}, {1, 2}},
		{{0, 1}, {1, 1}, {2, 1}, {2, 2}}, {{1, 0}, {1, 1}, {0, 2}, {1, 2}},
	},
	{ // L
		{{2, 0}, {0, 1}, {1, 1}, {2, 1}}, {{1, 0}, {1, 1}, {1, 2}, {2, 2}},
		{{0, 1}, {1, 1}, {2, 1}, {0, 2}}, {{0, 0}, {1, 0}, {1, 1}, {1, 2}},
	},
}

var colors = [7]string{"#56E1FF", "#FFD166", "#C792EA", "#7BD88F", "#FF5D5D", "#2E8BFF", "#FF8A4C"}

func init() {
	game.Register("tetris", "Tetris", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "tetris"}}
	})
}

type area struct {
	game.Walker
	grid       [playH][playW]int // 0 empty, else piece+1 (for colour)
	piece      int               // current piece type
	rot        int
	px, py     int // 4×4 box origin in well coords
	bag        []int
	rng        *rand.Rand
	lines      int
	score      int
	over       bool
	toast      string
	toastUntil time.Time
}

func (a *area) Name() string      { return "Tetris" }
func (a *area) HideAvatars() bool { return true }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	a.reset()
	return nil
}

func (a *area) reset() {
	a.grid = [playH][playW]int{}
	a.bag = nil
	a.lines, a.score = 0, 0
	a.over = false
	a.spawn()
	a.X, a.Y = tileW/2, tileH/2
	a.rebuild()
	a.Ctx.World.EnterArea(a.Ctx.Name, a.AreaID, a.X, a.Y, a.Name())
}

// next draws the next piece from a shuffled 7-bag (even distribution).
func (a *area) next() int {
	if len(a.bag) == 0 {
		a.bag = []int{0, 1, 2, 3, 4, 5, 6}
		a.rng.Shuffle(len(a.bag), func(i, j int) { a.bag[i], a.bag[j] = a.bag[j], a.bag[i] })
	}
	p := a.bag[len(a.bag)-1]
	a.bag = a.bag[:len(a.bag)-1]
	return p
}

func (a *area) spawn() {
	a.piece, a.rot = a.next(), 0
	a.px, a.py = 3, -1
	if !a.valid(a.px, a.py, a.rot) {
		a.over = true
		a.setToast(fmt.Sprintf("game over · %d lines · r restart · x leave", a.lines))
	}
}

// cells returns the absolute well coords of the piece at the given placement.
func (a *area) cells(px, py, rot int) [4]pt {
	var out [4]pt
	for i, c := range shapes[a.piece][rot] {
		out[i] = pt{px + c[0], py + c[1]}
	}
	return out
}

// valid reports whether the piece fits at a placement (walls, floor, stack).
func (a *area) valid(px, py, rot int) bool {
	for _, c := range a.cells(px, py, rot) {
		x, y := c[0], c[1]
		if x < 0 || x >= playW || y >= playH {
			return false
		}
		if y >= 0 && a.grid[y][x] != 0 {
			return false
		}
	}
	return true
}

func (a *area) TickInterval() time.Duration {
	ms := 500 - (a.lines/10)*45
	if ms < 100 {
		ms = 100
	}
	return time.Duration(ms) * time.Millisecond
}

func (a *area) GameTick() game.Area {
	if a.over {
		return a
	}
	a.gravity()
	a.rebuild()
	return a
}

// gravity drops the piece one row, locking and respawning when it can't fall.
func (a *area) gravity() {
	if a.valid(a.px, a.py+1, a.rot) {
		a.py++
		return
	}
	a.lock()
	if !a.over {
		a.clearLines()
		a.spawn()
	}
}

func (a *area) lock() {
	for _, c := range a.cells(a.px, a.py, a.rot) {
		if c[1] < 0 {
			a.over = true // stacked out the top
			a.setToast(fmt.Sprintf("game over · %d lines · r restart · x leave", a.lines))
			continue
		}
		a.grid[c[1]][c[0]] = a.piece + 1
	}
}

func (a *area) clearLines() {
	cleared := 0
	for y := playH - 1; y >= 0; {
		full := true
		for x := 0; x < playW; x++ {
			if a.grid[y][x] == 0 {
				full = false
				break
			}
		}
		if !full {
			y--
			continue
		}
		// drop everything above row y down by one
		for yy := y; yy > 0; yy-- {
			a.grid[yy] = a.grid[yy-1]
		}
		a.grid[0] = [playW]int{}
		cleared++
		// re-test the same row (now holding what fell into it)
	}
	if cleared > 0 {
		a.lines += cleared
		a.score += []int{0, 100, 300, 500, 800}[cleared] // 1/2/3/4-line rewards
	}
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	switch key.String() {
	case "x":
		return game.Transition{To: "arcade"}, nil
	case "r":
		a.reset()
		return a, nil
	}
	if a.over {
		return a, nil
	}
	dx, dy, _, ok := game.MoveKey(key.String())
	if !ok {
		return a, nil
	}
	switch {
	case dx != 0 && dy == 0: // shift
		if a.valid(a.px+dx, a.py, a.rot) {
			a.px += dx
		}
	case dy == -1 && dx == 0: // rotate, with simple wall kicks
		nr := (a.rot + 1) % 4
		for _, kick := range []int{0, -1, 1, -2, 2} {
			if a.valid(a.px+kick, a.py, nr) {
				a.px, a.rot = a.px+kick, nr
				break
			}
		}
	case dy == 1 && dx == 0: // soft drop
		a.gravity()
	}
	a.rebuild()
	return a, nil
}

func (a *area) rebuild() {
	tiles := make([][]game.Tile, tileH)
	for y := 0; y < tileH; y++ {
		row := make([]game.Tile, tileW)
		for x := 0; x < tileW; x++ {
			if x == 0 || x == tileW-1 || y == tileH-1 {
				row[x] = game.Tile{Kind: game.TileWall, Ch: '█', Tex: game.TexBrick, Ground: "#2A3550"}
			} else {
				row[x] = game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexFloor, Ground: "#121620"}
			}
		}
		tiles[y] = row
	}
	put := func(gx, gy, kind int) {
		if gy < 0 || gy >= playH || gx < 0 || gx >= playW {
			return
		}
		hex := colors[kind]
		tiles[gy][gx+1] = game.Tile{Kind: game.TileDecor, Ch: '█', Walkable: false,
			Tex: game.TexFloor, Ground: hex, Color: hex}
	}
	for y := 0; y < playH; y++ {
		for x := 0; x < playW; x++ {
			if a.grid[y][x] != 0 {
				put(x, y, a.grid[y][x]-1)
			}
		}
	}
	if !a.over {
		for _, c := range a.cells(a.px, a.py, a.rot) {
			put(c[0], c[1], a.piece)
		}
	}
	a.Map = &game.TileMap{W: tileW, H: tileH, Tiles: tiles}
}

func (a *area) setToast(s string) { a.toast, a.toastUntil = s, time.Now().Add(6*time.Second) }

func (a *area) Toast() (string, bool) {
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		return a.toast, true
	}
	return "", false
}

func (a *area) Hint() string {
	return fmt.Sprintf("lines %d · score %d · ←→ move · ↑ rotate · ↓ drop · x leave", a.lines, a.score)
}

func (a *area) Prompt() (string, bool) {
	if a.over {
		return "game over · r restart · x leave", true
	}
	return "←→ move · ↑ rotate · ↓ soft-drop · x leave", true
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	rows := []string{
		th.PanelTitle.Render("🟦 Tetris"), "",
		th.ChatText.Render("Clear lines. Don't"),
		th.ChatText.Render("stack to the top."), "",
		th.Dim.Render("Lines   ") + th.Bright.Render(fmt.Sprintf("%d", a.lines)),
		th.Dim.Render("Score   ") + th.Accent.Render(fmt.Sprintf("%d", a.score)),
		th.Dim.Render("Level   ") + th.ChatText.Render(fmt.Sprintf("%d", a.lines/10)), "",
		th.Dim.Render("← →     move"),
		th.Dim.Render("↑ / W   rotate"),
		th.Dim.Render("↓ / S   soft-drop"),
		th.Dim.Render("r       restart"),
		th.Dim.Render("x       leave"),
	}
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		rows = append(rows, "", th.Accent.Render(a.toast))
	}
	panel := th.Panel.Width(28).Render(strings.Join(rows, "\n"))

	const gap = 3
	mapW := width - lipgloss.Width(panel) - gap
	if mapW < 24 {
		mapW = 24
	}
	mapView := a.RenderBoard(mapW, height)
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", mapView)
}

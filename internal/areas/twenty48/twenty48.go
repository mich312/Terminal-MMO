// Package twenty48 is the arcade's sliding-tile puzzle (2048): shove the board,
// merge equal tiles, and chase the big numbers. It's keypress-driven (no clock)
// and a board game (camera-framed, no avatar). The glyph client shows the
// numbers; the HD client reads value by colour, the way the original game does.
// Arrow/WASD slide, 'r' restarts, 'x' leaves.
package twenty48

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
	size  = 4 // 4×4 board
	cell  = 3 // HD: each value is a cell×cell block of coloured tiles
	gap   = 1 // …with a one-tile gutter between blocks
	tileW = gap + size*(cell+gap)
)

func init() {
	game.Register("2048", "2048", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "2048"}}
	})
}

type area struct {
	game.Walker
	grid       [size][size]int
	score      int
	best       int
	won        bool // reached 2048 (play continues)
	over       bool // no moves left
	rng        *rand.Rand
	toast      string
	toastUntil time.Time
}

func (a *area) Name() string      { return "2048" }
func (a *area) HideAvatars() bool { return true }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	a.reset()
	return nil
}

func (a *area) reset() {
	a.grid = [size][size]int{}
	a.score = 0
	a.won, a.over = false, false
	a.spawn()
	a.spawn()
	a.X, a.Y = tileW/2, tileW/2
	a.rebuild()
	a.Ctx.World.EnterArea(a.Ctx.Name, a.AreaID, a.X, a.Y, a.Name())
}

// spawn drops a 2 (90%) or 4 (10%) onto a random empty square.
func (a *area) spawn() {
	var empty [][2]int
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if a.grid[y][x] == 0 {
				empty = append(empty, [2]int{x, y})
			}
		}
	}
	if len(empty) == 0 {
		return
	}
	c := empty[a.rng.Intn(len(empty))]
	v := 2
	if a.rng.Float64() < 0.1 {
		v = 4
	}
	a.grid[c[1]][c[0]] = v
}

// compress slides one line towards index 0, merging equal neighbours once each.
func compress(line [size]int) ([size]int, int, bool) {
	var packed []int
	for _, v := range line {
		if v != 0 {
			packed = append(packed, v)
		}
	}
	var out [size]int
	gained, i, w := 0, 0, 0
	for i < len(packed) {
		if i+1 < len(packed) && packed[i] == packed[i+1] {
			out[w] = packed[i] * 2
			gained += out[w]
			i += 2
		} else {
			out[w] = packed[i]
			i++
		}
		w++
	}
	moved := out != line
	return out, gained, moved
}

// slide applies a move; dir is one of "left","right","up","down".
func (a *area) slide(dir string) bool {
	moved := false
	switch dir {
	case "left", "right":
		for y := 0; y < size; y++ {
			line := a.grid[y]
			if dir == "right" {
				line = reverse(line)
			}
			out, g, m := compress(line)
			if dir == "right" {
				out = reverse(out)
			}
			a.grid[y] = out
			a.score += g
			moved = moved || m
		}
	case "up", "down":
		for x := 0; x < size; x++ {
			var line [size]int
			for y := 0; y < size; y++ {
				line[y] = a.grid[y][x]
			}
			if dir == "down" {
				line = reverse(line)
			}
			out, g, m := compress(line)
			if dir == "down" {
				out = reverse(out)
			}
			for y := 0; y < size; y++ {
				a.grid[y][x] = out[y]
			}
			a.score += g
			moved = moved || m
		}
	}
	return moved
}

func reverse(l [size]int) [size]int {
	var r [size]int
	for i := range l {
		r[i] = l[size-1-i]
	}
	return r
}

// movesLeft reports whether any slide or merge is still possible.
func (a *area) movesLeft() bool {
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if a.grid[y][x] == 0 {
				return true
			}
			if x+1 < size && a.grid[y][x] == a.grid[y][x+1] {
				return true
			}
			if y+1 < size && a.grid[y][x] == a.grid[y+1][x] {
				return true
			}
		}
	}
	return false
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
	if !ok || (dx == 0) == (dy == 0) {
		return a, nil
	}
	dir := map[[2]int]string{{-1, 0}: "left", {1, 0}: "right", {0, -1}: "up", {0, 1}: "down"}[[2]int{dx, dy}]
	if a.slide(dir) {
		a.spawn()
		if a.score > a.best {
			a.best = a.score
		}
		if !a.won && a.max() >= 2048 {
			a.won = true
			a.setToast("🏆 2048! keep going · x leave")
		}
		if !a.movesLeft() {
			a.over = true
			a.setToast(fmt.Sprintf("no moves left · score %d · r restart · x leave", a.score))
		}
		a.rebuild()
	}
	return a, nil
}

func (a *area) max() int {
	m := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if a.grid[y][x] > m {
				m = a.grid[y][x]
			}
		}
	}
	return m
}

// valueHex maps a tile value to its colour (HD reads value by colour).
func valueHex(v int) string {
	switch v {
	case 0:
		return "#26222E"
	case 2:
		return "#8894A8"
	case 4:
		return "#56A0D8"
	case 8:
		return "#56E1FF"
	case 16:
		return "#7BD88F"
	case 32:
		return "#C7E86A"
	case 64:
		return "#FFD166"
	case 128:
		return "#FFB04C"
	case 256:
		return "#FF8A4C"
	case 512:
		return "#FF6B6B"
	case 1024:
		return "#E0608A"
	case 2048:
		return "#C792EA"
	default:
		return "#FF5DF0"
	}
}

func (a *area) rebuild() {
	tiles := make([][]game.Tile, tileW)
	for y := 0; y < tileW; y++ {
		row := make([]game.Tile, tileW)
		for x := 0; x < tileW; x++ {
			row[x] = game.Tile{Kind: game.TileFloor, Ch: ' ', Walkable: true, Tex: game.TexFloor, Ground: "#15131C"}
		}
		tiles[y] = row
	}
	for gy := 0; gy < size; gy++ {
		for gx := 0; gx < size; gx++ {
			hex := valueHex(a.grid[gy][gx])
			ox, oy := gap+gx*(cell+gap), gap+gy*(cell+gap)
			for dy := 0; dy < cell; dy++ {
				for dx := 0; dx < cell; dx++ {
					tiles[oy+dy][ox+dx] = game.Tile{Kind: game.TileDecor, Ch: '█', Walkable: false,
						Tex: game.TexFloor, Ground: hex, Color: hex}
				}
			}
		}
	}
	a.Map = &game.TileMap{W: tileW, H: tileW, Tiles: tiles}
}

func (a *area) setToast(s string) { a.toast, a.toastUntil = s, time.Now().Add(5*time.Second) }

func (a *area) Toast() (string, bool) {
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		return a.toast, true
	}
	return "", false
}

func (a *area) Hint() string {
	return fmt.Sprintf("score %d · best %d · arrows/WASD slide · r restart · x leave", a.score, a.best)
}

func (a *area) Prompt() (string, bool) {
	if a.over {
		return "no moves left · r restart · x leave", true
	}
	return "slide with arrows/WASD · merge to 2048 · x leave", true
}

// boardGlyph draws the numeric board for the glyph client (HD reads colour).
func (a *area) boardGlyph(th *ui.Theme) string {
	var b strings.Builder
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			v := a.grid[y][x]
			txt := "    ·"
			if v != 0 {
				txt = fmt.Sprintf("%5d", v)
			}
			b.WriteString(th.Fg(lipgloss.Color(valueHex(v))).Render(txt) + " ")
		}
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	rows := []string{
		th.PanelTitle.Render("🔢 2048"), "",
		th.ChatText.Render("Merge tiles to reach"),
		th.ChatText.Render("2048."), "",
		th.Dim.Render("Score   ") + th.Bright.Render(fmt.Sprintf("%d", a.score)),
		th.Dim.Render("Best    ") + th.Accent.Render(fmt.Sprintf("%d", a.best)),
		th.Dim.Render("Max     ") + th.Fg(lipgloss.Color(valueHex(a.max()))).Render(fmt.Sprintf("%d", a.max())), "",
		th.Dim.Render("arrows / WASD  slide"),
		th.Dim.Render("r              restart"),
		th.Dim.Render("x              leave"),
	}
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		rows = append(rows, "", th.Accent.Render(a.toast))
	}
	panel := th.Panel.Width(28).Render(strings.Join(rows, "\n"))
	board := th.Panel.Render(a.boardGlyph(th))
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", board)
}

// Package doom is the arcade's first-person raycaster — a tiny "Doom": walk a
// maze of walls rendered in pseudo-3D, find the exit. It is neither a tilemap
// nor a board: the glyph client draws an ASCII first-person view from View(),
// and the HD client paints the pixel frame directly via game.HDFramer. It still
// embeds game.Walker (with a 1×1 stand-in map) so the HD client accepts it and
// skips the avatar. W/S walk, A/D turn (arrows too); 'r' resets, 'x' leaves.
package doom

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// level: '#'/'1'/'2' walls (different hues), '.' floor, 'S' start, 'E' exit.
var level = []string{
	"################",
	"#S.....#......2#",
	"#.####.#.####.2#",
	"#.#..#.#....#..#",
	"#.#.2#.####.##.#",
	"#.#.2#....#....#",
	"#.#.####.###.###",
	"#.#....#.....#.#",
	"#.####.#####.#.#",
	"#....#.....#.#.#",
	"####.####.##.#.#",
	"#......#.....#.#",
	"#.####.#.#####.#",
	"#.#....#.....#E#",
	"#.#.########.#.#",
	"################",
}

const (
	fov      = 0.66 // camera-plane half-width → ~66° field of view
	moveStep = 0.25
	turnStep = 0.20 // radians per key
)

func init() {
	game.Register("doom", "Doom", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "doom",
			Map: &game.TileMap{W: 1, H: 1, Tiles: [][]game.Tile{{{Kind: game.TileFloor, Walkable: true, Ground: "#101014"}}}}}}
	})
}

type area struct {
	game.Walker
	grid           [][]byte
	w, h           int
	exit           [2]int
	px, py         float64 // player position (map units)
	dirX, dirY     float64 // facing
	planeX, planeY float64 // camera plane
	wins           int
	toast          string
	toastUnt       time.Time
}

func (a *area) Name() string      { return "Doom" }
func (a *area) HideAvatars() bool { return true }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.load()
	return nil
}

func (a *area) load() {
	a.h = len(level)
	a.w = len(level[0])
	a.grid = make([][]byte, a.h)
	for y, row := range level {
		a.grid[y] = []byte(row)
		for x := 0; x < len(row); x++ {
			switch row[x] {
			case 'S':
				a.px, a.py = float64(x)+0.5, float64(y)+0.5
				a.grid[y][x] = '.'
			case 'E':
				a.exit = [2]int{x, y}
			}
		}
	}
	// Face east to start.
	a.dirX, a.dirY = 1, 0
	a.planeX, a.planeY = 0, fov
	a.Ctx.World.EnterArea(a.Ctx.Name, a.AreaID, 0, 0, a.Name())
}

func (a *area) wall(x, y int) bool {
	if y < 0 || y >= a.h || x < 0 || x >= a.w {
		return true
	}
	c := a.grid[y][x]
	return c == '#' || c == '1' || c == '2'
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
		a.load()
		a.setToast("reset")
		return a, nil
	}
	dx, dy, _, ok := game.MoveKey(key.String())
	if !ok || (dx != 0 && dy != 0) {
		return a, nil
	}
	switch {
	case dy < 0:
		a.step(+1)
	case dy > 0:
		a.step(-1)
	case dx < 0:
		a.rotate(-turnStep)
	case dx > 0:
		a.rotate(+turnStep)
	}
	return a, nil
}

// step walks forward (s=+1) or back (s=−1), sliding along walls it bumps.
func (a *area) step(s float64) {
	nx := a.px + a.dirX*moveStep*s
	ny := a.py + a.dirY*moveStep*s
	if !a.wall(int(nx), int(a.py)) {
		a.px = nx
	}
	if !a.wall(int(a.px), int(ny)) {
		a.py = ny
	}
	if int(a.px) == a.exit[0] && int(a.py) == a.exit[1] {
		a.wins++
		a.setToast("🏆 found the exit! · r play again · x leave")
		a.load()
	}
}

func (a *area) rotate(t float64) {
	cos, sin := math.Cos(t), math.Sin(t)
	a.dirX, a.dirY = a.dirX*cos-a.dirY*sin, a.dirX*sin+a.dirY*cos
	a.planeX, a.planeY = a.planeX*cos-a.planeY*sin, a.planeX*sin+a.planeY*cos
}

// cast does a DDA ray march for camera column cx∈[-1,1], returning the
// perpendicular wall distance, which side was hit (0 NS, 1 EW) and the wall byte.
func (a *area) cast(cx float64) (dist float64, side int, cell byte) {
	rdx := a.dirX + a.planeX*cx
	rdy := a.dirY + a.planeY*cx
	mapX, mapY := int(a.px), int(a.py)
	ddx, ddy := math.Inf(1), math.Inf(1)
	if rdx != 0 {
		ddx = math.Abs(1 / rdx)
	}
	if rdy != 0 {
		ddy = math.Abs(1 / rdy)
	}
	stepX, stepY := 1, 1
	var sdx, sdy float64
	if rdx < 0 {
		stepX, sdx = -1, (a.px-float64(mapX))*ddx
	} else {
		sdx = (float64(mapX) + 1 - a.px) * ddx
	}
	if rdy < 0 {
		stepY, sdy = -1, (a.py-float64(mapY))*ddy
	} else {
		sdy = (float64(mapY) + 1 - a.py) * ddy
	}
	for i := 0; i < 64; i++ {
		if sdx < sdy {
			sdx += ddx
			mapX += stepX
			side = 0
		} else {
			sdy += ddy
			mapY += stepY
			side = 1
		}
		if a.wall(mapX, mapY) {
			cell = a.cellByte(mapX, mapY)
			break
		}
	}
	if side == 0 {
		dist = sdx - ddx
	} else {
		dist = sdy - ddy
	}
	if dist < 0.01 {
		dist = 0.01
	}
	return
}

func (a *area) cellByte(x, y int) byte {
	if y < 0 || y >= a.h || x < 0 || x >= a.w {
		return '#'
	}
	return a.grid[y][x]
}

// ── HD: paint the pixel frame in first person ────────────────────────────────

func wallRGBA(cell byte, side int, dist float64) color.RGBA {
	var r, g, b float64
	switch cell {
	case '2':
		r, g, b = 0x2E, 0x8B, 0xFF // blue brick
	case '1':
		r, g, b = 0x7B, 0xD8, 0x8F // green
	default:
		r, g, b = 0xC2, 0x4A, 0x3A // red brick
	}
	if side == 1 { // EW faces shaded darker, like Doom
		r, g, b = r*0.66, g*0.66, b*0.66
	}
	fog := 1.0 / (1.0 + dist*dist*0.04) // darken with distance
	return color.RGBA{uint8(r * fog), uint8(g * fog), uint8(b * fog), 0xFF}
}

func (a *area) HDFrame(img *image.RGBA) {
	b := img.Bounds()
	W, H := b.Dx(), b.Dy()
	ceil := color.RGBA{0x1A, 0x1C, 0x26, 0xFF}
	floor := color.RGBA{0x26, 0x22, 0x1E, 0xFF}
	for x := 0; x < W; x++ {
		cx := 2*float64(x)/float64(W) - 1
		dist, side, cell := a.cast(cx)
		lineH := int(float64(H) / dist)
		top := H/2 - lineH/2
		bot := H/2 + lineH/2
		wc := wallRGBA(cell, side, dist)
		for y := 0; y < H; y++ {
			c := ceil
			if y >= top && y <= bot {
				c = wc
			} else if y > bot {
				c = floor
			}
			img.SetRGBA(b.Min.X+x, b.Min.Y+y, c)
		}
	}
}

// ── Glyph: an ASCII first-person view ────────────────────────────────────────

func shadeRune(dist float64, side int) rune {
	switch {
	case dist < 2:
		if side == 1 {
			return '▓'
		}
		return '█'
	case dist < 4:
		if side == 1 {
			return '▒'
		}
		return '▓'
	case dist < 7:
		return '▒'
	case dist < 11:
		return '░'
	default:
		return ' '
	}
}

func (a *area) raycastText(cols, rows int) string {
	if cols < 1 || rows < 1 {
		return ""
	}
	grid := make([][]rune, rows)
	for y := range grid {
		grid[y] = make([]rune, cols)
		for x := range grid[y] {
			switch {
			case y < rows/2:
				grid[y][x] = ' ' // ceiling
			default:
				grid[y][x] = '.' // floor
			}
		}
	}
	for x := 0; x < cols; x++ {
		cx := 2*float64(x)/float64(cols) - 1
		dist, side, _ := a.cast(cx)
		lineH := int(float64(rows) / dist)
		if lineH > rows {
			lineH = rows
		}
		top := rows/2 - lineH/2
		if top < 0 {
			top = 0
		}
		ch := shadeRune(dist, side)
		if ch == ' ' {
			continue
		}
		for y := top; y < top+lineH && y < rows; y++ {
			grid[y][x] = ch
		}
	}
	var b strings.Builder
	for y, row := range grid {
		if y > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(string(row))
	}
	return b.String()
}

func (a *area) setToast(s string) { a.toast, a.toastUnt = s, time.Now().Add(5*time.Second) }

func (a *area) Toast() (string, bool) {
	if a.toast != "" && time.Now().Before(a.toastUnt) {
		return a.toast, true
	}
	return "", false
}

func (a *area) Hint() string {
	return "W/S walk · A/D turn · find the exit · r reset · x leave"
}

func (a *area) Prompt() (string, bool) {
	return "W/S walk · A/D turn · find the exit · x leave", true
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	rows := []string{
		th.PanelTitle.Render("🔫 Doom"), "",
		th.ChatText.Render("Find the exit in the"),
		th.ChatText.Render("maze. (HD looks best.)"), "",
		th.Dim.Render("Cleared ") + th.Accent.Render(itoa(a.wins)), "",
		th.Dim.Render("W / S   walk"),
		th.Dim.Render("A / D   turn"),
		th.Dim.Render("r       reset"),
		th.Dim.Render("x       leave"),
	}
	if a.toast != "" && time.Now().Before(a.toastUnt) {
		rows = append(rows, "", th.Accent.Render(a.toast))
	}
	panel := th.Panel.Width(26).Render(strings.Join(rows, "\n"))

	const gap = 3
	viewW := width - lipgloss.Width(panel) - gap
	if viewW < 24 {
		viewW = 24
	}
	scene := th.Bright.Render(a.raycastText(viewW, height))
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", scene)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	if neg {
		d = append([]byte{'-'}, d...)
	}
	return string(d)
}

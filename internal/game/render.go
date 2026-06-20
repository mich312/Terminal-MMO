package game

import (
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// PlayerW, PlayerH is a player's collision footprint in tiles. A single tile,
// so the hero reads as one small figure in a large world; the HD avatar sprite
// is drawn to roughly fill that tile, while the glyph renderer shows a single
// token (it can't fit a face in one cell).
const (
	PlayerW = 1
	PlayerH = 1
)

// Camera is the window of a (possibly larger-than-screen) map that is drawn.
type Camera struct {
	X, Y, W, H int
}

// CameraOn returns a camera of size vw×vh centered on (cx,cy) and clamped so
// it never shows past the map edges.
func CameraOn(tm *TileMap, cx, cy, vw, vh int) Camera {
	w, h := vw, vh
	if w > tm.W {
		w = tm.W
	}
	if h > tm.H {
		h = tm.H
	}
	x := cx - w/2
	y := cy - h/2
	if x > tm.W-w {
		x = tm.W - w
	}
	if y > tm.H-h {
		y = tm.H - h
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return Camera{X: x, Y: y, W: w, H: h}
}

// Light is a radial light source: tiles fade toward darkness past Radius from
// (X,Y) in world coordinates. A zero Radius means uniform light. Warm makes it a
// lantern rather than a vignette — a generous, soft-edged warm pool over a faint
// ambient floor, the way a held light reads underground.
type Light struct {
	X, Y, Radius int
	Warm         bool
}

const nightFloor = 0.12

// caveFloorLight is how dark a seen-but-unlit cave cell gets under a warm lantern
// — a touch brighter than the night floor so explored rock stays faintly legible
// (your eyes adjust) instead of crushing to black.
const caveFloorLight = 0.19

// lightBands is how many discrete brightness steps the radial light snaps to —
// stepped rings (retro) rather than a smooth gradient.
const lightBands = 4

var shadowColor = mustHex("#05070B")

// rcell is one composited screen cell: a glyph in fg, optionally over a bg
// color (to pack two half-block pixels into one cell).
type rcell struct {
	ch    rune
	fg    colorful.Color
	bg    colorful.Color
	hasBg bool
	bold  bool
	blank bool // nothing drawn — a plain space
}

// RenderMap draws a whole tilemap (no camera, uniform light), players on top.
func RenderMap(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame int) string {
	cam := Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	return renderAll(th, tm, players, self, frame, cam, Light{}, cam.X, cam.Y)
}

// RenderViewport draws a camera window of the map (no light falloff).
func RenderViewport(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame int, cam Camera) string {
	return renderAll(th, tm, players, self, frame, cam, Light{}, cam.X, cam.Y)
}

// RenderLitViewport draws a camera window with a radial light.
func RenderLitViewport(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame int, cam Camera, light Light) string {
	return renderAll(th, tm, players, self, frame, cam, light, cam.X, cam.Y)
}

// RenderWindow renders a tilemap that already is the visible window (its tile
// (0,0) maps to world (originX,originY)). Used by the Wilds, whose window is
// regenerated around the player each frame.
func RenderWindow(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame, originX, originY int, light Light) string {
	cam := Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	return renderAll(th, tm, players, self, frame, cam, light, originX, originY)
}

func renderAll(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame int, cam Camera, light Light, originX, originY int) string {
	if th == nil {
		th = ui.Default
	}
	if cam.W <= 0 || cam.H <= 0 {
		cam = Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	}
	grid := buildGrid(th, tm, cam, light, frame, originX, originY)
	stampPlayers(grid, th, players, self, frame, originX, originY)
	return serializeGrid(th, grid)
}

// buildGrid lays the terrain into a cell grid with day/night tint and lighting.
// Tile indices into tm are cam-relative (cam.X+vx), but lighting is evaluated in
// absolute world coordinates (originX+vx) — for a window tilemap cam.X is 0
// while originX is the window's true world origin, so the two differ and the
// radial light must use the world origin to stay centered on its source.
func buildGrid(th *ui.Theme, tm *TileMap, cam Camera, light Light, frame, originX, originY int) [][]rcell {
	ambHex, ambStr := ui.Ambient(ui.Now())
	amb := mustHex(ambHex)
	base := map[TileKind]colorful.Color{
		TileWall:   tint(mustHex(ui.HexDim), amb, ambStr),
		TileFloor:  tint(mustHex(ui.HexFaint), amb, ambStr),
		TileDecor:  tint(mustHex(ui.HexDim), amb, ambStr),
		TileObject: tint(mustHex(ui.HexAccent), amb, ambStr),
	}
	labelC := tint(mustHex(ui.HexText), amb, ambStr)
	portalC := tint(portalColor(frame), amb, ambStr)

	grid := make([][]rcell, cam.H)
	for vy := 0; vy < cam.H; vy++ {
		grid[vy] = make([]rcell, cam.W)
		for vx := 0; vx < cam.W; vx++ {
			x, y := cam.X+vx, cam.Y+vy
			if y < 0 || y >= tm.H || x < 0 || x >= tm.W {
				grid[vy][vx] = rcell{blank: true}
				continue
			}
			t := tm.Tiles[y][x]
			if t.Kind == TileVoid && t.Anim == nil {
				grid[vy][vx] = rcell{blank: true}
				continue
			}
			ch, col, bold := tileLook(t, frame, base, labelC, portalC, amb, ambStr)
			col = applyLight(col, originX+vx, originY+vy, light)
			grid[vy][vx] = rcell{ch: ch, fg: col, bold: bold}
		}
	}
	return grid
}

func serializeGrid(th *ui.Theme, grid [][]rcell) string {
	var b strings.Builder
	for y, row := range grid {
		if y > 0 {
			b.WriteByte('\n')
		}
		for _, c := range row {
			if c.blank {
				b.WriteByte(' ')
				continue
			}
			fg := lipgloss.Color(c.fg.Clamped().Hex())
			var st lipgloss.Style
			if c.hasBg {
				st = th.FgBg(fg, lipgloss.Color(c.bg.Clamped().Hex()))
			} else {
				st = th.Fg(fg)
			}
			if c.bold {
				st = st.Bold(true)
			}
			b.WriteString(st.Render(string(c.ch)))
		}
	}
	return b.String()
}

// tileLook resolves a tile's glyph, color and weight for the given frame.
func tileLook(t Tile, frame int, base map[TileKind]colorful.Color, labelC, portalC, amb colorful.Color, ambStr float64) (rune, colorful.Color, bool) {
	ch := t.Ch
	if t.Anim != nil {
		speed := t.Anim.Speed
		if speed < 1 {
			speed = 1
		}
		step := frame / speed
		if n := len(t.Anim.Frames); n > 0 {
			ch = t.Anim.Frames[step%n]
		}
		if t.Anim.ColorA != "" && t.Anim.ColorB != "" {
			s := 0.5 + 0.5*math.Sin(float64(step)*0.5)
			ramp := mustHex(string(ui.Blend(t.Anim.ColorA, t.Anim.ColorB, s)))
			return ch, tint(ramp, amb, ambStr), t.Kind == TileObject
		}
	}

	if t.Color != "" {
		bold := t.Kind == TileObject || t.Kind == TilePortal
		return ch, tint(mustHex(t.Color), amb, ambStr), bold
	}

	switch t.Kind {
	case TilePortal:
		return ch, portalC, true
	case TileObject:
		return ch, base[TileObject], true
	case TileWall:
		return ch, base[TileWall], false
	case TileFloor:
		return ch, base[TileFloor], false
	case TileDecor:
		return ch, base[TileDecor], false
	default:
		return ch, labelC, false
	}
}

func applyLight(col colorful.Color, x, y int, light Light) colorful.Color {
	if light.Radius <= 0 {
		return col
	}
	dx := float64(x - light.X)
	dy := float64(y - light.Y)
	d := math.Sqrt(dx*dx + dy*dy)
	r := d / float64(light.Radius)
	floor := nightFloor
	var f float64
	if light.Warm {
		// A lantern: a bright plateau out to a quarter of the reach, then a soft
		// quadratic falloff — a wide, legible pool rather than a steep spotlight.
		floor = caveFloorLight
		t := math.Max(0, (r-0.25)/0.75)
		f = 1 - t*t
	} else {
		f = 1 - r // a plain radial vignette
	}
	if f < floor {
		f = floor
	}
	if f > 1 {
		f = 1
	}
	// Quantize brightness into discrete bands so the light reads as stepped
	// retro rings (lit / dim / dark) instead of a smooth modern gradient.
	t := (f - floor) / (1 - floor)
	t = math.Round(t*lightBands) / lightBands
	f = floor + t*(1-floor)
	out := col.BlendLab(shadowColor, 1-f)
	if light.Warm {
		// Warm the lit core like firelight; strongest near the source, gone by the
		// dim edge — so the held light reads warm against cool bioluminescence.
		warm := colorful.Color{R: 1, G: 0.72, B: 0.40}
		out = out.BlendLab(warm, 0.22*math.Max(0, f-floor))
	}
	return out.Clamped()
}

// stampPlayers draws every visible player's sprite onto the grid; oldest
// movers first, self last and highlighted.
func stampPlayers(grid [][]rcell, th *ui.Theme, players []world.Player, self string, frame, originX, originY int) {
	sorted := make([]world.Player, len(players))
	copy(sorted, players)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name == self {
			return false
		}
		if sorted[j].Name == self {
			return true
		}
		return sorted[i].LastMoved.Before(sorted[j].LastMoved)
	})
	for _, p := range sorted {
		fc := p.X - originX // footprint top-left column
		fr := p.Y - originY // footprint top-left row
		stampSprite(grid, th, p, p.Name == self, frame, fc, fr)
	}
}

func portalColor(frame int) colorful.Color {
	s := 0.5 + 0.5*math.Sin(float64(frame)*0.6)
	return mustHex(string(ui.Blend(ui.HexPortalA, ui.HexPortalB, s)))
}

func tint(base, ambient colorful.Color, strength float64) colorful.Color {
	if strength <= 0 {
		return base
	}
	return base.BlendLab(ambient, strength).Clamped()
}

func mustHex(s string) colorful.Color {
	c, err := colorful.Hex(s)
	if err != nil {
		return colorful.Color{R: 0.5, G: 0.5, B: 0.5}
	}
	return c
}

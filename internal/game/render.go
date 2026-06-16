package game

import (
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// Camera is the window of a (possibly larger-than-screen) map that is drawn.
// X,Y is the top-left tile; W,H the size in tiles. The zero Camera means
// "draw the whole map" — used by the small fixed areas.
type Camera struct {
	X, Y, W, H int
}

// CameraOn returns a camera of size vw×vh centered on (cx,cy) and clamped so
// it never shows past the map edges. If the map is smaller than the viewport
// the camera collapses to the map size (callers center the result).
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

// RenderMap draws a whole tilemap with players on top (no camera). Kept for
// the small fixed areas whose maps fit on screen.
func RenderMap(th *ui.Theme, tm *TileMap, players []world.Player, self string, pulse bool) string {
	return RenderViewport(th, tm, players, self, pulse, Camera{X: 0, Y: 0, W: tm.W, H: tm.H})
}

// RenderViewport draws the camera's window of the map with players on top.
// Players are drawn in LastMoved order (most recent wins a contested tile);
// the local player is always drawn last and highlighted.
func RenderViewport(th *ui.Theme, tm *TileMap, players []world.Player, self string, pulse bool, cam Camera) string {
	if th == nil {
		th = ui.Default
	}
	if cam.W <= 0 || cam.H <= 0 {
		cam = Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	}

	// glyph layer: tile display runes, then stamped text labels
	type cell struct {
		ch    rune
		style int
	}
	const (
		stWall = iota
		stFloor
		stDecor
		stPortal
		stObject
		stLabel
		stVoid
	)

	grid := make([][]cell, tm.H)
	for y := 0; y < tm.H; y++ {
		grid[y] = make([]cell, tm.W)
		for x := 0; x < tm.W; x++ {
			t := tm.Tiles[y][x]
			c := cell{ch: t.Ch}
			switch t.Kind {
			case TileWall:
				c.style = stWall
			case TileFloor:
				c.style = stFloor
			case TileDecor:
				c.style = stDecor
			case TilePortal:
				c.style = stPortal
			case TileObject:
				c.style = stObject
			default:
				c.style = stVoid
			}
			grid[y][x] = c
		}
	}

	for _, txt := range tm.Texts {
		for i, r := range []rune(txt.S) {
			x := txt.X + i
			if txt.Y < 0 || txt.Y >= tm.H || x < 0 || x >= tm.W {
				continue
			}
			grid[txt.Y][x] = cell{ch: r, style: stLabel}
		}
	}

	// players: oldest movers first so recent movers draw on top
	sorted := make([]world.Player, len(players))
	copy(sorted, players)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name == self {
			return false // self last
		}
		if sorted[j].Name == self {
			return true
		}
		return sorted[i].LastMoved.Before(sorted[j].LastMoved)
	})

	playerAt := make(map[[2]int]world.Player)
	for _, p := range sorted {
		playerAt[[2]int{p.X, p.Y}] = p
	}

	var b strings.Builder
	for vy := 0; vy < cam.H; vy++ {
		y := cam.Y + vy
		if vy > 0 {
			b.WriteByte('\n')
		}
		for vx := 0; vx < cam.W; vx++ {
			x := cam.X + vx
			if y < 0 || y >= tm.H || x < 0 || x >= tm.W {
				b.WriteByte(' ')
				continue
			}
			if p, ok := playerAt[[2]int{x, y}]; ok {
				b.WriteString(playerGlyph(th, p, p.Name == self))
				continue
			}
			c := grid[y][x]
			s := string(c.ch)
			switch c.style {
			case stWall:
				b.WriteString(th.Wall.Render(s))
			case stFloor:
				b.WriteString(th.Floor.Render(s))
			case stDecor:
				b.WriteString(th.Decor.Render(s))
			case stPortal:
				if pulse {
					b.WriteString(th.PortalA.Render(s))
				} else {
					b.WriteString(th.PortalB.Render(s))
				}
			case stObject:
				b.WriteString(th.Object.Render(s))
			case stLabel:
				b.WriteString(th.Label.Render(s))
			default:
				b.WriteString(s)
			}
		}
	}
	return b.String()
}

// playerGlyph renders a player as the first letter of their name in their
// avatar color; the local player is bold + inverse.
func playerGlyph(th *ui.Theme, p world.Player, isSelf bool) string {
	r := '☺'
	for _, c := range p.Name {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			r = unicode.ToUpper(c)
			break
		}
	}
	st := lipgloss.NewStyle().Bold(true).Foreground(p.Color)
	if th != nil {
		st = th.ChatName.Foreground(p.Color)
	}
	if isSelf {
		st = st.Reverse(true)
	}
	return st.Render(string(r))
}

package game

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// Camera is the window of a (possibly larger-than-screen) map that is drawn.
// X,Y is the top-left tile; W,H the size in tiles.
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

// Light is a radial light source: tiles fade toward darkness past Radius from
// (X,Y). A zero Radius means the scene is uniformly lit (no falloff).
type Light struct {
	X, Y, Radius int
}

// nightFloor caps how dark lighting drives a tile, so unlit corners read as
// shadow rather than pure black.
const nightFloor = 0.12

// shadowColor is the color tiles fade toward in the dark.
var shadowColor = mustHex("#05070B")

// RenderMap draws a whole tilemap (no camera, uniform light). Used by the
// small fixed areas whose maps fit on screen.
func RenderMap(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame int) string {
	return renderScene(th, tm, players, self, frame, Camera{X: 0, Y: 0, W: tm.W, H: tm.H}, Light{})
}

// RenderViewport draws a camera window of the map (no light falloff).
func RenderViewport(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame int, cam Camera) string {
	return renderScene(th, tm, players, self, frame, cam, Light{})
}

// RenderLitViewport draws a camera window with a radial light — for caves and
// machine halls that should sit in shadow beyond the player's reach.
func RenderLitViewport(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame int, cam Camera, light Light) string {
	return renderScene(th, tm, players, self, frame, cam, light)
}

func renderScene(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame int, cam Camera, light Light) string {
	if th == nil {
		th = ui.Default
	}
	if cam.W <= 0 || cam.H <= 0 {
		cam = Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	}

	// Day/night ambient is the same for the whole frame: pre-tint each tile
	// kind's base color once, then only lighting varies per cell.
	ambHex, ambStr := ui.Ambient(time.Now())
	amb := mustHex(ambHex)
	base := map[TileKind]colorful.Color{
		TileWall:   tint(mustHex(ui.HexDim), amb, ambStr),
		TileFloor:  tint(mustHex(ui.HexFaint), amb, ambStr),
		TileDecor:  tint(mustHex(ui.HexDim), amb, ambStr),
		TileObject: tint(mustHex(ui.HexAccent), amb, ambStr),
	}
	labelC := tint(mustHex(ui.HexText), amb, ambStr)
	portalC := tint(portalColor(frame), amb, ambStr)

	playerAt := indexPlayers(players, self)

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

			t := tm.Tiles[y][x]
			ch, col, bold := tileLook(t, frame, base, labelC, portalC, amb, ambStr)
			if t.Kind == TileVoid && t.Anim == nil {
				b.WriteByte(' ')
				continue
			}
			col = applyLight(col, x, y, light)
			st := th.Fg(lipgloss.Color(col.Clamped().Hex()))
			if bold {
				st = st.Bold(true)
			}
			b.WriteString(st.Render(string(ch)))
		}
	}
	return b.String()
}

// tileLook resolves a tile's glyph, color and weight for the given frame,
// folding in any per-tile animation. Day/night tint is already baked into the
// base colors; animated tiles tint their own ramp here.
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

// applyLight fades a tile color toward shadow by its distance past the light's
// radius. A zero radius leaves the color untouched.
func applyLight(col colorful.Color, x, y int, light Light) colorful.Color {
	if light.Radius <= 0 {
		return col
	}
	dx := float64(x - light.X)
	dy := float64(y - light.Y)
	d := math.Sqrt(dx*dx + dy*dy)
	f := 1 - d/float64(light.Radius) // 1 at the source, 0 at the edge
	if f < nightFloor {
		f = nightFloor
	}
	if f > 1 {
		f = 1
	}
	return col.BlendLab(shadowColor, 1-f).Clamped()
}

// indexPlayers maps tile → player to draw, oldest movers first so recent
// movers win a contested tile and self always draws last.
func indexPlayers(players []world.Player, self string) map[[2]int]world.Player {
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
	at := make(map[[2]int]world.Player, len(sorted))
	for _, p := range sorted {
		at[[2]int{p.X, p.Y}] = p
	}
	return at
}

// portalColor shimmers the portal between its two phase colors by frame.
func portalColor(frame int) colorful.Color {
	s := 0.5 + 0.5*math.Sin(float64(frame)*0.6)
	return mustHex(string(ui.Blend(ui.HexPortalA, ui.HexPortalB, s)))
}

// playerGlyph renders a player as the first letter of their name in their
// avatar color; the local player is bold + inverse. Players are not dimmed by
// lighting so avatars stay readable in the dark.
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

// tint blends a base color toward the ambient tint by strength.
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

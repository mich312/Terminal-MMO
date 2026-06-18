package game

import (
	"math/rand"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// Walker bundles what every walkable area needs: a tilemap, the local
// player's position, the portal-pulse phase, and shared movement handling.
// Areas embed it and keep their own extra logic on top.
type Walker struct {
	Ctx    *Ctx
	Map    *TileMap
	AreaID string
	X, Y   int
	Pulse  bool
	Frame  int  // monotonic animation frame, advanced by world ticks
	armed  bool // portal latch: false on spawn so you don't bounce straight back
}

// Enter places the player at a spawn point (jittered within radius so players
// don't stack) where the whole footprint fits, and announces the area change.
func (w *Walker) Enter(x, y, jitter int) {
	for try := 0; try < 20; try++ {
		cx, cy := x, y
		if jitter > 0 {
			cx += rand.Intn(2*jitter+1) - jitter
			cy += rand.Intn(jitter + 1) // only downward jitter keeps spawns tidy
		}
		if footprintWalkable(w.Map.Walkable, cx, cy) {
			x, y = cx, cy
			break
		}
	}
	w.X, w.Y = x, y
	w.armed = false
	w.Ctx.World.EnterArea(w.Ctx.Name, w.AreaID, x, y, DisplayName(w.AreaID))
}

// HandleCommon processes movement keys and tick events. It returns the
// destination area id if the player stepped onto a portal, and whether the
// message was consumed.
func (w *Walker) HandleCommon(msg tea.Msg) (portal string, handled bool) {
	switch msg := msg.(type) {
	case WorldEventMsg:
		if ev := world.Event(msg); ev.Type == world.EventTick {
			w.Pulse = ev.Pulse
			w.Frame = int(ev.Frame)
		}
		return "", true

	case tea.KeyMsg:
		dx, dy, steps, ok := MoveKey(msg.String())
		if !ok {
			return "", false
		}
		sx, sy := w.X, w.Y
		for i := 0; i < steps; i++ {
			if !CanStep(w.Map.Walkable, w.X, w.Y, dx, dy) {
				break
			}
			w.X, w.Y = w.X+dx, w.Y+dy
		}
		if w.X != sx || w.Y != sy {
			w.Ctx.World.Move(w.Ctx.Name, w.X, w.Y)
		}
		// A multi-tile body can't always stand on a wall-embedded portal tile,
		// so triggering is by proximity. The armed latch (cleared on spawn)
		// stops you bouncing straight back through the portal you arrived from.
		if p, near := w.portalNear(w.X, w.Y); near {
			if w.armed {
				return p, true
			}
		} else {
			w.armed = true
		}
		return "", true
	}
	return "", false
}

// portalNear returns a portal on or one tile around the body's footprint.
func (w *Walker) portalNear(x, y int) (string, bool) {
	for dy := -1; dy <= PlayerH; dy++ {
		for dx := -1; dx <= PlayerW; dx++ {
			if t := w.Map.At(x+dx, y+dy); t.Kind == TilePortal {
				return t.Portal, true
			}
		}
	}
	return "", false
}

// Render draws the walker's whole map with everyone in the area on it.
// Used by the small fixed areas whose maps fit on screen.
func (w *Walker) Render() string {
	players := w.Ctx.World.PlayersInArea(w.AreaID)
	return RenderMap(w.Ctx.Theme, w.Map, players, w.Ctx.Name, w.Frame)
}

// RenderViewport draws a vw×vh camera window centered on the local player,
// for maps larger than the screen (the chunked overworld). The result is at
// most vw×vh tiles; the caller centers it when the map is smaller.
func (w *Walker) RenderViewport(vw, vh int) string {
	players := w.Ctx.World.PlayersInArea(w.AreaID)
	cam := CameraOn(w.Map, w.X, w.Y, vw, vh)
	return RenderViewport(w.Ctx.Theme, w.Map, players, w.Ctx.Name, w.Frame, cam)
}

// RenderLit is RenderViewport with a radial light centered on the player, so
// the map sits in shadow beyond radius tiles — for dim areas like Kraftwerk.
func (w *Walker) RenderLit(vw, vh, radius int) string {
	players := w.Ctx.World.PlayersInArea(w.AreaID)
	cam := CameraOn(w.Map, w.X, w.Y, vw, vh)
	light := Light{X: w.X, Y: w.Y, Radius: radius}
	return RenderLitViewport(w.Ctx.Theme, w.Map, players, w.Ctx.Name, w.Frame, cam, light)
}

// HDView returns a vw×vh tile window centered on the player for the HD pixel
// renderer, plus its absolute top-left origin. Tiles outside the map come back
// as void; portal tiles are tagged as animated gate props so they read as
// entrances in pixel mode. Implements HDViewer for every Walker-based area.
func (w *Walker) HDView(vw, vh int) (*TileMap, int, int) {
	ox, oy := w.X-vw/2, w.Y-vh/2
	tiles := make([][]Tile, vh)
	for ly := 0; ly < vh; ly++ {
		row := make([]Tile, vw)
		for lx := 0; lx < vw; lx++ {
			t := w.Map.At(ox+lx, oy+ly)
			if t.Kind == TilePortal {
				t.Prop, t.PropHex = PropPortal, t.Color
				if t.PropHex == "" {
					t.PropHex = ui.HexPortalB
				}
			}
			row[lx] = t
		}
		tiles[ly] = row
	}
	return &TileMap{W: vw, H: vh, Tiles: tiles}, ox, oy
}

// PortalHint returns the status-bar hint for a portal the player stands on
// or next to, or "".
func (w *Walker) PortalHint() string {
	for dy := -1; dy <= PlayerH; dy++ {
		for dx := -1; dx <= PlayerW; dx++ {
			if t := w.Map.At(w.X+dx, w.Y+dy); t.Kind == TilePortal {
				return "↪ " + t.Label + " — walk in to enter"
			}
		}
	}
	return ""
}

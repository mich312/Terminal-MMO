package game

import (
	"math/rand"

	tea "github.com/charmbracelet/bubbletea"

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
	Frame  int // monotonic animation frame, advanced by world ticks
}

// Enter places the player at a spawn point (jittered within radius so
// players don't stack) and announces the area change to the world.
func (w *Walker) Enter(x, y, jitter int) {
	for try := 0; try < 10; try++ {
		dx, dy := 0, 0
		if jitter > 0 {
			dx = rand.Intn(2*jitter+1) - jitter
			dy = rand.Intn(jitter + 1) // only downward jitter keeps spawns tidy
		}
		if w.Map.Walkable(x+dx, y+dy) {
			x, y = x+dx, y+dy
			break
		}
	}
	w.X, w.Y = x, y
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
		dx, dy := 0, 0
		switch msg.String() {
		case "up", "w":
			dy = -1
		case "down", "s":
			dy = 1
		case "left", "a":
			dx = -1
		case "right", "d":
			dx = 1
		default:
			return "", false
		}
		nx, ny := w.X+dx, w.Y+dy
		if !w.Map.Walkable(nx, ny) {
			return "", true
		}
		w.X, w.Y = nx, ny
		w.Ctx.World.Move(w.Ctx.Name, nx, ny)
		if t := w.Map.At(nx, ny); t.Kind == TilePortal {
			return t.Portal, true
		}
		return "", true
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

// PortalHint returns the status-bar hint for a portal the player stands on
// or next to, or "".
func (w *Walker) PortalHint() string {
	if t, ok := w.Map.PortalNear(w.X, w.Y); ok {
		return "↪ " + t.Label + " — walk in to enter"
	}
	return ""
}

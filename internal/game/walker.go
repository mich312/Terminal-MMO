package game

import (
	"math/rand"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// Walker bundles what every walkable area needs: a tilemap, the local
// player's position, the portal-pulse phase, and shared movement handling.
// Areas embed it and keep their own extra logic on top.
//
// Movement is continuous (see Mover). A movement key no longer takes a step —
// it points the body, and the body walks while the key is held. X, Y is still
// the cell that body is standing in, which is what every area and both terminal
// renderers read; Body.FX, FY is where in the cell it actually is, which only
// the browser cares about.
type Walker struct {
	Ctx    *Ctx
	Map    *TileMap
	AreaID string
	X, Y   int   // the cell the body stands in — derived from Body
	Body   Mover // the body itself: continuous position and steering intent
	Pulse  bool
	Frame  int  // monotonic animation frame, advanced by world ticks
	armed  bool // portal latch: false on spawn so you don't bounce straight back

	// OnTile, if set, is called once each time the body walks into a new cell.
	//
	// This is where the per-step hooks went. Under discrete movement an area
	// could hang work off "a movement key arrived" — a cave lantern burning
	// oil, a maze counting moves — because one key was one tile. It no longer
	// is, so the hook is the tile crossing itself, which is what those
	// mechanics always actually meant. It is strictly more honest, too: the
	// old per-key hooks also fired for a move that a wall refused.
	OnTile func(x, y int)

	// Clock replaces time.Now for movement when set. Walking is now measured in
	// seconds rather than counted in keypresses, so a test that wants to cross
	// five tiles would otherwise have to sleep for most of two real seconds.
	// Production leaves this nil.
	Clock func() time.Time

	lastTick time.Time // for measuring dt ourselves, whatever cadence a host ticks at
}

// WalkFor holds a steering direction for d and drives the body through it,
// exactly as a client holding a key down would: the intent is re-asserted each
// slice (it expires otherwise — see Mover.IntentTTL) and the clock is advanced
// by hand. It returns a portal if the walk carried the player onto one.
//
// It exists because walking is now measured in seconds rather than counted in
// keypresses, so a test that wants to cross a few tiles would otherwise have to
// sleep for most of a second. Nothing in production calls it.
func (w *Walker) WalkFor(dx, dy float64, running bool, d time.Duration) (portal string, crossed bool) {
	const slice = 30 * time.Millisecond
	prev := w.Clock
	defer func() { w.Clock = prev }()

	base := w.now()
	w.lastTick = base
	for elapsed := time.Duration(0); elapsed < d; {
		step := slice
		if rest := d - elapsed; rest < step {
			step = rest
		}
		elapsed += step
		at := base.Add(elapsed)
		w.Clock = func() time.Time { return at }
		w.Body.SetIntent(dx, dy, running, at)
		if p, ok := w.advance(); ok {
			crossed = true
			if p != "" {
				return p, true
			}
		}
	}
	return "", crossed
}

// now is the movement clock: Clock when a caller has supplied one, else real time.
func (w *Walker) now() time.Time {
	if w.Clock != nil {
		return w.Clock()
	}
	return time.Now()
}

// maxTick bounds the dt one advance will believe. A host that stalled — a slow
// frame, a laptop resuming, a debugger — would otherwise hand us a gap of
// seconds and lurch the body across the map. Skipping the frame loses a few
// hundredths of a tile; honouring it teleports you through a wall.
const maxTick = 250 * time.Millisecond

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
	w.Body.Place(x, y)
	w.armed = false
	w.Ctx.World.EnterArea(w.Ctx.Name, w.AreaID, x, y, DisplayName(w.AreaID))
}

// HandleCommon processes movement keys and clock ticks. It returns the
// destination area id if walking has carried the player onto a portal, and
// whether the message was consumed.
//
// Both kinds of tick land here — the world's EventTick, which the SSH client
// forwards, and TickMsg, which the polling clients send off their own frame
// timer — because a body that moves under its own power has to be advanced by
// something other than a keypress.
func (w *Walker) HandleCommon(msg tea.Msg) (portal string, handled bool) {
	switch msg := msg.(type) {
	case WorldEventMsg:
		if ev := world.Event(msg); ev.Type == world.EventTick {
			w.Pulse = ev.Pulse
			w.Frame = int(ev.Frame)
		}
		p, _ := w.advance()
		return p, true

	case TickMsg:
		p, _ := w.advance()
		return p, true

	case tea.KeyMsg:
		dx, dy, steps, ok := MoveKey(msg.String())
		if !ok {
			return "", false
		}
		// A movement key points the body rather than moving it. The intent
		// stands until it expires or another key replaces it, which is what
		// makes a held key keep walking over SSH, where there is no key-up
		// event to tell us it was let go (see Mover.IntentTTL). steps > 1 is
		// how MoveKey spells Shift, i.e. run.
		w.Body.SetIntent(float64(dx), float64(dy), steps > 1, w.now())
		return "", true
	}
	return "", false
}

// advance walks the body by however long it has been since the last tick, and
// reports a portal if that carried us onto one.
//
// The dt is measured here rather than passed in, so the rate a host happens to
// render at — 12Hz over SSH, 15 in the browser, 30 in the HD terminal — changes
// how smooth walking looks and not how fast anyone actually walks.
func (w *Walker) advance() (portal string, crossed bool) {
	now := w.now()
	dt := now.Sub(w.lastTick)
	w.lastTick = now
	if dt <= 0 || dt > maxTick {
		return "", false // first tick of the area, or a stall: skip it
	}
	moved, onNewTile := w.Body.Advance(dt, now, w.Map.Walkable)
	if !moved {
		return "", false
	}
	w.X, w.Y = w.Body.Tile()
	w.Ctx.World.MoveTo(w.Ctx.Name, w.Body.FX, w.Body.FY, w.Body.Angle)
	if !onNewTile {
		return "", false
	}
	if w.OnTile != nil {
		w.OnTile(w.X, w.Y)
		// The hook is allowed to move us — the maze regenerates under your feet
		// when you reach its exit, the cave drops you back at the mouth if you
		// walk into a chasm — so re-read the cell before testing it for portals.
		w.X, w.Y = w.Body.Tile()
	}
	// A body can't always stand on a wall-embedded portal tile, so triggering is
	// by proximity. The armed latch (cleared on spawn) stops you bouncing
	// straight back through the portal you arrived from.
	if p, near := w.portalNear(w.X, w.Y); near {
		if w.armed {
			return p, true
		}
	} else {
		w.armed = true
	}
	return "", true
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

// RenderBoard draws the map centered on its geometric middle with no player
// avatars — for board games (Pong, Tetris…) where the player is not a token on
// the grid. Pairs with HideAvatars() so the HD client skips avatars too.
func (w *Walker) RenderBoard(vw, vh int) string {
	cam := CameraOn(w.Map, w.Map.W/2, w.Map.H/2, vw, vh)
	return RenderViewport(w.Ctx.Theme, w.Map, nil, w.Ctx.Name, w.Frame, cam)
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
			if t.Kind == TilePortal && t.Prop == PropNone {
				// A portal with no sprite of its own becomes the animated gate; one
				// that already carries a prop (e.g. a cave mouth) keeps it.
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

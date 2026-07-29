package web

import (
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// retainRadius is how far outside the camera window a tile stays in the
// client's scene before it is dropped. Keeping a margin means walking back and
// forth across a boundary doesn't churn geometry, and it covers the case where
// a client's window is briefly larger than the server's (a resize in flight).
const retainRadius = 6

// sceneState is one session's view of what the browser already knows. It exists
// so a frame can be a delta: the tiles the client holds, keyed by absolute world
// position, and the color table those tiles index into.
//
// The HD client solves the same problem with an IncrementalRenderer that
// re-rasterizes only changed tiles. This is the network twin of that idea — the
// expensive resource here is bandwidth rather than CPU, so the unit of reuse is
// the tile record rather than the tile's pixels.
type sceneState struct {
	area    string
	tiles   map[[2]int]tileWire
	palette map[string]int
	palList []string
	newPal  []string
	frames  int
}

// tileWire is the wire form of a tile: everything the renderer needs and
// nothing it doesn't. Comparing two of these decides whether a tile is resent,
// so it deliberately holds no timestamps or animation phase — those ride the
// frame counter instead, and would otherwise make every tile dirty every frame.
type tileWire struct {
	kind  int
	tex   int
	gcol  int
	prop  int
	pcol  int
	flags int
}

func newSceneState() *sceneState {
	return &sceneState{
		tiles:   make(map[[2]int]tileWire),
		palette: make(map[string]int),
	}
}

// color interns a hex color into the session's append-only palette, returning
// its index. Index 0 is always the empty color, so a tile with no explicit
// color resolves to "use the default for your kind" on the client.
func (s *sceneState) color(hex string) int {
	if hex == "" {
		return 0
	}
	if idx, ok := s.palette[hex]; ok {
		return idx
	}
	if len(s.palList) == 0 {
		s.palList = append(s.palList, "") // reserve index 0
	}
	idx := len(s.palList)
	s.palList = append(s.palList, hex)
	s.palette[hex] = idx
	s.newPal = append(s.newPal, hex)
	return idx
}

// reset forgets everything the client knew — used when the player changes area,
// where every tile coordinate now means something different.
func (s *sceneState) reset(area string) {
	s.area = area
	s.tiles = make(map[[2]int]tileWire)
	// The palette is kept: colors are session-global and reusing them across
	// areas is exactly the point of an append-only table.
}

// sceneInput is everything the session loop knows that a frame needs. Gathering
// it in a struct keeps Build's signature honest as the UI grows, and makes the
// builder testable without a live session.
type sceneInput struct {
	Area      game.Area
	View      game.HDViewer
	World     *world.World
	Name      string
	AreaID    string
	VW, VH    int
	Frame     int
	Flare     float64
	Reset     bool
	Now       time.Time
	Prompt    string
	Building  bool
	Creatures []world.Creature
}

// Build turns the live area into one frame for the browser.
//
// It reads exactly what the HD renderer reads — the same HDView window, the
// same player list, the same light and overlays — so the two clients cannot
// disagree about what is where. The difference is only in what comes out: HD
// rasterizes to pixels, this returns a Scene the GPU draws.
func (s *sceneState) Build(in sceneInput) *Scene {
	if in.Reset || s.area != in.AreaID {
		s.reset(in.AreaID)
		in.Reset = true
	}
	s.newPal = s.newPal[:0]
	s.frames++

	window, ox, oy := in.View.HDView(in.VW, in.VH)
	sc := &Scene{
		T:        MsgScene,
		Area:     in.AreaID,
		AreaName: in.Area.Name(),
		Reset:    in.Reset,
		Flare:    in.Flare,
		OX:       ox,
		OY:       oy,
		W:        in.VW,
		H:        in.VH,
		Frame:    in.Frame,
		Prompt:   in.Prompt,
	}

	// Tiles: send only what the client doesn't already hold. On a step, that's
	// the leading row or column; standing still, usually nothing at all.
	seen := make(map[[2]int]bool, in.VW*in.VH)
	for ly := 0; ly < window.H; ly++ {
		for lx := 0; lx < window.W; lx++ {
			t := window.At(lx, ly)
			ax, ay := ox+lx, oy+ly
			key := [2]int{ax, ay}
			seen[key] = true
			tw := s.encode(t)
			if prev, ok := s.tiles[key]; ok && prev == tw {
				continue
			}
			s.tiles[key] = tw
			sc.Tiles = append(sc.Tiles,
				ax, ay, tw.kind, tw.tex, tw.gcol, tw.prop, tw.pcol, tw.flags)
			if t.Kind == game.TilePortal && t.Label != "" {
				sc.Labels = append(sc.Labels, Label{X: ax, Y: ay, Text: t.Label, Kind: "portal"})
			}
		}
	}

	// Evict what has fallen well outside the window, so a long walk across the
	// infinite overworld doesn't grow either side's scene without bound.
	for key := range s.tiles {
		if seen[key] {
			continue
		}
		if key[0] < ox-retainRadius || key[0] >= ox+in.VW+retainRadius ||
			key[1] < oy-retainRadius || key[1] >= oy+in.VH+retainRadius {
			delete(s.tiles, key)
			sc.Drop = append(sc.Drop, key[0], key[1])
		}
	}

	// Actors. Players and creatures are resent in full every frame: there are
	// few of them, they change constantly, and a delta would cost more to track
	// than it saves.
	hideAvatars := false
	if h, ok := in.Area.(game.AvatarHider); ok {
		hideAvatars = h.HideAvatars()
	}
	if !hideAvatars {
		for _, p := range in.World.PlayersInArea(in.AreaID) {
			sc.Players = append(sc.Players, Actor{
				Name:   p.Name,
				X:      p.X,
				Y:      p.Y,
				Color:  s.color(string(p.Color)),
				Facing: int(p.Facing),
				Style:  p.Style,
				Access: p.Accessory,
				Weapon: p.Weapon,
				HP:     p.HP,
				MaxHP:  p.MaxHP,
				Downed: !p.DownedUntil.IsZero() && p.DownedUntil.After(in.Now),
				Self:   p.Name == in.Name,
			})
		}
		for _, c := range in.Creatures {
			// Resolve the species to the same look the terminal clients use: its
			// prop silhouette and its hue. The client then draws the shape it was
			// already told about in the hello message, so a new species needs no
			// browser change at all.
			shape, hex := "", ""
			if sp, ok := game.SpeciesByKind(c.Kind); ok {
				if key, ok := propShapeKey(sp.Prop); ok {
					shape = key
				}
				hex = sp.Hex
			}
			sc.Creatures = append(sc.Creatures, Actor{
				Name:   c.ID,
				Kind:   shape,
				X:      c.X,
				Y:      c.Y,
				Color:  s.color(hex),
				Facing: int(c.Facing),
				Owner:  c.Owner,
			})
		}
	}

	// Lighting: the area's own radial light, plus the live sky.
	if l, ok := in.Area.(game.HDLighter); ok {
		lt := l.HDLight()
		if lt.Radius > 0 {
			sc.Light = &Light{X: lt.X, Y: lt.Y, Radius: lt.Radius,
				Warm: lt.Warm, Sunless: lt.Sunless, Floor: lt.Floor}
		}
		hex, strength := ui.Ambient(in.Now)
		if lt.Sunless {
			hex, strength = ui.SunlessAmbient()
		}
		sc.Ambient = &Ambient{Hex: hex, Strength: strength}
	} else {
		hex, strength := ui.Ambient(in.Now)
		sc.Ambient = &Ambient{Hex: hex, Strength: strength}
	}

	// Contextual chrome the area exposes. These are the same interfaces the HD
	// client polls; the browser just renders them as HTML instead of pixels.
	if tz, ok := in.Area.(game.Toaster); ok {
		if msg, show := tz.Toast(); show {
			sc.Toast = msg
		}
	}
	if cl, ok := in.Area.(game.ClaimLabeler); ok {
		if label, show := cl.ClaimLabel(); show {
			sc.Claim = label
		}
	}
	if hz, ok := in.Area.(game.Hurtable); ok {
		sc.Hurt = hz.Hurt()
	}
	if bv, ok := in.Area.(game.BuildViewer); ok {
		if sel, footer, warn, show := bv.BuildPanel(); show {
			sc.Build = &Build{Sel: sel, Footer: footer, Warn: warn, Items: placeableNames()}
		}
	}
	if mm, ok := in.Area.(game.HDMinimapper); ok {
		if title, rows, show := mm.HDMinimap(); show {
			sc.Minimap = miniToWire(title, rows)
		}
	}
	if ov, ok := in.Area.(game.HDOverlayer); ok {
		if src, footer, show := ov.HDSlide(); show {
			sc.Slide = &Slide{Source: src, Footer: footer}
		}
	}

	if len(s.newPal) > 0 {
		sc.PalAdd = append([]string(nil), s.newPal...)
	}
	return sc
}

// encode reduces a tile to its wire form, interning its colors.
func (s *sceneState) encode(t game.Tile) tileWire {
	flags := 0
	if t.Walkable {
		flags |= FlagWalkable
	}
	if t.Kind == game.TilePortal {
		flags |= FlagPortal
	}
	if t.Anim != nil {
		flags |= FlagAnimated
	}
	ground := t.Ground
	if ground == "" {
		ground = t.Color
	}
	propHex := t.PropHex
	if propHex == "" {
		propHex = t.Color
	}
	return tileWire{
		kind:  int(t.Kind),
		tex:   int(t.Tex),
		gcol:  s.color(ground),
		prop:  int(t.Prop),
		pcol:  s.color(propHex),
		flags: flags,
	}
}

// miniToWire flattens the minimap's cells to hex strings, marking the player's
// own block by remembering its row and column instead of a per-cell flag.
func miniToWire(title string, rows [][]game.MiniCell) *Minimap {
	out := &Minimap{Title: title, SelfX: -1, SelfY: -1}
	for y, row := range rows {
		line := make([]string, len(row))
		for x, c := range row {
			line[x] = c.Hex
			if c.Self {
				out.SelfX, out.SelfY = x, y
			}
		}
		out.Rows = append(out.Rows, line)
	}
	return out
}

// placeableNames lists the build palette in order, so the browser can render
// the same choices the HD build panel offers.
func placeableNames() []string {
	out := make([]string, 0, len(game.Placeables))
	for _, p := range game.Placeables {
		out = append(out, p.Name)
	}
	return out
}

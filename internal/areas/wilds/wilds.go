// Package wilds is the Wilds: Durst World's procedurally generated, infinite
// overworld and main hub. The player carries absolute world coordinates and a
// multi-tile body; every frame a window of terrain is sampled from worldgen
// around them and rendered through the shared tile renderer. Generation is a
// pure function of the seed, so every session shares one world. Landmark
// portals near the origin lead to the hand-built areas.
package wilds

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// worldSeed is fixed so the Wilds are identical for everyone.
const worldSeed uint64 = 0xD0117_C0FFEE_5742

// Seed is the fixed overworld seed, exported so the experimental HD pixel
// renderer generates exactly the same Wilds as this area.
const Seed = worldSeed

func init() {
	game.Register("wilds", "The Wilds", func(ctx *game.Ctx) game.Area {
		return &area{ctx: ctx, gen: worldgen.New(worldSeed)}
	})
}

type area struct {
	ctx     *game.Ctx
	gen     *worldgen.Generator
	wx, wy  int // absolute world position (top-left of the body's footprint)
	frame   int
	showMap bool
}

func (a *area) Name() string { return "The Wilds" }

func (a *area) Init(*world.Player) tea.Cmd {
	a.wx, a.wy = a.spawn()
	a.ctx.World.EnterArea(a.ctx.Name, "wilds", a.wx, a.wy, "The Wilds")
	return nil
}

// spawn finds an open footprint near the HQ gate (but not on a portal).
func (a *area) spawn() (int, int) {
	for _, off := range [][2]int{{2, 2}, {-3, 2}, {2, -3}, {-3, -3}, {3, 0}, {0, 3}} {
		x, y := worldgen.GateX+off[0], worldgen.GateY+off[1]
		if _, isPortal := a.portalUnder(x, y); a.fits(x, y) && !isPortal {
			return x, y
		}
	}
	return worldgen.GateX + 2, worldgen.GateY + 2
}

func (a *area) fits(x, y int) bool { return footprintWalkable(a.gen, x, y) }

func (a *area) portalUnder(x, y int) (string, bool) {
	for dy := 0; dy < game.PlayerH; dy++ {
		for dx := 0; dx < game.PlayerW; dx++ {
			if c := a.gen.At(x+dx, y+dy); c.Portal != "" {
				return c.Portal, true
			}
		}
	}
	return "", false
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	switch msg := msg.(type) {
	case game.WorldEventMsg:
		if ev := world.Event(msg); ev.Type == world.EventTick {
			a.frame = int(ev.Frame)
		}
		return a, nil

	case tea.KeyMsg:
		if msg.String() == "m" {
			a.showMap = !a.showMap
			return a, nil
		}
		if a.showMap {
			a.showMap = false // any other key closes the map
		}
		dx, dy, steps, ok := game.MoveKey(msg.String())
		if !ok {
			return a, nil
		}
		sx, sy := a.wx, a.wy
		for i := 0; i < steps; i++ {
			nx, ny := a.wx+dx, a.wy+dy
			if !a.fits(nx, ny) {
				break
			}
			a.wx, a.wy = nx, ny
			if portal, ok := a.portalUnder(nx, ny); ok {
				a.ctx.World.Move(a.ctx.Name, nx, ny)
				return game.Transition{To: portal}, nil
			}
		}
		if a.wx != sx || a.wy != sy {
			a.ctx.World.Move(a.ctx.Name, a.wx, a.wy)
		}
	}
	return a, nil
}

func (a *area) Hint() string {
	if name, ok := a.portalUnder(a.wx, a.wy); ok {
		return "◈ step in to enter " + game.DisplayName(name)
	}
	dx, dy := worldgen.GateX-a.wx, worldgen.GateY-a.wy
	return fmt.Sprintf("⌂ Durst HQ %s · y u b n diagonals · m map", bearing(dx, dy))
}

// sample builds a vw×vh window of the overworld centered on the player and
// returns it with its absolute top-left origin. Shared by the glyph View and
// the HD pixel renderer.
func (a *area) sample(vw, vh int) (*game.TileMap, int, int) {
	ox, oy := a.wx-vw/2, a.wy-vh/2
	tiles := make([][]game.Tile, vh)
	for ly := 0; ly < vh; ly++ {
		row := make([]game.Tile, vw)
		for lx := 0; lx < vw; lx++ {
			row[lx] = CellTile(a.gen.At(ox+lx, oy+ly))
		}
		tiles[ly] = row
	}
	return &game.TileMap{W: vw, H: vh, Tiles: tiles}, ox, oy
}

// HDView implements game.HDViewer so the Wilds renders in HD pixel mode.
func (a *area) HDView(vw, vh int) (*game.TileMap, int, int) { return a.sample(vw, vh) }

func (a *area) View(width, height int) string {
	tm, ox, oy := a.sample(width, height)
	players := a.ctx.World.PlayersInArea("wilds")
	view := game.RenderWindow(a.ctx.Theme, tm, players, a.ctx.Name, a.frame, ox, oy, game.Light{})

	if a.showMap {
		panel := a.minimap()
		pw := lipgloss.Width(panel)
		view = ui.Overlay(view, panel, (width-pw)/2, 1)
	}
	return view
}

// CellTile converts a generated overworld cell into a renderable tile. It is
// the single source of truth for the Wilds, shared by the glyph and HD
// renderers. Color/Ch keep the original cell look for the glyph renderer; Tex,
// Ground and Prop drive the HD tileset (decorations become sprites over the
// biome ground rather than solid squares).
func CellTile(c worldgen.Cell) game.Tile {
	kind := game.TileFloor
	switch {
	case c.Portal != "":
		kind = game.TilePortal
	case c.Object:
		kind = game.TileObject
	case !c.Walkable:
		kind = game.TileDecor
	}
	t := game.Tile{Kind: kind, Ch: c.Glyph, Walkable: c.Walkable, Color: c.Color, Portal: c.Portal, Tex: texForBiome(c.Biome)}
	if c.AnimA != "" && c.AnimB != "" {
		t.Anim = &game.TileAnim{Frames: c.Frames, ColorA: c.AnimA, ColorB: c.AnimB, Speed: 3}
	}
	if c.Object {
		// Landmark area-entrances are animated portal gates, color-coded to the
		// destination — distinct from decorative houses.
		t.Prop, t.PropHex, t.Ground, t.Tex = game.PropPortal, c.Color, groundColor(worldgen.Grass), game.TexGrass
		return t
	}
	switch c.Glyph {
	case '*': // flower on grass
		t.Prop, t.PropHex, t.Ground = game.PropFlower, c.Color, groundColor(c.Biome)
	case ',': // grass tuft
		t.Prop, t.PropHex, t.Ground = game.PropTuft, "#3E7A4F", groundColor(c.Biome)
	case 'o': // bush
		t.Prop, t.PropHex, t.Ground = game.PropBush, c.Color, groundColor(c.Biome)
	case 'u': // tree stump
		t.Prop, t.PropHex, t.Ground = game.PropStump, c.Color, groundColor(c.Biome)
	case '°': // small rock
		t.Prop, t.PropHex, t.Ground = game.PropRock, c.Color, groundColor(c.Biome)
	case 'H': // a homestead — decorative house (blocks)
		t.Prop, t.PropHex, t.Ground = game.PropHouse, c.Color, groundColor(c.Biome)
	case '♣': // tree on forest floor
		t.Prop, t.PropHex, t.Ground, t.Tex = game.PropTree, c.Color, groundColor(worldgen.Forest), game.TexForest
	case '▲': // boulder on hill earth (mountain peaks stay a plain rock tile)
		if c.Biome == worldgen.Hill {
			t.Prop, t.PropHex, t.Ground, t.Tex = game.PropBoulder, "#8A8170", groundColor(worldgen.Hill), game.TexDirt
		}
	}
	return t
}

// texForBiome maps an overworld biome to an HD ground texture.
func texForBiome(b worldgen.Biome) game.TileTex {
	switch b {
	case worldgen.Grass, worldgen.Savanna:
		return game.TexGrass
	case worldgen.Sand:
		return game.TexSand
	case worldgen.Water, worldgen.Deep:
		return game.TexWater
	case worldgen.Forest:
		return game.TexForest
	case worldgen.Hill, worldgen.Path:
		return game.TexDirt
	case worldgen.Mountain, worldgen.Snow:
		return game.TexRock
	default:
		return game.TexFlat
	}
}

// groundColor is the base surface color the HD renderer paints under a prop.
func groundColor(b worldgen.Biome) string {
	switch b {
	case worldgen.Grass:
		return "#5FA86B"
	case worldgen.Forest:
		return "#3F8A5A"
	case worldgen.Hill:
		return "#9C8D67"
	case worldgen.Sand:
		return "#E6D6A0"
	case worldgen.Mountain:
		return "#9AA0A8"
	case worldgen.Path:
		return "#9B8B6A"
	case worldgen.Snow:
		return "#E8EEF5"
	case worldgen.Savanna:
		return "#B8A659"
	default:
		return ""
	}
}

// minimap renders a coarse overview of the surrounding terrain (one cell per
// few tiles), marking the player (☺), landmarks (their glyph) and the gate.
func (a *area) minimap() string {
	const (
		stride = 4
		halfW  = 19
		halfH  = 9
	)
	th := a.ctx.Theme
	if th == nil {
		th = ui.Default
	}
	var b strings.Builder
	b.WriteString(th.PanelTitle.Render("Map — The Wilds") + "\n")
	for ry := -halfH; ry <= halfH; ry++ {
		for rx := -halfW; rx <= halfW; rx++ {
			wx := a.wx + rx*stride
			wy := a.wy + ry*stride
			if rx == 0 && ry == 0 {
				b.WriteString(th.Bright.Render("☺"))
				continue
			}
			c := a.gen.At(wx, wy)
			color := c.Color
			if color == "" {
				color = ui.HexDim
			}
			b.WriteString(th.Fg(lipgloss.Color(color)).Render(string(c.Glyph)))
		}
		b.WriteByte('\n')
	}
	b.WriteString(th.Dim.Render("m or move to close"))
	return th.Panel.Render(b.String())
}

func footprintWalkable(g *worldgen.Generator, x, y int) bool {
	for dy := 0; dy < game.PlayerH; dy++ {
		for dx := 0; dx < game.PlayerW; dx++ {
			if !g.Walkable(x+dx, y+dy) {
				return false
			}
		}
	}
	return true
}

func bearing(dx, dy int) string {
	s := ""
	switch {
	case dy < 0:
		s += fmt.Sprintf("%d↑ ", -dy)
	case dy > 0:
		s += fmt.Sprintf("%d↓ ", dy)
	}
	switch {
	case dx < 0:
		s += fmt.Sprintf("%d←", -dx)
	case dx > 0:
		s += fmt.Sprintf("%d→", dx)
	}
	if s == "" {
		return "(here)"
	}
	return strings.TrimSpace(s)
}

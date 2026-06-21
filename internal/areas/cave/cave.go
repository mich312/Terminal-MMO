// Package cave is the underground: dark, bioluminescent caverns that open off
// cave mouths in the overworld's hills. Each mouth always leads to the same cave
// (the layout is seeded by the entrance's world coordinates) and different mouths
// to different caves, so the hills are dotted with caverns to explore.
//
// A cave is a procedurally carved cellular-automaton cave system — rounded
// chambers joined by winding passages — rendered through the shared Walker base
// (movement, the HD pixel renderer) under a tight lantern, so you only see as far
// as your light throws. The dark is broken by the cave's own life: clusters of
// glowing mushrooms, still pools lit from within, and seams of ice crystal that
// twinkle their own cold light — all of which you can mine or gather.
package cave

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/areas/wilds"
	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// gen resolves cave systems (which overworld mouths belong to which cave) from
// the same fixed overworld seed as the Wilds.
var gen = worldgen.New(wilds.Seed)

const (
	lanternR  = 11 // the warm circle a full lantern throws
	lanternLo = 3  // …and what a guttering, near-dry lantern manages
	chunkN    = 8  // fog-of-war chunk is 8×8 cells, one uint64 mask

	fuelMax    = 90 // steps of light a full lantern holds
	fuelBurn   = 1  // fuel a step in the dark spends
	fuelRefill = 7  // fuel a step beside the cave's own glow restores
	fuelLow    = 22 // at/under this the lantern visibly gutters (and warns)
)

func init() {
	game.Register("cave", "a cave", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "cave"}}
	})
}

type area struct {
	game.Walker
	w, h           int               // this cave's size (the bbox of its mouths, padded)
	caveKey        string            // this cave's id (its origin mouth), for persistence
	overworldDoors [][2]int          // each surface mouth's overworld cell
	interiorDoors  [][2]int          // …and the matching mouth inside the cave (parallel)
	nodes          map[[2]int]string // gatherable position → item id
	mined          map[[2]int]bool   // worked out this visit
	discovered     map[[2]int]uint64 // uncovered fog chunks (chunk coord → 64-cell mask)
	dirty          map[[2]int]bool   // chunks changed since the last flush
	showMap        bool              // the fill-in cave map is open (m)
	fuel           int               // lantern oil left this visit; light shrinks as it runs low
	warnedLow      bool              // already told the player the lantern's guttering
	toast          string
	toastUntil     time.Time
}

func (a *area) Name() string { return "a cave" }

// Init carves the cavern. The cave mouth the player stepped through (carried on
// the player at transition time) resolves to a cave system — its origin and its
// 1–3 surface mouths. The cave is seeded and named by the origin, so every mouth
// of a system opens the same cavern and shares one remembered map; the player is
// dropped at the inner mouth matching the one they entered by.
func (a *area) Init(p *world.Player) tea.Cmd {
	if a.Ctx.Inventory == nil {
		a.Ctx.Inventory = map[string]int{}
	}
	sys, doorIdx, ok := gen.CaveSystemAt(p.X, p.Y)
	if !ok { // entered somewhere that isn't a known mouth — treat it as a lone cave
		sys = worldgen.CaveSystem{Origin: [2]int{p.X, p.Y}, Doors: [][2]int{{p.X, p.Y}}}
		doorIdx = 0
	}
	a.overworldDoors = sys.Doors
	a.caveKey = fmt.Sprintf("%d,%d", sys.Origin[0], sys.Origin[1])
	ox, oy := sys.Origin[0], sys.Origin[1]
	seed := int64(uint64(uint32(ox))*0x9E3779B1 ^ uint64(uint32(oy))*0x85EBCA77 ^ 0x0CA7E)
	a.Map, a.interiorDoors, a.nodes, a.w, a.h = genCaveFromWilds(gen, sys.Doors, rand.New(rand.NewSource(seed)))
	a.mined = map[[2]int]bool{}
	a.fuel = fuelMax // a freshly-trimmed lantern each descent
	a.discovered = a.Ctx.Store.LoadCaveDiscovery(a.Ctx.Name, a.caveKey)
	if a.discovered == nil {
		a.discovered = map[[2]int]uint64{}
	}
	a.dirty = map[[2]int]bool{}
	if doorIdx >= len(a.interiorDoors) {
		doorIdx = 0
	}
	sp := a.interiorDoors[doorIdx]
	a.Enter(sp[0], sp[1], 0)
	a.reveal()
	a.persist()
	return nil
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "e", " ":
			if pos, item, ok := a.nodeNear(); ok {
				a.gather(pos, item)
			}
			return a, nil
		case "m":
			a.showMap = !a.showMap // toggle the fill-in map
			return a, nil
		}
		if a.showMap {
			a.showMap = false // any other key closes the map (and still acts)
		}
	}
	portal, handled := a.HandleCommon(msg)
	if _, isKey := msg.(tea.KeyMsg); isKey && handled {
		a.burnLantern() // a step spends oil, or the glow tops it up
		a.reveal()      // a step lifts the dark as far as the light now throws
		a.persist()
	}
	if handled && portal != "" {
		a.surfaceAt() // leave by whichever mouth we reached
		return game.Transition{To: portal}, nil
	}
	return a, nil
}

// surfaceAt records, on the way out, the overworld mouth matching the inner one
// the player is leaving through, so the Wilds drops them there — climb in one
// mouth, tunnel under the hills, and step out of another.
func (a *area) surfaceAt() {
	best, bestD := 0, 1<<30
	for i, d := range a.interiorDoors {
		if dd := abs(d[0]-a.X) + abs(d[1]-a.Y); dd < bestD {
			best, bestD = i, dd
		}
	}
	if best < len(a.overworldDoors) {
		o := a.overworldDoors[best]
		a.Ctx.Store.SavePosition(a.Ctx.Name, "wilds", o[0], o[1])
	}
}

// chunkOf splits a cell into its 8×8 chunk coordinate and bit index within it.
func chunkOf(x, y int) (cx, cy int, bit uint) {
	return x >> 3, y >> 3, uint((y&(chunkN-1))*chunkN + (x & (chunkN - 1)))
}

// seen reports whether a cave cell has been uncovered.
func (a *area) seen(x, y int) bool {
	cx, cy, bit := chunkOf(x, y)
	return a.discovered[[2]int{cx, cy}]&(1<<bit) != 0
}

// markSeen records a cell as uncovered, flagging its chunk dirty if changed.
func (a *area) markSeen(x, y int) {
	cx, cy, bit := chunkOf(x, y)
	key := [2]int{cx, cy}
	if nw := a.discovered[key] | (1 << bit); nw != a.discovered[key] {
		a.discovered[key] = nw
		a.dirty[key] = true
	}
}

// reveal uncovers the disc of cave around the player — what the lantern has shown
// stays remembered (dim) once you move on, so the cavern is mapped as you walk it.
func (a *area) reveal() {
	r := a.lanternRadius() + 2 // uncover a touch past the light — less, as it dims
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if x, y := a.X+dx, a.Y+dy; dx*dx+dy*dy <= r*r &&
				x >= 0 && y >= 0 && x < a.w && y < a.h {
				a.markSeen(x, y)
			}
		}
	}
}

// persist flushes newly-uncovered chunks for this cave so the map survives the
// climb back out and the next descent.
func (a *area) persist() {
	for ch := range a.dirty {
		a.Ctx.Store.SaveCaveDiscovery(a.Ctx.Name, a.caveKey, ch[0], ch[1], a.discovered[ch])
		delete(a.dirty, ch)
	}
}

// nodeNear returns the first ungathered seam or mushroom on or one tile around
// the player.
func (a *area) nodeNear() ([2]int, string, bool) {
	for dy := -1; dy <= game.PlayerH; dy++ {
		for dx := -1; dx <= game.PlayerW; dx++ {
			p := [2]int{a.X + dx, a.Y + dy}
			if item, ok := a.nodes[p]; ok && !a.mined[p] {
				return p, item, true
			}
		}
	}
	return [2]int{}, "", false
}

// gather works out a seam or picks a mushroom: it drops into the player's pack,
// the spot becomes plain cave floor, and a toast confirms the haul.
func (a *area) gather(pos [2]int, item string) {
	a.mined[pos] = true
	a.Map.Tiles[pos[1]][pos[0]] = caveFloor
	a.Ctx.Inventory[item]++
	a.Ctx.Store.AddItem(a.Ctx.Name, item)
	name := item
	if it, ok := game.ItemByID(item); ok {
		name = it.Name
	}
	verb := "⛏ mined"
	if item == "mushroom" {
		verb = "🍄 picked"
	}
	a.setToast(verb + " " + name)
}

func (a *area) setToast(s string) { a.toast, a.toastUntil = s, time.Now().Add(3*time.Second) }

// Toast implements game.Toaster so both renderers surface the gathering message.
func (a *area) Toast() (string, bool) {
	return a.toast, a.toast != "" && time.Now().Before(a.toastUntil)
}

func (a *area) Hint() string {
	if _, item, ok := a.nodeNear(); ok {
		name := item
		if it, ok := game.ItemByID(item); ok {
			name = it.Name
		}
		verb := "mine"
		if item == "mushroom" {
			verb = "pick"
		}
		return "e — " + verb + " the " + name
	}
	if a.fuel <= fuelLow {
		return "🕯 your lantern is guttering — rest beside the glow (mushrooms, pools, daylight) to rekindle it"
	}
	if h := a.PortalHint(); h != "" {
		return h
	}
	return "🕯 a cave — follow the glow into the dark · ∩ return to the mouth to leave"
}

// lanternRadius is how far the light currently throws — full at a brimming
// lantern, shrinking toward a groping glow as the oil burns down.
func (a *area) lanternRadius() int {
	f := float64(a.fuel) / fuelMax
	return lanternLo + int(math.Round(f*float64(lanternR-lanternLo)))
}

// HDLight gives the HD renderer a lantern around the player so the cavern falls
// away into darkness past its reach — and closes in as the lantern runs dry.
func (a *area) HDLight() game.Light {
	return game.Light{X: a.X + game.PlayerW/2, Y: a.Y + game.PlayerH/2, Radius: a.lanternRadius(), Warm: true}
}

// isGlow reports whether a prop sheds its own light — daylight shafts and the
// cave's bioluminescence — the things a lantern can be rekindled beside.
func isGlow(p game.TileProp) bool {
	switch p {
	case game.PropLightShaft, game.PropGlowPool, game.PropCaveShroom,
		game.PropGemGlow, game.PropGeode, game.PropRelic:
		return true
	}
	return false
}

// nearGlow reports whether the player stands within a step of any natural light.
func (a *area) nearGlow() bool {
	for dy := -2; dy <= game.PlayerH+1; dy++ {
		for dx := -2; dx <= game.PlayerW+1; dx++ {
			x, y := a.X+dx, a.Y+dy
			if x >= 0 && y >= 0 && x < a.w && y < a.h && isGlow(a.Map.At(x, y).Prop) {
				return true
			}
		}
	}
	return false
}

// burnLantern spends a step of oil, or tops the lantern up where the cave glows,
// and surfaces the turn when the light starts to gutter or is rekindled.
func (a *area) burnLantern() {
	if a.nearGlow() {
		was := a.fuel
		if a.fuel += fuelRefill; a.fuel > fuelMax {
			a.fuel = fuelMax
		}
		if a.warnedLow && was <= fuelLow && a.fuel > fuelLow {
			a.warnedLow = false
			a.setToast("🕯 the glow rekindles your lantern")
		}
		return
	}
	if a.fuel -= fuelBurn; a.fuel < 0 {
		a.fuel = 0
	}
	if !a.warnedLow && a.fuel <= fuelLow {
		a.warnedLow = true
		a.setToast("🕯 your lantern gutters — make for the glow")
	}
}

// window builds a vw×vh view centered on the player in which every cell the
// player hasn't uncovered yet is pure black — the cave is explored out of total
// darkness, like the Wilds. Collision still reads the real map, so the fog only
// hides the cave, it never blocks the way.
func (a *area) window(vw, vh int) (*game.TileMap, int, int) {
	ox, oy := a.X-(vw-game.PlayerW)/2, a.Y-(vh-game.PlayerH)/2
	tiles := make([][]game.Tile, vh)
	for ly := 0; ly < vh; ly++ {
		row := make([]game.Tile, vw)
		for lx := 0; lx < vw; lx++ {
			if wx, wy := ox+lx, oy+ly; a.seen(wx, wy) {
				row[lx] = a.Map.At(wx, wy)
			} else {
				row[lx] = caveFog()
			}
		}
		tiles[ly] = row
	}
	return &game.TileMap{W: vw, H: vh, Tiles: tiles}, ox, oy
}

// HDView feeds the fogged window to the HD pixel renderer (overriding Walker's,
// which would draw the whole map).
func (a *area) HDView(vw, vh int) (*game.TileMap, int, int) { return a.window(vw, vh) }

func (a *area) View(width, height int) string {
	tm, ox, oy := a.window(width, height)
	players := a.Ctx.World.PlayersInArea(a.AreaID)
	view := game.RenderWindow(a.Ctx.Theme, tm, players, a.Ctx.Name, a.Frame, ox, oy, a.HDLight())
	if a.showMap {
		panel := a.minimap()
		view = ui.Overlay(view, panel, (width-lipgloss.Width(panel))/2, 1)
	} else if msg, show := a.Toast(); show {
		th := a.Ctx.Theme
		if th == nil {
			th = ui.Default
		}
		line := th.Toast.Render(msg)
		view = ui.Overlay(view, line, (width-lipgloss.Width(line))/2, 1)
	}
	return view
}

// minimap draws the cave as a small chart that fills in as you explore: rock,
// floor, the glittering seams and pools you've found, and the mouth(s) to the
// surface, with the unexplored dark left blank.
func (a *area) minimap() string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	// Adapt the scale so even a large cave's chart fits a tidy panel.
	stride := 2
	for a.w/stride > 46 || a.h/stride > 30 {
		stride++
	}
	sx, sy := stride, stride
	var b strings.Builder
	b.WriteString(th.PanelTitle.Render("Map — the cave") + "\n")
	for my := 0; my < a.h; my += sy {
		for mx := 0; mx < a.w; mx += sx {
			if a.X >= mx && a.X < mx+sx && a.Y >= my && a.Y < my+sy {
				b.WriteString(th.Bright.Render("☺"))
				continue
			}
			glyph, color, ok := a.miniBlock(mx, my, sx, sy)
			if !ok {
				b.WriteByte(' ') // unexplored dark
				continue
			}
			b.WriteString(th.Fg(lipgloss.Color(color)).Render(glyph))
		}
		b.WriteByte('\n')
	}
	b.WriteString(th.Dim.Render("m or move to close"))
	return th.Panel.Render(b.String())
}

// miniBlock summarises an sx×sy patch of cave for the map, picking the most
// telling feature in it (a mouth or a seam over plain rock/floor). ok is false
// when nothing in the patch has been uncovered.
func (a *area) miniBlock(mx, my, sx, sy int) (glyph, color string, ok bool) {
	glyph, color = "", ""
	rank := -1
	rankOf := func(t game.Tile) (string, string, int) {
		switch t.Prop {
		case game.PropCaveMouth:
			return "∩", "#9BE0FF", 5
		case game.PropGemGlow:
			return "◆", "#7DF0FF", 4
		case game.PropGlowPool:
			return "≈", "#6CE0E6", 4
		case game.PropCaveShroom:
			return "♣", "#7CF2C4", 4
		case game.PropGem:
			return "◆", "#FFC861", 4
		case game.PropStone:
			return "◊", "#C2C8D0", 3
		}
		if t.Kind == game.TileWall || t.Kind == game.TileDecor {
			return "█", "#473F4F", 1
		}
		return "·", "#5A5260", 2
	}
	for y := my; y < my+sy && y < a.h; y++ {
		for x := mx; x < mx+sx && x < a.w; x++ {
			if !a.seen(x, y) {
				continue
			}
			ok = true
			if g, c, r := rankOf(a.Map.At(x, y)); r > rank {
				glyph, color, rank = g, c, r
			}
		}
	}
	return glyph, color, ok
}

// caveFog is the unbroken black of cave the lantern hasn't found yet.
func caveFog() game.Tile {
	return game.Tile{Kind: game.TileWall, Ch: ' ', Color: "#05070A", Tex: game.TexFlat, Ground: "#05070A"}
}

// --- cavern generation ---------------------------------------------------------

var (
	rockWall  = game.Tile{Kind: game.TileWall, Ch: '▓', Walkable: false, Color: "#564E5E", Tex: game.TexRock, Ground: "#3A3442"}
	caveFloor = game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true, Color: "#9A91A0", Tex: game.TexDirt, Ground: "#6A6270"}
	// The mouth is a cave-mouth sprite (not a glowing gate); the warm hex is the
	// daylight beyond it. Its prop is kept by Walker.HDView instead of the portal.
	caveMouth = game.Tile{Kind: game.TilePortal, Ch: '∩', Walkable: true, Color: "#C8BFA0",
		Portal: "wilds", Label: "the cave mouth", Prop: game.PropCaveMouth, PropHex: "#B6A483", Tex: game.TexRock, Ground: "#6B5A44"}
	mushroom = game.Tile{Kind: game.TileObject, Ch: 'ψ', Walkable: true, Color: "#7CF2C4",
		Tex: game.TexDirt, Ground: "#6A6270", Prop: game.PropCaveShroom, PropHex: "#7CF2C4"}
	glowPool = game.Tile{Kind: game.TileFloor, Ch: '≈', Walkable: true, Color: "#5BD8E0",
		Tex: game.TexWater, Ground: "#1E5560", Prop: game.PropGlowPool, PropHex: "#6CE0E6"}
	// Speleothems: stone shaped by water over ages. Stalagmites and flowstone are
	// in-tile (you squeeze past); a column runs floor-to-ceiling and blocks.
	stalagmite = game.Tile{Kind: game.TileFloor, Ch: '▲', Walkable: true, Color: "#B9B0BE",
		Tex: game.TexRock, Ground: "#6A6270", Prop: game.PropStalagmite, PropHex: "#9A92A0"}
	flowstone = game.Tile{Kind: game.TileFloor, Ch: '╫', Walkable: true, Color: "#C9B894",
		Tex: game.TexRock, Ground: "#6A6270", Prop: game.PropFlowstone, PropHex: "#BBAA86"}
	column = game.Tile{Kind: game.TileDecor, Ch: '█', Walkable: false, Color: "#A89A82",
		Tex: game.TexRock, Ground: "#5C5560", Prop: game.PropColumn, PropHex: "#A1937B"}
	// A shaft of daylight breaking through where the rock above runs thinnest.
	lightShaft = game.Tile{Kind: game.TileFloor, Ch: '░', Walkable: true, Color: "#FFF3D6",
		Tex: game.TexDirt, Ground: "#8E8468", Prop: game.PropLightShaft, PropHex: "#FFF1CE"}
	// Old mine timbers under a peak (you pass under the frame); a glowing relic
	// half-buried in deep ruins.
	timbering = game.Tile{Kind: game.TileFloor, Ch: '╬', Walkable: true, Color: "#9C6B3F",
		Tex: game.TexRock, Ground: "#5C5560", Prop: game.PropTimbering, PropHex: "#8A5E37"}
	relicTile = game.Tile{Kind: game.TileObject, Ch: '◈', Walkable: true, Color: "#C9B0FF",
		Tex: game.TexRock, Ground: "#6A6270", Prop: game.PropRelic, PropHex: "#C9B0FF"}
	geodeTile = game.Tile{Kind: game.TileObject, Ch: '◈', Walkable: true, Color: "#9CE0FF",
		Tex: game.TexRock, Ground: "#6A6270", Prop: game.PropGeode, PropHex: "#9CE0FF"}
)

var nb4 = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// cavePalette is a cave's colour mood — a rock hue and the colours its life and
// crystal glow — taken from the land overhead so no two stretches of cave look
// quite alike: ice under the cold heights, moss under wet woods, ochre under
// warm dry country, cool slate under temperate hills.
type cavePalette struct {
	rock    colorful.Color // hue the rock (walls, floor, stone) is shifted toward
	glow    string         // bioluminescence: mushrooms and pools
	crystal string         // crystal seams and the geode core
	slate   bool           // the default mood — leave the rock as authored
}

func paletteFor(temp, moist float64) cavePalette {
	switch {
	case temp < 0.34: // cold heights: blue ice and frost
		return cavePalette{rock: mustHex("#3B4A6B"), glow: "#8FE0FF", crystal: "#CFEEFF"}
	case moist > 0.60: // wet woods: green moss and glow
		return cavePalette{rock: mustHex("#36482F"), glow: "#8BF29C", crystal: "#7DF0C6"}
	case temp > 0.60 && moist < 0.42: // warm dry country: ochre sandstone
		return cavePalette{rock: mustHex("#54422C"), glow: "#FFC871", crystal: "#FFE3A0"}
	default: // temperate hills: cool slate (as authored)
		return cavePalette{glow: "#7CF2C4", crystal: "#7DF0FF", slate: true}
	}
}

// moodTint shifts a colour toward the palette's hue and chroma while keeping its
// own lightness, so a recolour swaps the rock's colour family without flattening
// the light floors and dark walls that give the cave its depth.
func moodTint(hex string, mood colorful.Color, amt float64) string {
	c, err := colorful.Hex(hex)
	if err != nil {
		return hex
	}
	_, _, l := c.Hcl()
	mh, mc, _ := mood.Hcl()
	target := colorful.Hcl(mh, mc, l).Clamped()
	return c.BlendHcl(target, amt).Clamped().Hex()
}

// recolour repaints a finished cave in its palette: rock and stone shift toward
// the mood hue; the living glow and crystal take the palette's colours; daylight
// shafts, relics and timber keep their own.
func (c *carver) recolour(tiles [][]game.Tile, pal cavePalette) {
	if pal.slate {
		return
	}
	for y := range tiles {
		for x := range tiles[y] {
			t := &tiles[y][x]
			switch t.Prop {
			case game.PropCaveShroom, game.PropGlowPool:
				t.PropHex, t.Color = pal.glow, pal.glow
			case game.PropGemGlow, game.PropGeode:
				t.PropHex, t.Color = pal.crystal, pal.crystal
			case game.PropLightShaft, game.PropRelic, game.PropTimbering, game.PropGem:
				// daylight, relic-glow, wood and gold (a metal) keep their own colours
			default:
				if t.Ground != "" {
					t.Ground = moodTint(t.Ground, pal.rock, 0.55)
				}
				if t.PropHex != "" {
					t.PropHex = moodTint(t.PropHex, pal.rock, 0.45)
				}
			}
		}
	}
}

// seam is one mineable mineral: the item it yields and the tile that marks it in
// the rock. Stone is common, gold rarer, glittering ice crystals (which twinkle
// their own light in the dark) rarest.
type seam struct {
	item string
	tile game.Tile
}

var (
	stoneSeam   = seam{"stone", game.Tile{Kind: game.TileObject, Ch: '◊', Walkable: true, Color: "#C2C8D0", Tex: game.TexRock, Ground: "#6A6270", Prop: game.PropStone, PropHex: "#C2C8D0"}}
	goldSeam    = seam{"nugget", game.Tile{Kind: game.TileObject, Ch: '◆', Walkable: true, Color: "#FFC861", Tex: game.TexRock, Ground: "#6A6270", Prop: game.PropGem, PropHex: "#FFC861"}}
	crystalSeam = seam{"crystal", game.Tile{Kind: game.TileObject, Ch: '◆', Walkable: true, Color: "#7DF0FF", Tex: game.TexRock, Ground: "#6A6270", Prop: game.PropGemGlow, PropHex: "#7DF0FF"}}
)

// seamFor picks a mineral seam for a rock face by the height of the land above:
// under the peaks the rock runs to gold and glittering crystal, while the lower
// hills give up mostly plain stone.
func seamFor(surf float64, rng *rand.Rand) seam {
	r := rng.Float64()
	if surf >= caveDeepElev { // under the mountains — the precious veins
		switch {
		case r < 0.34:
			return crystalSeam
		case r < 0.66:
			return goldSeam
		default:
			return stoneSeam
		}
	}
	switch {
	case r < 0.80:
		return stoneSeam
	case r < 0.93:
		return goldSeam
	default:
		return crystalSeam
	}
}

// The cave is the underground of the patch of Wilds its mouths span. Its grid is
// the bounding box of the mouths (padded with rock), at 1:1 scale with the
// surface, so each mouth sits at its true position and the distances inside match
// the distances overhead. The cavern only opens where the hills rise above
// caveFloorElev — below that (valleys, water) the rock is solid — so the cave is
// shaped by the land, with the mouths linked by passages bored at their real
// offsets. Walk from one mouth to another underground and you've walked the same
// way you would on the surface.
const (
	caveMargin    = 16   // rock border around the mouth bounding box
	caveMinDim    = 40   // smallest cave grid (a lone mouth still gets room)
	caveMaxDim    = 150  // safety cap on a cave's size
	caveFloorElev = 0.50 // surface elevation above which the cave can open out
	caveDeepElev  = 0.78 // …above which the rock runs to precious veins (under peaks)
	caveLowElev   = 0.58 // …below which cave water gathers (under the low ground)
)

func genCaveFromWilds(g *worldgen.Generator, overDoors [][2]int, rng *rand.Rand) (*game.TileMap, [][2]int, map[[2]int]string, int, int) {
	minX, minY, maxX, maxY := overDoors[0][0], overDoors[0][1], overDoors[0][0], overDoors[0][1]
	for _, d := range overDoors {
		minX, minY = min(minX, d[0]), min(minY, d[1])
		maxX, maxY = max(maxX, d[0]), max(maxY, d[1])
	}
	c := &carver{g: g, rng: rng, ox: minX - caveMargin, oy: minY - caveMargin,
		w: clamp((maxX-minX)+2*caveMargin+1, caveMinDim, caveMaxDim),
		h: clamp((maxY-minY)+2*caveMargin+1, caveMinDim, caveMaxDim)}
	doors := make([][2]int, len(overDoors)) // mouths in local coords (truly mapped)
	for i, d := range overDoors {
		doors[i] = [2]int{d[0] - c.ox, d[1] - c.oy}
	}
	c.carve(doors)
	region := c.flood(doors[0])
	if len(region) < 60 { // pathological (no hills?) — open a plain chamber instead
		c.openInterior()
		region = c.flood(doors[0])
	}
	inMain := make(map[[2]int]bool, len(region))
	for _, p := range region {
		inMain[p] = true
	}
	tiles := make([][]game.Tile, c.h)
	for y := 0; y < c.h; y++ {
		tiles[y] = make([]game.Tile, c.w)
		for x := 0; x < c.w; x++ {
			if inMain[[2]int{x, y}] {
				tiles[y][x] = caveFloor
			} else {
				tiles[y][x] = rockWall
			}
		}
	}
	for _, d := range doors {
		tiles[d[1]][d[0]] = caveMouth
	}
	texture(tiles, c.w, c.h)
	nodes := c.scatterLife(rng, tiles, region, doors)
	c.special(tiles, region, doors, nodes)
	clutter(rng, tiles, region, c.w, c.h)
	_, moist, temp := g.Climate(overDoors[0][0], overDoors[0][1]) // mood from the land above
	c.recolour(tiles, paletteFor(temp, moist))
	return &game.TileMap{W: c.w, H: c.h, Tiles: tiles}, doors, nodes, c.w, c.h
}

// carver hollows one cave out of the rock under a patch of Wilds.
type carver struct {
	g      *worldgen.Generator
	rng    *rand.Rand
	w, h   int
	ox, oy int // overworld coordinates of local (0,0)
	wall   [][]bool
	surf   [][]float64 // surface elevation overhead, per cell — the cave's echo of the land
}

func (c *carver) border(x, y int) bool { return x == 0 || y == 0 || x == c.w-1 || y == c.h-1 }

// hill reports whether the land overhead stands high enough for the cave to open
// out here; under valleys and water the rock stays solid.
func (c *carver) hill(x, y int) bool { return c.surf[y][x] >= caveFloorElev }

// carve hollows chambers where the hills rise, then links every mouth to the
// first by a winding passage bored at the mouths' true offsets.
func (c *carver) carve(doors [][2]int) {
	c.wall = make([][]bool, c.h)
	c.surf = make([][]float64, c.h)
	for y := 0; y < c.h; y++ {
		c.wall[y] = make([]bool, c.w)
		c.surf[y] = make([]float64, c.w)
		for x := 0; x < c.w; x++ {
			c.surf[y][x] = c.g.Elevation(c.ox+x, c.oy+y)
			c.wall[y][x] = c.border(x, y) || !c.hill(x, y) || c.rng.Float64() < 0.46
		}
	}
	c.smooth(4)
	for y := 0; y < c.h; y++ { // re-impose the surface boundary after smoothing
		for x := 0; x < c.w; x++ {
			if c.border(x, y) || !c.hill(x, y) {
				c.wall[y][x] = true
			}
		}
	}
	for _, d := range doors {
		c.openDisc(d[0], d[1], 3) // a clearing at every mouth
	}
	for i := 1; i < len(doors); i++ {
		c.tunnel(doors[i], doors[0]) // link the mouths, mapped 1:1 to the surface
	}
}

func (c *carver) smooth(passes int) {
	for it := 0; it < passes; it++ {
		next := make([][]bool, c.h)
		for y := 0; y < c.h; y++ {
			next[y] = make([]bool, c.w)
			copy(next[y], c.wall[y])
		}
		for y := 1; y < c.h-1; y++ {
			for x := 1; x < c.w-1; x++ {
				n := 0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if (dx != 0 || dy != 0) && c.wall[y+dy][x+dx] {
							n++
						}
					}
				}
				if n >= 5 {
					next[y][x] = true
				} else if n <= 2 {
					next[y][x] = false
				}
			}
		}
		c.wall = next
	}
}

func (c *carver) openDisc(cx, cy, r int) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				if x, y := cx+dx, cy+dy; x > 0 && y > 0 && x < c.w-1 && y < c.h-1 {
					c.wall[y][x] = false
				}
			}
		}
	}
}

// tunnel bores a winding two-wide passage from a toward b (a drunkard's walk
// biased at the target) — it will dig under a valley if the mouths sit on
// separate hills.
func (c *carver) tunnel(a, b [2]int) {
	x, y := a[0], a[1]
	open := func(px, py int) {
		for dy := 0; dy <= 1; dy++ {
			for dx := 0; dx <= 1; dx++ {
				if nx, ny := px+dx, py+dy; nx > 0 && ny > 0 && nx < c.w-1 && ny < c.h-1 {
					c.wall[ny][nx] = false
				}
			}
		}
	}
	for i := 0; i < 8000; i++ {
		open(x, y)
		if x == b[0] && y == b[1] {
			return
		}
		if c.rng.Float64() < 0.80 {
			if abs(b[0]-x) > abs(b[1]-y) {
				x += sign(b[0] - x)
			} else {
				y += sign(b[1] - y)
			}
		} else if c.rng.Intn(2) == 0 {
			x += sign(c.rng.Intn(3) - 1)
		} else {
			y += sign(c.rng.Intn(3) - 1)
		}
		x = clamp(x, 1, c.w-2)
		y = clamp(y, 1, c.h-2)
	}
}

// flood returns the open region connected to start (4-connected).
func (c *carver) flood(start [2]int) [][2]int {
	if c.wall[start[1]][start[0]] {
		return nil
	}
	seen := make([][]bool, c.h)
	for y := range seen {
		seen[y] = make([]bool, c.w)
	}
	var region [][2]int
	stack := [][2]int{start}
	seen[start[1]][start[0]] = true
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		region = append(region, p)
		for _, d := range nb4 {
			if nx, ny := p[0]+d[0], p[1]+d[1]; nx >= 0 && ny >= 0 && nx < c.w && ny < c.h && !c.wall[ny][nx] && !seen[ny][nx] {
				seen[ny][nx] = true
				stack = append(stack, [2]int{nx, ny})
			}
		}
	}
	return region
}

func (c *carver) openInterior() {
	for y := 1; y < c.h-1; y++ {
		for x := 1; x < c.w-1; x++ {
			c.wall[y][x] = false
		}
	}
}

var nb8 = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}}

func (c *carver) in(p [2]int) bool { return p[0] >= 0 && p[1] >= 0 && p[0] < c.w && p[1] < c.h }

// special places the cave's set-pieces, every one keyed to the land overhead:
// shafts of daylight where the rock runs thinnest, a glittering cache in the
// chamber deepest from any mouth, and — under a notable surface feature — an old
// mine beneath a peak or ruins beneath a landmark. Gatherable spots go into nodes.
func (c *carver) special(tiles [][]game.Tile, region, doors [][2]int, nodes map[[2]int]string) {
	plain := func(p [2]int) bool {
		t := tiles[p[1]][p[0]]
		return t.Kind == game.TileFloor && t.Prop == game.PropNone
	}
	openAround := func(p [2]int) int {
		n := 0
		for _, d := range nb8 {
			if q := [2]int{p[0] + d[0], p[1] + d[1]}; c.in(q) && tiles[q[1]][q[0]].Kind != game.TileWall {
				n++
			}
		}
		return n
	}
	far := func(p [2]int, d int) bool {
		for _, m := range doors {
			if abs(p[0]-m[0])+abs(p[1]-m[1]) <= d {
				return false
			}
		}
		return true
	}
	put := func(p [2]int, t game.Tile, item string) {
		tiles[p[1]][p[0]] = t
		if item != "" {
			nodes[p] = item
		}
	}

	// 1) Light shafts where the rock overhead runs thinnest — the lowest carveable
	// ground, where the cave roof rises nearest the surface.
	var thin [][2]int
	for _, p := range region {
		if plain(p) && openAround(p) >= 7 && far(p, 6) && c.surf[p[1]][p[0]] < caveFloorElev+0.05 {
			thin = append(thin, p)
		}
	}
	c.rng.Shuffle(len(thin), func(i, j int) { thin[i], thin[j] = thin[j], thin[i] })
	for i := 0; i < len(thin)/140+1 && i < len(thin); i++ {
		put(thin[i], lightShaft, "")
	}

	// 4) A treasure cache in the deepest chamber: a glowing geode ringed in crystal.
	deep, bestD := [2]int{}, -1
	for _, p := range region {
		if !plain(p) {
			continue
		}
		md := 1 << 30
		for _, m := range doors {
			if d := abs(p[0]-m[0]) + abs(p[1]-m[1]); d < md {
				md = d
			}
		}
		if md > bestD {
			bestD, deep = md, p
		}
	}
	if bestD > 6 {
		put(deep, geodeTile, "geode")
		for _, d := range nb8 {
			if n := [2]int{deep[0] + d[0], deep[1] + d[1]}; c.in(n) && plain(n) && c.rng.Float64() < 0.6 {
				put(n, crystalSeam.tile, "crystal")
			}
		}
	}

	// 5) A chamber under a notable surface feature. A surface landmark/gate overhead
	// makes ruins with a relic to recover; failing that, a peak overhead makes an
	// old mine — support timbers and a rich vein.
	var landmark [2]int
	haveLM := false
	hi, hiP := 0.0, [2]int{}
	for i, p := range region {
		if s := c.surf[p[1]][p[0]]; s > hi {
			hi, hiP = s, p
		}
		if !haveLM && i%4 == 0 { // sample the surface for a landmark overhead
			if cell := c.g.At(c.ox+p[0], c.oy+p[1]); cell.Portal != "" && cell.Portal != "cave" {
				landmark, haveLM = p, true
			}
		}
	}
	switch {
	case haveLM && plain(landmark):
		put(landmark, relicTile, "relic")
		for _, d := range nb8 {
			if n := [2]int{landmark[0] + d[0], landmark[1] + d[1]}; c.in(n) && plain(n) && c.rng.Float64() < 0.4 {
				put(n, stoneSeam.tile, "stone")
			}
		}
	case hi >= 0.82 && plain(hiP) && far(hiP, 5):
		put(hiP, goldSeam.tile, "nugget")
		timbers := 0
		for _, d := range nb8 {
			n := [2]int{hiP[0] + d[0], hiP[1] + d[1]}
			if !c.in(n) || !plain(n) {
				continue
			}
			if timbers < 2 && openAround(n) <= 6 { // timbers stand against the rock
				put(n, timbering, "")
				timbers++
			} else if c.rng.Float64() < 0.5 {
				s := goldSeam
				if c.rng.Float64() < 0.5 {
					s = crystalSeam
				}
				put(n, s.tile, s.item)
			}
		}
	}
}

// --- surface texture: what stops a cave looking like a flat grid ----------------
//
// Real rock is never one colour. texture gives every floor and wall cell its own
// shade — damp hollows and dry rises across the floor, deep dark in the heart of
// the rock and a lit lip where a wall faces open air — and, crucially, an ambient
// occlusion that pools shadow into every crevice where rock meets floor. The
// renderer dithers these per-tile colours into one another, so the hard tile grid
// dissolves into mottled, uneven stone.

func mustHex(s string) colorful.Color { c, _ := colorful.Hex(s); return c }

var (
	floorBase = mustHex("#655D6B")
	floorDry  = mustHex("#7C6F5E") // sandy rises
	floorWet  = mustHex("#37445E") // damp, bluish hollows
	crevice   = mustHex("#201C28") // the dark that pools against rock
	wallBase  = mustHex("#3C3644")
	wallDeep  = mustHex("#1E1A26") // the heart of solid rock
	wallFace  = mustHex("#544C5E") // a wall edge catching the light
	tintHi    = mustHex("#9C93A2")
)

// nhash is a cheap deterministic value in [0,1) for a cell.
func nhash(x, y int) float64 {
	h := uint32(x)*0x9E3779B1 + uint32(y)*0x85EBCA77 + 0x632BE5AB
	h ^= h >> 13
	h *= 0x2C1B3C6D
	h ^= h >> 16
	return float64(h) / float64(1<<32)
}

// patch is low-frequency noise (coarse blobs) for damp/dry floor patches.
func patch(x, y int) float64 {
	return 0.6*nhash(x>>2, y>>2) + 0.3*nhash(x>>1, y>>1) + 0.1*nhash(x, y)
}

// wallFrac is the share of a cell's 8 neighbours that are solid rock.
func wallFrac(tiles [][]game.Tile, x, y, w, h int) float64 {
	n := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			if nx, ny := x+dx, y+dy; nx >= 0 && ny >= 0 && nx < w && ny < h && tiles[ny][nx].Kind == game.TileWall {
				n++
			}
		}
	}
	return float64(n) / 8
}

func grain(c colorful.Color, x, y int, amt float64) colorful.Color {
	g := nhash(x*7+1, y*13+5)
	if g > 0.5 {
		return c.BlendLab(tintHi, (g-0.5)*2*amt)
	}
	return c.BlendLab(crevice, (0.5-g)*2*amt)
}

func texture(tiles [][]game.Tile, w, h int) {
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			t := &tiles[y][x]
			switch {
			case t.Kind == game.TileFloor && t.Prop == game.PropNone:
				c := floorBase
				if p := patch(x, y); p < 0.34 {
					c = c.BlendLab(floorWet, 0.55*(0.34-p)/0.34)
				} else if p > 0.70 {
					c = c.BlendLab(floorDry, 0.5*(p-0.70)/0.30)
				}
				c = grain(c, x, y, 0.08)
				// Ambient occlusion: pool shadow where the floor meets rock, fading
				// out a tile or two in (radius-2 reach for a soft contact shadow).
				ao := 0.7*wallFrac(tiles, x, y, w, h) + 0.3*wallFracR2(tiles, x, y, w, h)
				c = c.BlendLab(crevice, 0.6*ao)
				t.Ground, t.Color = c.Hex(), c.BlendLab(tintHi, 0.45).Hex()
			case t.Kind == game.TileWall:
				deep := wallFrac(tiles, x, y, w, h)
				c := wallBase.BlendLab(wallDeep, deep*0.9)
				if deep < 1 { // a face onto open air catches a little light
					c = c.BlendLab(wallFace, 0.3*(1-deep))
				}
				c = grain(c, x, y, 0.06)
				t.Ground, t.Color = c.Hex(), c.Hex()
			}
		}
	}
}

// wallFracR2 is the share of rock in the 5×5 around a cell — a wider, softer
// reach for the ambient-occlusion falloff.
func wallFracR2(tiles [][]game.Tile, x, y, w, h int) float64 {
	n, tot := 0, 0
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			tot++
			if nx, ny := x+dx, y+dy; nx >= 0 && ny >= 0 && nx < w && ny < h && tiles[ny][nx].Kind == game.TileWall {
				n++
			}
		}
	}
	return float64(n) / float64(tot)
}

// clutter strews the floor with breakdown — small rocks gathered at the foot of
// walls, the odd boulder or stalagmite out in the open — so chambers read as
// rubble-strewn rock rather than swept grey rooms.
func clutter(rng *rand.Rand, tiles [][]game.Tile, region [][2]int, w, h int) {
	for _, c := range region {
		x, y := c[0], c[1]
		t := &tiles[y][x]
		if t.Kind != game.TileFloor || t.Prop != game.PropNone {
			continue
		}
		wf := wallFrac(tiles, x, y, w, h)
		r := rng.Float64()
		switch {
		case wf >= 0.30 && r < 0.06: // flowstone draping a rock face (in-tile)
			t.Prop, t.PropHex, t.Ch = game.PropFlowstone, "#BBAA86", '╫'
		case wf > 0 && wf < 0.30 && r < 0.06: // a stalagmite rising near a wall
			t.Prop, t.PropHex, t.Ch = game.PropStalagmite, "#9A92A0", '▲'
		case wf == 0 && r < 0.016: // a column in a wide chamber (floor-to-ceiling, blocks)
			t.Kind, t.Walkable = game.TileDecor, false
			t.Prop, t.PropHex, t.Ch = game.PropColumn, "#A1937B", '█'
		case wf == 0 && r < 0.034: // a boulder fallen in the open chamber (blocks)
			t.Kind, t.Walkable = game.TileDecor, false
			t.Prop, t.PropHex = game.PropBoulder, mustHex("#46414E").Hex()
		case wf >= 0.25 && r < 0.20: // scree banked against the walls
			t.Prop, t.PropHex = game.PropRock, mustHex("#544E5A").Hex()
		case r < 0.013: // the odd loose stone underfoot
			t.Prop, t.PropHex = game.PropRock, mustHex("#4E4854").Hex()
		}
	}
}

// scatterLife stocks the cave with its mineral and living features and returns
// the gatherable ones (position → item). Mineral seams stud the rock faces; cave
// mushrooms cluster on the floor of the deep dark away from the mouth; still
// glow-pools pool in the wider chambers. All three light the dark.
func (c *carver) scatterLife(rng *rand.Rand, tiles [][]game.Tile, region, doors [][2]int) map[[2]int]string {
	w, h := c.w, c.h
	nodes := map[[2]int]string{}
	inBounds := func(p [2]int) bool { return p[0] >= 0 && p[1] >= 0 && p[0] < w && p[1] < h }
	free := func(p [2]int) bool { return inBounds(p) && tiles[p[1]][p[0]].Kind == game.TileFloor }
	openCount := func(p [2]int) int {
		n := 0
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if x, y := p[0]+dx, p[1]+dy; x >= 0 && y >= 0 && x < w && y < h && tiles[y][x].Kind != game.TileWall {
					n++
				}
			}
		}
		return n
	}
	farFromMouths := func(p [2]int, d int) bool {
		for _, m := range doors {
			if abs(p[0]-m[0])+abs(p[1]-m[1]) <= d {
				return false
			}
		}
		return true
	}

	// Mineral seams on rock faces — you work the cavern walls. The land overhead
	// sets what they yield: the richest veins lie under the peaks.
	var faces, richFaces [][2]int
	for _, p := range region {
		if !free(p) {
			continue
		}
		for _, d := range nb4 {
			if nx, ny := p[0]+d[0], p[1]+d[1]; nx >= 0 && ny >= 0 && nx < w && ny < h && tiles[ny][nx].Kind == game.TileWall {
				faces = append(faces, p)
				if c.surf[p[1]][p[0]] >= caveDeepElev {
					richFaces = append(richFaces, p) // under the mountains — work them harder
				}
				break
			}
		}
	}
	place := func(p [2]int) {
		if _, taken := nodes[p]; taken || !free(p) {
			return
		}
		s := seamFor(c.surf[p[1]][p[0]], rng)
		nodes[p] = s.item
		tiles[p[1]][p[0]] = s.tile
	}
	rng.Shuffle(len(faces), func(i, j int) { faces[i], faces[j] = faces[j], faces[i] })
	for i := 0; i < len(region)/40+6 && i < len(faces); i++ {
		place(faces[i])
	}
	rng.Shuffle(len(richFaces), func(i, j int) { richFaces[i], richFaces[j] = richFaces[j], richFaces[i] })
	for i := 0; i < len(richFaces)/8+1 && i < len(richFaces); i++ { // a bonus seam under the peaks
		place(richFaces[i])
	}

	// Mushroom clusters in the deep dark, well away from any mouth.
	var deep [][2]int
	for _, p := range region {
		if free(p) && farFromMouths(p, 14) {
			deep = append(deep, p)
		}
	}
	rng.Shuffle(len(deep), func(i, j int) { deep[i], deep[j] = deep[j], deep[i] })
	for i := 0; i < len(deep)/90+4 && i < len(deep); i++ {
		for _, p := range append([][2]int{deep[i]}, neighboursOf(deep[i], rng)...) {
			if free(p) {
				if _, taken := nodes[p]; !taken {
					nodes[p] = "mushroom"
					tiles[p[1]][p[0]] = mushroom
				}
			}
		}
	}

	// Glow-pools: cave water gathers in the chambers under the low ground — under
	// the valleys and the foot of the hills, where the water table runs nearest —
	// rather than up under the peaks. Kept walkable so they never seal a way.
	var basins [][2]int
	for _, p := range region {
		if free(p) && openCount(p) >= 7 && farFromMouths(p, 8) && c.surf[p[1]][p[0]] < caveLowElev {
			basins = append(basins, p)
		}
	}
	if len(basins) == 0 { // a dry, high cave still keeps a pool or two in its widest room
		for _, p := range region {
			if free(p) && openCount(p) >= 8 && farFromMouths(p, 8) {
				basins = append(basins, p)
			}
		}
	}
	rng.Shuffle(len(basins), func(i, j int) { basins[i], basins[j] = basins[j], basins[i] })
	for i := 0; i < len(basins)/30+2 && i < len(basins); i++ {
		for _, p := range append([][2]int{basins[i]}, neighboursOf(basins[i], rng)...) {
			if free(p) {
				if _, taken := nodes[p]; !taken {
					tiles[p[1]][p[0]] = glowPool
				}
			}
		}
	}
	return nodes
}

// neighboursOf returns a couple of random orthogonal neighbours of c, for growing
// little clusters.
func neighboursOf(c [2]int, rng *rand.Rand) [][2]int {
	var out [][2]int
	for _, d := range nb4 {
		if rng.Float64() < 0.55 {
			out = append(out, [2]int{c[0] + d[0], c[1] + d[1]})
		}
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}

func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

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
	"sort"
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
	caveName       string            // the mood's name for this cave ("an ice cave", …)
	overworldDoors [][2]int          // each surface mouth's overworld cell
	interiorDoors  [][2]int          // …and the matching mouth inside the cave (parallel)
	nodes          map[[2]int]string // gatherable position → item id
	floors         map[[2]int]game.Tile // the dressed floor under each node, for restore on gather
	mined          map[[2]int]bool   // worked out this visit
	discovered     map[[2]int]uint64 // uncovered fog chunks (chunk coord → 64-cell mask)
	dirty          map[[2]int]bool   // chunks changed since the last flush
	showMap        bool              // the fill-in cave map is open (m)
	fuel           int               // lantern oil left this visit; light shrinks as it runs low
	warnedLow      bool              // already told the player the lantern's guttering
	crossing       bool              // currently striding a chasm on the diadem's power
	stepN          int               // steps taken this visit (paces the lantern-cap's oil saving)
	toast          string
	toastUntil     time.Time
}

func (a *area) Name() string {
	if a.caveName != "" {
		return a.caveName // the mood names the cave: "an ice cave", "a glowspore cavern", …
	}
	return "a cave"
}

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
	a.Map, a.interiorDoors, a.nodes, a.floors, a.caveName, a.w, a.h = genCaveFromWilds(gen, sys.Doors, rand.New(rand.NewSource(seed)))
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
	a.OnTile = a.onTile // burn oil, cross chasms and lift the dark per cell walked
	a.Enter(sp[0], sp[1], 0)
	a.reveal()
	a.persist()
	if msg := a.wornPowerHint(); msg != "" {
		a.setToast(msg) // so you know what the thing on your head is doing down here
	}
	return nil
}

// wornPowerHint names the power of a cave-relevant wearable on descent, so its
// effect isn't a mystery the player never connects to their hat.
func (a *area) wornPowerHint() string {
	switch {
	case a.Ctx.Wearing("crown"):
		return "👑 the crown senses treasure through the rock"
	case a.Ctx.Wearing("diadem"):
		return "✦ the relic diadem will steady you over chasms"
	case a.Ctx.Wearing("glowcap"):
		return "🍄 the lantern-cap lights your way and spares your oil"
	}
	return ""
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
	if handled && portal != "" {
		a.surfaceAt() // leave by whichever mouth we reached
		return game.Transition{To: portal}, nil
	}
	return a, nil
}

// onTile is the cave's per-step work, hung off walking into a new cell rather
// than off a keypress (game.Walker.OnTile).
//
// The lantern's oil has always been denominated in steps — fuelMax is "steps of
// light" — and a step has always meant a tile, so this keeps the mechanic
// exactly as it was while the movement underneath it became continuous. It is
// also more honest than the per-key hook it replaces, which burned oil for a
// step a wall had refused, and could drop you down a chasm you never entered.
func (a *area) onTile(int, int) {
	a.burnLantern() // a step spends oil, or the glow tops it up
	switch {
	case !a.onChasm():
		a.crossing = false
	case a.Ctx.Wearing("diadem"): // the relic steadies your step over the void
		if !a.crossing {
			a.crossing = true
			a.setToast("✦ the relic diadem carries you over the chasm")
		}
	default:
		a.fall() // a misstep into the dark drops you back at the mouth
	}
	a.reveal() // a step lifts the dark as far as the light now throws
	a.persist()
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
	a.senseTreasure()
}

// senseTreasure is the crown's power: it picks out valuables through the rock,
// uncovering each gatherable node and mouth within a wide radius so they glimmer
// in the dark ahead (the luminous ones — crystals, geodes — even shine through).
func (a *area) senseTreasure() {
	if !a.Ctx.Wearing("crown") {
		return
	}
	const senseR = 26 // far past the lantern — the crown's whole point
	within := func(x, y int) bool {
		dx, dy := x-a.X, y-a.Y
		return dx*dx+dy*dy <= senseR*senseR
	}
	for p := range a.nodes {
		if within(p[0], p[1]) {
			a.markSeen(p[0], p[1])
		}
	}
	for _, d := range a.interiorDoors {
		if within(d[0], d[1]) {
			a.markSeen(d[0], d[1])
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

// isForage reports whether an item is soft glowing life you gather by hand
// (the mood gatherables) rather than rock you have to mine.
func isForage(item string) bool {
	switch item {
	case "mushroom", "spore", "amber":
		return true
	}
	return false
}

// gather works out a seam or picks a mushroom: it drops into the player's pack,
// the spot becomes plain cave floor, and a toast confirms the haul.
func (a *area) gather(pos [2]int, item string) {
	a.mined[pos] = true
	// Restore the dressed floor that was under the node — shaded, mood-tinted
	// stone — not the raw caveFloor constant, which left flat grey holes in a
	// moss-green or ochre cave.
	if t, ok := a.floors[pos]; ok {
		a.Map.Tiles[pos[1]][pos[0]] = t
	} else {
		a.Map.Tiles[pos[1]][pos[0]] = caveFloor
	}
	yield := 1
	if a.Ctx.ForagerBoon() { // a gatherer's wearable draws a richer haul
		yield = 2
	}
	for i := 0; i < yield; i++ {
		a.Ctx.Inventory[item]++
		a.Ctx.Store.AddItem(a.Ctx.Name, item)
	}
	name := item
	it, known := game.ItemByID(item)
	if known {
		name = it.Name
	}
	verb := "⛏ mined"
	if isForage(item) {
		verb = "🍄 gathered"
	}
	if yield > 1 {
		name = fmt.Sprintf("%s ×%d", name, yield)
	}
	a.setToast(verb + " " + name)
	// The deep prizes double as trophies: a geode wins its circlet, a relic its
	// diadem, the first time you carry one out into your hands.
	if known && it.Wear != "" {
		if idx, ok := game.AccessoryIndex(it.Wear); ok && a.unlockHat(idx) {
			a.setToast("✦ " + name + " — now wearable! (c to equip)")
		}
	}
}

// unlockHat marks an accessory owned and persists it, idempotently; returns
// whether it was newly unlocked (so the caller can announce the trophy).
func (a *area) unlockHat(idx int) bool {
	if a.Ctx.Hats == nil {
		a.Ctx.Hats = map[int]bool{}
	}
	if a.Ctx.Hats[idx] {
		return false
	}
	a.Ctx.Hats[idx] = true
	a.Ctx.Store.UnlockHat(a.Ctx.Name, idx)
	return true
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
		if isForage(item) {
			verb = "gather"
		}
		return "e — " + verb + " the " + name
	}
	if a.fuel <= fuelLow {
		return "🕯 your lantern is guttering — " + a.mouthBearing() + " · rest beside the glow to rekindle it"
	}
	if h := a.PortalHint(); h != "" {
		return h
	}
	return "🕯 a cave — follow the glow into the dark · ∩ return to the mouth to leave"
}

// mouthBearing names the compass direction and rough distance to the nearest
// mouth, so a guttering lantern comes with a way out instead of blind panic.
func (a *area) mouthBearing() string {
	best, bestD := a.interiorDoors[0], 1<<30
	for _, d := range a.interiorDoors {
		if dd := abs(d[0]-a.X) + abs(d[1]-a.Y); dd < bestD {
			best, bestD = d, dd
		}
	}
	dx, dy := best[0]-a.X, best[1]-a.Y
	dir := ""
	switch {
	case dy < -2:
		dir = "N"
	case dy > 2:
		dir = "S"
	}
	switch {
	case dx > 2:
		dir += "E"
	case dx < -2:
		dir += "W"
	}
	if dir == "" {
		return "the mouth is here"
	}
	return fmt.Sprintf("the mouth lies %s, ~%d paces", dir, bestD)
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
	return game.Light{X: a.X + game.PlayerW/2, Y: a.Y + game.PlayerH/2, Radius: a.lanternRadius(), Warm: true, Sunless: true}
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

// onChasm reports whether the player's body is over an open chasm.
func (a *area) onChasm() bool {
	for dy := 0; dy < game.PlayerH; dy++ {
		for dx := 0; dx < game.PlayerW; dx++ {
			x, y := a.X+dx, a.Y+dy
			if x >= 0 && y >= 0 && x < a.w && y < a.h && a.Map.At(x, y).Prop == game.PropChasm {
				return true
			}
		}
	}
	return false
}

// fall handles a misstep into a chasm: you scramble out at the nearest mouth,
// keeping your map but losing your trek — and the drop jostles the lantern
// half-dark. A real cost the light helps you avoid, and the dark invites.
func (a *area) fall() {
	best, bestD := a.interiorDoors[0], 1<<30
	for _, d := range a.interiorDoors {
		if dd := abs(d[0]-a.X) + abs(d[1]-a.Y); dd < bestD {
			best, bestD = d, dd
		}
	}
	a.Enter(best[0], best[1], 0)
	if a.fuel > fuelLow+8 {
		a.fuel = fuelLow + 8
	}
	a.warnedLow = a.fuel <= fuelLow
	a.setToast("🕳 you stumble into a chasm — and scramble out at the mouth")
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
	a.stepN++
	burn := fuelBurn
	if a.Ctx.Wearing("glowcap") && a.stepN%2 == 0 {
		burn = 0 // the lantern-cap's own glow lets the lantern sip oil — half the burn
	}
	if a.fuel -= burn; a.fuel < 0 {
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
				// The dark keeps the floor's shape (like the Wilds' fog), so
				// the 3D relief doesn't jump at the lantern's edge.
				f := caveFog()
				f.Elev = a.Map.At(wx, wy).Elev
				row[lx] = f
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
		case game.PropChasm:
			return "▽", "#6A6470", 5 // a hazard worth charting
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

// HDMinimap supplies the same chart to the HD pixel client, which draws the
// cells as colored blocks. It mirrors minimap: the same adaptive stride and
// per-block feature colors (via miniBlock), the player's block marked.
func (a *area) HDMinimap() (string, [][]game.MiniCell, bool) {
	if !a.showMap {
		return "", nil, false
	}
	stride := 2
	for a.w/stride > 46 || a.h/stride > 30 {
		stride++
	}
	rows := make([][]game.MiniCell, 0, a.h/stride+1)
	for my := 0; my < a.h; my += stride {
		row := make([]game.MiniCell, 0, a.w/stride+1)
		for mx := 0; mx < a.w; mx += stride {
			switch {
			case a.X >= mx && a.X < mx+stride && a.Y >= my && a.Y < my+stride:
				row = append(row, game.MiniCell{Self: true})
			default:
				if _, color, ok := a.miniBlock(mx, my, stride, stride); ok {
					row = append(row, game.MiniCell{Hex: color})
				} else {
					row = append(row, game.MiniCell{}) // unexplored dark
				}
			}
		}
		rows = append(rows, row)
	}
	return "Map — the cave", rows, true
}

// caveFog is the unbroken black of cave the lantern hasn't found yet.
func caveFog() game.Tile {
	return game.Tile{Kind: game.TileWall, Ch: ' ', Color: "#05070A", Tex: game.TexFlat, Ground: "#05070A"}
}

// --- cavern generation ---------------------------------------------------------

var (
	rockWall  = game.Tile{Kind: game.TileWall, Ch: '▓', Walkable: false, Color: "#4E4656", Tex: game.TexRock, Ground: "#2E2935"}
	caveFloor = game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true, Color: "#A39AA9", Tex: game.TexDirt, Ground: "#746C7C"}
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
	// A chasm in the floor. Walkable so it never walls off a passage (and so you
	// *can* misstep into it) — the black ground is the drop, the prop a lit lip.
	chasm = game.Tile{Kind: game.TileFloor, Ch: '▽', Walkable: true, Color: "#1A1620",
		Tex: game.TexRock, Ground: "#08060C", Prop: game.PropChasm, PropHex: "#8C8494"}
)

var nb4 = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// cavePalette is a cave's colour mood — a rock hue and the colours its life and
// crystal glow — taken from the land overhead so no two stretches of cave look
// quite alike: ice under the cold heights, moss under wet woods, ochre under
// warm dry country, and one of three slates under temperate hills. The mood
// also bends the cave's structure (how gladly it opens chasms, how thickly its
// life glows) and names the cave, so a haul and a memory both say where you
// were.
type cavePalette struct {
	name     string         // what the HUD calls this cave
	rock     colorful.Color // hue the rock (walls, floor, stone) is shifted toward
	glow     string         // bioluminescence: mushrooms and pools
	crystal  string         // crystal seams and the geode core
	material string         // the signature thing its glowing life is gathered as
	chasmDiv int            // chasm density divisor — smaller is more chasms
	glowDiv  int            // glow-cluster density divisor — smaller is more life
}

func paletteFor(temp, moist, variant float64) cavePalette {
	switch {
	case temp < 0.34: // cold heights: blue ice and frost — harsh, sparse life, split floors
		return cavePalette{name: "an ice cave", rock: mustHex("#3B4A6B"),
			glow: "#8FE0FF", crystal: "#CFEEFF", material: "crystal", chasmDiv: 45, glowDiv: 140}
	case moist > 0.60: // wet woods: green moss and glowspore — a lush resting cave
		return cavePalette{name: "a glowspore cavern", rock: mustHex("#36482F"),
			glow: "#8BF29C", crystal: "#7DF0C6", material: "spore", chasmDiv: 140, glowDiv: 50}
	case temp > 0.60 && moist < 0.42: // warm dry country: ochre sandstone and amber
		return cavePalette{name: "an amber cave", rock: mustHex("#54422C"),
			glow: "#FFC871", crystal: "#FFE3A0", material: "amber", chasmDiv: 90, glowDiv: 90}
	}
	// Temperate hills: the commonest case once got no treatment at all, which
	// is why most caves looked identical. Three close-but-distinct slates now
	// split it, picked per cave.
	slates := []cavePalette{
		{name: "a slate cavern", rock: mustHex("#4A4458")},
		{name: "a bluestone cavern", rock: mustHex("#3E4756")},
		{name: "a greenstone cavern", rock: mustHex("#414F49")},
	}
	pal := slates[int(variant*float64(len(slates)))%len(slates)]
	pal.glow, pal.crystal, pal.material = "#7CF2C4", "#7DF0FF", "mushroom"
	pal.chasmDiv, pal.glowDiv = 90, 90
	return pal
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

// seamAt picks a mineral seam for a rock face by how deep into the cave it
// lies — near the mouths the rock gives up plain stone, the mid-cave runs to
// gold, and the deep glitters. Depth is something the player can feel and
// learn (the old key, the height of the land overhead, was invisible from
// underground); the peaks still add their bonus tier on top.
func (c *carver) seamAt(p [2]int, rng *rand.Rand) seam {
	tier := c.tierAt(p)
	if c.surf[p[1]][p[0]] >= caveDeepElev && tier < 2 {
		tier++ // under the mountains the veins run one tier richer
	}
	r := rng.Float64()
	switch tier {
	case 0:
		if r < 0.92 {
			return stoneSeam
		}
		return goldSeam
	case 1:
		switch {
		case r < 0.60:
			return stoneSeam
		case r < 0.88:
			return goldSeam
		default:
			return crystalSeam
		}
	default:
		switch {
		case r < 0.25:
			return stoneSeam
		case r < 0.60:
			return goldSeam
		default:
			return crystalSeam
		}
	}
}

// tierAt is a cell's depth tier: 0 by the mouths, 1 in the mid-cave, 2 in the
// deep. The thresholds scale with the cave, so a small cave still has a deep.
func (c *carver) tierAt(p [2]int) int {
	d := c.depth[p[1]][p[0]]
	switch {
	case d < 0 || d < c.d1:
		return 0
	case d < c.d2:
		return 1
	}
	return 2
}

// computeDepth is a multi-source BFS from every mouth across the open floor —
// the "how far in am I" field the reward tiers key off.
func (c *carver) computeDepth(doors [][2]int) {
	c.depth = make([][]int, c.h)
	for y := range c.depth {
		c.depth[y] = make([]int, c.w)
		for x := range c.depth[y] {
			c.depth[y][x] = -1
		}
	}
	var q [][2]int
	for _, d := range doors {
		if !c.wall[d[1]][d[0]] {
			c.depth[d[1]][d[0]] = 0
			q = append(q, d)
		}
	}
	for len(q) > 0 {
		p := q[0]
		q = q[1:]
		for _, d := range nb4 {
			nx, ny := p[0]+d[0], p[1]+d[1]
			if nx >= 0 && ny >= 0 && nx < c.w && ny < c.h && !c.wall[ny][nx] && c.depth[ny][nx] < 0 {
				c.depth[ny][nx] = c.depth[p[1]][p[0]] + 1
				q = append(q, [2]int{nx, ny})
			}
		}
	}
}

// pruneDeadEnds fills the CA fringe's dead-end nubs — one-wide pockets that
// cost attention and pay nothing — while sparing the mouths and chamber cores.
func (c *carver) pruneDeadEnds(doors [][2]int) {
	keep := map[[2]int]bool{}
	for _, d := range doors {
		keep[d] = true
	}
	for _, ch := range c.chambers {
		keep[[2]int{ch[0], ch[1]}] = true
	}
	for round := 0; round < 3; round++ {
		var fill [][2]int
		for y := 1; y < c.h-1; y++ {
			for x := 1; x < c.w-1; x++ {
				if c.wall[y][x] || keep[[2]int{x, y}] {
					continue
				}
				n := 0
				for _, d := range nb4 {
					if !c.wall[y+d[1]][x+d[0]] {
						n++
					}
				}
				if n <= 1 {
					fill = append(fill, [2]int{x, y})
				}
			}
		}
		if len(fill) == 0 {
			break
		}
		for _, p := range fill {
			c.wall[p[1]][p[0]] = true
		}
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

func genCaveFromWilds(g *worldgen.Generator, overDoors [][2]int, rng *rand.Rand) (*game.TileMap, [][2]int, map[[2]int]string, map[[2]int]game.Tile, string, int, int) {
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
	c.pruneDeadEnds(doors)
	region := c.flood(doors[0])
	if len(region) < 60 { // pathological (no hills?) — open a plain chamber instead
		c.openInterior()
		region = c.flood(doors[0])
	}
	// The depth field: how far each floor cell lies from the nearest mouth.
	// Its tiers scale with the cave, so even a small cave has a deep to earn.
	c.computeDepth(doors)
	maxD := 0
	for _, p := range region {
		if d := c.depth[p[1]][p[0]]; d > maxD {
			maxD = d
		}
	}
	c.d1, c.d2 = max(8, maxD*35/100), max(16, maxD*70/100)

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
	// Snapshot the dressed, empty floor before any content is stamped on it:
	// gathering a seam later restores this exact tile, so a worked-out spot
	// keeps its shading and mood instead of reverting to flat slate.
	base := make([][]game.Tile, c.h)
	for y := range tiles {
		base[y] = append([]game.Tile(nil), tiles[y]...)
	}
	_, moist, temp := g.Climate(overDoors[0][0], overDoors[0][1]) // mood from the land above
	pal := paletteFor(temp, moist, rng.Float64())
	nodes := c.scatterLife(rng, tiles, region, doors, pal)
	c.special(tiles, region, doors, nodes, pal)
	c.vestibules(tiles, doors, nodes)
	c.clutter(rng, tiles, region)
	c.recolour(tiles, pal)
	c.recolour(base, pal)
	// The floor keeps a gentle echo of the land overhead, so chambers under
	// the peaks rise and the low galleries dip in the 3D client. Imposed last,
	// over every stamped tile, so no content pass can flatten it.
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			e := 0.34 + (c.surf[y][x]-0.50)*0.35
			if e < 0.30 {
				e = 0.30
			}
			tiles[y][x].Elev = e
			base[y][x].Elev = e
		}
	}
	floors := make(map[[2]int]game.Tile, len(nodes))
	for p := range nodes {
		floors[p] = base[p[1]][p[0]]
	}
	// The cave's glowing life is gathered as its mood's signature material, so a
	// haul tells you which cave it came from: spores from moss, amber from ochre.
	if pal.material != "mushroom" {
		for p, item := range nodes {
			if item == "mushroom" {
				nodes[p] = pal.material
			}
		}
	}
	return &game.TileMap{W: c.w, H: c.h, Tiles: tiles}, doors, nodes, floors, pal.name, c.w, c.h
}

// carver hollows one cave out of the rock under a patch of Wilds.
type carver struct {
	g      *worldgen.Generator
	rng    *rand.Rand
	w, h   int
	ox, oy int // overworld coordinates of local (0,0)
	wall   [][]bool
	surf   [][]float64 // surface elevation overhead, per cell — the cave's echo of the land
	// The chamber graph (see carve): the CA mask supplies the rock's texture,
	// but rooms, corridors and loops come from a deliberate structure over it.
	chambers [][3]int // chamber cores: x, y, radius
	depth    [][]int  // BFS steps from the nearest mouth; -1 in rock
	d1, d2   int      // depth-tier thresholds (mouth / mid / deep)
}

func (c *carver) border(x, y int) bool { return x == 0 || y == 0 || x == c.w-1 || y == c.h-1 }

// hill reports whether the land overhead stands high enough for the cave to open
// out here; under valleys and water the rock stays solid.
func (c *carver) hill(x, y int) bool { return c.surf[y][x] >= caveFloorElev }

// carve hollows the cave in two registers. The cellular automaton supplies the
// organic register — ragged rock shaped by the land overhead — and a chamber
// graph supplies the deliberate one: readable rooms at the mask's natural
// centres, corridors that vary in width, and guaranteed loops so there is
// always a second way back. A pure CA blob has no rooms, no thresholds and no
// loops; a pure graph has no geology. The cave needs both.
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

	// Chamber cores: the deep interior points of the CA mask, found with a
	// distance transform — where the rock naturally wants a room.
	dist := c.distTransform()
	c.chambers = c.chamberCores(dist)

	// The node set: every mouth, then every chamber. Their order is fixed, so
	// the MST below is deterministic.
	nodes := make([][2]int, 0, len(doors)+len(c.chambers))
	nodes = append(nodes, doors...)
	for _, ch := range c.chambers {
		nodes = append(nodes, [2]int{ch[0], ch[1]})
	}

	// A minimum spanning tree links every room and mouth; a few extra edges
	// between far-apart nodes turn the tree into a network. Loops are the
	// difference between "backtrack through everything you've seen" and "come
	// out a different way" — the single cheapest navigability feature a cave
	// can have.
	mst, extra := c.planEdges(nodes)

	// Vestibules first, so every mouth opens into a real entry chamber.
	for _, d := range doors {
		c.openDisc(d[0], d[1], 4)
	}
	// Carve the network. Trunk corridors (touching a mouth) run wide; the rest
	// vary between snug and comfortable — never below the 2×2 body's passage.
	for _, e := range mst {
		wdt := 2
		if e[0] < len(doors) || e[1] < len(doors) {
			wdt = 3
		}
		c.tunnel(nodes[e[0]], nodes[e[1]], wdt)
	}
	for _, e := range extra {
		c.tunnel(nodes[e[0]], nodes[e[1]], 2)
	}
	// Open the chambers themselves out to their natural radius.
	for _, ch := range c.chambers {
		c.openDisc(ch[0], ch[1], clamp(ch[2], 3, 7))
	}
}

// distTransform is the L1 distance from every open cell to the nearest rock,
// by the classic two-pass sweep. Rock is 0.
func (c *carver) distTransform() [][]int {
	const inf = 1 << 20
	d := make([][]int, c.h)
	for y := 0; y < c.h; y++ {
		d[y] = make([]int, c.w)
		for x := 0; x < c.w; x++ {
			if c.wall[y][x] {
				d[y][x] = 0
			} else {
				d[y][x] = inf
			}
		}
	}
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			if y > 0 && d[y-1][x]+1 < d[y][x] {
				d[y][x] = d[y-1][x] + 1
			}
			if x > 0 && d[y][x-1]+1 < d[y][x] {
				d[y][x] = d[y][x-1] + 1
			}
		}
	}
	for y := c.h - 1; y >= 0; y-- {
		for x := c.w - 1; x >= 0; x-- {
			if y < c.h-1 && d[y+1][x]+1 < d[y][x] {
				d[y][x] = d[y+1][x] + 1
			}
			if x < c.w-1 && d[y][x+1]+1 < d[y][x] {
				d[y][x] = d[y][x+1] + 1
			}
		}
	}
	return d
}

// chamberCores picks the mask's natural room centres: local maxima of the
// distance transform, greedily thinned so cores keep their elbow room.
func (c *carver) chamberCores(dist [][]int) [][3]int {
	type core struct{ x, y, r int }
	var cand []core
	for y := 1; y < c.h-1; y++ {
		for x := 1; x < c.w-1; x++ {
			r := dist[y][x]
			if r < 3 {
				continue
			}
			peak := true
			for dy := -1; dy <= 1 && peak; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dist[y+dy][x+dx] > r {
						peak = false
						break
					}
				}
			}
			if peak {
				cand = append(cand, core{x, y, r})
			}
		}
	}
	// Deepest first; a stable sort keeps scan order on ties, so the pick is
	// deterministic.
	sort.SliceStable(cand, func(i, j int) bool { return cand[i].r > cand[j].r })
	var out [][3]int
	for _, cd := range cand {
		ok := true
		for _, o := range out {
			dx, dy := cd.x-o[0], cd.y-o[1]
			if dx*dx+dy*dy < 9*9 {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, [3]int{cd.x, cd.y, cd.r})
		}
	}
	return out
}

// planEdges builds a Prim MST over the nodes plus a few loop edges between
// nodes that the tree leaves far apart (≥4 hops), so the cave is a network
// rather than a corridor tree.
func (c *carver) planEdges(nodes [][2]int) (mst, extra [][2]int) {
	n := len(nodes)
	if n < 2 {
		return nil, nil
	}
	d2 := func(a, b [2]int) int {
		dx, dy := a[0]-b[0], a[1]-b[1]
		return dx*dx + dy*dy
	}
	in := make([]bool, n)
	best := make([]int, n)   // cheapest cost into the tree
	parent := make([]int, n) // …and via which node
	for i := range best {
		best[i] = 1 << 30
		parent[i] = -1
	}
	in[0], best[0] = true, 0
	for i := 1; i < n; i++ {
		best[i] = d2(nodes[0], nodes[i])
		parent[i] = 0
	}
	adj := make([][]int, n)
	for range make([]struct{}, n-1) {
		pick, pc := -1, 1<<30
		for i := 0; i < n; i++ {
			if !in[i] && best[i] < pc {
				pick, pc = i, best[i]
			}
		}
		if pick < 0 {
			break
		}
		in[pick] = true
		mst = append(mst, [2]int{parent[pick], pick})
		adj[parent[pick]] = append(adj[parent[pick]], pick)
		adj[pick] = append(adj[pick], parent[pick])
		for i := 0; i < n; i++ {
			if !in[i] {
				if d := d2(nodes[pick], nodes[i]); d < best[i] {
					best[i], parent[i] = d, pick
				}
			}
		}
	}
	// Loop edges: the shortest non-tree edges whose endpoints sit ≥4 hops
	// apart in the tree, up to one per five nodes (at least one).
	type ced struct{ a, b, d int }
	var cands []ced
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			cands = append(cands, ced{a, b, d2(nodes[a], nodes[b])})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].d < cands[j].d })
	want := n/5 + 1
	for _, e := range cands {
		if want == 0 {
			break
		}
		if hopDist(adj, e.a, e.b) >= 4 {
			extra = append(extra, [2]int{e.a, e.b})
			adj[e.a] = append(adj[e.a], e.b)
			adj[e.b] = append(adj[e.b], e.a)
			want--
		}
	}
	return mst, extra
}

// hopDist is the BFS hop count between two nodes of the small chamber graph.
func hopDist(adj [][]int, a, b int) int {
	if a == b {
		return 0
	}
	seen := make([]int, len(adj))
	for i := range seen {
		seen[i] = -1
	}
	seen[a] = 0
	q := []int{a}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		for _, nx := range adj[cur] {
			if seen[nx] < 0 {
				seen[nx] = seen[cur] + 1
				if nx == b {
					return seen[nx]
				}
				q = append(q, nx)
			}
		}
	}
	return 1 << 30
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

// tunnel bores a winding passage from a toward b (a drunkard's walk biased at
// the target) — it will dig under a valley if the endpoints sit on separate
// hills. wdt is the brush width in cells; 2 is the floor (the player's 2×2
// body must fit), 3 is a trunk corridor.
func (c *carver) tunnel(a, b [2]int, wdt int) {
	if wdt < 2 {
		wdt = 2
	}
	x, y := a[0], a[1]
	open := func(px, py int) {
		for dy := 0; dy < wdt; dy++ {
			for dx := 0; dx < wdt; dx++ {
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
func (c *carver) special(tiles [][]game.Tile, region, doors [][2]int, nodes map[[2]int]string, pal cavePalette) {
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

	// 2) Chasms split the open chambers — only in wide-open ground (so you can
	// always round them) and clear of the mouths, never in a passage (they stay
	// walkable, so they can't wall a route off; you just don't want to walk in).
	// The mood sets their number: an ice cave's floor is riven, a moss cavern's
	// barely at all.
	var brink [][2]int
	for _, p := range region {
		if plain(p) && openAround(p) == 8 && far(p, 8) {
			brink = append(brink, p)
		}
	}
	c.rng.Shuffle(len(brink), func(i, j int) { brink[i], brink[j] = brink[j], brink[i] })
	for i := 0; i < len(brink)/pal.chasmDiv+1 && i < len(brink); i++ {
		put(brink[i], chasm, "")
	}

	// 3) A treasure cache in the deepest chamber: a glowing geode ringed in
	// crystal. Deepest by walked distance (the depth field), and required to
	// sit in open ground, so the prize crowns a real room rather than hiding
	// in a crack.
	deep, bestD := [2]int{}, -1
	for _, p := range region {
		if !plain(p) || openAround(p) < 7 {
			continue
		}
		if d := c.depth[p[1]][p[0]]; d > bestD {
			bestD, deep = d, p
		}
	}
	if bestD > c.d1 {
		put(deep, geodeTile, "geode")
		for _, d := range nb8 {
			if n := [2]int{deep[0] + d[0], deep[1] + d[1]}; c.in(n) && plain(n) && c.rng.Float64() < 0.6 {
				put(n, crystalSeam.tile, "crystal")
			}
		}
	}

	// 4) A chamber under a notable surface feature. A surface landmark/gate overhead
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
// rubble-strewn rock rather than swept grey rooms. The entry halls stay clear
// of blocking breakdown, so a mouth never opens onto an obstacle course.
func (c *carver) clutter(rng *rand.Rand, tiles [][]game.Tile, region [][2]int) {
	w, h := c.w, c.h
	for _, cell := range region {
		x, y := cell[0], cell[1]
		t := &tiles[y][x]
		if t.Kind != game.TileFloor || t.Prop != game.PropNone {
			continue
		}
		nearMouth := c.depth[y][x] >= 0 && c.depth[y][x] < 10
		wf := wallFrac(tiles, x, y, w, h)
		r := rng.Float64()
		switch {
		case wf >= 0.30 && r < 0.06: // flowstone draping a rock face (in-tile)
			t.Prop, t.PropHex, t.Ch = game.PropFlowstone, "#BBAA86", '╫'
		case wf > 0 && wf < 0.30 && r < 0.06: // a stalagmite rising near a wall
			t.Prop, t.PropHex, t.Ch = game.PropStalagmite, "#9A92A0", '▲'
		case wf == 0 && r < 0.016 && !nearMouth: // a column in a wide chamber (floor-to-ceiling, blocks)
			t.Kind, t.Walkable = game.TileDecor, false
			t.Prop, t.PropHex, t.Ch = game.PropColumn, "#A1937B", '█'
		case wf == 0 && r < 0.034 && !nearMouth: // a boulder fallen in the open chamber (blocks)
			t.Kind, t.Walkable = game.TileDecor, false
			t.Prop, t.PropHex = game.PropBoulder, mustHex("#46414E").Hex()
		case wf >= 0.25 && r < 0.20: // scree banked against the walls
			t.Prop, t.PropHex = game.PropRock, mustHex("#544E5A").Hex()
		case r < 0.013: // the odd loose stone underfoot
			t.Prop, t.PropHex = game.PropRock, mustHex("#4E4854").Hex()
		}
	}
}

// vestibules dresses every entry chamber: a shaft of daylight just inside the
// mouth (the "day behind you" read, and a guaranteed lantern top-up anchor at
// the door), and a starter stone seam within reach — a first-time visitor
// mines something inside their first ten steps.
func (c *carver) vestibules(tiles [][]game.Tile, doors [][2]int, nodes map[[2]int]string) {
	plain := func(x, y int) bool {
		return c.in([2]int{x, y}) && tiles[y][x].Kind == game.TileFloor && tiles[y][x].Prop == game.PropNone
	}
	for _, d := range doors {
		shaft := false
		for _, o := range nb8 { // daylight breaks through beside the mouth
			if x, y := d[0]+o[0], d[1]+o[1]; !shaft && plain(x, y) {
				tiles[y][x] = lightShaft
				shaft = true
			}
		}
		seam := false
		for r := 2; r <= 3 && !seam; r++ { // a starter seam a few steps in
			for dy := -r; dy <= r && !seam; dy++ {
				for dx := -r; dx <= r; dx++ {
					if abs(dx) != r && abs(dy) != r {
						continue
					}
					x, y := d[0]+dx, d[1]+dy
					p := [2]int{x, y}
					if _, taken := nodes[p]; !taken && plain(x, y) {
						tiles[y][x] = stoneSeam.tile
						nodes[p] = stoneSeam.item
						seam = true
						break
					}
				}
			}
		}
	}
}

// scatterLife stocks the cave with its mineral and living features and returns
// the gatherable ones (position → item). Mineral seams stud the rock faces; cave
// mushrooms cluster on the floor of the deep dark away from the mouth; still
// glow-pools pool in the wider chambers. All three light the dark.
func (c *carver) scatterLife(rng *rand.Rand, tiles [][]game.Tile, region, doors [][2]int, pal cavePalette) map[[2]int]string {
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

	// Mineral seams on rock faces — you work the cavern walls. Depth into the
	// cave sets what they yield (see seamAt): the deep is worked harder too.
	var faces, richFaces [][2]int
	for _, p := range region {
		if !free(p) {
			continue
		}
		for _, d := range nb4 {
			if nx, ny := p[0]+d[0], p[1]+d[1]; nx >= 0 && ny >= 0 && nx < w && ny < h && tiles[ny][nx].Kind == game.TileWall {
				faces = append(faces, p)
				if c.tierAt(p) == 2 {
					richFaces = append(richFaces, p) // the deep — work it harder
				}
				break
			}
		}
	}
	place := func(p [2]int) {
		if _, taken := nodes[p]; taken || !free(p) {
			return
		}
		s := c.seamAt(p, rng)
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

	// Mushroom clusters in the deep dark — past the first depth tier, so the
	// glow (and the lantern top-ups it carries) begins where the light ends.
	// The mood sets how thickly the cave lives: moss caverns bloom, ice caves
	// barely glimmer.
	var deep [][2]int
	for _, p := range region {
		if free(p) && c.tierAt(p) >= 1 {
			deep = append(deep, p)
		}
	}
	rng.Shuffle(len(deep), func(i, j int) { deep[i], deep[j] = deep[j], deep[i] })
	for i := 0; i < len(deep)/pal.glowDiv+4 && i < len(deep); i++ {
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
	for i := 0; i < len(basins)/max(20, pal.glowDiv/3)+2 && i < len(basins); i++ {
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

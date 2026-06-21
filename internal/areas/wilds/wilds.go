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
	"time"

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
		return &area{ctx: ctx, gen: worldgen.New(worldSeed),
			discovered: map[[2]int]uint64{}, dirty: map[[2]int]bool{}}
	})
}

// Discovery: the overworld starts hidden and is revealed as the player walks.
// sightR is the brightly-lit circle around the player; discoverR (a touch
// wider) is the radius committed to memory, so explored ground stays visible —
// dimmed — once you move on.
//
// Memory is stored as a sparse grid of chunkN×chunkN cells, each chunk packed
// into a uint64 bitmask — so a fully-explored chunk costs 8 bytes while a
// frontier chunk still keeps exact per-tile bits. Chunks persist to the store,
// so the map (and the player's position) survive disconnects and re-entry.
const (
	sightR    = 7
	discoverR = 9
	chunkN    = 8 // cells per chunk side; 8×8 = 64 = one uint64 mask
)

type area struct {
	ctx        *game.Ctx
	gen        *worldgen.Generator
	wx, wy     int // absolute world position (top-left of the body's footprint)
	frame      int
	showMap    bool
	discovered map[[2]int]uint64 // chunk coord → 64-bit mask of revealed cells
	dirty      map[[2]int]bool   // chunks changed since the last persist
	collected  map[[2]int]bool   // world cells whose item this player has taken
	toast      string            // transient pickup feedback
	toastUntil time.Time         // when the toast expires (wall-clock; works in both renderers)
}

// toastDuration is how long a pickup message lingers.
const toastDuration = 3 * time.Second

// Toast implements game.Toaster: the current pickup message, if still fresh.
// Both renderers poll it — the glyph View and the HD overlay.
func (a *area) Toast() (string, bool) {
	return a.toast, a.toast != "" && time.Now().Before(a.toastUntil)
}

func (a *area) setToast(msg string) {
	a.toast, a.toastUntil = msg, time.Now().Add(toastDuration)
}

func (a *area) Name() string { return "The Wilds" }

func (a *area) Init(*world.Player) tea.Cmd {
	a.discovered = a.ctx.Store.LoadDiscovery(a.ctx.Name)
	if a.discovered == nil {
		a.discovered = map[[2]int]uint64{}
	}
	a.dirty = map[[2]int]bool{}
	a.collected = a.ctx.Store.LoadCollected(a.ctx.Name)
	if a.collected == nil {
		a.collected = map[[2]int]bool{}
	}
	if a.ctx.Inventory == nil {
		a.ctx.Inventory = map[string]int{}
	}
	if a.ctx.FixedGates == nil {
		a.ctx.FixedGates = a.ctx.Store.LoadPersonalGates(a.ctx.Name)
		if a.ctx.FixedGates == nil {
			a.ctx.FixedGates = map[string]bool{}
		}
	}
	a.wx, a.wy = a.resume()
	a.reveal()
	a.persist()
	a.ctx.World.EnterArea(a.ctx.Name, "wilds", a.wx, a.wy, "The Wilds")
	return nil
}

// resume returns the saved position if it's still an open, non-portal footprint
// (so you don't re-trigger the door you arrived through), else a fresh spawn by
// the HQ gate.
func (a *area) resume() (int, int) {
	if x, y, ok := a.ctx.Store.LoadPosition(a.ctx.Name, "wilds"); ok && a.fits(x, y) {
		if _, isPortal := a.portalUnder(x, y); !isPortal {
			return x, y
		}
		// The saved spot is a portal — e.g. a cave mouth we just stepped out of.
		// Surface on a walkable cell right beside it rather than back at the hub.
		for _, o := range [][2]int{{0, 1}, {1, 0}, {-1, 0}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}} {
			if nx, ny := x+o[0], y+o[1]; a.fits(nx, ny) {
				if _, p := a.portalUnder(nx, ny); !p {
					return nx, ny
				}
			}
		}
	}
	return a.spawn()
}

// chunkOf splits a world cell into its chunk coordinate and bit index.
func chunkOf(x, y int) (cx, cy int, bit uint) {
	return x >> 3, y >> 3, uint((y&(chunkN-1))*chunkN + (x & (chunkN - 1)))
}

// seen reports whether a world cell has been discovered.
func (a *area) seen(x, y int) bool {
	cx, cy, bit := chunkOf(x, y)
	return a.discovered[[2]int{cx, cy}]&(1<<bit) != 0
}

// markSeen records a cell as discovered, flagging its chunk dirty if changed.
func (a *area) markSeen(x, y int) {
	cx, cy, bit := chunkOf(x, y)
	key := [2]int{cx, cy}
	if nw := a.discovered[key] | (1 << bit); nw != a.discovered[key] {
		a.discovered[key] = nw
		a.dirty[key] = true
	}
}

// reveal uncovers every cell within discoverR of the player's body center.
// Centered on the 2×2 footprint so the circle sits under the avatar.
func (a *area) reveal() {
	cx, cy := a.wx+game.PlayerW/2, a.wy+game.PlayerH/2
	for dy := -discoverR; dy <= discoverR; dy++ {
		for dx := -discoverR; dx <= discoverR; dx++ {
			if dx*dx+dy*dy <= discoverR*discoverR {
				a.markSeen(cx+dx, cy+dy)
			}
		}
	}
}

// persist flushes newly-revealed chunks and the player's position to the store,
// so the map and where-you-stand survive disconnects and re-entry.
func (a *area) persist() {
	for ch := range a.dirty {
		a.ctx.Store.SaveDiscovery(a.ctx.Name, ch[0], ch[1], a.discovered[ch])
		delete(a.dirty, ch)
	}
	a.ctx.Store.SavePosition(a.ctx.Name, "wilds", a.wx, a.wy)
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
			cx, cy := x+dx, y+dy
			if c := a.gen.At(cx, cy); c.Portal != "" {
				return c.Portal, true
			}
			// An opened gate behaves like a portal; a sealed one does not.
			if g, ok := gateAtCell(cx, cy); ok && a.gateOpen(g) {
				return g.Portal, true
			}
		}
	}
	return "", false
}

// gateOpen reports whether a gate is repaired for this player (personal) or for
// everyone (co-op).
func (a *area) gateOpen(g gate) bool {
	if g.kind == gateCoop {
		return a.ctx.World.GateFixed(g.Portal)
	}
	return a.ctx.FixedGates[g.Portal]
}

// sealedGateUnderBody returns the sealed gate the player is standing on, if any.
func (a *area) sealedGateUnderBody() (gate, bool) {
	for dy := 0; dy < game.PlayerH; dy++ {
		for dx := 0; dx < game.PlayerW; dx++ {
			if g, ok := gateAtCell(a.wx+dx, a.wy+dy); ok && !a.gateOpen(g) {
				return g, true
			}
		}
	}
	return gate{}, false
}

// offerToGate spends the required item to repair a personal gate or contribute
// to a co-op gate's pool.
func (a *area) offerToGate(g gate) {
	if a.ctx.Inventory[g.item] <= 0 {
		a.setToast("the " + g.Name + " needs a " + itemName(g.item))
		return
	}
	a.ctx.Inventory[g.item]--
	if a.ctx.Inventory[g.item] <= 0 {
		delete(a.ctx.Inventory, g.item)
	}
	a.ctx.Store.SpendItem(a.ctx.Name, g.item)

	if g.kind == gateCoop {
		pool, fixed := a.ctx.World.OfferToGate(g.Portal, g.need)
		if fixed {
			a.setToast("the " + g.Name + " roars to life!")
		} else {
			a.setToast(fmt.Sprintf("offered a %s — %d/%d", itemName(g.item), pool, g.need))
		}
		return
	}
	a.fixPersonalGate(g)
}

// fixPersonalGate marks a personal gate repaired for this player.
func (a *area) fixPersonalGate(g gate) {
	a.ctx.FixedGates[g.Portal] = true
	a.ctx.Store.FixPersonalGate(a.ctx.Name, g.Portal)
	a.setToast("the " + g.Name + " opens for you!")
}

// gatePrompt is the status-bar hint shown while standing at a sealed gate. The
// co-op gate leads with the action key so the HD client badges it as a keycap;
// the personal gate is riddle-first (you answer in chat), so it stays plain.
func (a *area) gatePrompt(g gate) string {
	if g.kind == gateCoop {
		return fmt.Sprintf("e — offer a %s to open the %s  (%d/%d given)",
			itemName(g.item), g.Name, a.ctx.World.GatePool(g.Portal), g.need)
	}
	return fmt.Sprintf("%s riddle — %s  (answer in chat, or e: offer a %s)",
		g.Name, g.riddle, itemName(g.item))
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	switch msg := msg.(type) {
	case game.WorldEventMsg:
		ev := world.Event(msg)
		switch ev.Type {
		case world.EventTick:
			a.frame = int(ev.Frame)
		case world.EventChat:
			// Answer a personal gate's riddle by saying it aloud at the gate.
			if ev.Player == a.ctx.Name {
				if g, ok := a.sealedGateUnderBody(); ok && g.kind == gatePersonal &&
					g.answer != "" && normalizeAnswer(ev.Detail) == normalizeAnswer(g.answer) {
					a.fixPersonalGate(g)
				}
			}
		}
		return a, nil

	case tea.KeyMsg:
		if msg.String() == "m" {
			a.showMap = !a.showMap
			return a, nil
		}
		if msg.String() == "e" {
			if g, ok := a.sealedGateUnderBody(); ok {
				a.offerToGate(g)
			} else {
				a.pickUp()
			}
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
			if !game.CanStep(a.gen.Walkable, a.wx, a.wy, dx, dy) {
				break
			}
			a.wx, a.wy = nx, ny
			a.reveal()
			a.collectItem() // items are gathered just by walking over them
			if portal, ok := a.portalUnder(nx, ny); ok {
				a.ctx.World.Move(a.ctx.Name, nx, ny)
				// Persist the cell we stepped in from, not the portal itself, so
				// returning to the Wilds drops us beside the entrance (a cave mouth
				// out in the hills) rather than back at the distant HQ spawn.
				a.wx, a.wy = nx-dx, ny-dy
				a.persist()
				return game.Transition{To: portal}, nil
			}
		}
		if a.wx != sx || a.wy != sy {
			a.ctx.World.Move(a.ctx.Name, a.wx, a.wy)
			a.persist()
		}
	}
	return a, nil
}

func (a *area) Hint() string {
	if name, ok := a.portalUnder(a.wx, a.wy); ok {
		return "◈ step in to enter " + game.DisplayName(name)
	}
	if g, ok := a.sealedGateUnderBody(); ok {
		return a.gatePrompt(g)
	}
	if h, _, _, ok := a.hatUnderBody(); ok {
		return "e — wear the " + h.name
	}
	dx, dy := worldgen.GateX-a.wx, worldgen.GateY-a.wy
	return fmt.Sprintf("⌂ Durst HQ %s · y u b n diagonals · m map", bearing(dx, dy))
}

// Prompt implements game.Prompter: the single action available right where the
// player stands. The bearing-to-home fallback in Hint is ambient navigation,
// not an action, so here it returns ok=false to keep the HD bottom clear.
func (a *area) Prompt() (string, bool) {
	if name, ok := a.portalUnder(a.wx, a.wy); ok {
		return "step in to enter " + game.DisplayName(name), true
	}
	if g, ok := a.sealedGateUnderBody(); ok {
		return a.gatePrompt(g), true
	}
	if h, _, _, ok := a.hatUnderBody(); ok {
		return "e — wear the " + h.name, true
	}
	if it, _, _, ok := a.itemUnderBody(); ok {
		return "e — take " + it.Name, true
	}
	return "", false
}

// itemUnderBody returns the first uncollected item beneath the 2×2 footprint.
func (a *area) itemUnderBody() (game.Item, int, int, bool) {
	for dy := 0; dy < game.PlayerH; dy++ {
		for dx := 0; dx < game.PlayerW; dx++ {
			x, y := a.wx+dx, a.wy+dy
			if a.collected[[2]int{x, y}] {
				continue
			}
			if it, ok := itemAt(a.gen.At(x, y), x, y); ok {
				return it, x, y, true
			}
		}
	}
	return game.Item{}, 0, 0, false
}

// hatUnderBody returns the first uncollected hat beneath the 2×2 footprint.
func (a *area) hatUnderBody() (hatLoot, int, int, bool) {
	for dy := 0; dy < game.PlayerH; dy++ {
		for dx := 0; dx < game.PlayerW; dx++ {
			x, y := a.wx+dx, a.wy+dy
			if a.collected[[2]int{x, y}] {
				continue
			}
			if h, ok := hatAt(a.gen.At(x, y), x, y); ok {
				return h, x, y, true
			}
		}
	}
	return hatLoot{}, 0, 0, false
}

// pickUp harvests whatever lies under the player. Hats take precedence: they
// unlock the accessory and equip it; ordinary items go into the pack. Both mark
// the cell collected and persist.
func (a *area) pickUp() {
	if h, x, y, ok := a.hatUnderBody(); ok {
		a.collected[[2]int{x, y}] = true
		a.ctx.Store.MarkCollected(a.ctx.Name, x, y)
		a.unlockHat(h.idx)
		if self, ok := a.ctx.World.Self(a.ctx.Name); ok {
			a.ctx.World.SetAvatar(a.ctx.Name, self.Style, h.idx)
			a.ctx.Store.SaveAvatar(a.ctx.Name, string(self.Color), self.Style, h.idx)
		}
		a.setToast("+ " + h.name + " (now worn!)")
		return
	}
	a.collectItem()
}

// unlockHat marks accessory idx as owned and persists it, idempotently.
// Returns whether it was newly unlocked (so callers can announce it).
func (a *area) unlockHat(idx int) bool {
	if a.ctx.Hats == nil {
		a.ctx.Hats = map[int]bool{}
	}
	if a.ctx.Hats[idx] {
		return false
	}
	a.ctx.Hats[idx] = true
	a.ctx.Store.UnlockHat(a.ctx.Name, idx)
	return true
}

// collectItem harvests an ordinary item under the player into the pack, marks
// the cell collected and persists it. Hats are deliberately excluded — wearing
// one changes your avatar, so that stays a manual 'e' action — which is why this
// is split out from pickUp: movement calls it to gather loot just by walking
// over it. A find that has a matching wearable (a mushroom → the shroom
// accessory) also unlocks it, so some foraged loot doubles as an outfit.
// Returns whether anything was taken.
func (a *area) collectItem() bool {
	it, x, y, ok := a.itemUnderBody()
	if !ok {
		return false
	}
	a.collected[[2]int{x, y}] = true
	a.ctx.Store.MarkCollected(a.ctx.Name, x, y)
	a.ctx.Inventory[it.ID]++
	a.ctx.Store.AddItem(a.ctx.Name, it.ID)
	a.setToast("+ " + it.Name)
	if it.Wear != "" {
		if idx, ok := game.AccessoryIndex(it.Wear); ok && a.unlockHat(idx) {
			a.setToast("+ " + it.Name + " - now wearable! (c to equip)")
		}
	}
	return true
}

// sample builds a vw×vh window of the overworld centered on the player and
// returns it with its absolute top-left origin. Shared by the glyph View and
// the HD pixel renderer.
func (a *area) sample(vw, vh int) (*game.TileMap, int, int) {
	// Center the 2×2 body (not its top-left corner) in the window, so the avatar
	// sits dead center in both the glyph and HD views.
	ox, oy := a.wx-(vw-game.PlayerW)/2, a.wy-(vh-game.PlayerH)/2
	tiles := make([][]game.Tile, vh)
	for ly := 0; ly < vh; ly++ {
		row := make([]game.Tile, vw)
		for lx := 0; lx < vw; lx++ {
			wx, wy := ox+lx, oy+ly
			if a.seen(wx, wy) {
				cell := a.gen.At(wx, wy)
				t := CellTile(cell)
				if g, ok := gateAtCell(wx, wy); ok {
					// Both the open gate and the broken (sealed) arch carry the name of
					// where they lead, so the renderer can float a label above them.
					t.Label = game.DisplayName(g.Portal)
					if !a.gateOpen(g) { // sealed: a dull, broken arch (no swirl)
						t.Ch, t.Color = '⊘', "#8A8A98"
						t.Prop, t.PropHex = game.PropSealed, "#7A7A88"
						t.Tex, t.Ground = game.TexRock, "#33363E"
					}
				} else if !a.collected[[2]int{wx, wy}] {
					// Items/hats keep the biome ground under them; only the gem/hat on
					// top is colored. (Without an explicit Ground the HD renderer would
					// treat the loot color as the surface and dither a halo around it.)
					if h, ok := hatAt(cell, wx, wy); ok {
						t.Ch, t.Color = '♚', h.hex
						t.Prop, t.PropHex, t.Ground = game.PropHat, h.hex, groundColor(cell.Biome)
					} else if it, ok := itemAt(cell, wx, wy); ok {
						t.Ch, t.Color = it.Glyph, it.Hex
						switch it.ID {
						case "grain": // standing crop, over the field's furrows
							t.Prop, t.PropHex, t.Tex, t.Ground = game.PropCrop, it.Hex, game.TexField, "#86974A"
						case "stone": // cut stone on the quarry floor (keep the floor under it)
							t.Prop, t.PropHex = game.PropStone, it.Hex
						case "wood": // a log pile by the stump
							t.Prop, t.PropHex = game.PropLog, it.Hex
						case "fish": // a catch on the jetty planks
							t.Prop, t.PropHex = game.PropFish, it.Hex
						default:
							prop := game.PropGem
							if it.Glow { // crystals & mushrooms glow at night; other forage doesn't
								prop = game.PropGemGlow
							}
							t.Prop, t.PropHex, t.Ground = prop, it.Hex, groundColor(cell.Biome)
						}
					}
				}
				row[lx] = t
			} else {
				row[lx] = fogTile() // the unexplored world stays hidden
			}
		}
		tiles[ly] = row
	}
	return &game.TileMap{W: vw, H: vh, Tiles: tiles}, ox, oy
}

// fogColor is the near-black an unexplored cell shows in both renderers.
const fogColor = "#0B0E13"

// fogTile is a blank, dark cell for undiscovered ground — collision still reads
// the real generator (see fits), so fog only hides terrain, it never blocks.
func fogTile() game.Tile {
	return game.Tile{Kind: game.TileFloor, Ch: ' ', Walkable: true,
		Color: fogColor, Tex: game.TexFlat, Ground: fogColor}
}

// sightLight is the radial "torch" around the player: bright on the player,
// fading to the night floor at sightR so explored ground beyond it reads as dim
// memory. DayFadedLight eases that darkening out over the day/night cycle, so by
// midday the circle vanishes and only at night does it pool like a torch. (The
// wider discoverR reveal circle is unaffected — explored ground stays uncovered.)
func (a *area) sightLight() game.Light {
	return game.DayFadedLight(game.Light{X: a.wx + game.PlayerW/2, Y: a.wy + game.PlayerH/2, Radius: sightR})
}

// HDView implements game.HDViewer so the Wilds renders in HD pixel mode.
func (a *area) HDView(vw, vh int) (*game.TileMap, int, int) { return a.sample(vw, vh) }

// HDLight implements game.HDLighter so the HD renderer applies the same
// discovery circle as the glyph view.
func (a *area) HDLight() game.Light { return a.sightLight() }

func (a *area) View(width, height int) string {
	tm, ox, oy := a.sample(width, height)
	players := a.ctx.World.PlayersInArea("wilds")
	view := game.RenderWindow(a.ctx.Theme, tm, players, a.ctx.Name, a.frame, ox, oy, a.sightLight())

	if a.showMap {
		panel := a.minimap()
		pw := lipgloss.Width(panel)
		view = ui.Overlay(view, panel, (width-pw)/2, 1)
	} else if msg, show := a.Toast(); show {
		th := a.ctx.Theme
		if th == nil {
			th = ui.Default
		}
		line := th.Toast.Render(msg)
		view = ui.Overlay(view, line, (width-lipgloss.Width(line))/2, 1)
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
		if c.Portal == "cave" {
			// A cave mouth is a dark arch in the hillside, not a glowing gate.
			t.Prop, t.PropHex, t.Ground, t.Tex = game.PropCaveMouth, "#5C5560", "#6B5A44", game.TexRock
			return t
		}
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
	case 'W': // a village well (blocks)
		t.Prop, t.PropHex, t.Ground = game.PropWell, c.Color, groundColor(c.Biome)
	case 'i': // a city brazier — glows warm at night (blocks)
		t.Prop, t.PropHex, t.Ground = game.PropBrazier, c.Color, "#9A8E78"
	case 's': // a market stall on the square (blocks)
		t.Prop, t.PropHex, t.Ground = game.PropStall, c.Color, packedEarth
	case 'b': // a plank bridge over water (walkable) — Variant carries its run
		t.Prop = game.PropBridgeH
		if c.Variant == 1 {
			t.Prop = game.PropBridgeV
		}
		t.PropHex, t.Ground = c.Color, "#6B4A2B"
	case '=': // a palisade segment (blocks) — orientation carried in Variant
		switch c.Variant {
		case 1:
			t.Prop = game.PropFenceV
		case 2:
			t.Prop = game.PropFencePost
		default:
			t.Prop = game.PropFenceH
		}
		t.PropHex, t.Ground = c.Color, groundColor(c.Biome)
	case '#': // a town's stone curtain wall (blocks)
		t.Prop, t.PropHex, t.Ground, t.Tex = game.PropStoneWall, c.Color, "#6E7077", game.TexRock
	case 'I': // a town's stone wall tower (blocks, overhangs upward)
		t.Prop, t.PropHex, t.Ground, t.Tex = game.PropTower, c.Color, "#6E7077", game.TexRock
	case '"': // a cultivated field — crop rows (walkable)
		t.Tex, t.Ground = game.TexField, "#86974A"
	case '%': // a covered building footprint tile — drawn by its anchor (blocks)
		t.Prop, t.Ground = game.PropBldBody, packedEarth
	case 'h', 'H', 'L', 'B', 'C', 'K', 'T', 'M', 'S', 'V', 'r', 'n', 'd': // a settlement building anchor (blocks)
		t.Prop = buildingProp(c.Variant)
		t.PropHex, t.Ground = c.Color, packedEarth
		if t.Prop == game.PropHouse { // a lone wilderness cabin keeps its biome ground
			t.Ground = groundColor(c.Biome)
		}
	case 'Y': // an orchard/yard tree in a village — a real tree, but over grass
		t.Prop, t.PropHex, t.Ground = game.PropTree, c.Color, groundColor(c.Biome)
	case '♣': // tree on forest floor
		t.Prop, t.PropHex, t.Ground, t.Tex = game.PropTree, c.Color, groundColor(worldgen.Forest), game.TexForest
	case 'ϒ': // acacia on savanna
		t.Prop, t.PropHex, t.Ground, t.Tex = game.PropAcacia, c.Color, groundColor(worldgen.Savanna), game.TexSavanna
	case 'Ψ': // palm on the beach
		t.Prop, t.PropHex, t.Ground, t.Tex = game.PropPalm, c.Color, groundColor(worldgen.Sand), game.TexSand
	case '♠': // fir in the snow
		t.Prop, t.PropHex, t.Ground, t.Tex = game.PropFir, c.Color, groundColor(worldgen.Snow), game.TexSnow
	case '‖': // cattail reeds in the swamp
		t.Prop, t.PropHex, t.Ground, t.Tex = game.PropReed, c.Color, groundColor(worldgen.Swamp), game.TexSwamp
	case 'Δ': // rocky crag on the hills
		t.Prop, t.PropHex, t.Ground, t.Tex = game.PropCrag, c.Color, groundColor(worldgen.Hill), game.TexDirt
	case 'Λ': // a traveler's campfire
		t.Prop, t.PropHex, t.Ground = game.PropCampfire, c.Color, groundColor(c.Biome)
	case '▲': // boulder on hill earth (mountain peaks stay a plain rock tile)
		if c.Biome == worldgen.Hill {
			t.Prop, t.PropHex, t.Ground, t.Tex = game.PropBoulder, "#8A8170", groundColor(worldgen.Hill), game.TexDirt
		}
	}
	return t
}

// packedEarth is the trodden ground beneath village buildings.
const packedEarth = "#9B8A6A"

// buildingProp maps a settlement building's type (carried in Cell.Variant) to
// its sprite. Variant 0 is a lone wilderness cabin (the single-tile PropHouse).
func buildingProp(variant uint8) game.TileProp {
	switch variant {
	case 1:
		return game.PropBldCottage
	case 2:
		return game.PropBldHouse
	case 3:
		return game.PropBldLonghouse
	case 4:
		return game.PropBldBarn
	case 5:
		return game.PropBldChurch
	case 6:
		return game.PropBldKeep
	case 7:
		return game.PropBldCathedral
	case 8:
		return game.PropBldTownhouse
	case 9:
		return game.PropBldMarketHall
	case 10:
		return game.PropBldSmithy
	case 11:
		return game.PropBldTavern
	case 12:
		return game.PropBldRowhouse
	case 13:
		return game.PropBldNarrowhouse
	case 14:
		return game.PropBldDeephouse
	default:
		return game.PropHouse
	}
}

// texForBiome maps an overworld biome to an HD ground texture.
func texForBiome(b worldgen.Biome) game.TileTex {
	switch b {
	case worldgen.Grass:
		return game.TexGrass
	case worldgen.Savanna:
		return game.TexSavanna
	case worldgen.Swamp:
		return game.TexSwamp
	case worldgen.Snow:
		return game.TexSnow
	case worldgen.Sand:
		return game.TexSand
	case worldgen.Water, worldgen.Deep:
		return game.TexWater
	case worldgen.Forest:
		return game.TexForest
	case worldgen.Hill, worldgen.Path:
		return game.TexDirt
	case worldgen.Mountain:
		return game.TexRock
	default:
		return game.TexFlat
	}
}

// groundColor is the base surface color the HD renderer paints under a prop.
func groundColor(b worldgen.Biome) string {
	switch b {
	case worldgen.Grass:
		return "#5EAE63"
	case worldgen.Forest:
		return "#2E6B40"
	case worldgen.Hill:
		return "#9C8D67"
	case worldgen.Sand:
		return "#E6D6A0"
	case worldgen.Mountain:
		return "#9AA0A8"
	case worldgen.Path:
		return "#8C7A56"
	case worldgen.Snow:
		return "#E8EEF5"
	case worldgen.Savanna:
		return "#CDBA5C"
	case worldgen.Swamp:
		return "#45533C"
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
			if !a.seen(wx, wy) {
				b.WriteByte(' ') // unexplored — the map fills in as you roam
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

// Package wilds is the Wilds: Durst World's procedurally generated, infinite
// overworld and main hub. The player carries absolute world coordinates and a
// multi-tile body; every frame a window of terrain is sampled from worldgen
// around them and rendered through the shared tile renderer. Generation is a
// pure function of the seed, so every session shares one world. Landmark
// portals near the origin lead to the hand-built areas.
package wilds

import (
	"fmt"
	"math/rand"
	"sort"
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
	showBoard  bool              // the notice board panel is open
	discovered map[[2]int]uint64 // chunk coord → 64-bit mask of revealed cells
	dirty      map[[2]int]bool   // chunks changed since the last persist
	collected  map[[2]int]bool   // world cells whose item this player has taken
	rng        *rand.Rand        // per-session stream for hunting drop rolls
	toast      string            // transient pickup feedback
	toastUntil time.Time         // when the toast expires (wall-clock; works in both renderers)
	lastStrike time.Time         // when this session last landed a blow (weapon cooldown)
	hurtUntil  time.Time         // brief on-hit flash window after taking damage
	wieldSync  string            // last wielded weapon pushed to the world (avoids per-frame churn)

	// Hidden legends: the deterministic cell each unique weapon lies in, resolved
	// once in Init (docs/WEAPON_PLAN.md). Whether one still shows is the world's
	// shared artifact registry, checked live.
	artifactCell   map[string][2]int // weapon id → its hidden cell
	artifactAtCell map[[2]int]string // hidden cell → weapon id

	// build mode (the shared placements layer)
	building bool // placing a structure: movement drives the ghost, not the body
	buildSel int  // selected placeable in game.Placeables
	bx, by   int  // ghost cursor, absolute world coords

	inClaim string // plot id of the claim the body stands in ("" if none)
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
	if a.ctx.Compendium == nil {
		a.ctx.Compendium = a.ctx.Store.LoadCompendium(a.ctx.Name)
		if a.ctx.Compendium == nil {
			a.ctx.Compendium = map[string]bool{}
		}
	}
	a.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	if a.ctx.Inventory == nil {
		a.ctx.Inventory = map[string]int{}
	}
	if a.ctx.FixedGates == nil {
		a.ctx.FixedGates = a.ctx.Store.LoadPersonalGates(a.ctx.Name)
		if a.ctx.FixedGates == nil {
			a.ctx.FixedGates = map[string]bool{}
		}
	}
	a.computeArtifactCells()
	a.wx, a.wy = a.resume()
	a.reveal()
	a.persist()
	if c, _, ok := game.WorkspaceAt(a.ctx, a.wx, a.wy); ok {
		a.inClaim = c.PlotID // show the workspace label on arrival, without a toast
	}
	a.ctx.World.EnterArea(a.ctx.Name, "wilds", a.wx, a.wy, "The Wilds")
	return nil
}

// resume decides where to surface in the Wilds. Returning through a door (a
// hall, the lobby, the arcade) drops you beside that landmark, so stepping out
// of an area lands you back in the open world right where you went in — never
// teleported to the distant HQ. Otherwise it restores your last saved footprint
// (e.g. surfacing beside a cave mouth far from the hub), falling back to a fresh
// spawn by the HQ gate.
func (a *area) resume() (int, int) {
	if x, y, ok := a.landmarkReturn(); ok {
		return x, y
	}
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

// landmarkReturn, when the player has just stepped out of an area reached by a
// landmark door (a hall, the lobby, the arcade), returns a walkable cell beside
// that door. ok is false on a fresh connect or when arriving from somewhere
// without a fixed door (a cave mouth, a gate) — those restore the saved spot.
func (a *area) landmarkReturn() (int, int, bool) {
	if a.ctx.From == "" || a.ctx.From == "wilds" {
		return 0, 0, false
	}
	for _, lm := range worldgen.Landmarks {
		if lm.Portal != a.ctx.From {
			continue
		}
		// Surface on an open, non-portal cell next to the door so we don't bounce
		// straight back through it.
		for _, o := range [][2]int{{0, 1}, {1, 1}, {-1, 1}, {1, 0}, {-1, 0}, {0, 2}, {2, 0}, {-2, 0}, {0, -1}} {
			nx, ny := lm.X+o[0], lm.Y+o[1]
			if a.fits(nx, ny) {
				if _, isPortal := a.portalUnder(nx, ny); !isPortal {
					return nx, ny, true
				}
			}
		}
	}
	return 0, 0, false
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

// walkableAt is the movement collision test: a blocking placement (a fence, a
// machine) stops you, otherwise the terrain decides. This is what makes built
// structures solid, since the generator alone knows nothing about placements.
func (a *area) walkableAt(x, y int) bool {
	if pl, ok := a.ctx.World.PlacementAt(x, y); ok {
		if pb, ok := game.PlaceableByID(pl.Kind); ok && !pb.Walkable {
			return false
		}
	}
	return a.gen.Walkable(x, y) || game.IsCleared(a.ctx, x, y)
}

// clearedTile is how a felled/quarried cell reads: walkable ground in place of
// the tree or boulder — a grassy clearing over forest, bare dirt over rock.
func clearedTile(cell worldgen.Cell) game.Tile {
	switch cell.Biome {
	case worldgen.Hill, worldgen.Mountain:
		return game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true,
			Color: "#9C8D67", Tex: game.TexDirt, Ground: "#9C8D67"}
	default: // a felled forest reads as a grassy clearing
		return game.Tile{Kind: game.TileFloor, Ch: ',', Walkable: true,
			Color: "#5EAE63", Tex: game.TexGrass, Ground: "#5EAE63"}
	}
}

// canBuildAt reports whether the ghost cell is a legal spot: discovered, buildable
// ground, not already occupied, and not on a gate or the player's own footprint.
func (a *area) canBuildAt(x, y int) bool {
	if _, ok := a.ctx.World.PlacementAt(x, y); ok {
		return false
	}
	if !a.seen(x, y) || (!a.gen.Walkable(x, y) && !game.IsCleared(a.ctx, x, y)) {
		return false // undiscovered, or blocking terrain that hasn't been cleared
	}
	if _, ok := gateAtCell(x, y); ok {
		return false
	}
	if x >= a.wx && x < a.wx+game.PlayerW && y >= a.wy && y < a.wy+game.PlayerH {
		return false // can't build under yourself
	}
	if ok, _ := game.BuildRight(a.ctx, x, y); !ok {
		return false // inside someone else's claim or wilds buffer
	}
	return true
}

// placeStructure tries to build the selected placeable at the ghost. It reserves
// the cell in the shared world first (atomic occupancy check) and only spends
// materials once the spot is secured, so a lost race never costs you anything.
func (a *area) placeStructure() {
	if a.buildSel < 0 || a.buildSel >= len(game.Placeables) {
		return
	}
	p := game.Placeables[a.buildSel]
	if !a.canBuildAt(a.bx, a.by) {
		if ok, owner := game.BuildRight(a.ctx, a.bx, a.by); !ok && owner != "" {
			a.setToast(owner + "'s Workspace — protected")
		} else {
			a.setToast("can't build there")
		}
		return
	}
	if !game.CanAfford(p, a.ctx.Inventory) {
		a.setToast("need " + game.PlaceableCost(p))
		return
	}
	if !a.ctx.World.Place("wilds", world.Placement{X: a.bx, Y: a.by, Kind: p.ID, Owner: a.ctx.Name}) {
		a.setToast("something's already there")
		return
	}
	game.SpendFor(a.ctx, p)
	a.setToast("built " + p.Name)
}

// tendCleared refreshes the regrowth clock on the player's own cleared cells
// under and around the body, so a clearing you live in never grows back; cells
// at the unused edges lapse first, and the woods creep in from there.
func (a *area) tendCleared() {
	for y := a.wy - 1; y <= a.wy+game.PlayerH; y++ {
		for x := a.wx - 1; x <= a.wx+game.PlayerW; x++ {
			game.TouchCleared(a.ctx, x, y)
		}
	}
}

// updateClaimPresence refreshes the lease while the player stands on their own
// land, and toasts when they cross into a Workspace — so both clients announce
// the claim (the glyph status line also shows it persistently, via Hint).
func (a *area) updateClaimPresence() {
	c, mine, ok := game.WorkspaceAt(a.ctx, a.wx, a.wy)
	cur := ""
	if ok {
		cur = c.PlotID
		if mine {
			game.TouchWorkspace(a.ctx, c.PlotID)
		}
	}
	if cur == a.inClaim {
		return
	}
	a.inClaim = cur
	if ok {
		a.setToast("entering " + a.workspaceLabel(c, mine))
	}
}

// workspaceLabel renders a claim as "your Workspace, Brixen" / "Anna's
// Workspace, Brixen" (the settlement name dropped if unknown).
func (a *area) workspaceLabel(c world.Claim, mine bool) string {
	who := c.Owner + "'s"
	if mine {
		who = "your"
	}
	label := who + " Workspace"
	if name, ok := a.gen.SettlementNameAt(a.wx, a.wy); ok {
		label += ", " + name
	}
	return label
}

// Parcel tint hues: a soft green over your own claimed ground, amber over
// another player's — the same green/red-ghost language the build cursor uses.
const (
	tintMine  = "#7BD88F"
	tintOther = "#E0B44D"
)

// claimTint returns the tint hue for (x,y) if it lies in a claimed parcel.
func claimTint(claims []world.Claim, me string, x, y int) (string, bool) {
	for _, c := range claims {
		if c.Covers(x, y) {
			if c.Owner == me {
				return tintMine, true
			}
			return tintOther, true
		}
	}
	return "", false
}

// BuildPanel implements game.BuildViewer: the HD client draws the build palette
// from it while build mode is active.
func (a *area) BuildPanel() (int, string, bool, bool) {
	footer, warn := a.buildFooter()
	return a.buildSel, footer, warn, a.building
}

// selectedTool returns the selected placeable if it's a clearing tool.
func (a *area) selectedTool() (game.Placeable, bool) {
	if a.buildSel < 0 || a.buildSel >= len(game.Placeables) {
		return game.Placeable{}, false
	}
	p := game.Placeables[a.buildSel]
	return p, game.IsTool(p)
}

// canClearAt reports whether tool pb can clear the ghost cell: the right blocking
// terrain (a tree for the axe, a hill boulder for the pick — never a mountain
// peak), discovered, not already cleared, and where build-rights allow.
func (a *area) canClearAt(x, y int, pb game.Placeable) bool {
	if !a.seen(x, y) || game.IsCleared(a.ctx, x, y) {
		return false
	}
	if ok, _ := game.BuildRight(a.ctx, x, y); !ok {
		return false
	}
	c := a.gen.At(x, y)
	if c.Walkable { // nothing blocking to clear
		return false
	}
	switch pb.Clear {
	case game.ClearTree:
		return c.Biome == worldgen.Forest
	case game.ClearRock:
		return c.Biome == worldgen.Hill // peaks (Mountain) stay permanent
	}
	return false
}

// clearVerb is the action word for a tool ("fell" / "break").
func clearVerb(k game.ClearKind) string {
	switch k {
	case game.ClearTree:
		return "fell"
	case game.ClearRock:
		return "break"
	default:
		return "clear"
	}
}

// clearUnderGhost fells/breaks the cell under the ghost with tool pb, writing the
// cleared overlay and paying the yield into the pack. The tool isn't consumed.
func (a *area) clearUnderGhost(pb game.Placeable) {
	if !a.canClearAt(a.bx, a.by, pb) {
		a.setToast("nothing to " + clearVerb(pb.Clear) + " here")
		return
	}
	if !game.ClearGround(a.ctx, a.bx, a.by) {
		a.setToast("can't clear there")
		return
	}
	item, n := pb.Clear.Yield()
	game.AddToPack(a.ctx, item, n)
	name := item
	if it, ok := game.ItemByID(item); ok {
		name = it.Name
	}
	a.setToast(fmt.Sprintf("%sed it — +%d %s", clearVerb(pb.Clear), n, name))
}

// buildFooter is the palette's context line under the ghost: for a tool, the
// clear hint or why it can't; otherwise a claim hint, a block reason, or empty
// (the panel then shows the key legend). warn marks a problem (amber).
func (a *area) buildFooter() (string, bool) {
	if pb, ok := a.selectedTool(); ok {
		item, n := pb.Clear.Yield()
		name := item
		if it, ok := game.ItemByID(item); ok {
			name = it.Name
		}
		if a.canClearAt(a.bx, a.by, pb) {
			return fmt.Sprintf("e — %s here (+%d %s)", clearVerb(pb.Clear), n, name), false
		}
		return "nothing to " + clearVerb(pb.Clear) + " here", true
	}
	if s, ok := a.ghostClaimPrompt(); ok {
		return s, false
	}
	if r := a.blockReasonText(); r != "" {
		return "can't build: " + r, true
	}
	return "", false
}

// blockReasonText explains why the ghost cell can't be built on ("" when it's a
// legal spot). Claims are surfaced separately by buildFooter as a hint.
func (a *area) blockReasonText() string {
	x, y := a.bx, a.by
	if a.canBuildAt(x, y) {
		return ""
	}
	if x >= a.wx && x < a.wx+game.PlayerW && y >= a.wy && y < a.wy+game.PlayerH {
		return "you're standing there"
	}
	if _, ok := a.ctx.World.PlacementAt(x, y); ok {
		return "already occupied"
	}
	if !a.seen(x, y) {
		return "not explored yet"
	}
	if ok, owner := game.BuildRight(a.ctx, x, y); !ok && owner != "" {
		return owner + "'s land"
	}
	if _, ok := gateAtCell(x, y); ok {
		return "a gate stands here"
	}
	switch a.gen.At(x, y).Biome {
	case worldgen.Forest:
		return "trees in the way"
	case worldgen.Hill, worldgen.Mountain, worldgen.Snow:
		return "rock in the way"
	case worldgen.Water, worldgen.Deep:
		return "water"
	}
	return "can't build here"
}

// ClaimLabel implements game.ClaimLabeler: the Workspace the body stands in, for
// the HD banner (the glyph client shows the same label via Hint).
func (a *area) ClaimLabel() (string, bool) {
	if c, mine, ok := game.WorkspaceAt(a.ctx, a.wx, a.wy); ok {
		return a.workspaceLabel(c, mine), true
	}
	return "", false
}

// ghostClaimPrompt is the claim-related action under the build cursor, if it
// hovers a settlement plot: release your own, note another's, or claim a free one.
func (a *area) ghostClaimPrompt() (string, bool) {
	p, ok := a.gen.PlotAt(a.bx, a.by)
	if !ok {
		return "", false
	}
	if c, mine, ok := game.WorkspaceAt(a.ctx, a.bx, a.by); ok && c.PlotID == p.ID {
		if mine {
			return "x — release your Workspace (" + p.Settlement + ")", true
		}
		return c.Owner + "'s Workspace — protected", true
	}
	return "e — claim this " + p.Kind + " in " + p.Settlement, true
}

// claimUnderGhost deeds the settlement plot beneath the build cursor to the
// player (the Workspace Charter), or reports who already holds it — the
// land-tenure counterpart to placing a structure (docs/CLAIMS_PLAN.md).
func (a *area) claimUnderGhost() {
	p, ok := a.gen.PlotAt(a.bx, a.by)
	if !ok {
		a.setToast("no plot here — claim a building in a settlement")
		return
	}
	if c, mine, held := game.WorkspaceAt(a.ctx, a.bx, a.by); held && c.PlotID == p.ID {
		if mine {
			a.setToast("you already hold this Workspace (x to release)")
		} else {
			a.setToast(c.Owner + " already holds this plot")
		}
		return
	}
	if game.ClaimWorkspace(a.ctx, p.ID, p.AX, p.AY, p.W, p.H) {
		a.setToast("claimed a " + p.Kind + " in " + p.Settlement)
	} else {
		a.setToast("can't claim that")
	}
}

// releaseUnderGhost gives up the player's claim on the plot under the build
// cursor, if they hold it. Returns whether a release happened (with its toast).
func (a *area) releaseUnderGhost() bool {
	p, ok := a.gen.PlotAt(a.bx, a.by)
	if !ok {
		return false
	}
	if c, mine, held := game.WorkspaceAt(a.ctx, a.bx, a.by); held && mine && c.PlotID == p.ID {
		if game.ReleaseWorkspace(a.ctx, p.ID) {
			a.setToast("released your Workspace in " + p.Settlement)
			return true
		}
	}
	return false
}

func iabs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// stationAdjacent finds an interactable placement (a machine or a trade stall)
// on the ring of cells bordering the 2×2 body, so you can "use" something you're
// standing next to (they're solid, so you never stand on one).
func (a *area) stationAdjacent() (int, int, bool) {
	for y := a.wy - 1; y <= a.wy+game.PlayerH; y++ {
		for x := a.wx - 1; x <= a.wx+game.PlayerW; x++ {
			if x >= a.wx && x < a.wx+game.PlayerW && y >= a.wy && y < a.wy+game.PlayerH {
				continue // inside the body, not a border cell
			}
			if pl, ok := a.ctx.World.PlacementAt(x, y); ok &&
				(game.IsMachine(pl.Kind) || game.IsStall(pl.Kind) || game.IsWorkbench(pl.Kind)) {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// creatureAdjacent finds a live animal on the ring of cells bordering the
// player's footprint, so you can observe something you stand beside (animals
// flee, so you rarely share their cell). Returns the nearest such creature.
func (a *area) creatureAdjacent() (world.Creature, bool) {
	cs := a.ctx.World.CreaturesInArea("wilds")
	if len(cs) == 0 {
		return world.Creature{}, false
	}
	for _, c := range cs {
		if iabs(c.X-a.wx) <= 1 && iabs(c.Y-a.wy) <= 1 {
			return c, true
		}
	}
	return world.Creature{}, false
}

// creaturePrompt is the contextual action line for an adjacent animal: your own
// companion just reads as such, otherwise observe/hunt, plus tame when the
// species is tameable and you're carrying its bait.
func (a *area) creaturePrompt(c world.Creature) string {
	name := c.Kind
	sp, ok := game.SpeciesByKind(c.Kind)
	if ok && sp.Name != "" {
		name = sp.Name
	}
	if c.Owner == a.ctx.Name {
		return "your " + name + " — a loyal companion"
	}
	line := "e observe · f hunt the " + name
	if ok && sp.Tameable && c.Owner == "" && a.ctx.Inventory[sp.Bait] > 0 {
		line += " · t tame (" + itemName(sp.Bait) + ")"
	}
	return line
}

// observe logs a sighting: a first sighting adds the species to the player's
// field notes (the compendium); a repeat just acknowledges it. Persisted so the
// compendium survives between visits.
func (a *area) observe(c world.Creature) {
	name := c.Kind
	if sp, ok := game.SpeciesByKind(c.Kind); ok && sp.Name != "" {
		name = sp.Name
	}
	if a.ctx.Compendium[c.Kind] {
		a.setToast("a " + name + " eyes you, then looks away")
		return
	}
	a.ctx.Compendium[c.Kind] = true
	a.ctx.Store.MarkSpecies(a.ctx.Name, c.Kind)
	a.setToast("you spot a " + name + " — added to your field notes")
}

// noteSpecies records a species in the compendium (and persists it) the first
// time the player encounters it — shared by observing and hunting, since you
// learn an animal by catching it too.
func (a *area) noteSpecies(kind string) {
	if a.ctx.Compendium[kind] {
		return
	}
	a.ctx.Compendium[kind] = true
	a.ctx.Store.MarkSpecies(a.ctx.Name, kind)
}

// downedDuration is how long a knocked-out player stays down before reviving at
// the hub (docs/WEAPON_PLAN.md). The world's tick loop does the actual respawn;
// this is the game-layer policy it's told.
const downedDuration = 5 * time.Second

// hurtFlashDuration is how long the on-hit cue lingers after you take a blow.
const hurtFlashDuration = 350 * time.Millisecond

// hitByMessage phrases an incoming blow from the victim's side: who hit you and
// with what ("" detail means bare hands).
func hitByMessage(ev world.Event) string {
	if ev.Detail != "" {
		return ev.Player + "'s " + ev.Detail + " catches you"
	}
	return ev.Player + " strikes you"
}

// Hurt reports whether the on-hit flash is still active (read by the renderers
// to tint the frame red for an instant). Part of game.Hurtable.
func (a *area) Hurt() bool {
	return time.Now().Before(a.hurtUntil)
}

// strikePrompt offers the strike action when another player is in range and the
// spot allows fighting — so PvP is discoverable, not a hidden key.
func (a *area) strikePrompt() (string, bool) {
	if a.ctx.World.Downed(a.ctx.Name) {
		return "", false
	}
	p, ok := a.playerTarget(game.WieldedWeapon(a.ctx))
	if !ok || !a.pvpAllowed(p.X, p.Y) || !a.pvpAllowed(a.wx, a.wy) {
		return "", false
	}
	return "f — strike " + p.Name, true
}

// strike is the single combat action behind the `f` key: it lands the best
// weapon in the pack on whatever it can reach — a wild animal first, then a
// player out in the open Wilds. Reach 1 is the melee ring; a ranged weapon scans
// the tiles ahead. Ammo is spent only on a connecting shot.
// tickInterval mirrors the world's 2 Hz clock, so a weapon's Cooldown (in ticks)
// converts to wall-clock for the per-session strike throttle.
const tickInterval = 500 * time.Millisecond

func (a *area) strike() {
	if a.ctx.World.Downed(a.ctx.Name) {
		return // can't act while knocked out
	}
	wp := game.WieldedWeapon(a.ctx)
	if cd := time.Duration(wp.Cooldown) * tickInterval; cd > 0 && time.Since(a.lastStrike) < cd {
		return // still recovering from the last blow
	}

	cs, ps, blocked := a.gatherTargets(wp)
	if len(cs) == 0 && len(ps) == 0 {
		switch {
		case blocked:
			a.setToast("this is a peaceful place — no fighting here")
		case wp.Reach > 1:
			a.setToast("nothing within range")
		default:
			a.setToast("nothing within reach")
		}
		return
	}

	// Multi-target abilities (cleave/pierce) catch several foes; a plain strike
	// hits one. Multi-hits get a tidy summary instead of clobbering toasts.
	multi := wp.Pierce || wp.Cleave
	hits := 0
	for _, c := range cs {
		if a.applyCreatureHit(c, wp, multi) {
			hits++
		}
	}
	for _, p := range ps {
		if a.applyPlayerHit(p, wp, multi) {
			hits++
		}
	}
	if multi && hits > 1 {
		verb := "sweeps"
		if wp.Pierce {
			verb = "skewers"
		}
		a.setToast(fmt.Sprintf("your %s %s %d foes!", wp.Name, verb, hits))
	}
	a.spendAmmo(wp)
	a.lastStrike = time.Now()
}

// gatherTargets collects what a strike lands on, honoring the weapon's reach and
// abilities: a pierce shot rakes every foe along its line, a cleave catches every
// foe around you, and a plain blow takes the nearest single target (a creature
// before a player). Players are included only where PvP is allowed; blocked is
// true if the only thing in range was a player you can't fight here.
func (a *area) gatherTargets(wp game.Weapon) (cs []world.Creature, ps []world.Player, blocked bool) {
	creatures := a.ctx.World.CreaturesInArea("wilds")
	others := a.ctx.World.PlayersInArea("wilds")
	canHere := a.pvpAllowed(a.wx, a.wy)
	seenC := map[string]bool{}
	seenP := map[string]bool{}
	addP := func(p world.Player) {
		if p.Name == a.ctx.Name || seenP[p.Name] {
			return
		}
		if canHere && a.pvpAllowed(p.X, p.Y) {
			seenP[p.Name] = true
			ps = append(ps, p)
		} else {
			blocked = true
		}
	}
	bodyTouch := func(cx, cy int, p world.Player) bool {
		return cx >= p.X-1 && cx <= p.X+game.PlayerW && cy >= p.Y-1 && cy <= p.Y+game.PlayerH
	}

	if wp.Reach > 1 { // ranged: scan the facing line, near to far
		for _, t := range a.facingLine(wp.Reach) {
			cellHit := false
			for _, c := range creatures {
				if !seenC[c.ID] && iabs(c.X-t[0]) <= 1 && iabs(c.Y-t[1]) <= 1 {
					seenC[c.ID] = true
					cs = append(cs, c)
					cellHit = true
				}
			}
			for _, p := range others {
				if bodyTouch(t[0], t[1], p) {
					addP(p)
					cellHit = true
				}
			}
			if cellHit && !wp.Pierce {
				break // a normal shot stops at the first foe; a piercing one flies on
			}
		}
	} else { // melee: the adjacent ring of the footprint
		for _, c := range creatures {
			if iabs(c.X-a.wx) <= 1 && iabs(c.Y-a.wy) <= 1 {
				seenC[c.ID] = true
				cs = append(cs, c)
			}
		}
		for _, p := range others {
			if bodyTouch(a.wx, a.wy, p) || bodyTouch(a.wx+1, a.wy+1, p) {
				addP(p)
			}
		}
	}

	// A plain weapon takes a single target: the nearest creature, else the
	// nearest player. Cleave/pierce keep the whole set.
	if !wp.Pierce && !wp.Cleave {
		if len(cs) > 0 {
			cs, ps = cs[:1], nil
		} else if len(ps) > 0 {
			ps = ps[:1]
		}
	}
	return cs, ps, blocked
}

// pvpAllowed reports whether a player standing at (x,y) may be struck: only out
// in the open Wilds — never in the hub's peace ward, and never on a claimed
// homestead, which stays a sanctuary even in the wild. Delegates to the shared
// game.PvPAllowedAt so the strike action and the /pvp command never disagree.
func (a *area) pvpAllowed(x, y int) bool {
	return game.PvPAllowedAt(a.ctx, "wilds", x, y)
}

// spendAmmo consumes one round for a ranged weapon after a connecting strike.
func (a *area) spendAmmo(wp game.Weapon) {
	if wp.Ammo == "" {
		return
	}
	a.ctx.Inventory[wp.Ammo]--
	if a.ctx.Inventory[wp.Ammo] <= 0 {
		delete(a.ctx.Inventory, wp.Ammo)
	}
	a.ctx.Store.SpendItem(a.ctx.Name, wp.Ammo)
}

// creatureTarget finds the animal a strike would hit: the adjacent ring for a
// melee weapon, or the nearest creature along the facing line for a ranged one.
func (a *area) creatureTarget(wp game.Weapon) (world.Creature, bool) {
	if wp.Reach <= 1 {
		return a.creatureAdjacent()
	}
	for _, t := range a.facingLine(wp.Reach) {
		for _, c := range a.ctx.World.CreaturesInArea("wilds") {
			if iabs(c.X-t[0]) <= 1 && iabs(c.Y-t[1]) <= 1 {
				return c, true
			}
		}
	}
	return world.Creature{}, false
}

// playerTarget finds another player a strike would hit, mirroring creatureTarget
// (adjacent ring for melee, facing line for ranged). Never targets yourself.
func (a *area) playerTarget(wp game.Weapon) (world.Player, bool) {
	others := a.ctx.World.PlayersInArea("wilds")
	hit := func(px, py, tx, ty int) bool {
		// a player's 2×2 body is hit if its footprint touches the target cell
		return tx >= px-1 && tx <= px+game.PlayerW && ty >= py-1 && ty <= py+game.PlayerH
	}
	if wp.Reach <= 1 {
		for _, p := range others {
			if p.Name == a.ctx.Name {
				continue
			}
			if hit(a.wx, a.wy, p.X, p.Y) {
				return p, true
			}
		}
		return world.Player{}, false
	}
	for _, t := range a.facingLine(wp.Reach) {
		for _, p := range others {
			if p.Name == a.ctx.Name {
				continue
			}
			if hit(t[0], t[1], p.X, p.Y) {
				return p, true
			}
		}
	}
	return world.Player{}, false
}

// facingLine returns the cells straight ahead of the player, 1..reach tiles out
// in the current facing — the path a ranged shot travels. Facing is read from
// the world (set as the player moves).
func (a *area) facingLine(reach int) [][2]int {
	dir := world.DirS
	if self, ok := a.ctx.World.Self(a.ctx.Name); ok {
		dir = self.Facing
	}
	dx, dy := facingDelta(dir)
	out := make([][2]int, 0, reach)
	for i := 1; i <= reach; i++ {
		out = append(out, [2]int{a.wx + dx*i, a.wy + dy*i})
	}
	return out
}

// facingDelta maps an 8-way facing to a unit step. South is +Y (the world's down
// is positive), matching world.Facing8.
func facingDelta(d world.Dir) (int, int) {
	switch d {
	case world.DirN:
		return 0, -1
	case world.DirNE:
		return 1, -1
	case world.DirE:
		return 1, 0
	case world.DirSE:
		return 1, 1
	case world.DirS:
		return 0, 1
	case world.DirSW:
		return -1, 1
	case world.DirW:
		return -1, 0
	case world.DirNW:
		return -1, -1
	}
	return 0, 1
}

// weaponDamage is the blow a weapon lands on a target at (tx,ty) facing tdir,
// including the backstab bonus when the attacker strikes from behind.
func (a *area) weaponDamage(wp game.Weapon, tx, ty int, tdir world.Dir) int {
	dmg := wp.Damage
	if dmg < 1 {
		dmg = 1
	}
	if wp.Backstab && a.isBehind(tx, ty, tdir) {
		dmg += game.BackstabBonus
	}
	return dmg
}

// isBehind reports whether the attacker stands behind a target facing tdir — the
// dot of the target's facing with the target→attacker vector is negative.
func (a *area) isBehind(tx, ty int, tdir world.Dir) bool {
	fx, fy := facingDelta(tdir)
	return fx*(a.wx-tx)+fy*(a.wy-ty) < 0
}

// pushDir is the unit knockback away from the attacker toward (tx,ty).
func (a *area) pushDir(tx, ty int) (int, int) {
	dx, dy := isign(tx-a.wx), isign(ty-a.wy)
	if dx == 0 && dy == 0 {
		dy = 1
	}
	return dx, dy
}

func isign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	default:
		return 0
	}
}

// applyPlayerHit lands a blow on another player via the world's atomic Strike: a
// non-lethal hit just hurts, the blow that empties their HP knocks them out.
// Knockback shoves them a tile back. quiet suppresses the per-target toast (for
// multi-hit sweeps, which print a summary). Returns whether it connected.
func (a *area) applyPlayerHit(p world.Player, wp game.Weapon, quiet bool) bool {
	dmg := a.weaponDamage(wp, p.X, p.Y, p.Facing)
	_, downed, ok := a.ctx.World.Strike(a.ctx.Name, p.Name, wp.Name, dmg, downedDuration)
	if !ok {
		if !quiet {
			if a.ctx.World.Immune(p.Name) {
				a.setToast(p.Name + " is still catching their breath")
			} else {
				a.setToast(p.Name + " is already down")
			}
		}
		return false
	}
	if wp.Knockback && !downed {
		dx, dy := a.pushDir(p.X, p.Y)
		nx, ny := p.X+dx, p.Y+dy
		if a.fits(nx, ny) {
			a.ctx.World.Shove(a.ctx.Name, p.Name, nx, ny)
		}
	}
	if !quiet {
		with := ""
		if wp.Item != "" {
			with = " with the " + wp.Name
		}
		if downed {
			a.setToast("you knock " + p.Name + " out" + with + "!")
		} else {
			a.setToast("you strike " + p.Name + with)
		}
	}
	return true
}

// applyCreatureHit lands a blow on a wild animal. A strike decrements its HP
// atomically (so two hunters can't both claim one kill); a non-killing hit just
// spooks it (and a knockback weapon shoves it), and the blow that drops it
// despawns the animal and rolls its spoils into the pack. quiet suppresses the
// per-target toast for sweeps. Returns whether it connected.
func (a *area) applyCreatureHit(c world.Creature, wp game.Weapon, quiet bool) bool {
	sp, ok := game.SpeciesByKind(c.Kind)
	if !ok {
		return false
	}
	if c.Owner == a.ctx.Name {
		if !quiet {
			a.setToast("you won't hunt your own companion")
		}
		return false
	}
	a.noteSpecies(c.Kind)

	dmg := a.weaponDamage(wp, c.X, c.Y, c.Facing)
	killed := false
	changed := a.ctx.World.MutateCreature(c.ID, func(cc *world.Creature) bool {
		if cc.HP <= 0 {
			return false // already down — someone else's catch
		}
		cc.HP -= dmg
		cc.State = "flee"
		if cc.HP <= 0 {
			killed = true
		}
		return true
	})
	if !changed {
		if !quiet {
			a.setToast("the " + sp.Name + " is already gone")
		}
		return false
	}
	if !killed {
		if wp.Knockback {
			a.shoveCreature(c, sp)
		}
		if !quiet {
			a.setToast("you strike the " + sp.Name + " — it bolts")
		}
		return true
	}

	a.ctx.World.DespawnCreature(c.ID)
	drops := game.RollDrops(sp, a.rng)
	for id, n := range drops {
		a.ctx.Inventory[id] += n
		for i := 0; i < n; i++ {
			a.ctx.Store.AddItem(a.ctx.Name, id)
		}
	}
	if !quiet {
		if len(drops) == 0 {
			a.setToast("you catch the " + sp.Name)
			return true
		}
		parts := make([]string, 0, len(drops))
		for id, n := range drops {
			parts = append(parts, fmt.Sprintf("+%d %s", n, itemName(id)))
		}
		sort.Strings(parts)
		a.setToast("you catch the " + sp.Name + " — " + strings.Join(parts, ", "))
	}
	return true
}

// shoveCreature knocks a live land creature back a tile onto walkable ground.
func (a *area) shoveCreature(c world.Creature, sp game.Species) {
	if sp.Aquatic {
		return // fish don't get bumped across the bank
	}
	dx, dy := a.pushDir(c.X, c.Y)
	nx, ny := c.X+dx, c.Y+dy
	if !a.walkableAt(nx, ny) {
		return
	}
	a.ctx.World.MutateCreature(c.ID, func(cc *world.Creature) bool {
		if cc.HP <= 0 {
			return false
		}
		cc.X, cc.Y = nx, ny
		return true
	})
}

// tameChance is the odds one offering of bait befriends a wary animal.
const tameChance = 0.4

// hasCompanion reports whether the player already keeps a pet — saved in the
// store, or live in the world right now (so it holds even without persistence).
func (a *area) hasCompanion() bool {
	if _, ok := a.ctx.Store.LoadCompanion(a.ctx.Name); ok {
		return true
	}
	for _, c := range a.ctx.World.CreaturesInArea("wilds") {
		if c.Owner == a.ctx.Name {
			return true
		}
	}
	return false
}

// tame offers bait to an adjacent animal. It costs one bait item; on a lucky
// roll the creature is befriended (Owner set, persisted as the player's
// companion, and it starts following); on a miss the bait is gone and the animal
// bolts. One companion per player.
func (a *area) tame(c world.Creature) {
	sp, ok := game.SpeciesByKind(c.Kind)
	if !ok {
		return
	}
	if !sp.Tameable {
		a.setToast("the " + sp.Name + " can't be tamed")
		return
	}
	if c.Owner != "" {
		a.setToast("the " + sp.Name + " already has a companion")
		return
	}
	if a.hasCompanion() {
		a.setToast("you already have a companion")
		return
	}
	if a.ctx.Inventory[sp.Bait] <= 0 {
		a.setToast("you need a " + itemName(sp.Bait) + " to tame the " + sp.Name)
		return
	}

	a.ctx.Inventory[sp.Bait]--
	if a.ctx.Inventory[sp.Bait] <= 0 {
		delete(a.ctx.Inventory, sp.Bait)
	}
	a.ctx.Store.SpendItem(a.ctx.Name, sp.Bait)
	a.noteSpecies(c.Kind)

	if a.rng.Float64() > tameChance {
		a.ctx.World.MutateCreature(c.ID, func(cc *world.Creature) bool {
			cc.State = "flee"
			return true
		})
		a.setToast("the " + sp.Name + " snatches the " + itemName(sp.Bait) + " and bolts")
		return
	}
	ok = a.ctx.World.MutateCreature(c.ID, func(cc *world.Creature) bool {
		if cc.Owner != "" {
			return false
		}
		cc.Owner, cc.State = a.ctx.Name, "tamed"
		return true
	})
	if !ok {
		a.setToast("the " + sp.Name + " slips away")
		return
	}
	a.ctx.Store.SaveCompanion(a.ctx.Name, c.Kind)
	a.setToast("the " + sp.Name + " befriends you — it'll follow along!")
}

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
		case world.EventPlayerDamaged:
			// You took a blow — react even though you didn't act.
			if ev.Target == a.ctx.Name {
				a.setToast(hitByMessage(ev))
				a.hurtUntil = time.Now().Add(hurtFlashDuration)
			}
		case world.EventPlayerDowned:
			if ev.Target == a.ctx.Name {
				a.setToast("you're knocked out! reviving at Durst HQ…")
				a.hurtUntil = time.Now().Add(hurtFlashDuration)
			}
		case world.EventPlayerRespawn:
			if ev.Target == a.ctx.Name {
				a.setToast("you come to, safe at Durst HQ")
			}
		case world.EventPlayerShoved:
			// Knocked back: your own client owns your position, so apply it here,
			// re-checking the cell is open (terrain is shared, so this rarely fails).
			if ev.Target == a.ctx.Name && !a.building && a.fits(ev.X, ev.Y) {
				a.wx, a.wy = ev.X, ev.Y
				a.ctx.World.Move(a.ctx.Name, a.wx, a.wy)
				a.reveal()
				a.persist()
			}
		}
		return a, nil

	case tea.KeyMsg:
		ks := msg.String()

		// Build mode toggle (works from anywhere).
		if ks == "b" {
			a.building = !a.building
			if a.building {
				if a.buildSel < 0 || a.buildSel >= len(game.Placeables) {
					a.buildSel = 0
				}
				a.bx, a.by = a.wx, a.wy-1 // ghost just above the body to start
			}
			return a, nil
		}
		// While building, keys drive the ghost and the picker, not the body.
		if a.building {
			switch ks {
			case "esc":
				a.building = false
			case "r", "]":
				a.buildSel = (a.buildSel + 1) % len(game.Placeables)
			case "[":
				a.buildSel = (a.buildSel + len(game.Placeables) - 1) % len(game.Placeables)
			case "1", "2", "3", "4", "5", "6", "7", "8", "9":
				if idx, ok := game.PaletteHotkey(a.ctx, int(ks[0]-'0')); ok {
					a.buildSel = idx
				}
			case "e", "enter":
				// A tool clears; over a settlement building e deeds the plot; on open
				// ground it places the selected structure.
				if pb, ok := a.selectedTool(); ok {
					a.clearUnderGhost(pb)
				} else if _, ok := a.gen.PlotAt(a.bx, a.by); ok {
					a.claimUnderGhost()
				} else {
					a.placeStructure()
				}
			case "x":
				// Over your own claimed plot, x releases the deed; otherwise it
				// demolishes your structure under the ghost.
				if a.releaseUnderGhost() {
					// released — toast set inside
				} else if game.Demolish(a.ctx, a.bx, a.by) {
					a.setToast("removed")
				} else {
					a.setToast("nothing of yours there")
				}
			default:
				if dx, dy, _, ok := game.MoveKey(ks); ok {
					nx, ny := a.bx+dx, a.by+dy
					if iabs(nx-a.wx) <= 10 && iabs(ny-a.wy) <= 8 { // keep the ghost on-screen, near home
						a.bx, a.by = nx, ny
					}
				}
			}
			return a, nil
		}

		if ks == "m" {
			a.showMap = !a.showMap
			return a, nil
		}
		if ks == "e" {
			if g, ok := a.sealedGateUnderBody(); ok {
				a.offerToGate(g)
			} else if _, _, _, ok := a.hatUnderBody(); ok {
				a.pickUp()
			} else if _, _, _, ok := a.itemUnderBody(); ok {
				a.pickUp()
			} else if c, ok := a.creatureAdjacent(); ok {
				a.observe(c)
			} else if mx, my, ok := a.stationAdjacent(); ok {
				a.ctx.UseStation = &[2]int{mx, my} // the client opens the right panel
			} else if a.boardAdjacent() {
				a.showBoard = !a.showBoard // read / dismiss the notice board
			} else {
				a.pickUp()
			}
			return a, nil
		}
		if ks == "f" { // strike what you face — hunt an animal or, in the wild, a player
			a.strike()
			return a, nil
		}
		if ks == "t" { // tame an adjacent animal with bait
			if c, ok := a.creatureAdjacent(); ok {
				a.tame(c)
			}
			return a, nil
		}
		if a.showMap {
			a.showMap = false // any other key closes the map
		}
		if a.showBoard {
			a.showBoard = false // any other key closes the notice board
		}
		dx, dy, steps, ok := game.MoveKey(ks)
		if !ok {
			return a, nil
		}
		if a.ctx.World.Downed(a.ctx.Name) {
			return a, nil // knocked out: no walking until you revive at the hub
		}
		sx, sy := a.wx, a.wy
		for i := 0; i < steps; i++ {
			nx, ny := a.wx+dx, a.wy+dy
			if !game.CanStep(a.walkableAt, a.wx, a.wy, dx, dy) {
				break
			}
			a.wx, a.wy = nx, ny
			a.reveal()
			a.pickUp() // hats and items are gathered just by walking over them
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
			a.updateClaimPresence()
			a.tendCleared()
		}
	}
	return a, nil
}

// healthHint surfaces a little HP bar while you're hurt, so the bottom line
// stays quiet during peaceful play but tells you where you stand mid-fight.
func (a *area) healthHint() (string, bool) {
	self, ok := a.ctx.World.Self(a.ctx.Name)
	if !ok || self.MaxHP <= 0 || self.HP >= self.MaxHP {
		return "", false
	}
	const w = 10
	filled := self.HP * w / self.MaxHP
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat("▰", filled) + strings.Repeat("▱", w-filled)
	return fmt.Sprintf("HP %s %d/%d", bar, self.HP, self.MaxHP), true
}

func (a *area) Hint() string {
	if a.ctx.World.Downed(a.ctx.Name) {
		return "✖ knocked out — reviving at Durst HQ…"
	}
	if s, ok := a.healthHint(); ok {
		return s
	}
	if a.building {
		if s, ok := a.ghostClaimPrompt(); ok {
			return s + " · b done"
		}
		pb := game.Placeables[a.buildSel]
		return fmt.Sprintf("build: %s (%s) · e place · x remove · r next · b done", pb.Name, game.PlaceableCost(pb))
	}
	if name, ok := a.portalUnder(a.wx, a.wy); ok {
		return "◈ step in to enter " + game.DisplayName(name)
	}
	if g, ok := a.sealedGateUnderBody(); ok {
		return a.gatePrompt(g)
	}
	if wp, _, _, ok := a.artifactUnderBody(); ok {
		return "✦ e — claim " + wp.Name + ", a legend!"
	}
	if h, _, _, ok := a.hatUnderBody(); ok {
		return "e — wear the " + h.name
	}
	if c, ok := a.creatureAdjacent(); ok {
		return a.creaturePrompt(c)
	}
	if s, ok := a.strikePrompt(); ok {
		return s
	}
	if s, ok := a.projectSiteAdjacent(); ok {
		return "⌸ " + a.projectStatus(s)
	}
	if a.boardAdjacent() {
		return "e — read the notice board"
	}
	if c, mine, ok := game.WorkspaceAt(a.ctx, a.wx, a.wy); ok {
		dx, dy := worldgen.GateX-a.wx, worldgen.GateY-a.wy
		return a.workspaceLabel(c, mine) + " · ⌂ " + bearing(dx, dy)
	}
	dx, dy := worldgen.GateX-a.wx, worldgen.GateY-a.wy
	return fmt.Sprintf("⌂ Durst HQ %s · y u b n diagonals · m map", bearing(dx, dy))
}

// Prompt implements game.Prompter: the single action available right where the
// player stands. The bearing-to-home fallback in Hint is ambient navigation,
// not an action, so here it returns ok=false to keep the HD bottom clear.
func (a *area) Prompt() (string, bool) {
	if a.building {
		if s, ok := a.ghostClaimPrompt(); ok {
			return s, true
		}
		pb := game.Placeables[a.buildSel]
		return fmt.Sprintf("e build %s (%s) · r next · x remove · b done", pb.Name, game.PlaceableCost(pb)), true
	}
	if name, ok := a.portalUnder(a.wx, a.wy); ok {
		return "step in to enter " + game.DisplayName(name), true
	}
	if g, ok := a.sealedGateUnderBody(); ok {
		return a.gatePrompt(g), true
	}
	if h, _, _, ok := a.hatUnderBody(); ok {
		return "e — wear the " + h.name, true
	}
	if wp, _, _, ok := a.artifactUnderBody(); ok {
		return "✦ e — claim " + wp.Name + ", a legend!", true
	}
	if it, _, _, ok := a.itemUnderBody(); ok {
		return "e — take " + it.Name, true
	}
	if c, ok := a.creatureAdjacent(); ok {
		return a.creaturePrompt(c), true
	}
	if s, ok := a.strikePrompt(); ok {
		return s, true
	}
	if mx, my, ok := a.stationAdjacent(); ok {
		if pl, ok := a.ctx.World.PlacementAt(mx, my); ok {
			if pb, ok := game.PlaceableByID(pl.Kind); ok {
				return "e — open the " + pb.Name, true
			}
		}
	}
	if a.boardAdjacent() {
		return "e — read the notice board", true
	}
	return "", false
}

// boardAdjacent reports whether the notice board sits next to the player's 2×2
// body (the board blocks, so you read it from an abutting tile). The ring is the
// footprint grown by one cell in every direction.
func (a *area) boardAdjacent() bool {
	bx, by := worldgen.HubBoard()
	return bx >= a.wx-1 && bx <= a.wx+game.PlayerW &&
		by >= a.wy-1 && by <= a.wy+game.PlayerH
}

// projectSiteAdjacent returns the community-build anchor the player's body
// stands beside (within one tile), if any — the cue to show its progress.
func (a *area) projectSiteAdjacent() (worldgen.ProjectSite, bool) {
	for _, s := range worldgen.ProjectSites {
		if s.X >= a.wx-1 && s.X <= a.wx+game.PlayerW &&
			s.Y >= a.wy-1 && s.Y <= a.wy+game.PlayerH {
			return s, true
		}
	}
	return worldgen.ProjectSite{}, false
}

// projectStatus reads a build's live state and formats its progress line from
// the game catalog (docs/COMMUNITY_PLAN.md). An absent project (no contribution
// yet) reads as phase 0 with an empty pool.
func (a *area) projectStatus(s worldgen.ProjectSite) string {
	def, ok := game.ProjectByID(s.ID)
	if !ok {
		return s.Name
	}
	st, _ := a.ctx.World.ProjectState(s.ID)
	return def.ProjectStatus(st.Phase, st.Pool, st.Done)
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
// the cell collected and persist. Called both by the 'e' key and by movement, so
// finds — hats included — are gathered just by walking over them.
func (a *area) pickUp() {
	if wp, _, _, ok := a.artifactUnderBody(); ok {
		a.claimArtifact(wp)
		return
	}
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
// the cell collected and persists it. Split out from pickUp so the hat branch can
// take precedence: pickUp tries a hat first, then falls back here for loot. A
// find that has a matching wearable (a mushroom → the shroom accessory) also
// unlocks it, so some foraged loot doubles as an outfit. Returns whether anything
// was taken.
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
	// Keep our wielded weapon current on the shared player record so it draws
	// in-hand on every client's view of us — synced here (the one place both
	// clients run each frame) but only on a change, to avoid lock churn.
	if wid := game.WieldedWeapon(a.ctx).Item; wid != a.wieldSync {
		a.ctx.World.SetWeapon(a.ctx.Name, wid)
		a.wieldSync = wid
	}
	// Center the 2×2 body (not its top-left corner) in the window, so the avatar
	// sits dead center in both the glyph and HD views.
	ox, oy := a.wx-(vw-game.PlayerW)/2, a.wy-(vh-game.PlayerH)/2
	// Live wildlife, read fresh each frame (no events — the redraw is the sync).
	// Keyed by cell for O(1) lookup as we lay the window.
	creatures := map[[2]int]world.Creature{}
	for _, c := range a.ctx.World.CreaturesInArea("wilds") {
		creatures[[2]int{c.X, c.Y}] = c
	}
	// Land claims overlapping this window, fetched once: their parcels get a soft
	// ground tint so ownership reads at a glance (green yours, amber others).
	claims := game.LiveClaimsOverlapping(a.ctx, ox, oy, ox+vw-1, oy+vh-1)
	// Cleared cells in view, fetched once: a felled tree / broken boulder reads as
	// walkable ground instead of the original terrain feature.
	clearedSet := game.ActiveClearedSet(a.ctx, ox, oy, ox+vw-1, oy+vh-1)
	tiles := make([][]game.Tile, vh)
	for ly := 0; ly < vh; ly++ {
		row := make([]game.Tile, vw)
		for lx := 0; lx < vw; lx++ {
			wx, wy := ox+lx, oy+ly
			if a.seen(wx, wy) {
				cell := a.gen.At(wx, wy)
				t := CellTile(cell)
				if clearedSet[[2]int{wx, wy}] {
					t = clearedTile(cell) // felled/quarried → walkable ground
				}
				if g, ok := gateAtCell(wx, wy); ok {
					// Both the open gate and the broken (sealed) arch carry the name of
					// where they lead, so the renderer can float a label above them.
					t.Label = game.DisplayName(g.Portal)
					if !a.gateOpen(g) { // sealed: a dull, broken arch (no swirl)
						t.Ch, t.Color = '⊘', "#8A8A98"
						t.Prop, t.PropHex = game.PropSealed, "#7A7A88"
						t.Tex, t.Ground = game.TexRock, "#33363E"
					}
				} else if id, isArt := a.artifactAtCell[[2]int{wx, wy}]; isArt && a.artifactUnclaimed(id) {
					// A hidden legend, still unclaimed: a glowing arm on the ground that
					// shines at night so a far-off shimmer can draw you to it.
					if it, ok := game.ItemByID(id); ok {
						t.Ch, t.Color = it.Glyph, it.Hex
						t.Prop, t.PropHex, t.Ground = game.PropGemGlow, it.Hex, groundColor(cell.Biome)
					}
				} else if !a.collected[[2]int{wx, wy}] {
					// Items/hats keep the biome ground under them; only the gem/hat on
					// top is colored. (Without an explicit Ground the HD renderer would
					// treat the loot color as the surface and dither a halo around it.)
					if h, ok := hatAt(cell, wx, wy); ok {
						t.Ch, t.Color = '♚', h.hex
						t.Prop, t.PropHex, t.Ground = game.PropHat, h.hex, groundColor(cell.Biome)
					} else if it, ok := itemAt(cell, wx, wy); ok {
						// Pin the surface before recoloring: a plain cell carries no
						// explicit Ground, so the HD renderer falls back to Color —
						// recoloring to the loot hue without this paints the whole tile
						// in the loot color instead of keeping the floor under it.
						if t.Ground == "" {
							t.Ground = t.Color
						}
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
				// Player-built structures (the shared placements layer) sit on top of
				// the biome ground, overriding any loot the cell would otherwise show.
				if pl, ok := a.ctx.World.PlacementAt(wx, wy); ok {
					if pb, ok := game.PlaceableByID(pl.Kind); ok {
						t.Ch, t.Color = pb.Glyph, pb.Hex
						t.Prop, t.PropHex = pb.Prop, pb.Hex
						t.Walkable = pb.Walkable
						t.Ground = groundColor(cell.Biome)
					}
				}
				// Wildlife: the glyph client draws the species letter here; the HD
				// client draws a full animated sprite over this tile from the live
				// creature list (passed to the renderer), so we only set Ch/Color.
				// Pin the tile's ground color first: a plain ground cell carries no
				// explicit Ground, so the HD renderer falls back to Color for its
				// surface — recoloring Color to the species hue without this would
				// paint the whole tile (and bleed into neighbours' seam dither) in
				// the animal's color, a colored box behind the sprite.
				if c, ok := creatures[[2]int{wx, wy}]; ok {
					if sp, ok := game.SpeciesByKind(c.Kind); ok {
						if t.Ground == "" {
							t.Ground = t.Color
						}
						t.Ch, t.Color = sp.Glyph, sp.Hex
					}
				}
				if len(claims) > 0 {
					if tint, ok := claimTint(claims, a.ctx.Name, wx, wy); ok {
						if t.Ground != "" && t.Ground != fogColor {
							t.Ground = string(ui.Blend(t.Ground, tint, 0.16))
						}
						if t.Color != "" { // a fainter nudge so the glyph view hints at it too
							t.Color = string(ui.Blend(t.Color, tint, 0.10))
						}
					}
				}
				row[lx] = t
			} else {
				row[lx] = fogTile() // the unexplored world stays hidden
			}

			// The build ghost draws over everything (even fog), green where it can
			// go and red where it can't, so placement reads before you commit.
			if a.building && wx == a.bx && wy == a.by {
				pb := game.Placeables[a.buildSel]
				hex := "#7BD88F"
				if game.IsTool(pb) {
					if !a.canClearAt(wx, wy, pb) {
						hex = "#E0604D"
					}
				} else if !a.canBuildAt(wx, wy) {
					hex = "#E0604D"
				}
				g := row[lx]
				g.Ch, g.Color, g.Prop, g.PropHex = pb.Glyph, hex, pb.Prop, hex
				if g.Ground == "" || g.Ground == fogColor {
					g.Ground = "#33402F"
				}
				row[lx] = g
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
	} else if a.showBoard {
		panel := a.boardPanel()
		pw := lipgloss.Width(panel)
		view = ui.Overlay(view, panel, (width-pw)/2, 1)
	} else if a.building {
		panel := a.buildPanel()
		pw := lipgloss.Width(panel)
		view = ui.Overlay(view, panel, (width-pw)/2, 1) // centered, like the other panels
		if msg, show := a.Toast(); show {
			th := a.theme()
			view = ui.Overlay(view, th.Toast.Render(msg), (width-lipgloss.Width(th.Toast.Render(msg)))/2, height-2)
		}
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
	case 'N': // a village notice board on the green (blocks, readable)
		t.Prop, t.PropHex, t.Ground = game.PropNoticeBoard, c.Color, groundColor(c.Biome)
	case '◉': // a fountain — glowing water centerpiece (blocks)
		t.Prop, t.PropHex, t.Ground, t.Tex = game.PropFountain, c.Color, "#2E6BFF", game.TexWater
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

// boardEntries is the notice board's text: a welcome and a directory of the
// doors and gates around the green. Shared by the glyph panel and the HD slide.
func boardEntries() (title string, lines []string) {
	return "Notice Board", []string{
		"Welcome, traveller. The doors around the green:",
		"",
		"  ⌂  Durst HQ — the keep at the heart",
		"  P  Presentation — east, past the market hall",
		"  K  Kraftwerk — west, by the smithy",
		"  D  Demo Center — north, by the chapel",
		"",
		"Two old gates stand beyond:",
		"  ◈  Whispering Gate (east) — answer its riddle,",
		"     or offer an Ice Crystal",
		"  ◈  Sunken Gate (north) — the village pools",
		"     Gold Nuggets to raise it",
		"",
		"Stand on a door and step in.",
	}
}

// theme returns the session theme, falling back to the default (nil-safe).
func (a *area) theme() *ui.Theme {
	if a.ctx.Theme != nil {
		return a.ctx.Theme
	}
	return ui.Default
}

// buildPanel renders the build palette for the glyph client: the catalog grouped
// (Structures · Machines · Trade · Tools), each row with a 1-9 hotbar badge, its
// cost and afford count, the selected row marked, plus the current blurb and a
// block reason. Mirrors the HD game.DrawBuildPanel.
func (a *area) buildPanel() string {
	th := a.theme()
	green := th.Fg(lipgloss.Color("#7BD88F"))
	rows := []string{th.PanelTitle.Render("Build")}
	for _, g := range game.BuildPalette(a.ctx) {
		rows = append(rows, th.Dim.Render(g.Name))
		for _, e := range g.Entries {
			badge := "   "
			if e.Hotkey > 0 {
				badge = fmt.Sprintf("[%d]", e.Hotkey)
			}
			body := fmt.Sprintf("%-22s %s", e.P.Name, game.PlaceableCost(e.P))
			cnt := fmt.Sprintf("x%d", e.Max)
			if e.P.Cat == game.CatTool {
				cnt = "ready"
			}
			marker := "  "
			var line string
			switch {
			case e.Index == a.buildSel:
				marker = th.Accent.Render("► ")
				line = th.Bright.Render(body) + "  " + green.Render(cnt)
			case e.Afford:
				line = th.ChatText.Render(body) + "  " + green.Render(cnt)
			default:
				line = th.Dim.Render(body + "  " + cnt)
			}
			rows = append(rows, marker+badge+" "+line)
		}
	}
	rows = append(rows, "")
	if a.buildSel >= 0 && a.buildSel < len(game.Placeables) {
		rows = append(rows, th.Dim.Render("\""+game.Placeables[a.buildSel].Blurb+"\""))
	}
	if footer, warn := a.buildFooter(); footer != "" {
		if warn {
			rows = append(rows, th.Warn.Render(footer))
		} else {
			rows = append(rows, th.Accent.Render(footer))
		}
	} else {
		rows = append(rows, th.Dim.Render("1-9/r pick · e place · x remove · b done"))
	}
	return th.Panel.Render(strings.Join(rows, "\n"))
}

// boardPanel renders the notice board for the glyph client, styled to match the
// map and other overlays.
func (a *area) boardPanel() string {
	th := a.ctx.Theme
	if th == nil {
		th = ui.Default
	}
	title, lines := boardEntries()
	var b strings.Builder
	b.WriteString(th.PanelTitle.Render(title) + "\n")
	for _, ln := range lines {
		b.WriteString(ln + "\n")
	}
	b.WriteString(th.Dim.Render("e or move to close"))
	return th.Panel.Render(b.String())
}

// HDSlide implements game.HDOverlayer: when the notice board is open it renders
// its text as a panel over the HD frame (which has no glyph layer of its own).
func (a *area) HDSlide() (string, string, bool) {
	if !a.showBoard {
		return "", "", false
	}
	title, lines := boardEntries()
	src := "# " + title + "\n\n" + strings.Join(lines, "\n")
	return src, "e or move to close", true
}

// HDMinimap supplies the same coarse overview to the HD pixel client, which
// rasterizes the cells as colored blocks rather than glyphs. It mirrors minimap:
// one block per few tiles, centered on the player, filling in as you explore.
func (a *area) HDMinimap() (string, [][]game.MiniCell, bool) {
	if !a.showMap {
		return "", nil, false
	}
	const (
		stride = 4
		halfW  = 19
		halfH  = 9
	)
	rows := make([][]game.MiniCell, 0, 2*halfH+1)
	for ry := -halfH; ry <= halfH; ry++ {
		row := make([]game.MiniCell, 0, 2*halfW+1)
		for rx := -halfW; rx <= halfW; rx++ {
			switch {
			case rx == 0 && ry == 0:
				row = append(row, game.MiniCell{Self: true})
			case !a.seen(a.wx+rx*stride, a.wy+ry*stride):
				row = append(row, game.MiniCell{}) // unexplored
			default:
				c := a.gen.At(a.wx+rx*stride, a.wy+ry*stride)
				hex := c.Color
				if hex == "" {
					hex = ui.HexDim
				}
				row = append(row, game.MiniCell{Hex: hex})
			}
		}
		rows = append(rows, row)
	}
	return "Map — The Wilds", rows, true
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

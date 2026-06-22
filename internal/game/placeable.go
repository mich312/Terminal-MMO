package game

// Placeables: the catalog of structures a player can build into the world, plus
// the pure cost logic. This is Milestone 1, step 2 (docs/IMPLEMENTATION_PLAN.md):
// building spends materials (often crafted goods from step 1) and writes one row
// to the shared placements layer. A Placeable maps an id to the prop both
// renderers draw, a glyph for the glyph client, and a material cost. Voice is
// corporate × medieval.

// BuildCat groups placeables in the build palette.
type BuildCat int

const (
	CatStructure BuildCat = iota // fences, benches, storage, lights
	CatMachine                   // offline producers
	CatTrade                     // the Concession
	CatTool                      // clearing tools (Step C) — shown only when owned
)

// ClearKind is what a tool clears (0 = not a tool).
type ClearKind int

const (
	ClearNone ClearKind = iota
	ClearTree           // an axe fells a forest tree
	ClearRock           // a pick breaks a hill boulder
)

// Yield is what clearing one cell of this kind returns to the pack.
func (k ClearKind) Yield() (item string, n int) {
	switch k {
	case ClearTree:
		return "wood", 3
	case ClearRock:
		return "stone", 3
	default:
		return "", 0
	}
}

// Placeable is one buildable structure — or, when Clear is set, a clearing tool
// the palette lists in the Tools group (it isn't built or placed; selecting it
// turns the ghost into a clear cursor).
type Placeable struct {
	ID       string
	Name     string
	Glyph    rune     // glyph-client tile rune
	Prop     TileProp // HD sprite
	Hex      string   // prop color
	Cost     []Ingredient
	Walkable bool      // can a player stand on it? (a sign yes; a wall no)
	Cat      BuildCat  // build-palette group
	Clear    ClearKind // non-zero → a tool that clears terrain, not a structure
	Blurb    string    // one deadpan line for the build picker
}

// IsTool reports whether a placeable is a clearing tool rather than a structure.
func IsTool(p Placeable) bool { return p.Clear != ClearNone }

// Placeables is the catalog, in build-picker order. Costs lean on crafted goods
// (planks, lamps) so building gives step-1 crafting a purpose.
var Placeables = []Placeable{
	{ID: "fence", Name: "Wooden Fence", Glyph: '#', Prop: PropFenceH, Hex: "#8A6E3C",
		Cost: []Ingredient{{"plank", 1}}, Walkable: false, Cat: CatStructure,
		Blurb: "Demarcates your Workspace. Good fences, good colleagues."},
	{ID: "workbench", Name: "Workbench", Glyph: '⊓', Prop: PropWorkbench, Hex: "#B8924E",
		Cost: []Ingredient{{"plank", 4}}, Walkable: false, Cat: CatStructure,
		Blurb: "A Crafting (Self-Service) station. Self-assembly required."},
	{ID: "chest", Name: "Cold Storage", Glyph: '▣', Prop: PropChest, Hex: "#9C7A45",
		Cost: []Ingredient{{"plank", 3}}, Walkable: false, Cat: CatStructure,
		Blurb: "An asset locker. Durst Group is not liable for spoilage."},
	{ID: "lamppost", Name: "Lamppost", Glyph: '☼', Prop: PropLamp, Hex: "#FFD27A",
		Cost: []Ingredient{{"lamp", 1}}, Walkable: false, Cat: CatStructure,
		Blurb: "Casts a warm, compliant glow over the night shift."},
	{ID: "bedroll", Name: "Bedroll", Glyph: '▭', Prop: PropBedroll, Hex: "#A6764A",
		Cost: []Ingredient{{"leather", 1}, {"pelt", 1}, {"feather", 2}}, Walkable: true, Cat: CatStructure,
		Blurb: "A hide bedroll, pelt-lined and down-stuffed. The frontier's PTO."},
	{ID: "stall", Name: "Durst Group Concession", Glyph: '╒', Prop: PropStall, Hex: "#C98A4A",
		Cost: []Ingredient{{"plank", 4}}, Walkable: false, Cat: CatTrade,
		Blurb: "Vends your goods to passers-by. Trades while you're away."},
	// Machines (see machine.go): inert until fueled, then they produce offline.
	{ID: "sawmill", Name: "Sawmill", Glyph: '⊞', Prop: PropSawmill, Hex: "#8FB7FF",
		Cost: []Ingredient{{"plank", 6}, {"lamp", 1}}, Walkable: false, Cat: CatMachine,
		Blurb: "Timber in, planks out. Runs the night shift unattended."},
	{ID: "mill", Name: "Mill", Glyph: '❋', Prop: PropMill, Hex: "#C2A06A",
		Cost: []Ingredient{{"plank", 6}, {"lamp", 1}}, Walkable: false, Cat: CatMachine,
		Blurb: "Grain to flour. Q3 throughput, zero supervision."},
	{ID: "furnace", Name: "Ingot Synergy Furnace", Glyph: '♨', Prop: PropFurnace, Hex: "#C46A3A",
		Cost: []Ingredient{{"stone", 8}, {"plank", 4}}, Walkable: false, Cat: CatMachine,
		Blurb: "Synergizes raw nuggets into ingots. Glows while it works."},
	// Tools (shown only when owned): select one to turn the ghost into a clear
	// cursor. No cost — you craft the tool item, then wield it freely.
	{ID: "axe", Name: "Axe", Glyph: '⚒', Hex: "#B08D57", Cat: CatTool, Clear: ClearTree,
		Blurb: "Fell a tree: clears it to grassy ground and yields Timber."},
	{ID: "pick", Name: "Pickaxe", Glyph: '⚒', Hex: "#AEB7BE", Cat: CatTool, Clear: ClearRock,
		Blurb: "Break a hill boulder: clears it to open ground and yields Cut Stone."},
}

var placeableIndex = func() map[string]Placeable {
	m := make(map[string]Placeable, len(Placeables))
	for _, p := range Placeables {
		m[p.ID] = p
	}
	return m
}()

// PlaceableByID looks up a placeable; ok is false for an unknown id.
func PlaceableByID(id string) (Placeable, bool) {
	p, ok := placeableIndex[id]
	return p, ok
}

// CanAfford reports whether inv holds every material a placeable costs.
func CanAfford(p Placeable, inv map[string]int) bool {
	for _, c := range p.Cost {
		if inv[c.Item] < c.N {
			return false
		}
	}
	return true
}

// SpendFor deducts a placeable's cost from the pack (live + store), returning
// false (a no-op) if the pack can't afford it. Mirrors Craft's spend.
func SpendFor(ctx *Ctx, p Placeable) bool {
	if ctx.Inventory == nil {
		ctx.Inventory = map[string]int{}
	}
	if !CanAfford(p, ctx.Inventory) {
		return false
	}
	for _, c := range p.Cost {
		for i := 0; i < c.N; i++ {
			ctx.Inventory[c.Item]--
			ctx.Store.SpendItem(ctx.Name, c.Item)
		}
		if ctx.Inventory[c.Item] <= 0 {
			delete(ctx.Inventory, c.Item)
		}
	}
	return true
}

// PaletteEntry is one row of the build palette: a placeable plus live afford
// state. Index is its position in Placeables (so it maps straight to the ghost
// selection); Hotkey is its 1-9 number-key badge (0 = no badge).
type PaletteEntry struct {
	Index  int
	P      Placeable
	Afford bool
	Max    int // how many the pack could build (0 when unaffordable)
	Hotkey int
}

// PaletteGroup is a titled run of entries in build-palette order.
type PaletteGroup struct {
	Cat     BuildCat
	Name    string
	Entries []PaletteEntry
}

func (c BuildCat) String() string {
	switch c {
	case CatMachine:
		return "Machines"
	case CatTrade:
		return "Trade"
	case CatTool:
		return "Tools"
	default:
		return "Structures"
	}
}

// buildCatOrder is the palette's top-to-bottom group order.
var buildCatOrder = []BuildCat{CatStructure, CatMachine, CatTrade, CatTool}

// affordMax reports how many of a placeable the pack can build (min over its
// cost lines), 0 when short of any input.
func affordMax(p Placeable, inv map[string]int) int {
	if len(p.Cost) == 0 {
		return 0
	}
	best := -1
	for _, c := range p.Cost {
		if c.N <= 0 {
			continue
		}
		if n := inv[c.Item] / c.N; best < 0 || n < best {
			best = n
		}
	}
	if best < 0 {
		return 0
	}
	return best
}

// BuildPalette groups the buildable catalog for the build UI, annotating each
// entry with live affordability and a 1-9 hotbar badge (in catalog order). Tool
// placeables are included only when the player owns them (Step C).
func BuildPalette(ctx *Ctx) []PaletteGroup {
	inv := invOf(ctx)
	groups := make([]PaletteGroup, 0, len(buildCatOrder))
	hot := 0
	for _, cat := range buildCatOrder {
		var entries []PaletteEntry
		for i, p := range Placeables {
			if p.Cat != cat {
				continue
			}
			if cat == CatTool && !OwnsTool(ctx, p.ID) {
				continue // tools appear once found/crafted
			}
			e := PaletteEntry{Index: i, P: p, Max: affordMax(p, inv)}
			e.Afford = e.Max > 0
			if p.Cat == CatTool {
				e.Afford = true // owning it (filtered in below) means it's ready to wield
			}
			if hot < 9 {
				hot++
				e.Hotkey = hot
			}
			entries = append(entries, e)
		}
		if len(entries) > 0 {
			groups = append(groups, PaletteGroup{Cat: cat, Name: cat.String(), Entries: entries})
		}
	}
	return groups
}

// PaletteHotkey maps a 1-9 number key to the Placeables index it selects, in
// palette order. ok is false when no entry carries that badge.
func PaletteHotkey(ctx *Ctx, n int) (int, bool) {
	for _, g := range BuildPalette(ctx) {
		for _, e := range g.Entries {
			if e.Hotkey == n {
				return e.Index, true
			}
		}
	}
	return 0, false
}

// OwnsTool reports whether the player owns a clearing tool — its tool item (the
// placeable id doubles as the inventory item id, e.g. "axe") is in the pack.
func OwnsTool(ctx *Ctx, id string) bool { return invOf(ctx)[id] > 0 }

// PlaceableCost renders a placeable's cost as "4 Planks" or "1 Herb + 1 Amber".
func PlaceableCost(p Placeable) string {
	s := ""
	for i, c := range p.Cost {
		if i > 0 {
			s += " + "
		}
		name := c.Item
		if it, ok := ItemByID(c.Item); ok {
			name = it.Name
		}
		s += itoa(c.N) + " " + name
	}
	return s
}

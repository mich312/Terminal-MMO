package game

// Placeables: the catalog of structures a player can build into the world, plus
// the pure cost logic. This is Milestone 1, step 2 (docs/IMPLEMENTATION_PLAN.md):
// building spends materials (often crafted goods from step 1) and writes one row
// to the shared placements layer. A Placeable maps an id to the prop both
// renderers draw, a glyph for the glyph client, and a material cost. Voice is
// corporate × medieval.

// Placeable is one buildable structure.
type Placeable struct {
	ID       string
	Name     string
	Glyph    rune     // glyph-client tile rune
	Prop     TileProp // HD sprite
	Hex      string   // prop color
	Cost     []Ingredient
	Walkable bool   // can a player stand on it? (a sign yes; a wall no)
	Blurb    string // one deadpan line for the build picker
}

// Placeables is the catalog, in build-picker order. Costs lean on crafted goods
// (planks, lamps) so building gives step-1 crafting a purpose.
var Placeables = []Placeable{
	{ID: "fence", Name: "Wooden Fence", Glyph: '#', Prop: PropFenceH, Hex: "#8A6E3C",
		Cost: []Ingredient{{"plank", 1}}, Walkable: false,
		Blurb: "Demarcates your Workspace. Good fences, good colleagues."},
	{ID: "workbench", Name: "Workbench", Glyph: '⊓', Prop: PropWorkbench, Hex: "#B8924E",
		Cost: []Ingredient{{"plank", 4}}, Walkable: false,
		Blurb: "A Crafting (Self-Service) station. Self-assembly required."},
	{ID: "chest", Name: "Cold Storage", Glyph: '▣', Prop: PropChest, Hex: "#9C7A45",
		Cost: []Ingredient{{"plank", 3}}, Walkable: false,
		Blurb: "An asset locker. Durst Group is not liable for spoilage."},
	{ID: "lamppost", Name: "Lamppost", Glyph: '☼', Prop: PropLamp, Hex: "#FFD27A",
		Cost: []Ingredient{{"lamp", 1}}, Walkable: false,
		Blurb: "Casts a warm, compliant glow over the night shift."},
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

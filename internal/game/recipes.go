package game

// Crafting: a static recipe catalog plus the pure spend/yield logic, the twin of
// inventory.go. Recipes turn raw forage into refined goods using the same
// inventory plumbing pickups use (Ctx.Inventory + Store.AddItem/SpendItem). This
// is Milestone 1, step 1 (docs/IMPLEMENTATION_PLAN.md): pure inventory→inventory,
// no world state yet — machines and placeable structures arrive with the
// placements layer. Voice is corporate × medieval.

// Ingredient is one input line of a recipe: N of an item (by catalog ID).
type Ingredient struct {
	Item string
	N    int
}

// Recipe converts a set of inputs into N of an output item. Out is an item ID
// in the Items catalog; OutN is how many a single craft yields.
type Recipe struct {
	ID    string // stable id (keys nothing persisted yet; will key a station later)
	Name  string
	Blurb string // one deadpan line of flavor for the detail panel
	In    []Ingredient
	Out   string
	OutN  int
}

// Recipes is the catalog, in display order. Every input and output is an item
// ID in inventory.go's Items, so crafted goods show and persist like forage.
var Recipes = []Recipe{
	{ID: "plank", Name: "Planks", Blurb: "Dimensioned to Durst facility standard.",
		In: []Ingredient{{"wood", 2}}, Out: "plank", OutN: 1},
	{ID: "flour", Name: "Sack of Flour", Blurb: "Milled on-prem. Q3 yield, ground fresh.",
		In: []Ingredient{{"grain", 2}}, Out: "flour", OutN: 1},
	{ID: "ingot", Name: "Gold Ingot", Blurb: "Synergized from raw nuggets. Fungible.",
		In: []Ingredient{{"nugget", 2}}, Out: "ingot", OutN: 1},
	{ID: "salve", Name: "Field Salve", Blurb: "Herbal. Not evaluated by the guild apothecary.",
		In: []Ingredient{{"herb", 1}, {"mushroom", 1}}, Out: "salve", OutN: 1},
	{ID: "lamp", Name: "Wrought Lamp", Blurb: "Amber-lit. Casts a warm, compliant glow.",
		In: []Ingredient{{"nugget", 1}, {"amber", 1}}, Out: "lamp", OutN: 1},
	// Hunting spoils, refined. Cook meat for provisions; cure hide into leather,
	// which (with a pelt and down) builds a Bedroll.
	{ID: "ration", Name: "Field Ration", Blurb: "Cured game. Shelf-stable, morale-adjacent.",
		In: []Ingredient{{"meat", 2}}, Out: "ration", OutN: 1},
	{ID: "leather", Name: "Cured Leather", Blurb: "Tanned to spec. Supple, compliant.",
		In: []Ingredient{{"hide", 2}}, Out: "leather", OutN: 1},
	// Tools — gated by a rare found head, so the recipe is "unlocked" the moment
	// you turn one up in the world.
	{ID: "axe", Name: "Axe", Blurb: "A flint head, a Timber haft. Fells trees.",
		In: []Ingredient{{"axe_head", 1}, {"wood", 2}}, Out: "axe", OutN: 1},
	{ID: "pick", Name: "Pickaxe", Blurb: "An iron head, a Timber haft. Breaks rock.",
		In: []Ingredient{{"pick_head", 1}, {"wood", 2}}, Out: "pick", OutN: 1},
	// Arms — weapons for the hunt and the open Wilds (docs/WEAPON_PLAN.md). Every
	// input is an already-gathered good, so they need no new forage.
	{ID: "knife", Name: "Flint Knife", Blurb: "Knapped to an edge. Field-rated, not guild-certified.",
		In: []Ingredient{{"stone", 1}, {"wood", 1}}, Out: "knife", OutN: 1},
	{ID: "spear", Name: "Spear", Blurb: "Reach extended per Durst ergonomics memo.",
		In: []Ingredient{{"stone", 1}, {"wood", 3}}, Out: "spear", OutN: 1},
	{ID: "bow", Name: "Hunter's Bow", Blurb: "Tensioned yew. Loose responsibly.",
		In: []Ingredient{{"wood", 2}, {"leather", 1}, {"feather", 1}}, Out: "bow", OutN: 1},
	{ID: "arrow", Name: "Arrows", Blurb: "Fletched in batches. Consumables, per policy.",
		In: []Ingredient{{"wood", 1}, {"feather", 1}}, Out: "arrow", OutN: 4},
	{ID: "sword", Name: "Cast Blade", Blurb: "Poured from ingots. Asset, depreciating.",
		In: []Ingredient{{"ingot", 2}, {"wood", 1}}, Out: "sword", OutN: 1},
}

// Craftable reports how many times the recipe can be made from inv — the min,
// over inputs, of how many full sets the pack holds. Zero if any input is short.
func Craftable(r Recipe, inv map[string]int) int {
	n := -1
	for _, in := range r.In {
		if in.N <= 0 {
			continue
		}
		have := inv[in.Item] / in.N
		if n < 0 || have < n {
			n = have
		}
	}
	if n < 0 {
		return 0
	}
	return n
}

// Craft makes one of recipe r, spending its inputs and yielding its output
// through the live inventory and the store (so it persists like a pickup).
// Returns false (a no-op) if the pack can't afford it. Mirrors wilds pickup:
// Ctx.Inventory is the live map, Store the durable mirror.
func Craft(ctx *Ctx, r Recipe) bool {
	if ctx.Inventory == nil {
		ctx.Inventory = map[string]int{}
	}
	if Craftable(r, ctx.Inventory) < 1 {
		return false
	}
	for _, in := range r.In {
		for i := 0; i < in.N; i++ {
			ctx.Inventory[in.Item]--
			ctx.Store.SpendItem(ctx.Name, in.Item)
		}
		if ctx.Inventory[in.Item] <= 0 {
			delete(ctx.Inventory, in.Item)
		}
	}
	for i := 0; i < r.OutN; i++ {
		ctx.Inventory[r.Out]++
		ctx.Store.AddItem(ctx.Name, r.Out)
	}
	return true
}

// RecipeNeeds renders a recipe's inputs as "2 Timber + 1 Wild Herb", using the
// item catalog's display names. Shared by both clients' craft panels.
func RecipeNeeds(r Recipe) string {
	s := ""
	for i, in := range r.In {
		if i > 0 {
			s += " + "
		}
		name := in.Item
		if it, ok := ItemByID(in.Item); ok {
			name = it.Name
		}
		s += itoa(in.N) + " " + name
	}
	return s
}

// itoa is a tiny local int→string (avoids dragging strconv into this leaf file).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

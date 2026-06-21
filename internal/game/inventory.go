package game

// Item is a collectible kind found scattered in the world. The world scatter
// (which item appears where) lives in the wilds package; this is just the
// catalog the renderer and the /inventory panel read.
type Item struct {
	ID    string
	Name  string
	Glyph rune   // shown in the glyph renderer (HD draws a colored gem)
	Hex   string // display color
	Glow  bool   // emits a faint light at night (only luminous loot: crystals, mushrooms)
	Wear  string // accessory this find unlocks to wear, if any (a mushroom → shroom cap)
}

// Items is the catalog, in display order. IDs are stable — they key the store.
var Items = []Item{
	{"berry", "Sweet Berry", '◆', "#FF6B6B", false, ""},
	{"herb", "Wild Herb", '◆', "#7BD88F", false, ""},
	{"mushroom", "Mushroom", '◆', "#C792EA", true, "shroom"},
	{"shell", "Sea Shell", '◆', "#F2E9A0", false, ""},
	{"crystal", "Ice Crystal", '◆', "#7DF0FF", true, ""},
	{"nugget", "Gold Nugget", '◆', "#FFC861", false, ""},
	{"grain", "Sheaf of Grain", 'ψ', "#E6C84B", false, ""},
	{"stone", "Cut Stone", '◊', "#B8BEC6", false, ""},
	{"wood", "Timber", '‡', "#9C6B3F", false, ""},
	{"fish", "Fresh Fish", '⊰', "#7FD7E8", false, ""},
	{"geode", "Glittering Geode", '◈', "#9CE0FF", true, "circlet"},
	{"relic", "Ancient Relic", '◈', "#C9B0FF", true, "diadem"},
	{"spore", "Glowspore", '◆', "#8BF29C", true, "glowcap"},
	{"amber", "Cave Amber", '◆', "#FFB347", true, "ambergem"},
	// Crafted goods (made at the workbench, not foraged) — see recipes.go.
	{"plank", "Planks", '‡', "#C9A86A", false, ""},
	{"flour", "Sack of Flour", '∴', "#E8DEC0", false, ""},
	{"ingot", "Gold Ingot", '▰', "#FFD24A", false, ""},
	{"salve", "Field Salve", '✚', "#7BD88F", false, ""},
	{"lamp", "Wrought Lamp", '☼', "#FFC861", true, ""},
}

var itemIndex = func() map[string]Item {
	m := make(map[string]Item, len(Items))
	for _, it := range Items {
		m[it.ID] = it
	}
	return m
}()

// ItemByID looks up a catalog item; ok is false for an unknown id.
func ItemByID(id string) (Item, bool) {
	it, ok := itemIndex[id]
	return it, ok
}

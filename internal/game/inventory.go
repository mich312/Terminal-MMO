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
}

// Items is the catalog, in display order. IDs are stable — they key the store.
var Items = []Item{
	{"berry", "Sweet Berry", '◆', "#FF6B6B", false},
	{"herb", "Wild Herb", '◆', "#7BD88F", false},
	{"mushroom", "Mushroom", '◆', "#C792EA", true},
	{"shell", "Sea Shell", '◆', "#F2E9A0", false},
	{"crystal", "Ice Crystal", '◆', "#7DF0FF", true},
	{"nugget", "Gold Nugget", '◆', "#FFC861", false},
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

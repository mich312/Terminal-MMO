package game

// Rarity is how scarce a collectible is. It drives both the compendium's tag
// and (in the wilds) how often the item is scattered, so "Rare" on the card and
// rare on the ground stay the same fact.
type Rarity int

const (
	Common Rarity = iota
	Uncommon
	Rare
)

func (r Rarity) String() string {
	switch r {
	case Uncommon:
		return "Uncommon"
	case Rare:
		return "Rare"
	default:
		return "Common"
	}
}

// Source groups a collectible by how you come by it, so the compendium can lay
// the catalog out in sensible sections.
type Source int

const (
	Forage   Source = iota // scattered on biome ground, gathered by walking over it
	Worksite               // harvested at a settlement worksite (field, quarry, lumber, jetty)
	CaveFind               // mined or gathered down in the caves
)

func (s Source) String() string {
	switch s {
	case Worksite:
		return "Worksite"
	case CaveFind:
		return "Cave"
	default:
		return "Forage"
	}
}

// Item is a collectible kind found scattered in the world. The world scatter
// (which item appears where) lives in the wilds/cave packages; this is the
// catalog the renderers, the inventory HUD, and the /compendium panel read — the
// single source of truth for what each find is and what it does.
type Item struct {
	ID    string
	Name  string
	Glyph rune   // shown in the glyph renderer (HD draws a colored gem)
	Hex   string // display color
	Glow  bool   // emits a faint light at night (only luminous loot: crystals, mushrooms)
	Wear  string // accessory this find unlocks to wear, if any (a mushroom → shroom cap)

	Source Source // how it's gathered — groups the compendium
	Rarity Rarity // how scarce it is
	About  string // a sentence on what the find is
	Found  string // where/how you come across it
	Use    string // a non-wearable function (repairs a gate, …); "" if none
}

// Items is the catalog, in display order. IDs are stable — they key the store.
var Items = []Item{
	{ID: "berry", Name: "Sweet Berry", Glyph: '◆', Hex: "#FF6B6B",
		Source: Forage, Rarity: Common,
		About: "A plump wild berry, sweet straight off the bush.",
		Found: "Forests and meadows."},
	{ID: "herb", Name: "Wild Herb", Glyph: '◆', Hex: "#7BD88F",
		Source: Forage, Rarity: Common,
		About: "A fragrant sprig of wild greenery.",
		Found: "Meadows, savanna, and swamp edges."},
	{ID: "mushroom", Name: "Mushroom", Glyph: '◆', Hex: "#C792EA", Glow: true, Wear: "shroom",
		Source: Forage, Rarity: Uncommon,
		About: "A speckled cap that glows faintly after dark.",
		Found: "Forest and swamp floors (and down in the caves)."},
	{ID: "shell", Name: "Sea Shell", Glyph: '◆', Hex: "#F2E9A0",
		Source: Forage, Rarity: Common,
		About: "A polished shell left by the tide.",
		Found: "Sandy shores."},
	{ID: "crystal", Name: "Ice Crystal", Glyph: '◆', Hex: "#7DF0FF", Glow: true,
		Source: Forage, Rarity: Rare,
		About: "A shard of everfrost, cold and luminous.",
		Found: "Snowfields and high hills.",
		Use:   "Offer one to repair the Whispering Gate to The Grove."},
	{ID: "nugget", Name: "Gold Nugget", Glyph: '◆', Hex: "#FFC861",
		Source: Forage, Rarity: Rare,
		About: "A heavy fleck of raw gold.",
		Found: "Savanna and hill country.",
		Use:   "Pool with others to open the Sunken Gate to The Vault."},
	{ID: "grain", Name: "Sheaf of Grain", Glyph: 'ψ', Hex: "#E6C84B",
		Source: Worksite, Rarity: Common,
		About: "A golden sheaf cut from a ripe field.",
		Found: "Harvested from cultivated fields and gardens."},
	{ID: "stone", Name: "Cut Stone", Glyph: '◊', Hex: "#B8BEC6",
		Source: Worksite, Rarity: Common,
		About: "A squared block, fresh off the quarry floor.",
		Found: "Quarried in the mountains; also mined in caves."},
	{ID: "wood", Name: "Timber", Glyph: '‡', Hex: "#9C6B3F",
		Source: Worksite, Rarity: Common,
		About: "A sturdy log split from a felled tree.",
		Found: "Cut at lumber stumps along the wood paths."},
	{ID: "fish", Name: "Fresh Fish", Glyph: '⊰', Hex: "#7FD7E8",
		Source: Worksite, Rarity: Common,
		About: "A fresh catch, still glistening.",
		Found: "Landed off the jetty planks."},
	{ID: "geode", Name: "Glittering Geode", Glyph: '◈', Hex: "#9CE0FF", Glow: true, Wear: "circlet",
		Source: CaveFind, Rarity: Rare,
		About: "A hollow stone lined with cool blue crystal.",
		Found: "Cracked open deep in the caves."},
	{ID: "relic", Name: "Ancient Relic", Glyph: '◈', Hex: "#C9B0FF", Glow: true, Wear: "diadem",
		Source: CaveFind, Rarity: Rare,
		About: "A forgotten artifact, faintly aglow.",
		Found: "Guarded at cave landmarks."},
	{ID: "spore", Name: "Glowspore", Glyph: '◆', Hex: "#8BF29C", Glow: true, Wear: "glowcap",
		Source: CaveFind, Rarity: Uncommon,
		About: "A lantern-plant spore, bright warm green.",
		Found: "Gathered in glowspore caverns."},
	{ID: "amber", Name: "Cave Amber", Glyph: '◆', Hex: "#FFB347", Glow: true, Wear: "ambergem",
		Source: CaveFind, Rarity: Uncommon,
		About: "A bead of fossil amber, honey-warm.",
		Found: "Prised from the walls of amber caves."},
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

// WearPower reports the accessory an item unlocks when worn and what that
// wearable does — derived from the accessory catalog so the compendium can never
// drift from the real power. ok is false for a find that isn't a wearable.
func (it Item) WearPower() (accessory, power string, ok bool) {
	if it.Wear == "" {
		return "", "", false
	}
	idx, found := AccessoryIndex(it.Wear)
	if !found {
		return "", "", false
	}
	return AccessoryName(idx), AccessoryPower(idx), true
}

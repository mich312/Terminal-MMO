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
	Crafted                // made at a workbench or a machine, not gathered
	Hunt                   // taken from wild animals (the hunting loop)
)

func (s Source) String() string {
	switch s {
	case Worksite:
		return "Worksite"
	case CaveFind:
		return "Cave"
	case Crafted:
		return "Crafted"
	case Hunt:
		return "Hunt"
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
	// Crafted goods — made at a workbench (recipes.go) or produced by a machine,
	// never foraged. They group under "Crafted" in the compendium.
	{ID: "plank", Name: "Planks", Glyph: '‡', Hex: "#C9A86A",
		Source: Crafted, Rarity: Common,
		About: "Dimensioned boards, dressed to Durst facility standard.",
		Found: "Crafted from Timber at a workbench, or milled by a Sawmill."},
	{ID: "flour", Name: "Sack of Flour", Glyph: '∴', Hex: "#E8DEC0",
		Source: Crafted, Rarity: Common,
		About: "Finely milled flour, ground fresh on-prem.",
		Found: "Crafted from Grain, or ground by a Mill."},
	{ID: "ingot", Name: "Gold Ingot", Glyph: '▰', Hex: "#FFD24A",
		Source: Crafted, Rarity: Uncommon,
		About: "A tidy bar of synergized gold. Fungible.",
		Found: "Crafted from Gold Nuggets, or smelted by a Furnace."},
	{ID: "salve", Name: "Field Salve", Glyph: '✚', Hex: "#7BD88F",
		Source: Crafted, Rarity: Common,
		About: "A herbal salve. Not evaluated by the guild apothecary.",
		Found: "Crafted from a Wild Herb and a Mushroom at a workbench."},
	{ID: "lamp", Name: "Wrought Lamp", Glyph: '☼', Hex: "#FFC861", Glow: true,
		Source: Crafted, Rarity: Common,
		About: "An amber-lit lamp that casts a warm, compliant glow.",
		Found: "Crafted from a Gold Nugget and Cave Amber; powers a Lamppost."},
	{ID: "ration", Name: "Field Ration", Glyph: '⌑', Hex: "#C98B5A",
		Source: Crafted, Rarity: Common,
		About: "Cured game, provisioned for the night shift. Shelf-stable, morale-adjacent.",
		Found: "Cooked from Game Meat at a workbench."},
	{ID: "leather", Name: "Cured Leather", Glyph: '▤', Hex: "#8A5A36",
		Source: Crafted, Rarity: Common,
		About: "A supple worked hide, tanned to Durst Group spec.",
		Found: "Cured from Raw Hide at a workbench; builds a Bedroll."},
	// Hunting spoils — taken from wild animals (creature.go's drop tables).
	{ID: "meat", Name: "Game Meat", Glyph: '◖', Hex: "#C65B5B",
		Source: Hunt, Rarity: Common,
		About: "A fresh cut of wild game.",
		Found: "Caught from rabbits, deer and other animals in the Wilds."},
	{ID: "hide", Name: "Raw Hide", Glyph: '▱', Hex: "#A6764A",
		Source: Hunt, Rarity: Common,
		About: "A rough hide, ready for curing.",
		Found: "Taken from deer in the Wilds."},
	{ID: "pelt", Name: "Fox Pelt", Glyph: '▰', Hex: "#E0772E",
		Source: Hunt, Rarity: Uncommon,
		About: "A soft russet pelt.",
		Found: "Taken from foxes in the forest and hills."},
	{ID: "feather", Name: "Feather", Glyph: '✦', Hex: "#6FB7D8",
		Source: Hunt, Rarity: Common,
		About: "A light, banded flight feather.",
		Found: "Gathered from birds in the Wilds."},
	// Tool components — rare finds that unlock a tool recipe (docs/BUILD_TOOLS_PLAN.md).
	{ID: "axe_head", Name: "Flint Axe-head", Glyph: '⌐', Hex: "#C7BCA6",
		Source: Forage, Rarity: Rare,
		About: "A knapped flint head, waiting for a haft.",
		Found: "Rare in meadows and savanna at the woodland's edge.",
		Use:   "Craft it onto a Timber haft to make an Axe."},
	{ID: "pick_head", Name: "Iron Pick-head", Glyph: '⌐', Hex: "#9FB0BE",
		Source: Forage, Rarity: Rare,
		About: "A forged iron pick-head, pitted with age.",
		Found: "Rare in the hill country among the boulders.",
		Use:   "Craft it onto a Timber haft to make a Pickaxe."},
	// Tools — crafted, then owned; they enable clearing terrain (never consumed).
	{ID: "axe", Name: "Axe", Glyph: '⚒', Hex: "#B08D57",
		Source: Crafted, Rarity: Uncommon,
		About: "A hafted felling axe. Compliant with woodland policy.",
		Found: "Crafted from a Flint Axe-head and Timber.",
		Use:   "In build mode, fell a tree to clear it and yield Timber."},
	{ID: "pick", Name: "Pickaxe", Glyph: '⚒', Hex: "#AEB7BE",
		Source: Crafted, Rarity: Uncommon,
		About: "A hafted pickaxe. Breaks rock; does not break ground policy.",
		Found: "Crafted from an Iron Pick-head and Timber.",
		Use:   "In build mode, break a hill boulder to clear it and yield Cut Stone."},
	// Arms — crafted weapons (docs/WEAPON_PLAN.md). Wielded by owning them; the
	// best one in the pack lands your strike. They hunt wildlife anywhere and,
	// out in the open Wilds, can be turned on other players.
	{ID: "knife", Name: "Flint Knife", Glyph: '†', Hex: "#D9C7A3",
		Source: Crafted, Rarity: Uncommon,
		About: "A knapped flint blade on a short haft. Quick and close.",
		Found: "Crafted from Cut Stone and Timber.",
		Use:   "f — strike what you face. A keener blow than bare hands."},
	{ID: "spear", Name: "Spear", Glyph: '↑', Hex: "#C8A86A",
		Source: Crafted, Rarity: Uncommon,
		About: "A fire-hardened haft tipped with stone. Hits hard up close.",
		Found: "Crafted from Cut Stone and Timber.",
		Use:   "f — strike what you face. Heavier than a knife."},
	{ID: "bow", Name: "Hunter's Bow", Glyph: ')', Hex: "#A6753F",
		Source: Crafted, Rarity: Uncommon,
		About: "A supple bow of yew and cured leather. Strikes at range.",
		Found: "Crafted from Timber, Cured Leather and a Feather.",
		Use:   "f — loose an Arrow at what you face, several tiles off."},
	{ID: "arrow", Name: "Arrows", Glyph: '»', Hex: "#9C8D67",
		Source: Crafted, Rarity: Common,
		About: "Flint-tipped, feather-fletched shafts. Spent when loosed.",
		Found: "Crafted from Timber and a Feather.",
		Use:   "Ammunition for the Hunter's Bow — one per shot."},
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

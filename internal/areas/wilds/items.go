package wilds

import (
	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// biomeItems maps a biome to the collectibles that can be found there, so finds
// fit their surroundings: berries and mushrooms in the woods, shells on the
// beach, crystals in the snow, nuggets in the hills.
var biomeItems = map[worldgen.Biome][]string{
	worldgen.Forest:  {"berry", "mushroom"},
	worldgen.Grass:   {"herb", "berry"},
	worldgen.Savanna: {"herb", "nugget"},
	worldgen.Sand:    {"shell"},
	worldgen.Snow:    {"crystal"},
	worldgen.Hill:    {"nugget", "crystal"},
	worldgen.Swamp:   {"mushroom", "herb"},
}

// Which foraged finds double as an outfit (a mushroom → the shroom cap) now
// lives on the catalog item itself (game.Item.Wear), so the wilds and the caves
// unlock wearables the same way.

// itemRate is the share of eligible cells that carry an item — sparse, so a
// find feels earned.
const itemRate = 0.018

const (
	itemSalt  uint64 = 0x1726E_17E45_C0DE1
	itemSalt2 uint64 = 0x5A1771_C0FFEE_2B0B
	cropSalt  uint64 = 0xC0FFEE_F1E1D_5EED
	stoneSalt uint64 = 0x57014E_B10C5_2D1A
	woodSalt  uint64 = 0x000DBA_BE10C5_3E2B
	fishSalt  uint64 = 0xF15B00_47C0DE_4F3C
)

// Harvest rates: how much of each worksite is ready to gather.
const (
	cropRate  = 0.4  // ripe grain across a field
	stoneRate = 0.14 // cut stone littering the quarry floor
	woodRate  = 0.85 // a felled stump nearly always yields a log
	fishRate  = 0.5  // fish to be had off the jetty
)

// itemAt returns the item scattered at (x,y), if any: a sparse, deterministic
// roll on walkable, biome-appropriate ground (never on a portal). Like the
// terrain itself it's a pure function of the cell, so every player sees the
// same loot in the same place until they personally harvest it.
func itemAt(c worldgen.Cell, x, y int) (game.Item, bool) {
	if !c.Walkable || c.Portal != "" {
		return game.Item{}, false
	}
	if c.Glyph == '"' { // a cultivated field or garden — ripe grain to harvest
		if hash01(x, y, cropSalt) < cropRate {
			return game.ItemByID("grain")
		}
		return game.Item{}, false
	}
	// Worksite harvests, keyed off the cell's distinctive look (set in cellFor):
	switch {
	case c.Biome == worldgen.Mountain && c.Glyph == '·': // quarry floor → stone
		if hash01(x, y, stoneSalt) < stoneRate {
			return game.ItemByID("stone")
		}
		return game.Item{}, false
	case c.Biome == worldgen.Path && c.Glyph == 'u': // lumber stump → wood
		if hash01(x, y, woodSalt) < woodRate {
			return game.ItemByID("wood")
		}
		return game.Item{}, false
	case c.Glyph == '·' && c.Color == "#7A5A38": // jetty plank → fish
		if hash01(x, y, fishSalt) < fishRate {
			return game.ItemByID("fish")
		}
		return game.Item{}, false
	}
	ids, ok := biomeItems[c.Biome]
	if !ok || len(ids) == 0 {
		return game.Item{}, false
	}
	if hash01(x, y, itemSalt) >= itemRate {
		return game.Item{}, false
	}
	id := ids[int(hash01(x, y, itemSalt2)*float64(len(ids)))%len(ids)]
	return game.ItemByID(id)
}

// hatLoot is a wearable hat scattered in the world: the accessory it grants
// (by index), the biome it's themed to, and its display color.
type hatLoot struct {
	name  string
	idx   int
	biome worldgen.Biome
	hex   string
}

// hats are the wearables you can find — each in a thematic biome, so a trek
// somewhere new is rewarded with a distinctive look.
var hats = buildHats()

func buildHats() []hatLoot {
	defs := []struct {
		name  string
		biome worldgen.Biome
	}{
		{"cap", worldgen.Grass},
		{"band", worldgen.Savanna},
		{"horns", worldgen.Swamp},
		{"crown", worldgen.Hill},
		{"halo", worldgen.Snow},
		// A meadow flower as a found wearable; the woodland mushroom cap (shroom)
		// isn't dropped here — it's unlocked by foraging a mushroom item instead.
		{"flower", worldgen.Grass},
	}
	out := make([]hatLoot, 0, len(defs))
	for _, d := range defs {
		if idx, ok := game.AccessoryIndex(d.name); ok {
			// Color comes from the accessory itself, so the loot on the ground, the
			// worn avatar and the inventory icon all match.
			out = append(out, hatLoot{d.name, idx, d.biome, game.AccessoryColor(idx)})
		}
	}
	return out
}

// hatRate is how often a hat's themed biome yields it — very rare, so stumbling
// on one is a real moment rather than a regular sight.
const hatRate = 0.0008

const hatSalt uint64 = 0xA75EED_C0FFEE_42

// hatAt returns the hat lying at (x,y), if any: a rare, deterministic, themed
// drop on open ground. Hats take precedence over ordinary items.
func hatAt(c worldgen.Cell, x, y int) (hatLoot, bool) {
	if !c.Walkable || c.Portal != "" {
		return hatLoot{}, false
	}
	for _, h := range hats {
		if h.biome == c.Biome && hash01(x, y, hatSalt+uint64(h.idx)) < hatRate {
			return h, true
		}
	}
	return hatLoot{}, false
}

// hash01 is a deterministic [0,1) hash of (worldSeed, salt, x, y), independent
// of the terrain fields so item scatter doesn't correlate with biome edges.
func hash01(x, y int, salt uint64) float64 {
	h := worldSeed ^ salt
	h += 0x9E3779B97F4A7C15
	h ^= uint64(int64(x)) * 0xFF51AFD7ED558CCD
	h = (h ^ (h >> 30)) * 0xBF58476D1CE4E5B9
	h ^= uint64(int64(y)) * 0xC4CEB9FE1A85EC53
	h = (h ^ (h >> 27)) * 0x94D049BB133111EB
	h ^= h >> 31
	return float64(h>>11) / float64(uint64(1)<<53)
}

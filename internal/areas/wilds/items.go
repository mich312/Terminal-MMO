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

// itemRate is the share of eligible cells that carry an item — sparse, so a
// find feels earned.
const itemRate = 0.018

const (
	itemSalt  uint64 = 0x1726E_17E45_C0DE1
	itemSalt2 uint64 = 0x5A1771_C0FFEE_2B0B
)

// itemAt returns the item scattered at (x,y), if any: a sparse, deterministic
// roll on walkable, biome-appropriate ground (never on a portal). Like the
// terrain itself it's a pure function of the cell, so every player sees the
// same loot in the same place until they personally harvest it.
func itemAt(c worldgen.Cell, x, y int) (game.Item, bool) {
	if !c.Walkable || c.Portal != "" {
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

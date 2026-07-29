package wilds

import (
	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// biomeItems maps a biome to the collectibles that can be found there, so finds
// fit their surroundings: berries and mushrooms in the woods, shells on the
// beach, crystals in the snow, nuggets in the hills.
var biomeItems = map[worldgen.Biome][]string{
	worldgen.Forest:  {"berry", "mushroom", "dagger"},
	worldgen.Grass:   {"herb", "berry", "axe_head"},
	worldgen.Savanna: {"herb", "nugget", "axe_head", "sling"},
	worldgen.Sand:    {"shell"},
	worldgen.Snow:    {"crystal"},
	worldgen.Hill:    {"nugget", "crystal", "pick_head", "sling"},
	worldgen.Swamp:   {"mushroom", "herb", "dagger"},
}

// Which foraged finds double as an outfit (a mushroom → the shroom cap) now
// lives on the catalog item itself (game.Item.Wear), so the wilds and the caves
// unlock wearables the same way.

// itemRate is the share of eligible cells that even come up for a forage roll.
// It's deliberately low and then thinned again per-item by rarity (see
// rarityRate), so the open world reads uncluttered and an actual find feels
// earned rather than carpeting the ground.
const itemRate = 0.011

// Forage gathers in patches rather than an even statistical sprinkle: a smooth
// low-frequency field swings the roll from near-barren stretches to berry
// thickets, and the patch — not the cell — picks the species, so finding one
// mushroom means there's a ring of them here. The long empty walks between
// patches are what make stumbling into one a moment.
const (
	patchSalt   uint64 = 0x9A7CB_F00D5_EED77
	patchPeriod        = 12 // patch scale in tiles
)

const (
	itemSalt   uint64 = 0x1726E_17E45_C0DE1
	itemSalt2  uint64 = 0x5A1771_C0FFEE_2B0B
	raritySalt uint64 = 0x9A71D7_C0FFEE_E5C2
	cropSalt   uint64 = 0xC0FFEE_F1E1D_5EED
	stoneSalt  uint64 = 0x57014E_B10C5_2D1A
	woodSalt   uint64 = 0x000DBA_BE10C5_3E2B
	fishSalt   uint64 = 0xF15B00_47C0DE_4F3C
)

// Harvest rates: how much of each worksite is ready to gather. Worksites are
// small, themed footprints, so they stay denser than the open-world scatter —
// but not wall-to-wall.
const (
	cropRate  = 0.3  // ripe grain across a field
	stoneRate = 0.14 // cut stone littering the quarry floor
	woodRate  = 0.6  // most felled stumps still hold a log
	fishRate  = 0.4  // fish to be had off the jetty
)

// rarityRate thins the forage scatter by how scarce a find is: commons stay
// plentiful, while uncommons and rares are held back by a second roll. This both
// makes a rare find genuinely rare and brings the overall ground clutter down.
func rarityRate(r game.Rarity) float64 {
	switch r {
	case game.Rare:
		return 0.30
	case game.Uncommon:
		return 0.55
	default:
		return 1.0
	}
}

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
	// The patch field gates the roll (0.25×…2.15× the base rate; the mean sits
	// near the old flat rate, so overall density holds) and picks the species
	// for the whole patch.
	patch := patch01(x, y, patchPeriod, patchSalt)
	if hash01(x, y, itemSalt) >= itemRate*(0.25+1.9*patch) {
		return game.Item{}, false
	}
	sp := hash01(floorDiv(x, patchPeriod), floorDiv(y, patchPeriod), itemSalt2)
	id := ids[int(sp*float64(len(ids)))%len(ids)]
	it, ok := game.ItemByID(id)
	if !ok {
		return game.Item{}, false
	}
	// A second, independent roll thins the scatter by rarity, so a rare find
	// doesn't appear as often as a common one and the ground stays uncluttered.
	if hash01(x, y, raritySalt) >= rarityRate(it.Rarity) {
		return game.Item{}, false
	}
	return it, true
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

// Hats are placed on a coarse, jittered grid rather than rolled independently
// per cell, so two of the same hat can never cluster: each grid region holds at
// most one candidate spot for a given hat, kept clear of the region edges so the
// nearest same-hat candidate in a neighbouring region is always far away. A
// candidate only becomes a real find if it happens to land on matching, open
// ground — which keeps hats rare and themed to their biome.
const (
	hatGrid     = 44  // a region (in tiles) that holds at most one of each hat type
	hatMargin   = 14  // keep candidates this far inside a region → min same-hat spacing ≥ 2*margin
	hatHostFrac = 0.7 // share of regions that host a given hat's candidate
)

const hatSalt uint64 = 0xA75EED_C0FFEE_42

// floorDiv is integer division that floors toward negative infinity, so grid
// regions stay contiguous across the world origin (Go's / truncates toward zero).
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// hatCandidate returns the single cell within (x,y)'s grid region where a hat of
// type idx may sit, and whether the region hosts it at all. The position is
// jittered but pinned away from the region edges, guaranteeing same-type hats in
// adjacent regions stay at least 2*hatMargin tiles apart.
func hatCandidate(idx, x, y int) (cx, cy int, ok bool) {
	gx, gy := floorDiv(x, hatGrid), floorDiv(y, hatGrid)
	salt := hatSalt + uint64(idx)
	if hash01(gx, gy, salt) >= hatHostFrac {
		return 0, 0, false
	}
	span := hatGrid - 2*hatMargin
	ox := hatMargin + int(hash01(gx, gy, salt+0x9E37)*float64(span))
	oy := hatMargin + int(hash01(gx, gy, salt+0x1234)*float64(span))
	return gx*hatGrid + ox, gy*hatGrid + oy, true
}

// hatAt returns the hat lying at (x,y), if any: a rare, deterministic, themed
// find on open ground, placed so two of a kind never bunch up. Hats take
// precedence over ordinary items.
func hatAt(c worldgen.Cell, x, y int) (hatLoot, bool) {
	if !c.Walkable || c.Portal != "" {
		return hatLoot{}, false
	}
	for _, h := range hats {
		if h.biome != c.Biome {
			continue
		}
		if cx, cy, ok := hatCandidate(h.idx, x, y); ok && cx == x && cy == y {
			return h, true
		}
	}
	return hatLoot{}, false
}

// patch01 is smooth [0,1) noise over a coarse lattice: hash01 at the four
// surrounding lattice corners, bilinearly interpolated with a smoothstep fade.
// The cheap way to a clustered field without reaching into the terrain
// generator (and so, like hash01, independent of biome edges).
func patch01(x, y, period int, salt uint64) float64 {
	gx, gy := floorDiv(x, period), floorDiv(y, period)
	fade := func(t float64) float64 { return t * t * (3 - 2*t) }
	tx := fade(float64(x-gx*period) / float64(period))
	ty := fade(float64(y-gy*period) / float64(period))
	v00 := hash01(gx, gy, salt)
	v10 := hash01(gx+1, gy, salt)
	v01 := hash01(gx, gy+1, salt)
	v11 := hash01(gx+1, gy+1, salt)
	a := v00 + tx*(v10-v00)
	b := v01 + tx*(v11-v01)
	return a + ty*(b-a)
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

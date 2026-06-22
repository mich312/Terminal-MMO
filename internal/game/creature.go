package game

import (
	"math/rand"

	"github.com/durst-group/durstworld/internal/worldgen"
)

// Drop is one possible spoil from hunting a creature: a range of an inventory
// item, with an optional chance it occurs at all (0 means it always does).
type Drop struct {
	Item   string  // inventory item id (inventory.go)
	Min    int     // minimum count when it drops
	Max    int     // maximum count (== Min for a fixed amount)
	Chance float64 // probability 0..1 the drop occurs (0 treated as certain)
}

// Species is the static definition of one kind of wildlife: how it looks (the
// glyph for the text client, the prop sprite + hue for HD) and how it behaves
// (where it lives, how easily it spooks, how often it moves). It is the single
// source of truth both the renderer and the wildlife simulation read, so a
// creature's appearance and its rules never drift apart.
//
// This lives in game (beside Placeables and the tileset) because it is mostly
// presentation; the simulation in internal/wildlife consumes it for behavior.
type Species struct {
	Kind  string // stable id, matches world.Creature.Kind
	Name  string // display name, for observe/compendium text
	Glyph rune   // text client token
	Hex   string // species hue (drives both the glyph color and the HD sprite)
	Prop  TileProp

	Biomes     []worldgen.Biome // where it may spawn
	Aquatic    bool             // lives on water (Water/Deep) rather than walkable land
	FleeRadius int              // bolts when a player comes this close (0 = placid)
	MoveEvery  int              // steps once every N ticks (higher = calmer)
	MaxHP      int              // strikes to catch it (hunting)
	Drops      []Drop           // spoils rolled when it is caught
	Tameable   bool             // can be befriended into a follower (taming)
	Bait       string           // inventory item id that tames it
}

// speciesList is the MVP roster: a couple of common, biome-appropriate animals,
// all ambient and observable. Hunting/taming flags arrive in later phases.
var speciesList = []Species{
	{Kind: "rabbit", Name: "rabbit", Glyph: 'r', Hex: "#C9B79C", Prop: PropRabbit,
		Biomes: []worldgen.Biome{worldgen.Grass, worldgen.Savanna}, FleeRadius: 5, MoveEvery: 2, MaxHP: 2,
		Drops: []Drop{{Item: "meat", Min: 1, Max: 1}}, Tameable: true, Bait: "berry"},
	{Kind: "deer", Name: "deer", Glyph: 'd', Hex: "#A6703C", Prop: PropDeer,
		Biomes: []worldgen.Biome{worldgen.Forest, worldgen.Grass}, FleeRadius: 4, MoveEvery: 3, MaxHP: 6,
		Drops: []Drop{{Item: "hide", Min: 1, Max: 1}, {Item: "meat", Min: 1, Max: 2}}, Tameable: true, Bait: "grain"},
	{Kind: "fox", Name: "fox", Glyph: 'f', Hex: "#E0772E", Prop: PropFox,
		Biomes: []worldgen.Biome{worldgen.Forest, worldgen.Hill}, FleeRadius: 3, MoveEvery: 2, MaxHP: 4,
		Drops: []Drop{{Item: "pelt", Min: 1, Max: 1}, {Item: "meat", Min: 1, Max: 1, Chance: 0.5}}, Tameable: true, Bait: "meat"},
	{Kind: "bird", Name: "bird", Glyph: 'v', Hex: "#6FB7D8", Prop: PropBird,
		Biomes: []worldgen.Biome{worldgen.Grass, worldgen.Forest, worldgen.Savanna, worldgen.Hill}, FleeRadius: 6, MoveEvery: 1, MaxHP: 1,
		Drops: []Drop{{Item: "feather", Min: 1, Max: 2}}},
	{Kind: "fish", Name: "fish", Glyph: '~', Hex: "#8FD0C0", Prop: PropFishWild,
		Biomes: []worldgen.Biome{worldgen.Water}, Aquatic: true, FleeRadius: 2, MoveEvery: 2, MaxHP: 2,
		Drops: []Drop{{Item: "fish", Min: 1, Max: 1}}},
}

// RollDrops rolls a creature's spoils into item-id → count, using r for any
// random ranges/chances so callers (and tests) control the stream.
func RollDrops(sp Species, r *rand.Rand) map[string]int {
	out := map[string]int{}
	for _, d := range sp.Drops {
		if d.Chance > 0 && d.Chance < 1 && r.Float64() > d.Chance {
			continue
		}
		n := d.Min
		if d.Max > d.Min {
			n += r.Intn(d.Max - d.Min + 1)
		}
		if n > 0 {
			out[d.Item] += n
		}
	}
	return out
}

var speciesByKind = func() map[string]Species {
	m := make(map[string]Species, len(speciesList))
	for _, s := range speciesList {
		m[s.Kind] = s
	}
	return m
}()

// SpeciesList returns every wildlife species (the simulation iterates it to
// decide what may spawn in a biome).
func SpeciesList() []Species { return speciesList }

// SpeciesByKind resolves a creature's Kind to its Species; ok is false for an
// unknown id (so the renderer can fall back gracefully).
func SpeciesByKind(kind string) (Species, bool) {
	s, ok := speciesByKind[kind]
	return s, ok
}

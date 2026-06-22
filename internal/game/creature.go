package game

import "github.com/durst-group/durstworld/internal/worldgen"

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
	MaxHP      int              // reserved for hunting (Phase 2); unused at MVP
}

// speciesList is the MVP roster: a couple of common, biome-appropriate animals,
// all ambient and observable. Hunting/taming flags arrive in later phases.
var speciesList = []Species{
	{Kind: "rabbit", Name: "rabbit", Glyph: 'r', Hex: "#C9B79C", Prop: PropRabbit,
		Biomes: []worldgen.Biome{worldgen.Grass, worldgen.Savanna}, FleeRadius: 5, MoveEvery: 2, MaxHP: 2},
	{Kind: "deer", Name: "deer", Glyph: 'd', Hex: "#A6703C", Prop: PropDeer,
		Biomes: []worldgen.Biome{worldgen.Forest, worldgen.Grass}, FleeRadius: 4, MoveEvery: 3, MaxHP: 6},
	{Kind: "fox", Name: "fox", Glyph: 'f', Hex: "#E0772E", Prop: PropFox,
		Biomes: []worldgen.Biome{worldgen.Forest, worldgen.Hill}, FleeRadius: 3, MoveEvery: 2, MaxHP: 4},
	{Kind: "bird", Name: "bird", Glyph: 'v', Hex: "#6FB7D8", Prop: PropBird,
		Biomes: []worldgen.Biome{worldgen.Grass, worldgen.Forest, worldgen.Savanna, worldgen.Hill}, FleeRadius: 6, MoveEvery: 1, MaxHP: 1},
	{Kind: "fish", Name: "fish", Glyph: '~', Hex: "#8FD0C0", Prop: PropFishWild,
		Biomes: []worldgen.Biome{worldgen.Water}, Aquatic: true, FleeRadius: 2, MoveEvery: 2, MaxHP: 2},
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

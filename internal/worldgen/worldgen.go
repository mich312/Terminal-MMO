// Package worldgen is the deterministic terrain generator behind the Wilds —
// Durst World's infinite, chunkless overworld. Every cell is a pure function
// of (seed, x, y): there is no stored map, so the world is effectively
// infinite and — crucially for multiplayer — identical for every session
// running the same seed. Areas sample a window of cells around the player and
// hand them to the normal tile renderer.
package worldgen

import "math"

// Biome is the coarse terrain class a cell belongs to.
type Biome int

const (
	Deep Biome = iota
	Water
	Sand
	Grass
	Forest
	Hill
	Mountain
)

// Cell is one generated tile: how it looks, whether it blocks movement, and
// (for the home gate) where it leads. Color is a base hex; AnimA/AnimB and
// Frames, when set, make the cell shimmer/flow through the tile renderer.
type Cell struct {
	Biome    Biome
	Glyph    rune
	Color    string
	AnimA    string
	AnimB    string
	Frames   []rune
	Walkable bool
	Object   bool   // drawn bold (the gate)
	Portal   string // non-empty → a portal to this area id
}

// Generator produces cells for one seed.
type Generator struct {
	seed uint64
}

// New returns a generator for the given seed.
func New(seed uint64) *Generator { return &Generator{seed: seed} }

// Seed reports the generator's seed (for status display / regeneration).
func (g *Generator) Seed() uint64 { return g.seed }

// GateX, GateY is the home gate (Durst HQ) at the world origin. The spawn
// sits in its forced clearing.
const (
	GateX = 0
	GateY = 0
)

// Landmark is a fixed portal structure in the overworld — the hub's doors to
// the hand-built areas. A grassy clearing of radius Clear is forced around
// each so a multi-tile body can always reach it.
type Landmark struct {
	X, Y   int
	Portal string
	Name   string
	Glyph  rune
	Color  string
	Clear  int
}

// Landmarks are placed near the origin so spawning in the Wilds drops you in a
// small plaza with doors to everywhere.
var Landmarks = []Landmark{
	{0, 0, "lobby", "Durst HQ", '⌂', "#7DF0FF", 4},
	{16, 0, "presentation", "Presentation Wing", 'P', "#FFC861", 3},
	{-16, 0, "kraftwerk", "Kraftwerk", 'K', "#56E1FF", 3},
	{0, 12, "democenter", "Demo Center", 'D', "#C792EA", 3},
}

// At returns the cell at world coordinate (x,y). Deterministic and infinite.
func (g *Generator) At(x, y int) Cell {
	for _, lm := range Landmarks {
		if x == lm.X && y == lm.Y {
			return Cell{Biome: Grass, Glyph: lm.Glyph, Color: lm.Color,
				Walkable: true, Object: true, Portal: lm.Portal}
		}
	}
	// Forced grassy clearings around each landmark keep the plaza walkable.
	for _, lm := range Landmarks {
		if abs(x-lm.X) <= lm.Clear && abs(y-lm.Y) <= lm.Clear {
			return grassCell(g, x, y)
		}
	}

	elev := g.fbm(x, y, 0x1, 0.045, 4)
	moist := g.fbm(x, y, 0x9E37, 0.03, 3)

	switch {
	case elev < 0.26:
		return Cell{Biome: Deep, Glyph: '≈', Color: "#1E3A8A",
			AnimA: "#163073", AnimB: "#2E6BFF", Frames: []rune{'≈', '~', '≈', '≋'}}
	case elev < 0.34:
		return Cell{Biome: Water, Glyph: '~', Color: "#2E6BFF",
			AnimA: "#2E6BFF", AnimB: "#56E1FF", Frames: []rune{'~', '≈', '~', '≋'}}
	case elev < 0.38:
		return Cell{Biome: Sand, Glyph: '·', Color: "#E6D6A0", Walkable: true}
	case elev < 0.70:
		if moist > 0.52 {
			return forestCell(g, x, y)
		}
		return grassCell(g, x, y)
	case elev < 0.84:
		return hillCell(g, x, y)
	default:
		return Cell{Biome: Mountain, Glyph: '▲', Color: "#9AA0A8"} // peaks block
	}
}

// Walkable is a convenience over At for collision checks.
func (g *Generator) Walkable(x, y int) bool { return g.At(x, y).Walkable }

func grassCell(g *Generator, x, y int) Cell {
	c := Cell{Biome: Grass, Glyph: '·', Color: "#5FA86B", Walkable: true}
	switch r := g.prop(x, y); {
	case r < 0.04:
		c.Glyph, c.Color = '*', "#FF6B6B" // flower
	case r < 0.08:
		c.Glyph, c.Color = '*', "#FFC861" // flower
	case r < 0.16:
		c.Glyph, c.Color = ',', "#4F9460" // tuft
	}
	return c
}

func forestCell(g *Generator, x, y int) Cell {
	r := g.prop(x, y)
	if r < 0.45 { // a tree — blocks movement
		col := "#2F7D4F"
		if r < 0.18 {
			col = "#276B43"
		}
		return Cell{Biome: Forest, Glyph: '♣', Color: col}
	}
	return Cell{Biome: Forest, Glyph: '·', Color: "#3F8A5A", Walkable: true}
}

func hillCell(g *Generator, x, y int) Cell {
	r := g.prop(x, y)
	if r < 0.12 { // a boulder — blocks movement
		return Cell{Biome: Hill, Glyph: '▲', Color: "#8A8170"}
	}
	return Cell{Biome: Hill, Glyph: '∩', Color: "#9C8D67", Walkable: true}
}

// ── noise ──────────────────────────────────────────────────────────────────

// fbm sums octaves of value noise into [0,1]. freq is the base lattice
// frequency; salt separates independent fields (elevation vs moisture).
func (g *Generator) fbm(x, y int, salt uint64, freq float64, octaves int) float64 {
	var sum, norm float64
	a := 1.0
	f := freq
	for i := 0; i < octaves; i++ {
		sum += a * g.valueNoise(float64(x)*f, float64(y)*f, salt+uint64(i))
		norm += a
		a *= 0.5
		f *= 2
	}
	return sum / norm
}

// valueNoise bilinearly interpolates hashed lattice values with a smoothstep
// fade, giving smooth [0,1) noise.
func (g *Generator) valueNoise(x, y float64, salt uint64) float64 {
	x0 := math.Floor(x)
	y0 := math.Floor(y)
	tx := fade(x - x0)
	ty := fade(y - y0)
	ix, iy := int(x0), int(y0)

	v00 := g.lattice(ix, iy, salt)
	v10 := g.lattice(ix+1, iy, salt)
	v01 := g.lattice(ix, iy+1, salt)
	v11 := g.lattice(ix+1, iy+1, salt)

	a := v00 + tx*(v10-v00)
	b := v01 + tx*(v11-v01)
	return a + ty*(b-a)
}

func (g *Generator) lattice(ix, iy int, salt uint64) float64 {
	return unit(hashCoord(g.seed^salt, ix, iy))
}

// propSalt separates the prop-scatter field from the elevation/moisture noise.
const propSalt uint64 = 0x5CA77E12B10550DE

// prop is a per-cell hash in [0,1) used to scatter trees, rocks and flowers
// deterministically.
func (g *Generator) prop(x, y int) float64 {
	return unit(hashCoord(g.seed^propSalt, x, y))
}

func fade(t float64) float64 { return t * t * (3 - 2*t) }

// hashCoord is a fast integer hash of (seed, x, y) → uint64 (SplitMix-style).
func hashCoord(seed uint64, ix, iy int) uint64 {
	h := seed + 0x9E3779B97F4A7C15
	h ^= uint64(int64(ix)) * 0xFF51AFD7ED558CCD
	h = (h ^ (h >> 30)) * 0xBF58476D1CE4E5B9
	h ^= uint64(int64(iy)) * 0xC4CEB9FE1A85EC53
	h = (h ^ (h >> 27)) * 0x94D049BB133111EB
	return h ^ (h >> 31)
}

func unit(h uint64) float64 { return float64(h>>11) / float64(uint64(1)<<53) }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

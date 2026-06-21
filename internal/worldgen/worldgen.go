// Package worldgen is the deterministic terrain generator behind the Wilds —
// Durst World's infinite, chunkless overworld. Every cell is a pure function
// of (seed, x, y): there is no stored map, so the world is effectively
// infinite and — crucially for multiplayer — identical for every session
// running the same seed. Areas sample a window of cells around the player and
// hand them to the normal tile renderer.
package worldgen

import (
	"math"
	"sync"
)

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
	Path
	Snow
	Savanna
	Swamp
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
	Variant  uint8  // sub-type selector (e.g. fence orientation, building type)
}

// Generator produces cells for one seed. The caches memoize settlement metadata
// and generated village layouts per macro-cell (villages are sparse, so few are
// ever held); both are sync.Maps so a Generator stays safe to share.
type Generator struct {
	seed          uint64
	settleCache   sync.Map // macro-cell key → settlement
	layoutCache   sync.Map // macro-cell key → *layout
	partnerCache  sync.Map // macro-cell key → partnerResult
	partnersCache sync.Map // macro-cell key → []settlement (road network)
	caveCache     sync.Map // macro-cell key → caveSystem
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

// Gates are sealed portals out past the hub — optional, riddle/offering-gated
// doors to hidden areas. worldgen only fixes their position, clearing and a
// dormant marker; the Wilds decides (from world/player state) whether each is
// sealed or open, and what repairs it. Portal holds the destination area id.
var Gates = []Landmark{
	{22, 0, "grove", "Whispering Gate", '◈', "#C792EA", 2},
	{0, 18, "vault", "Sunken Gate", '◈', "#56E1FF", 2},
}

// At returns the cell at world coordinate (x,y). Deterministic and infinite.
func (g *Generator) At(x, y int) Cell {
	for _, lm := range Landmarks {
		if x == lm.X && y == lm.Y {
			return Cell{Biome: Grass, Glyph: lm.Glyph, Color: lm.Color,
				Walkable: true, Object: true, Portal: lm.Portal}
		}
	}
	// A sealed gate renders as a dormant marker on walkable ground; the Wilds
	// turns it into an open portal (or keeps it sealed) from live state.
	for _, gt := range Gates {
		if x == gt.X && y == gt.Y {
			return Cell{Biome: Grass, Glyph: gt.Glyph, Color: gt.Color, Walkable: true, Object: true}
		}
	}
	// The hub town: the redesigned spawn. A cobbled square ringed by the wing
	// buildings (whose doors are the portals above), market stalls and street
	// lamps, wired to the gates by cobbled streets — replacing the old four
	// grassy clearings and dirt trails. The forced street arms keep every door
	// reachable regardless of seed, exactly as the old trails did.
	if c, ok := g.hubCell(x, y); ok {
		return c
	}

	// Settlements (villages, hamlets, walled towns) are derived deterministically
	// from a macro-grid — placed after the hub carve-outs above so they never
	// intrude on the spawn plaza, the connecting trails or the portal gates.
	if c, ok := g.settlementAt(x, y); ok {
		return c
	}

	// Cave mouths: 1–3 linked openings per cave system, set in the high hills.
	// Placed after settlements so a town always wins the cell.
	if c, ok := g.caveMouthCell(x, y); ok {
		return c
	}

	elev, moist, temp, region := g.climate(x, y)

	switch {
	case elev < 0.24: // deep water
		return Cell{Biome: Deep, Glyph: '≈', Color: "#163A6B",
			AnimA: "#12305C", AnimB: "#22518F", Frames: []rune{'≈', '~', '≈', '≋'}}
	case elev < 0.30: // open water
		return Cell{Biome: Water, Glyph: '~', Color: "#2E6BD0",
			AnimA: "#2E6BD0", AnimB: "#3F9AE0", Frames: []rune{'~', '≈', '~', '≋'}}
	case elev < 0.34: // shallows
		return Cell{Biome: Water, Glyph: '~', Color: "#5BB0E0",
			AnimA: "#5BB0E0", AnimB: "#86D2EE", Frames: []rune{'~', '≈', '~', '≋'}}
	case elev < 0.38: // beach
		if g.prop(x, y) < 0.02 { // the occasional palm — blocks movement
			return Cell{Biome: Sand, Glyph: 'Ψ', Color: "#3E8E5A"}
		}
		return Cell{Biome: Sand, Glyph: '·', Color: "#E6D6A0", Walkable: true}
	case elev < 0.70: // lowland — climate decides the cover
		switch {
		case elev < 0.46 && moist > 0.62 && temp > 0.45:
			return swampCell(g, x, y) // warm, wet, low: wetlands by the water
		case moist > 0.52:
			return forestCell(g, x, y)
		case temp > 0.60 && moist < 0.44:
			return savannaCell(g, x, y)
		default:
			return grassCell(g, x, y)
		}
	case elev < 0.84: // highland — snow only in genuinely cold regions, else hills
		if temp < 0.40 && region < 0.42 {
			return snowCell(g, x, y)
		}
		return hillCell(g, x, y)
	default: // peaks — snow-capped except in warm regions, where they're bare rock
		if temp < 0.52 && region < 0.55 {
			return Cell{Biome: Snow, Glyph: '▲', Color: "#EAF0F7"} // snowy peak (blocks)
		}
		return Cell{Biome: Mountain, Glyph: '▲', Color: "#9AA0A8"} // bare peak (blocks)
	}
}

// Walkable is a convenience over At for collision checks.
func (g *Generator) Walkable(x, y int) bool { return g.At(x, y).Walkable }

// climate samples the three terrain fields at (x,y): elevation, moisture and
// temperature. Domain warping offsets the sample point by a low-frequency noise
// field first, so biome edges meander organically (wavy coastlines,
// interlocking forests) instead of forming smooth blobs. temp folds in
// elevation, so highlands and peaks run colder.
// region is the large-scale temperature field (low frequency = broad warm/cold
// regions); temp folds in elevation on top of it. Snow keys off region as well
// as temp, so a lone high knoll in a warm meadow stays a rocky hill instead of
// sprouting a snowcap.
func (g *Generator) climate(x, y int) (elev, moist, temp, region float64) {
	const warp = 18.0
	wx := float64(x) + warp*(g.fbmAt(float64(x), float64(y), 0x1233A, 0.02, 2)-0.5)
	wy := float64(y) + warp*(g.fbmAt(float64(x), float64(y), 0x77C2B, 0.02, 2)-0.5)
	elev = g.fbmAt(wx, wy, 0x1, 0.045, 4)
	moist = g.fbmAt(wx, wy, 0x9E37, 0.03, 3)
	region = g.fbmAt(wx, wy, 0x7E11, 0.014, 3)
	temp = region - 0.30*(elev-0.5)
	return elev, moist, temp, region
}

// biomeAt returns the coarse biome class at (x,y), ignoring props, landmarks and
// settlements — the bare terrain a settlement is built on, so its cleared ground
// keeps the surrounding look (grassy meadow, dry savanna, …).
func (g *Generator) biomeAt(x, y int) Biome {
	elev, moist, temp, region := g.climate(x, y)
	switch {
	case elev < 0.30:
		return Water
	case elev < 0.38:
		return Sand
	case elev < 0.70:
		switch {
		case elev < 0.46 && moist > 0.62 && temp > 0.45:
			return Swamp
		case moist > 0.52:
			return Forest
		case temp > 0.60 && moist < 0.44:
			return Savanna
		default:
			return Grass
		}
	case elev < 0.84:
		if temp < 0.40 && region < 0.42 {
			return Snow
		}
		return Hill
	default:
		return Mountain
	}
}

func grassCell(g *Generator, x, y int) Cell {
	c := Cell{Biome: Grass, Glyph: '·', Color: "#5EAE63", Walkable: true}
	switch r := g.prop(x, y); {
	case r < 0.0016: // a traveler's campfire — blocks, glows warm at night
		c.Glyph, c.Color, c.Walkable = 'Λ', "#FF7A1E", false
	case r < 0.0026: // a remote lone cabin — blocks movement (most houses cluster in villages)
		c.Glyph, c.Color, c.Walkable = 'H', houseColor(g.prop2(x, y)), false
	case r < 0.056:
		c.Glyph, c.Color = '*', flowerColor(g.prop2(x, y)) // flower (varied)
	case r < 0.086:
		c.Glyph, c.Color = 'o', "#3E8F57" // bush
	case r < 0.106:
		c.Glyph, c.Color = '°', "#9AA0A8" // small rock
	case r < 0.206:
		c.Glyph, c.Color = ',', "#4F9460" // tuft
	}
	return c
}

// forestSalt separates the tree-clustering field from the other noise.
const forestSalt uint64 = 0x0F0235713EE50000

func forestCell(g *Generator, x, y int) Cell {
	// Cluster trees into stands: a low-frequency density field thickens the
	// canopy in the heart of a wood and thins it toward the edges, so forests
	// read as groves with clearings rather than an even scatter.
	density := g.fbmAt(float64(x), float64(y), forestSalt, 0.07, 2)
	tree := 0.16 + 0.46*density // ~16% at edges … ~62% in dense cores
	switch r := g.prop(x, y); {
	case r < tree: // a tree — blocks movement (color varies; some autumn)
		return Cell{Biome: Forest, Glyph: '♣', Color: treeColor(g.prop2(x, y))}
	case r < tree+0.07: // a stump
		return Cell{Biome: Forest, Glyph: 'u', Color: "#6B4A2B", Walkable: true}
	case r < tree+0.22: // undergrowth bush
		return Cell{Biome: Forest, Glyph: 'o', Color: "#2F7D4F", Walkable: true}
	}
	return Cell{Biome: Forest, Glyph: '·', Color: "#2E6B40", Walkable: true}
}

// snowCell is cold high ground: pale, mostly open, with the odd ice-glazed
// rock. Nothing here blocks — snowfields stay crossable.
func snowCell(g *Generator, x, y int) Cell {
	c := Cell{Biome: Snow, Glyph: '·', Color: "#E8EEF5", Walkable: true}
	switch r := g.prop(x, y); {
	case r < 0.07: // a snow-tipped fir — blocks movement
		return Cell{Biome: Snow, Glyph: '♠', Color: "#2E5E43"}
	case r < 0.12:
		c.Glyph, c.Color = '°', "#C2CCD6" // an icy rock
	case r < 0.20:
		c.Glyph, c.Color = ',', "#D4DEEA" // a snow tuft / drift
	}
	return c
}

// savannaCell is warm, dry grassland: golden tufts and the occasional scrubby
// bush. Kept free of blocking props so the dry plains stay open.
func savannaCell(g *Generator, x, y int) Cell {
	c := Cell{Biome: Savanna, Glyph: '·', Color: "#CDBA5C", Walkable: true}
	switch r := g.prop(x, y); {
	case r < 0.013: // a lone acacia — blocks movement, a savanna landmark
		return Cell{Biome: Savanna, Glyph: 'ϒ', Color: "#7C9442"}
	case r < 0.06:
		c.Glyph, c.Color = 'o', "#7E8F3C" // a dry scrub bush
	case r < 0.23:
		c.Glyph, c.Color = ',', "#C9B85F" // a clump of dry grass
	}
	return c
}

// swampCell is warm, low, waterlogged ground: murky green flats with reeds,
// hummocks and the odd shallow pool. Stays walkable — boggy, not impassable.
func swampCell(g *Generator, x, y int) Cell {
	c := Cell{Biome: Swamp, Glyph: '·', Color: "#45533C", Walkable: true}
	switch r := g.prop(x, y); {
	case r < 0.12:
		c.Glyph, c.Color = '‖', "#7C8A45" // a clump of cattail reeds
	case r < 0.18:
		c.Glyph, c.Color = 'o', "#3A5A3A" // a mossy hummock
	case r < 0.24:
		c.Glyph, c.Color = '~', "#3E5E55" // a stagnant pool
	}
	return c
}

// onPath reports whether (x,y) lies on a forced trail: a 3-wide walkable band
// along the axes between the origin (Durst HQ) and each landmark, so the spawn
// plaza is always connected to every wing's door regardless of seed.
func onPath(x, y int) bool {
	if abs(y) <= 1 && x >= -16 && x <= 22 { // HQ ↔ Kraftwerk / Presentation / Whispering Gate
		return true
	}
	if abs(x) <= 1 && y >= 0 && y <= 18 { // HQ ↔ Demo Center / Sunken Gate
		return true
	}
	return false
}

func hillCell(g *Generator, x, y int) Cell {
	switch r := g.prop(x, y); {
	case r < 0.04: // a jagged crag — blocks movement, a highland landmark
		return Cell{Biome: Hill, Glyph: 'Δ', Color: "#8C8475"}
	case r < 0.12: // a boulder — blocks movement
		return Cell{Biome: Hill, Glyph: '▲', Color: "#8A8170"}
	case r < 0.20: // a small rock
		return Cell{Biome: Hill, Glyph: '°', Color: "#9AA0A8", Walkable: true}
	}
	return Cell{Biome: Hill, Glyph: '∩', Color: "#9C8D67", Walkable: true}
}

// flowerColor and treeColor add deterministic variety from a second hash field.
func flowerColor(r float64) string {
	cols := []string{"#FF6B6B", "#FFC861", "#FF8FB1", "#F2F2F2", "#C792EA", "#A0C7FF"}
	return cols[int(r*float64(len(cols)))%len(cols)]
}

func houseColor(r float64) string {
	cols := []string{"#C9A66B", "#B5835A", "#D9C089", "#A88B6A"}
	return cols[int(r*float64(len(cols)))%len(cols)]
}

func treeColor(r float64) string {
	switch {
	case r < 0.25:
		return "#276B43" // deep green
	case r < 0.87:
		return "#2F7D4F" // green
	case r < 0.93:
		return "#C99A3A" // gold (autumn)
	case r < 0.97:
		return "#C2602F" // orange
	default:
		return "#B5482C" // red
	}
}

// ── noise ──────────────────────────────────────────────────────────────────

// fbmAt sums octaves of value noise into [0,1] at a float coordinate (so the
// domain-warped sample point can be fractional). freq is the base lattice
// frequency; salt separates independent fields (elevation, moisture, temp).
func (g *Generator) fbmAt(x, y float64, salt uint64, freq float64, octaves int) float64 {
	var sum, norm float64
	a := 1.0
	f := freq
	for i := 0; i < octaves; i++ {
		sum += a * g.valueNoise(x*f, y*f, salt+uint64(i))
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

// prop2 is a second independent per-cell hash, for sub-variety (which flower
// color, whether a tree is autumn) without correlating with prop's scatter.
const prop2Salt uint64 = 0xA17E55EDB10DEC0F

func (g *Generator) prop2(x, y int) float64 {
	return unit(hashCoord(g.seed^prop2Salt, x, y))
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

package worldgen

import "math"

// Settlements — villages and hamlets scattered across the Wilds. Like
// everything else in worldgen they are a pure function of (seed, x, y): no
// settlement is ever "placed" or stored. Instead the world is partitioned into
// a coarse macro-grid; each macro-cell is hashed to decide whether it hosts a
// settlement and, if so, its centre, size and organic outline. A given cell
// then asks the (up to nine) nearby settlements whether it falls inside one and
// what it should be — a house, the well, a road, a fence or a field furrow.
//
// The outline is deliberately NOT a rectangle. A settlement's edge is a wobbly
// radial curve R(θ) = R0·(1 + Σ aₖ·sin(kθ+φₖ)): a lopsided "incorrectly shaped
// circle" that reads as something grown rather than drawn. Because R is a
// function of the angle alone, a cell decides wall/inside/outside in O(1) from
// its angle and distance to the centre — no boundary tracing, no flood fill.

const (
	settleSalt    uint64 = 0x5E771E3D0FAB12C7
	settleCell           = 64 // macro-grid cell size, in world tiles
	settleHubKeep        = 56 // keep settlement centres this far from the origin hub
	settleMaxReach       = 18 // a settlement's outline never extends past this from its centre
	plotSpan             = 3  // house-plot spacing inside a settlement
)

// settlement is one deterministically-derived village, computed from its
// macro-cell coordinate. The zero value (valid == false) means "no settlement
// here".
type settlement struct {
	id        uint64     // identity hash — salts all sub-placement noise
	cx, cy    int        // centre, in world coordinates
	r0        float64    // base outline radius
	amp       [4]float64 // organic-outline harmonic amplitudes (fraction of r0)
	phase     [4]float64 // organic-outline harmonic phases
	spokes    int        // number of radial roads (which also breach the fence)
	spoke0    float64    // angle of the first road
	fieldAng  float64    // centre angle of the farm wedge (villages only)
	hasFence  bool       // ringed by a fence (villages, not hamlets)
	hasFields bool       // has a farm wedge (villages, not hamlets)
	valid     bool
}

// settlementFor derives the settlement (if any) hosted by macro-cell (mx,my).
// It is cheap to call and short-circuits before the buildability sample for the
// ~half of macro-cells that hold nothing.
func (g *Generator) settlementFor(mx, my int) settlement {
	var s settlement
	h := hashCoord(g.seed^settleSalt, mx, my)
	if unit(h) > 0.5 { // only ~half of macro-cells host a settlement at all
		return s
	}
	// Centre, jittered well inside the macro-cell so the outline fits without
	// crossing into a neighbour's grid (keeps the 3×3 neighbour scan complete).
	hx := hashCoord(g.seed^settleSalt^0x1111, mx, my)
	hy := hashCoord(g.seed^settleSalt^0x2222, mx, my)
	s.cx = mx*settleCell + settleCell/4 + int(unit(hx)*float64(settleCell)/2)
	s.cy = my*settleCell + settleCell/4 + int(unit(hy)*float64(settleCell)/2)
	if abs(s.cx) < settleHubKeep && abs(s.cy) < settleHubKeep {
		return s // keep the spawn hub clear
	}
	// Buildability: only settle on temperate lowland — not water, beach, swamp
	// flats or cold/steep highland. A cheap, un-warped elevation/moisture probe
	// at the centre is enough to gate placement (the per-cell clip below handles
	// the edges meeting water).
	elev := g.fbmAt(float64(s.cx), float64(s.cy), 0x1, 0.045, 3)
	moist := g.fbmAt(float64(s.cx), float64(s.cy), 0x9E37, 0.03, 3)
	if elev < 0.40 || elev > 0.64 || moist > 0.58 {
		return s
	}
	s.id = h
	// Tier: roughly half hamlets (small, open), half villages (larger, fenced,
	// with a farm wedge).
	village := unit(hashCoord(s.id, 0xA, 0xB)) < 0.5
	if village {
		s.r0 = 9 + unit(hashCoord(s.id, 0xC, 0xD))*4 // 9..13
		s.hasFence, s.hasFields = true, true
	} else {
		s.r0 = 6 + unit(hashCoord(s.id, 0xC, 0xD))*3 // 6..9
	}
	// Organic outline: a few low harmonics, higher ones progressively weaker so
	// the curve stays a simple closed loop (Σ amp kept well under 1).
	for k := 0; k < 4; k++ {
		s.amp[k] = unit(hashCoord(s.id, 0x40+k, 0)) * 0.16 / float64(k+1)
		s.phase[k] = unit(hashCoord(s.id, 0x50+k, 0)) * 2 * math.Pi
	}
	s.spokes = 2 + int(unit(hashCoord(s.id, 0x66, 0x66))*3) // 2..4 radial roads
	s.spoke0 = unit(hashCoord(s.id, 0x77, 0x77)) * 2 * math.Pi
	s.fieldAng = unit(hashCoord(s.id, 0x88, 0x88)) * 2 * math.Pi
	s.valid = true
	return s
}

// radius returns the organic outline radius at angle θ. Periodic in θ, so the
// curve closes seamlessly with no join.
func (s settlement) radius(theta float64) float64 {
	r := s.r0
	for k := 0; k < 4; k++ {
		r += s.r0 * s.amp[k] * math.Sin(float64(k+1)*theta+s.phase[k])
	}
	return r
}

// settlementAt reports the cell at (x,y) if it lies inside a settlement. It
// scans the 3×3 block of macro-cells around (x,y), since a large settlement
// centred in a neighbouring macro-cell can still reach across the boundary.
func (g *Generator) settlementAt(x, y int) (Cell, bool) {
	mx := floorDiv(x, settleCell)
	my := floorDiv(y, settleCell)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			s := g.settlementFor(mx+dx, my+dy)
			if !s.valid {
				continue
			}
			if c, ok := g.cellInSettlement(s, x, y); ok {
				return c, true
			}
		}
	}
	return Cell{}, false
}

// cellInSettlement decides what (x,y) is within settlement s: outside, the
// central well, a road, the fence (with road gaps acting as gates), a field
// furrow, a house, or open yard. Returns ok == false when the cell is outside
// the outline or clipped by water.
func (g *Generator) cellInSettlement(s settlement, x, y int) (Cell, bool) {
	dx := float64(x - s.cx)
	dy := float64(y - s.cy)
	d := math.Hypot(dx, dy)
	theta := math.Atan2(dy, dx)
	R := s.radius(theta)
	if d > R+0.5 {
		return Cell{}, false
	}
	// Let the settlement meet nature: never pave over open water or climb onto
	// bare peaks — those cells fall back to the underlying terrain.
	if elev, _, _, _ := g.climate(x, y); elev < 0.34 || elev > 0.82 {
		return Cell{}, false
	}

	// Central well — a single tile at the heart of the settlement.
	if d < 1 {
		return Cell{Biome: Grass, Glyph: 'W', Color: "#9AA7B0"}, true
	}
	// Radial dirt roads. They extend just past the outline so they always breach
	// the fence ring, forming the gateways.
	if s.onSpoke(theta, d, R) {
		return Cell{Biome: Path, Glyph: '·', Color: "#8C7A56", Walkable: true}, true
	}
	// Fence ring at the outline (villages only). Road cells already returned
	// above, so the spokes leave walkable gaps — natural gates.
	if s.hasFence && d >= R-1 {
		return Cell{Biome: Grass, Glyph: '=', Color: "#7A5A3A"}, true
	}
	// Farm wedge: plowed fields fill one angular sector toward the edge.
	if s.hasFields && d > s.r0*0.55 && angDiff(theta, s.fieldAng) < 0.6 {
		return Cell{Biome: Grass, Glyph: '"', Color: "#8A6E44", Walkable: true}, true
	}
	// Houses on a jittered plot grid, kept off the central well and clear of the
	// fence ring (villages) or just inside the outline (open hamlets).
	edge := 0.7
	if s.hasFence {
		edge = 1.6
	}
	if d > 1.4 && d < R-edge && s.houseHere(x, y) {
		return Cell{Biome: Grass, Glyph: 'H', Color: houseColor(g.prop2(x, y))}, true
	}
	// Otherwise: trodden, cleared yard — walkable grass.
	return Cell{Biome: Grass, Glyph: '·', Color: "#6BB36F", Walkable: true}, true
}

// onSpoke reports whether (θ,d) lies on one of the settlement's radial roads. A
// road's angular half-width shrinks with distance so it stays roughly one tile
// wide all the way out.
func (s settlement) onSpoke(theta, d, R float64) bool {
	if d < 1 || d > R+0.6 {
		return false
	}
	half := 0.9 / d
	step := 2 * math.Pi / float64(s.spokes)
	for i := 0; i < s.spokes; i++ {
		if angDiff(theta, s.spoke0+float64(i)*step) < half {
			return true
		}
	}
	return false
}

// houseHere reports whether (x,y) is the (deterministically jittered) building
// anchor of an occupied plot. Each plotSpan×plotSpan plot holds at most one
// single-tile house.
func (s settlement) houseHere(x, y int) bool {
	px := floorDiv(x-s.cx, plotSpan)
	py := floorDiv(y-s.cy, plotSpan)
	if unit(hashCoord(s.id^0x51A1, px, py)) > 0.7 { // ~70% of plots are built
		return false
	}
	jx := int(unit(hashCoord(s.id^0x61B2, px, py)) * plotSpan)
	jy := int(unit(hashCoord(s.id^0x71C3, px, py)) * plotSpan)
	return x == s.cx+px*plotSpan+jx && y == s.cy+py*plotSpan+jy
}

// angDiff is the absolute smallest angle between two angles, in [0,π].
func angDiff(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 2*math.Pi)
	if d > math.Pi {
		d = 2*math.Pi - d
	}
	return d
}

// floorDiv is integer division that floors toward negative infinity, so the
// macro-grid tiles cleanly across the origin.
func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

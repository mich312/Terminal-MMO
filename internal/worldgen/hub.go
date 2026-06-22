package worldgen

import "math"

// The hub town is the redesigned Wilds spawn — a forest hamlet rather than a
// stamped square. A central green commons (the HQ keep, a well, a little market
// and street lamps, the HQ portal at its heart) sits in an irregular clearing
// with a noise-ragged tree line; worn dirt trails radiate out to the outlying
// buildings — smithy, market hall, chapel — each tucked in its own small glade,
// and on to the gates. The portal/gate coordinates (see Landmarks and Gates)
// are unchanged, and the trails keep every door reachable just as before.
//
// Everything renders through the existing settlement palette (meadow, dirt
// trail, well, stall, brazier, building anchor/body), so both the glyph and HD
// clients draw it with sprites they already have.

const (
	hubGreenR = 7.0 // radius of the central green clearing (before noise jitter)
	hubGladeR = 3.5 // radius of the small glade around each outlying door
	hubApronR = 4.5 // near the spawn heart, keep the ground clear of blocking props
)

// hubBldg is a building in the hub: a footprint whose anchor (bottom-left) tile
// carries the sprite and whose remaining body tiles block. The matching portal
// sits just outside the footprint, as the building's door.
type hubBldg struct {
	ax, ay int       // anchor (bottom-left, i.e. min-x/min-y) cell
	bt     buildType // sprite + footprint
}

// hubBldgs flank each wing portal so stepping onto a door sends you indoors.
var hubBldgs = []hubBldg{
	{-1, -3, btKeep},      // Durst HQ — keep/town hall on the green; its north door is the (0,0) portal
	{14, 2, btMarketHall}, // Presentation — market hall north of the east trail, door (16,0)
	{-16, 2, btSmithy},    // Kraftwerk — smithy north of the west trail, door (-16,0)
	{-4, 10, btCathedral}, // Demo Center — chapel west of the north trail, door (0,12)
}

// hubGlades are the clearings carved around the outlying doors (the wing portals
// and the two gates) so each sits in a glade on its trail rather than jammed in
// the trees. The central green (origin) is handled separately.
var hubGlades = [][2]int{{16, 0}, {-16, 0}, {0, 12}, {22, 0}, {0, 18}}

// Hamlet furniture, placed asymmetrically on the green and clear of both trail
// bows (all cells have |x|>=4 and |y|>=4, so a meandering trail never runs onto
// them). A well, a little west-side market, and a scatter of street lamps.
var (
	hubWell   = [2]int{4, 4}
	hubStalls = [][2]int{{-4, 5}, {-5, 4}, {-5, 5}}
	hubLamps  = [][2]int{{-4, 4}, {4, -4}, {-4, -4}}
)

// hubCell returns the redressed cell for the hub hamlet, or ok=false for cells
// the hamlet does not cover (so generation falls through to settlements/biome).
// It is consulted after the portal/gate overrides in At, so those cells win.
func (g *Generator) hubCell(x, y int) (Cell, bool) {
	// Buildings first — their footprints sit out on the trails, in their glades.
	if c, ok := hubBuilding(x, y); ok {
		return c, true
	}
	// Hamlet furniture always wins its cell, regardless of where the ragged
	// clearing rim falls, so a lamp/well/stall never goes missing.
	if x == hubWell[0] && y == hubWell[1] {
		return Cell{Biome: Grass, Glyph: 'W', Color: "#9AA7B0"}, true // village well (blocks)
	}
	for _, p := range hubLamps {
		if x == p[0] && y == p[1] {
			return Cell{Biome: Grass, Glyph: 'i', Color: "#FF7A1E"}, true // brazier (blocks, glows at night)
		}
	}
	for _, p := range hubStalls {
		if x == p[0] && y == p[1] {
			return Cell{Biome: Grass, Glyph: 's', Color: "#C24A3A"}, true // market stall (blocks)
		}
	}
	onTrail, edge := g.trailAt(x, y)
	d, inClear := g.inHubClearing(x, y)
	if !inClear && !onTrail {
		return Cell{}, false
	}
	// Worn dirt trails wind through the green and out to every door. Their
	// shoulders let the green encroach where the noise says so, so a trail reads
	// as a trodden desire-path rather than a ruler line.
	if onTrail {
		fx, fy := float64(x), float64(y)
		if edge && g.fbmAt(fx, fy, 0x51A7, 0.6, 2) > 0.56 {
			return g.hubMeadow(x, y, d, false), true // grass reclaims the shoulder (no canopy on a trail)
		}
		c := Cell{Biome: Path, Glyph: '·', Color: "#8C7A56", Walkable: true}
		if g.prop(x, y) < 0.12 {
			c.Glyph, c.Color = '∘', "#857653" // a cobble
		}
		return c, true
	}
	// The green itself: natural meadow, with a few shade trees for canopy.
	return g.hubMeadow(x, y, d, true), true
}

// hubMeadow is the hamlet's grassy ground: natural meadow scatter (flowers,
// tufts, the odd bush) plus, when canopy is set, the occasional shade tree or
// old stump so the commons isn't a bare lawn. The ground is kept clear of
// blocking props right around the spawn heart so a body can always step out.
func (g *Generator) hubMeadow(x, y int, d float64, canopy bool) Cell {
	if canopy && d > hubApronR+1 {
		switch r := g.prop2(x, y); {
		case r < 0.030:
			return Cell{Biome: Grass, Glyph: 'Y', Color: "#2F7D4F"} // a shade tree (blocks)
		case r < 0.050:
			return Cell{Biome: Grass, Glyph: 'u', Color: "#6B4A2B", Walkable: true} // an old stump
		}
	}
	c := grassCell(g, x, y)
	if d <= hubApronR && !c.Walkable {
		c = Cell{Biome: Grass, Glyph: '·', Color: "#5EAE63", Walkable: true}
	}
	return c
}

// trailAt reports whether (x,y) lies on a forced trail, and whether it is the
// trail's edge row. The centreline bows gently inside the green (tapered to zero
// at the heart and at the rim, see trailOffset) so the path curves through the
// commons, then runs dead straight out through the forest to each door — keeping
// the 2×2 body's passage guaranteed everywhere.
func (g *Generator) trailAt(x, y int) (on, edge bool) {
	if x >= -16 && x <= 22 {
		if dy := abs(y - g.trailOffset(float64(x), 0x7A11)); dy <= 1 {
			on, edge = true, dy == 1
		}
	}
	if !on && y >= 0 && y <= 18 {
		if dx := abs(x - g.trailOffset(float64(y), 0x9B22)); dx <= 1 {
			on, edge = true, dx == 1
		}
	}
	return
}

// trailOffset is the trail centreline's lateral wander at position `along` the
// arm. A low-frequency noise gives the bow; a sine window tapers it to zero at
// the origin and at the green's rim, so the curve lives entirely inside the
// walkable commons and the forest segments stay straight (and never pinch).
func (g *Generator) trailOffset(along float64, salt uint64) int {
	a := math.Abs(along)
	if a >= hubGreenR {
		return 0
	}
	taper := math.Sin(math.Pi * a / hubGreenR)
	n := g.fbmAt(along, 0, salt, 0.45, 2)*2 - 1 // [-1,1]
	return int(math.Round(2.2 * taper * n))
}

// inHubClearing reports whether (x,y) falls in the central green or one of the
// outlying glades, and returns the distance to the nearest clearing centre (used
// to protect the spawn heart). A low-frequency noise jitters every rim so the
// clearings meander like natural glades instead of forming clean discs.
func (g *Generator) inHubClearing(x, y int) (float64, bool) {
	fx, fy := float64(x), float64(y)
	jitter := 3.2 * (g.fbmAt(fx, fy, 0x6A7E, 0.35, 2) - 0.5)
	if d := math.Hypot(fx, fy); d <= hubGreenR+jitter {
		return d, true
	}
	for _, p := range hubGlades {
		dx, dy := fx-float64(p[0]), fy-float64(p[1])
		if d := math.Hypot(dx, dy); d <= hubGladeR+jitter*0.7 {
			return d, true
		}
	}
	return 0, false
}

// hubBuilding reports the cell for any tile inside a hub building's footprint:
// the anchor carries the sprite (glyph + type in Variant), the rest is blocking
// body. It mirrors how settlement.cellFor renders lBuildAnchor / lBuildBody.
func hubBuilding(x, y int) (Cell, bool) {
	for _, b := range hubBldgs {
		w, h := footprint(b.bt)
		if x >= b.ax && x < b.ax+w && y >= b.ay && y < b.ay+h {
			if x == b.ax && y == b.ay {
				return Cell{Biome: Grass, Glyph: buildingGlyph(b.bt),
					Color: buildingColor(b.bt, Grass), Variant: uint8(b.bt)}, true // anchor (blocks)
			}
			return Cell{Biome: Grass, Glyph: '%'}, true // covered body (blocks; drawn by its anchor)
		}
	}
	return Cell{}, false
}

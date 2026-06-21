package worldgen

// The hub town is the redesigned Wilds spawn. It replaces the old four grassy
// clearings + dirt trails with a small cobbled town square: a central plaza
// (the HQ portal its glowing centrepiece) ringed by the wing buildings whose
// doors are the portals, with market stalls and street lamps, wired out to the
// gates by pale cobbled streets. The portal/gate coordinates (see Landmarks and
// Gates) are unchanged — the hub only redresses what surrounds them, and the
// forced street arms keep every door reachable just as the old trails did.
//
// Everything here renders through the existing settlement palette (cobble,
// well, stall, brazier, building anchor/body), so both the glyph and HD clients
// draw it with sprites they already have.

// hubRadius is the plaza core's half-extent: a cobbled square of this Chebyshev
// radius around the origin (Durst HQ).
const hubRadius = 6

// hubBldg is a building in the hub: a footprint whose anchor (bottom-left) tile
// carries the sprite and whose remaining body tiles block. The matching portal
// sits just outside the footprint, as the building's door.
type hubBldg struct {
	ax, ay int       // anchor (bottom-left, i.e. min-x/min-y) cell
	bt     buildType // sprite + footprint
}

// hubBldgs flank each wing portal so stepping onto a door sends you indoors.
// Footprints are kept clear of the axial streets and of every spawn footprint.
var hubBldgs = []hubBldg{
	{-1, -3, btKeep},      // Durst HQ — keep/town hall; its north door is the (0,0) portal
	{14, 2, btMarketHall}, // Presentation — market hall north of the east street, door (16,0)
	{-16, 2, btSmithy},    // Kraftwerk — smithy north of the west street, door (-16,0)
	{-4, 10, btCathedral}, // Demo Center — cathedral west of the north street, door (0,12)
}

// hubLamps are the plaza's corner braziers — they light the square after dark.
// On the diagonals, clear of the streets and of every spawn footprint.
var hubLamps = [][2]int{{4, 4}, {-4, 4}, {4, -4}, {-4, -4}}

// hubStalls are the market stalls lining the square (block movement). Placing the
// market at spawn gives /trade a diegetic home. All sit off the axial streets.
var hubStalls = [][2]int{{2, 5}, {-2, 5}, {5, 2}, {-5, 2}, {5, -2}, {-5, -2}}

// hubCell returns the redressed cell for the hub town, or ok=false for cells the
// town does not cover (so generation falls through to settlements/biome). It is
// consulted after the portal/gate overrides in At, so those cells still win.
func (g *Generator) hubCell(x, y int) (Cell, bool) {
	// Buildings first: their footprints sit out on the arms, beyond the core.
	if c, ok := hubBuilding(x, y); ok {
		return c, true
	}
	inCore := abs(x) <= hubRadius && abs(y) <= hubRadius
	onArm := onPath(x, y)
	if !inCore && !onArm {
		return Cell{}, false
	}
	// Plaza furniture.
	for _, p := range hubLamps {
		if x == p[0] && y == p[1] {
			return Cell{Biome: Path, Glyph: 'i', Color: "#FF7A1E"}, true // brazier (blocks, glows at night)
		}
	}
	for _, p := range hubStalls {
		if x == p[0] && y == p[1] {
			return Cell{Biome: Path, Glyph: 's', Color: "#C24A3A"}, true // market stall (blocks)
		}
	}
	// Streets beyond the plaza core run pale cobbles out to the wings and gates.
	if onArm && !inCore {
		return Cell{Biome: Path, Glyph: '·', Color: "#BBB29B", Walkable: true}, true
	}
	// The plaza itself: a cobbled square around the origin.
	return Cell{Biome: Path, Glyph: '·', Color: "#A89B82", Walkable: true}, true
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

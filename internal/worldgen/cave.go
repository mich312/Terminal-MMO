package worldgen

import "math"

// Cave systems: a cave opens onto the surface through 1–3 linked mouths set in
// the high hills, all leading to the same cavern. Systems are derived
// deterministically from a macro-grid (like settlements), so any cell can be
// resolved to its system cheaply by probing the 3×3 macro neighbourhood — no
// scan of the whole world. A cave is named by its origin mouth, so every mouth
// of a system shares one cavern, one seed and one remembered map.

const (
	caveCell    = 72               // macro-grid cell — cave systems sit far apart
	caveSalt    = 0xCA7E0F711075DE // separates cave noise from other fields
	caveRate    = 0.42             // share of macro-cells that roll a cave system
	caveHubKeep = 140              // keep caves out of the spawn hub
	caveDoorMin = 16.0             // a secondary mouth's nearest distance from the origin
	caveDoorMax = 44.0             // …and its farthest (well under caveCell)
)

// CaveSystem is one cave's surface footprint: the origin mouth that names it and
// every mouth (1–3, origin first) that opens onto the same cavern.
type CaveSystem struct {
	Origin [2]int
	Doors  [][2]int
	valid  bool
}

// caveSystemFor derives the cave system (if any) for a macro-cell, cached.
func (g *Generator) caveSystemFor(mx, my int) CaveSystem {
	key := uint64(uint32(mx))<<32 | uint64(uint32(my))
	if v, ok := g.caveCache.Load(key); ok {
		return v.(CaveSystem)
	}
	s := g.computeCaveSystem(mx, my)
	g.caveCache.Store(key, s)
	return s
}

func (g *Generator) computeCaveSystem(mx, my int) CaveSystem {
	var s CaveSystem
	h := hashCoord(g.seed^caveSalt, mx, my)
	if unit(h) > caveRate {
		return s // most macro-cells have no cave
	}
	hx := hashCoord(g.seed^caveSalt^0x1111, mx, my)
	hy := hashCoord(g.seed^caveSalt^0x2222, mx, my)
	ox := mx*caveCell + caveCell/4 + int(unit(hx)*float64(caveCell)/2)
	oy := my*caveCell + caveCell/4 + int(unit(hy)*float64(caveCell)/2)
	if abs(ox) < caveHubKeep && abs(oy) < caveHubKeep {
		return s // keep the spawn hub clear
	}
	if !g.caveHill(ox, oy) {
		return s // the origin must sit in the high hills, near the peaks
	}
	s.Origin = [2]int{ox, oy}
	s.Doors = [][2]int{{ox, oy}}
	n := 1 + int(unit(hashCoord(h, 0x3333, 0x4444))*3) // aim for 1..3 mouths
	for i := 1; i < n; i++ {
		for try := 0; try < 10; try++ { // hunt for an offset that lands on good ground
			ha := hashCoord(h, i*0x55+try*7+1, 0x66)
			hd := hashCoord(h, i*0x77+try*13+1, 0x88)
			ang := unit(ha) * 2 * math.Pi
			dist := caveDoorMin + unit(hd)*(caveDoorMax-caveDoorMin)
			c := [2]int{ox + int(math.Cos(ang)*dist), oy + int(math.Sin(ang)*dist)}
			if g.caveLand(c[0], c[1]) && !hasCell(s.Doors, c) {
				s.Doors = append(s.Doors, c)
				break
			}
		}
	}
	s.valid = true
	return s
}

// caveHill reports whether a cell is genuine high hill — walkable highland just
// below the peaks, where a mouth reads as cut into the mountainside. The band is
// above the elevations settlements use, so caves never land in a town.
func (g *Generator) caveHill(x, y int) bool {
	elev, _, _, _ := g.climate(x, y)
	return elev >= 0.70 && elev < 0.84
}

// caveLand reports whether a cell is walkable land a secondary mouth may open
// onto — anywhere from the foothills up to (but not into) the peaks, so linked
// mouths can spill a little down the slopes.
func (g *Generator) caveLand(x, y int) bool {
	elev, _, _, _ := g.climate(x, y)
	return elev >= 0.58 && elev < 0.84
}

// CaveSystemAt resolves a cell to the cave system it's a mouth of, and which
// mouth (index into Doors). ok is false when the cell is not a cave mouth. It
// probes only the 3×3 macro neighbourhood, since a mouth lies within caveDoorMax
// (< caveCell) of its origin.
func (g *Generator) CaveSystemAt(x, y int) (CaveSystem, int, bool) {
	mx, my := floorDiv(x, caveCell), floorDiv(y, caveCell)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			s := g.caveSystemFor(mx+dx, my+dy)
			if !s.valid {
				continue
			}
			for i, d := range s.Doors {
				if d[0] == x && d[1] == y {
					return s, i, true
				}
			}
		}
	}
	return CaveSystem{}, 0, false
}

// caveMouthCell returns the cell for a cave mouth at (x,y), or ok=false.
func (g *Generator) caveMouthCell(x, y int) (Cell, bool) {
	if _, _, ok := g.CaveSystemAt(x, y); ok {
		return Cell{Biome: Hill, Glyph: 'Ω', Color: "#2A2630",
			Walkable: true, Object: true, Portal: "cave"}, true
	}
	return Cell{}, false
}

func hasCell(cs [][2]int, c [2]int) bool {
	for _, x := range cs {
		if x == c {
			return true
		}
	}
	return false
}

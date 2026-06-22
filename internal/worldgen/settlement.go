package worldgen

import (
	"fmt"
	"math"
)

// Settlements — villages scattered across the Wilds. Like everything in worldgen
// they are a pure function of the seed: a settlement is never "placed" or
// stored. The world is partitioned into a coarse macro-grid; each macro-cell is
// hashed to decide whether it hosts a settlement and where its centre sits.
//
// Unlike the terrain (decided one cell at a time), a settlement's *layout* is
// generated as a whole — a small local grid is built once from the settlement's
// hash and cached, then At() just looks the cell up. Generating the whole plan
// at once is what lets villages have multi-tile buildings, lanes that bend
// around housing, a wall that follows the built-up edge with real corners, and
// roads that leave through the gates toward a neighbour — none of which a
// per-cell decision could express.
//
// The plan is a medieval nucleated village: a central green with a well and a
// church, a main road passing through (in one gate, out the other) toward the
// nearest neighbouring village, houses of varied sizes fronting the road and
// lanes (densest at the core), an irregular palisade enclosing the built area
// with gates where the roads cross it, and open fields along the approach.

const (
	settleSalt    uint64 = 0x5E771E3D0FAB12C7
	settleCell           = 168 // macro-grid cell size — settlements sit far apart
	settleHubKeep        = 132 // keep settlement centres this far from the origin hub
	// Each settlement draws its own core reach in [minReach, maxReach]; at or above
	// cityThreshold it is a stone-walled city, below it a timber village. So sizes
	// run as a continuum from a hamlet up to a large city.
	minReach      = 15
	maxReach      = 46
	cityThreshold = 33
	linkMax       = 360.0 // longest settlement-to-settlement connecting road
	linkFade      = 200.0 // beyond this a connecting road dwindles to a trail
)

// dims returns the settlement's core reach, layout-grid half-extent and worksite
// reach, scaled from its own size. The half-extent stays below settleCell so a
// settlement spans at most one macro-cell.
func (s settlement) dims() (reach, half, outpost int) {
	return s.reach, s.reach + 22, s.reach + 18
}

// nb4/nb8 are the 4- and 8-neighbour offsets used by the layout flood fills.
var (
	nb4 = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	nb8 = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
)

// ── settlement identity ──────────────────────────────────────────────────────

type settlement struct {
	mx, my int    // macro-cell
	id     uint64 // identity hash — seeds the whole layout
	cx, cy int    // centre, world coordinates
	reach  int    // core size, in tiles (its own scale)
	town   bool   // a stone-walled city (vs a timber-palisade village)
	valid  bool
}

// settlementFor derives the settlement (if any) for a macro-cell, cached.
func (g *Generator) settlementFor(mx, my int) settlement {
	key := uint64(uint32(mx))<<32 | uint64(uint32(my))
	if v, ok := g.settleCache.Load(key); ok {
		return v.(settlement)
	}
	s := g.computeSettlement(mx, my)
	g.settleCache.Store(key, s)
	return s
}

func (g *Generator) computeSettlement(mx, my int) settlement {
	var s settlement
	s.mx, s.my = mx, my
	h := hashCoord(g.seed^settleSalt, mx, my)
	if unit(h) > 0.42 { // a little under half of macro-cells host a settlement
		return s
	}
	hx := hashCoord(g.seed^settleSalt^0x1111, mx, my)
	hy := hashCoord(g.seed^settleSalt^0x2222, mx, my)
	s.cx = mx*settleCell + settleCell/4 + int(unit(hx)*float64(settleCell)/2)
	s.cy = my*settleCell + settleCell/4 + int(unit(hy)*float64(settleCell)/2)
	if abs(s.cx) < settleHubKeep && abs(s.cy) < settleHubKeep {
		return s // keep the spawn hub clear
	}
	// Buildability: temperate lowland only — not water, beach, dense wetland or
	// cold/steep highland. A cheap un-warped probe at the centre gates placement.
	elev := g.fbmAt(float64(s.cx), float64(s.cy), 0x1, 0.045, 3)
	moist := g.fbmAt(float64(s.cx), float64(s.cy), 0x9E37, 0.03, 3)
	if elev < 0.40 || elev > 0.64 || moist > 0.58 {
		return s
	}
	s.id = h
	// Every settlement gets its own size, skewed toward the small end so most are
	// villages and a minority grow into cities — a continuum, not two fixed tiers.
	sz := unit(hashCoord(h, 0x717, 0x727))
	s.reach = minReach + int(sz*sz*float64(maxReach-minReach))
	s.town = s.reach >= cityThreshold
	s.valid = true
	return s
}

type partnerResult struct {
	p  settlement
	ok bool
}

// partnerOf finds the nearest neighbouring settlement within reach, for the
// connecting road (cached). Returns ok=false if the village stands alone. It
// reads only settlement metadata (never another partner), so there is no
// cascade. Returns ok=false for an invalid settlement.
func (g *Generator) partnerOf(s settlement) (settlement, bool) {
	if !s.valid {
		return settlement{}, false
	}
	key := uint64(uint32(s.mx))<<32 | uint64(uint32(s.my))
	if v, ok := g.partnerCache.Load(key); ok {
		r := v.(partnerResult)
		return r.p, r.ok
	}
	best, bestD2, found := settlement{}, math.MaxFloat64, false
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			n := g.settlementFor(s.mx+dx, s.my+dy)
			if !n.valid {
				continue
			}
			d2 := float64((n.cx-s.cx)*(n.cx-s.cx) + (n.cy-s.cy)*(n.cy-s.cy))
			if d2 < bestD2 && d2 <= linkMax*linkMax {
				best, bestD2, found = n, d2, true
			}
		}
	}
	g.partnerCache.Store(key, partnerResult{best, found})
	return best, found
}

// partnersOf returns every neighbour a settlement is road-linked to — the local
// road network, not just the single nearest town. Candidates are the valid
// settlements in the surrounding macro-cells within linkMax (so towns that are
// simply too far apart are never joined). A Gabriel-graph rule then prunes
// redundant links: an edge s→n is dropped if any other settlement sits inside
// the circle that has s–n as its diameter — so a town links to the neighbours
// genuinely near it and never throws a long road clear over a closer town. The
// result is a sparse, natural web; isolated towns simply return an empty list.
func (g *Generator) partnersOf(s settlement) []settlement {
	if !s.valid {
		return nil
	}
	key := uint64(uint32(s.mx))<<32 | uint64(uint32(s.my))
	if v, ok := g.partnersCache.Load(key); ok {
		return v.([]settlement)
	}
	var cand []settlement
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			n := g.settlementFor(s.mx+dx, s.my+dy)
			if !n.valid {
				continue
			}
			d2 := float64((n.cx-s.cx)*(n.cx-s.cx) + (n.cy-s.cy)*(n.cy-s.cy))
			if d2 <= linkMax*linkMax {
				cand = append(cand, n)
			}
		}
	}
	var partners []settlement
	for _, n := range cand {
		mx, my := float64(s.cx+n.cx)/2, float64(s.cy+n.cy)/2 // midpoint of s–n
		rad2 := (float64(s.cx-n.cx)*float64(s.cx-n.cx) + float64(s.cy-n.cy)*float64(s.cy-n.cy)) / 4
		blocked := false
		for _, m := range cand {
			if m.mx == n.mx && m.my == n.my {
				continue
			}
			if (float64(m.cx)-mx)*(float64(m.cx)-mx)+(float64(m.cy)-my)*(float64(m.cy)-my) < rad2 {
				blocked = true // a nearer town lies between s and n — n is reached through it
				break
			}
		}
		if !blocked {
			partners = append(partners, n)
		}
	}
	g.partnersCache.Store(key, partners)
	return partners
}

// ── layout grid ──────────────────────────────────────────────────────────────

type lkind uint8

const (
	lEmpty       lkind = iota // not part of the settlement → fall through to terrain
	lYard                     // cleared, trodden ground (walkable)
	lGreen                    // the village green (walkable)
	lRoad                     // a village's dirt road / lane (walkable)
	lStreet                   // a city's cobbled street (walkable)
	lWell                     // the central well (blocks)
	lFence                    // palisade segment (blocks)
	lGate                     // opening where a road crosses the palisade (walkable)
	lField                    // cultivated field (walkable)
	lGarden                   // a small kitchen garden inside the village (walkable)
	lPond                     // a village pond by the green (blocks)
	lQuarry                   // cut-stone quarry floor (walkable)
	lQuarryRock               // a boulder at the quarry (blocks)
	lClearing                 // a cleared worksite floor — packed earth (walkable)
	lStump                    // a cut stump / log at a lumber camp (walkable)
	lJetty                    // a wooden jetty out over the water (walkable)
	lBridge                   // a plank bridge spanning water inside a settlement (walkable)
	lWall                     // a stone curtain wall, for towns (blocks)
	lTower                    // a stone wall tower, for towns (blocks)
	lPlaza                    // a cobbled market square, for towns (walkable)
	lPaved                    // packed/cobbled ground between city buildings (walkable)
	lCourtyard                // the open bailey inside a city's citadel (walkable)
	lBrazier                  // a fire brazier lighting a city's gates/squares (blocks)
	lStall                    // a market stall on a city's square (blocks)
	lBuildAnchor              // base tile of a building (blocks) — bt names the kind
	lBuildBody                // a non-base tile of a building (blocks)
)

type buildType uint8

const (
	btNone       buildType = iota
	btCottage              // 1×1
	btHouse                // 2×2
	btLonghouse            // 3×2
	btBarn                 // 2×2
	btChurch               // 2×3 (tall, village centrepiece)
	btKeep                 // 3×3 (a city's castle keep)
	btCathedral            // 3×4 (a city's great church)
	btTownhouse            // 2×3 (tall, multi-storey — a city's wealthy core)
	btMarketHall           // 3×3 (a city's market hall)
	btSmithy               // 2×2 (a blacksmith's forge, glows warm at night)
	btTavern               // 2×2 (a tavern, warm lit windows)
	// Burgage plots: a medieval town's blocks are divided into narrow, deep
	// parcels, so its streets are lined with houses of varied frontage and depth
	// rather than uniform squares.
	btRowhouse    // 2×3 (a deep burgage house)
	btNarrowhouse // 1×2 (a narrow-fronted, deep house)
	btDeephouse   // 2×4 (a deep, tall burgage house)
)

// footprint reports a building's width and height in tiles. The anchor is the
// bottom-left tile; the body extends up (north) and right (east) from it.
func footprint(bt buildType) (w, h int) {
	switch bt {
	case btHouse:
		return 2, 2
	case btLonghouse:
		return 3, 2
	case btBarn:
		return 2, 2
	case btChurch:
		return 2, 3
	case btKeep:
		return 3, 3
	case btCathedral:
		return 3, 4
	case btTownhouse:
		return 2, 3
	case btMarketHall:
		return 3, 3
	case btSmithy, btTavern:
		return 2, 2
	case btRowhouse:
		return 2, 3
	case btNarrowhouse:
		return 1, 2
	case btDeephouse:
		return 2, 4
	default:
		return 1, 1
	}
}

type lcell struct {
	kind  lkind
	bt    buildType // for lBuildAnchor
	fv    uint8     // fence orientation (for lFence): 0 horizontal, 1 vertical, 2 post/corner
	decor uint8     // yard greenery: 0 none, 1 bush, 2 flower, 3 tuft
	biome Biome     // underlying biome, for ground colour
}

type layout struct {
	ox, oy int // world coordinate of grid cell (0,0)
	n      int // grid is n×n
	cells  []lcell
}

func (l *layout) at(gx, gy int) *lcell { return &l.cells[gy*l.n+gx] }
func (l *layout) in(gx, gy int) bool   { return gx >= 0 && gy >= 0 && gx < l.n && gy < l.n }

// layoutOf returns the (cached) generated plan for a settlement.
func (g *Generator) layoutOf(s settlement) *layout {
	key := uint64(uint32(s.mx))<<32 | uint64(uint32(s.my))
	if v, ok := g.layoutCache.Load(key); ok {
		return v.(*layout)
	}
	l := g.genLayout(s)
	g.layoutCache.Store(key, l)
	return l
}

// ── small deterministic PRNG, seeded per settlement ──────────────────────────

type srng struct{ s uint64 }

func (r *srng) u() uint64 {
	r.s += 0x9E3779B97F4A7C15
	z := r.s
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}
func (r *srng) f() float64               { return float64(r.u()>>11) / float64(uint64(1)<<53) }
func (r *srng) n(k int) int              { return int(r.u() % uint64(k)) }
func (r *srng) rng(a, b float64) float64 { return a + (b-a)*r.f() }

// ── layout generation ─────────────────────────────────────────────────────────

func (g *Generator) genLayout(s settlement) *layout {
	reachI, half, outpost := s.dims()
	n := 2*half + 1
	l := &layout{ox: s.cx - half, oy: s.cy - half, n: n, cells: make([]lcell, n*n)}
	cgx, cgy := half, half // centre in grid space
	rng := &srng{s: s.id ^ 0xBEEF}

	// Buildable mask + underlying biome, sampled once.
	buildable := make([]bool, n*n)
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			wx, wy := l.ox+gx, l.oy+gy
			b := g.biomeAt(wx, wy)
			l.at(gx, gy).biome = b
			switch b {
			case Water, Mountain, Deep:
				buildable[gy*n+gx] = false
			default:
				buildable[gy*n+gx] = true
			}
		}
	}
	canBuild := func(gx, gy int) bool { return l.in(gx, gy) && buildable[gy*n+gx] }

	// A city is shaped by the land: its footprint is the patch of contiguous open
	// lowland reachable from the centre, so it fills a plain, hugs a coast or a
	// wood, and stops at water, forest and hills rather than being a disc stamped
	// on the map. (Villages are small enough to just clip against terrain.)
	var cityArea []bool
	if s.town {
		cityArea = make([]bool, n*n)
		// Lobed boundary (a few harmonics) so even a city on open ground isn't a
		// disc; the terrain flood-fill then clips it further.
		var amp, ph [3]float64
		for k := 0; k < 3; k++ {
			amp[k] = rng.rng(0.10, 0.26)
			ph[k] = rng.f() * 2 * math.Pi
		}
		maxRAt := func(nx, ny int) float64 {
			ang := math.Atan2(float64(ny-cgy), float64(nx-cgx))
			m := 1.0
			for k := 0; k < 3; k++ {
				m += amp[k] * math.Sin(float64(k+1)*ang+ph[k])
			}
			return (float64(reachI) + 4) * m
		}
		open := func(gx, gy int) bool {
			if !l.in(gx, gy) {
				return false
			}
			switch l.at(gx, gy).biome { // open ground a city sprawls over
			case Grass, Savanna, Sand, Snow, Swamp:
				return true
			}
			return false
		}
		cityArea[cgy*n+cgx] = true // seed the centre even if it sits on an edge
		q := [][2]int{{cgx, cgy}}
		for len(q) > 0 {
			p := q[0]
			q = q[1:]
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := p[0]+d[0], p[1]+d[1]
				if !l.in(nx, ny) || cityArea[ny*n+nx] {
					continue
				}
				if math.Hypot(float64(nx-cgx), float64(ny-cgy)) > maxRAt(nx, ny) || !open(nx, ny) {
					continue
				}
				cityArea[ny*n+nx] = true
				q = append(q, [2]int{nx, ny})
			}
		}
	}
	inCity := func(gx, gy int) bool { return cityArea == nil || (l.in(gx, gy) && cityArea[gy*n+gx]) }

	// A city's citadel: a castle keep in a small walled bailey, placed before the
	// streets (so the lanes route around it) as a rectangle — a sharp, fortified
	// contrast to the organic town — with corner towers and one gate to the city.
	if s.town {
		for try := 0; try < 14; try++ {
			cw, ch := 8+rng.n(4), 8+rng.n(4) // 8..11
			a := rng.f() * 2 * math.Pi
			dist := 8 + float64(cw+ch)/4
			qx := cgx + int(math.Cos(a)*dist)
			qy := cgy + int(math.Sin(a)*dist)
			x0, y0 := qx-cw/2, qy-ch/2
			fits := true
			for yy := y0 - 1; yy <= y0+ch && fits; yy++ {
				for xx := x0 - 1; xx <= x0+cw; xx++ {
					if !inCity(xx, yy) || !canBuild(xx, yy) || l.at(xx, yy).kind != lEmpty {
						fits = false
						break
					}
				}
			}
			if !fits || !placeBuilding(l, canBuild, qx-1, qy+1, btKeep, 1) {
				continue
			}
			for yy := y0; yy < y0+ch; yy++ {
				for xx := x0; xx < x0+cw; xx++ {
					if perim := xx == x0 || xx == x0+cw-1 || yy == y0 || yy == y0+ch-1; perim {
						l.at(xx, yy).kind = lWall
					} else if l.at(xx, yy).kind == lEmpty {
						l.at(xx, yy).kind = lCourtyard
					}
				}
			}
			for _, c := range [][2]int{{x0, y0}, {x0 + cw - 1, y0}, {x0, y0 + ch - 1}, {x0 + cw - 1, y0 + ch - 1}} {
				l.at(c[0], c[1]).kind = lTower
			}
			ggx, ggy := qx, y0 // gate on the side facing the city centre
			if cgy > y0+ch/2 {
				ggy = y0 + ch - 1
			}
			if d := cgx - qx; d > ch/2 || d < -ch/2 {
				ggx, ggy = x0, qy
				if cgx > x0+cw/2 {
					ggx = x0 + cw - 1
				}
			}
			for _, o := range [][2]int{{0, 0}, {1, 0}, {0, 1}} {
				if l.in(ggx+o[0], ggy+o[1]) && l.at(ggx+o[0], ggy+o[1]).kind == lWall {
					l.at(ggx+o[0], ggy+o[1]).kind = lGate
				}
			}
			break
		}
	}

	// Main road axis: toward the nearest neighbour if there is one (so the road
	// lines up with the connecting road through the gate), else a random bearing.
	var axis float64
	if p, ok := g.partnerOf(s); ok {
		axis = math.Atan2(float64(p.cy-s.cy), float64(p.cx-s.cx))
	} else {
		axis = rng.f() * 2 * math.Pi
	}

	// Road network as line segments in grid space. The main road runs across the
	// village along the axis (bent at a jittered midpoint, so it isn't a ruler-
	// straight highway), and a rough loop lane rings the core with a couple of
	// connecting lanes — so houses cluster in two dimensions around a nucleus
	// rather than strung out along one street.
	// Each segment carries a half-width: main roads are broad and legible, ring
	// roads middling, and the tangle of alleys narrow. Broad lanes are what read the
	// city into blocks rather than letting the built mass bleed together.
	type seg struct {
		ax, ay, bx, by float64
		w              float64 // carve half-width
	}
	const (
		wMain  = 1.3 // main thoroughfares
		wRing  = 1.0 // ring roads
		wAlley = 0.7 // back alleys
		wLane  = 0.8 // a village lane
	)
	reach := float64(reachI)
	dx, dy := math.Cos(axis), math.Sin(axis)
	px, py := -dy, dx // perpendicular
	off := rng.rng(-1.5, 1.5)
	bend := rng.rng(-3, 3)
	cfx, cfy := float64(cgx)+px*off, float64(cgy)+py*off // road passes just off-centre
	mid := [2]float64{cfx + px*bend, cfy + py*bend}
	mainW := wLane
	if s.town {
		mainW = wMain
	}
	segs := []seg{
		{cfx + dx*reach, cfy + dy*reach, mid[0], mid[1], mainW},
		{mid[0], mid[1], cfx - dx*reach, cfy - dy*reach, mainW},
	}
	if s.town {
		// A tangled medieval street plan: winding lanes radiating from the market
		// out to the gates, a few wobbly ring roads tying them together, and a
		// scatter of short alleys. The radials meet at the centre and reach the
		// wall (becoming gates), so the whole web is connected and walkable.
		nRad := 5 + rng.n(3)
		for i := 0; i < nRad; i++ {
			a := float64(i)/float64(nRad)*2*math.Pi + rng.rng(-0.35, 0.35)
			prevx, prevy := float64(cgx), float64(cgy)
			const steps = 4
			for st := 1; st <= steps; st++ {
				rr := reach * float64(st) / float64(steps)
				a += rng.rng(-0.22, 0.22) // wind as the lane runs outward
				nx, ny := float64(cgx)+math.Cos(a)*rr, float64(cgy)+math.Sin(a)*rr
				segs = append(segs, seg{prevx, prevy, nx, ny, wMain}) // broad radials
				prevx, prevy = nx, ny
			}
		}
		for ri := 1; ri <= 2+rng.n(2); ri++ { // wobbly ring roads
			ringR := reach * float64(ri) / 3.5
			k := 9 + rng.n(4)
			rp := make([][2]float64, k)
			for j := 0; j < k; j++ {
				a := float64(j)/float64(k)*2*math.Pi + rng.rng(-0.15, 0.15)
				rr := ringR * rng.rng(0.82, 1.18)
				rp[j] = [2]float64{float64(cgx) + math.Cos(a)*rr, float64(cgy) + math.Sin(a)*rr}
			}
			for j := 0; j < k; j++ {
				segs = append(segs, seg{rp[j][0], rp[j][1], rp[(j+1)%k][0], rp[(j+1)%k][1], wRing})
			}
		}
		for i := 0; i < 8+rng.n(7); i++ { // narrow back alleys splitting the blocks
			a := rng.f() * 2 * math.Pi
			r1 := reach * rng.rng(0.2, 0.92)
			a2, r2 := a+rng.rng(-0.5, 0.5), r1+rng.rng(-7, 7)
			segs = append(segs, seg{
				float64(cgx) + math.Cos(a)*r1, float64(cgy) + math.Sin(a)*r1,
				float64(cgx) + math.Cos(a2)*r2, float64(cgy) + math.Sin(a2)*r2, wAlley,
			})
		}
	} else {
		// A village clusters organically: a loop lane round the core with a couple
		// of connecting lanes.
		loopR := reach * 0.42
		const loopK = 6
		var lp [][2]float64
		for i := 0; i < loopK; i++ {
			a := float64(i)/loopK*2*math.Pi + rng.rng(-0.25, 0.25)
			rr := loopR * rng.rng(0.78, 1.18)
			lp = append(lp, [2]float64{float64(cgx) + math.Cos(a)*rr, float64(cgy) + math.Sin(a)*rr})
		}
		for i := 0; i < loopK; i++ {
			j := (i + 1) % loopK
			segs = append(segs, seg{lp[i][0], lp[i][1], lp[j][0], lp[j][1], wLane})
		}
		for i := 0; i < 2; i++ {
			if i == 1 && rng.f() < 0.4 {
				continue
			}
			v := lp[rng.n(loopK)]
			segs = append(segs, seg{v[0], v[1], cfx + dx*rng.rng(-5, 5), cfy + dy*rng.rng(-5, 5), wLane})
		}
	}

	onAnyRoad := func(gx, gy float64) float64 {
		best := math.MaxFloat64
		for _, sg := range segs {
			if d := distPointSeg(gx, gy, sg.ax, sg.ay, sg.bx, sg.by); d < best {
				best = d
			}
		}
		return best
	}
	// onStreet reports whether a cell falls inside any segment's own half-width —
	// so broad thoroughfares carve wide and back alleys carve narrow.
	onStreet := func(gx, gy float64) bool {
		for _, sg := range segs {
			if distPointSeg(gx, gy, sg.ax, sg.ay, sg.bx, sg.by) < sg.w {
				return true
			}
		}
		return false
	}

	// Carve roads (skip non-buildable so a road meets water/rock instead of
	// bridging it). A city's lanes stay within its terrain footprint; a village's
	// can reach to the grid edge. Either way they pass through the wall as gates.
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			if l.at(gx, gy).kind != lEmpty { // don't pave over the citadel
				continue
			}
			ok, laneKind := canBuild(gx, gy), lRoad
			if s.town {
				ok, laneKind = inCity(gx, gy), lStreet // cities are cobbled
			}
			if ok && onStreet(float64(gx), float64(gy)) {
				l.at(gx, gy).kind = laneKind
			}
		}
	}

	// Central focus: the well, then the church fronting it from the south, then
	// the green filling the open ground around them (skipping what's taken).
	gpx, gpy := float64(cgx)-px*off, float64(cgy)-py*off // green sits off the road
	if canBuild(cgx, cgy) {
		l.at(cgx, cgy).kind = lWell
	}
	// The centrepiece: a village fronts a church on its green; a city's market
	// square is anchored by a great cathedral on one side and a castle keep on
	// the other. Try a ring of spots so a lane through the middle can't block it.
	gx0, gy0 := int(gpx), int(gpy)
	churchType := btChurch
	if s.town {
		churchType = btCathedral
	}
	for _, o := range [][2]int{{-1, 4}, {0, 4}, {-2, 4}, {1, 4}, {-1, 5}, {-2, 3}, {2, 4}, {-3, 4}} {
		if placeBuilding(l, canBuild, gx0+o[0], gy0+o[1], churchType, 1) {
			break
		}
	}
	if s.town { // a market hall flanks the square, opposite the cathedral
		for _, o := range [][2]int{{3, -1}, {4, -1}, {3, 0}, {-4, -1}, {3, 1}, {4, 0}, {-5, -1}, {4, 1}} {
			if placeBuilding(l, canBuild, gx0+o[0], gy0+o[1], btMarketHall, 1) {
				break
			}
		}
	}
	// Trades on the square: a smithy (its forge glowing after dark) and a tavern,
	// iconic civic buildings every settlement has, fronting the central space.
	// Search outward in rings from a preferred bearing so a lane through the middle
	// can't crowd them out; the two start opposite each other so they don't clump.
	placeNearGreen := func(bearing float64, bt buildType) {
		for r := 3.0; r <= 8.0; r += 1.0 {
			for da := 0; da < 8; da++ {
				a := bearing + float64(da)*0.7*math.Pi // fan out around the bearing
				gx := gx0 + int(math.Round(math.Cos(a)*r))
				gy := gy0 + int(math.Round(math.Sin(a)*r))
				if placeBuilding(l, canBuild, gx, gy, bt, 1) {
					return
				}
			}
		}
	}
	placeNearGreen(rng.f()*2*math.Pi, btSmithy)
	placeNearGreen(rng.f()*2*math.Pi+math.Pi, btTavern)
	squareR, squareKind := 2.4, lGreen
	if s.town { // a city has a broad cobbled market square, not a little green
		squareR, squareKind = 4.6, lPlaza
	}
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			if l.at(gx, gy).kind != lEmpty || !canBuild(gx, gy) {
				continue
			}
			if math.Hypot(float64(gx)-gpx, float64(gy)-gpy) < squareR {
				l.at(gx, gy).kind = squareKind
			}
		}
	}
	if s.town { // secondary squares break up the dense blocks
		for i := 0; i < 2; i++ {
			a := rng.f() * 2 * math.Pi
			d := float64(reach) * rng.rng(0.35, 0.7)
			sx, sy := cgx+int(math.Cos(a)*d), cgy+int(math.Sin(a)*d)
			sr := rng.rng(1.8, 2.8)
			for gy := 0; gy < n; gy++ {
				for gx := 0; gx < n; gx++ {
					if l.at(gx, gy).kind == lEmpty && inCity(gx, gy) &&
						math.Hypot(float64(gx-sx), float64(gy-sy)) < sr {
						l.at(gx, gy).kind = lPlaza
					}
				}
			}
		}
	}

	// Garden courtyards: kitchen-garden and tree-shaded pockets scattered through a
	// city's blocks, so it has the village's crofts and greenery rather than
	// wall-to-wall building. Stamped as lGarden (which the houses build around)
	// before the terraces go down; a larger one keeps a grassy centre that the
	// greenery pass later plants with a tree or hedge. This is also what keeps the
	// blocks from reading as one solid mass.
	if s.town {
		for i := 0; i < 5+int(reach)/3; i++ {
			a := rng.f() * 2 * math.Pi
			d := float64(reach) * rng.rng(0.18, 0.94)
			qx, qy := cgx+int(math.Cos(a)*d), cgy+int(math.Sin(a)*d)
			cw, ch := 3, 3 // a 3×3 garden with a grassy, plantable centre
			if rng.f() < 0.4 {
				cw, ch = 2, 2 // some smaller bare kitchen plots
			}
			fits := true
			for yy := qy; yy < qy+ch && fits; yy++ {
				for xx := qx; xx < qx+cw; xx++ {
					if !inCity(xx, yy) || !canBuild(xx, yy) || l.at(xx, yy).kind != lEmpty {
						fits = false
						break
					}
				}
			}
			if !fits {
				continue
			}
			for yy := qy; yy < qy+ch; yy++ {
				for xx := qx; xx < qx+cw; xx++ {
					l.at(xx, yy).kind = lGarden
				}
			}
			if cw == 3 && ch == 3 { // a grassy centre for a tree/hedge, walled in by the garden
				l.at(qx+1, qy+1).kind = lYard
			}
		}
	}

	// A duck pond beside the green, in some villages — placed before the houses
	// so they build around it.
	if rng.f() < 0.4 {
		pcx := gpx + rng.rng(-2, 2)
		pcy := gpy + rng.rng(2.5, 4.5)
		pr := rng.rng(1.6, 2.6)
		for gy := 0; gy < n; gy++ {
			for gx := 0; gx < n; gx++ {
				if k := l.at(gx, gy).kind; k != lEmpty && k != lYard && k != lGreen {
					continue
				}
				if math.Hypot(float64(gx)-pcx, float64(gy)-pcy) < pr {
					l.at(gx, gy).kind = lPond
				}
			}
		}
	}

	// Wards: a city is a patchwork of quarters, each with its own character — a
	// wealthy merchant quarter of tall tiled townhouses, ordinary common streets, a
	// craftsmen's quarter of workshops and barns, and a poor quarter of packed
	// little cottages. A handful of seed points (a coarse Voronoi) parcel the
	// interior into wards; types are assigned roughly inside-out (wealth toward the
	// centre) with jitter, so adjacent quarters read as distinctly different places
	// — which the building mix, and through it the roof materials, then express.
	type wardType uint8
	const (
		wMerchant wardType = iota
		wCommon
		wCraft
		wPoor
	)
	type wseed struct {
		x, y float64
		t    wardType
	}
	var wards []wseed
	if s.town {
		nW := 5 + rng.n(4)
		wards = make([]wseed, nW)
		for i := range wards {
			a := rng.f() * 2 * math.Pi
			d := reach * rng.rng(0.0, 0.95)
			var t wardType
			switch dd := d / reach; {
			case dd < 0.32: // the wealthy heart
				t = wMerchant
			case dd < 0.62:
				if rng.f() < 0.5 {
					t = wCommon
				} else {
					t = wCraft
				}
			default: // the poor and working edges
				if rng.f() < 0.55 {
					t = wPoor
				} else {
					t = wCraft
				}
			}
			wards[i] = wseed{float64(cgx) + math.Cos(a)*d, float64(cgy) + math.Sin(a)*d, t}
		}
	}
	wardAt := func(gx, gy int) wardType {
		best, bt := math.MaxFloat64, wCommon
		for _, w := range wards {
			if d := math.Hypot(float64(gx)-w.x, float64(gy)-w.y); d < best {
				best, bt = d, w.t
			}
		}
		return bt
	}

	buildR := reach - 4 // buildings stay inside this; the wall encloses them
	base, slope, floor := 0.95, 0.6, 0.22
	if s.town { // a city: nearly every plot built, just thinning a touch outward
		base, slope, floor = 1.35, 0.45, 0.9
	}
	density := func(r float64) float64 {
		p := base - slope*(r/buildR) // dense core, thinning toward the edge
		if p < floor {
			p = floor
		}
		return p
	}
	chooseType := func(r float64, ward wardType, rr *srng) buildType {
		if s.town { // a city's quarters each favour their own kind of building
			switch ward {
			case wMerchant: // the wealthy quarter: tall townhouses and deep merchant houses
				switch rr.n(5) {
				case 0:
					return btHouse
				case 1:
					return btDeephouse
				default:
					return btTownhouse
				}
			case wCraft: // the craftsmen's quarter: barns, longhouses and narrow workshops
				switch rr.n(5) {
				case 0:
					return btBarn
				case 1:
					return btNarrowhouse
				case 2:
					return btRowhouse
				default:
					return btLonghouse
				}
			case wPoor: // the poor quarter: packed narrow-fronted cottages
				switch rr.n(5) {
				case 0:
					return btNarrowhouse
				case 1:
					return btRowhouse
				default:
					return btCottage
				}
			default: // common streets: a burgage mix of frontages and depths
				switch rr.n(6) {
				case 0:
					return btCottage
				case 1:
					return btLonghouse
				case 2:
					return btNarrowhouse
				case 3:
					return btRowhouse
				case 4:
					return btDeephouse
				default:
					return btHouse
				}
			}
		}
		switch {
		case r > buildR*0.62: // village outskirts: small cottages and the odd barn
			if rr.f() < 0.28 {
				return btBarn
			}
			return btCottage
		case r > buildR*0.34: // mid: a mix
			switch rr.n(4) {
			case 0:
				return btLonghouse
			case 1:
				return btCottage
			default:
				return btHouse
			}
		default: // core: the bigger dwellings
			switch rr.n(3) {
			case 0, 1:
				return btLonghouse
			default:
				return btHouse
			}
		}
	}

	// The built-up limit. A village fills a near-disc; a city's edge is lobed and
	// irregular (a few harmonics), so its wall and outskirts wobble like a real
	// medieval town instead of forming a tidy circle. A village's built-up edge is
	// lobed (a few harmonics) so its wall wobbles rather than ringing a circle.
	buildLimit := func(float64) float64 { return buildR }
	if !s.town {
		var amp, ph [3]float64
		for k := 0; k < 3; k++ {
			amp[k] = rng.rng(0.08, 0.2)
			ph[k] = rng.f() * 2 * math.Pi
		}
		buildLimit = func(ang float64) float64 {
			m := 1.0
			for k := 0; k < 3; k++ {
				m += amp[k] * math.Sin(float64(k+1)*ang+ph[k])
			}
			return buildR * m
		}
	}

	// Houses front the streets or ring the green. The front band is wide and its
	// acceptance is jittered per plot, so dwellings sit at varied setbacks (some
	// on the street, some back in their croft) rather than in a tidy line.
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			r := math.Hypot(float64(gx-cgx), float64(gy-cgy))
			if l.at(gx, gy).kind != lEmpty || !canBuild(gx, gy) {
				continue
			}
			greenD := math.Hypot(float64(gx)-gpx, float64(gy)-gpy)
			cellHash := &srng{s: s.id ^ uint64(uint32(gx*73856093^gy*19349663))}
			if s.town {
				// A city packs each block solid between its streets; the wide lanes
				// (carved below) are what separate the blocks, and a scattered garden
				// court breaks up the bigger ones.
				if !inCity(gx, gy) || greenD < squareR+0.6 {
					continue
				}
			} else {
				if r > buildLimit(math.Atan2(float64(gy-cgy), float64(gx-cgx))) {
					continue
				}
				// A village fronts the streets or rings the green, at jittered setbacks.
				roadD := onAnyRoad(float64(gx), float64(gy))
				setback := 2.0 + cellHash.f()*1.6
				if !((roadD >= 0.9 && roadD <= setback) || (greenD >= squareR+0.2 && greenD <= squareR+2.8)) {
					continue
				}
			}
			if cellHash.f() > density(r) {
				continue
			}
			gap := 1 // villages keep alleys between buildings
			if s.town {
				gap = 0 // cities terrace: houses share walls into solid blocks
			}
			placeBuilding(l, canBuild, gx, gy, chooseType(r, wardAt(gx, gy), cellHash), gap)
		}
	}

	// Pack the blocks: fill the gaps left between a city's placed houses with small
	// cottages so each block reads as a solid built mass rather than a scatter of
	// roofs on bare earth. Only cells abutting a building fill, and only a couple of
	// sweeps run — so the fill grows inward from each block's street frontage but
	// the deep centre of a large block is left open as a green court (an emergent
	// perimeter block). Garden courts (lGarden) and lanes are never touched.
	if s.town {
		for sweep := 0; sweep < 3; sweep++ {
			for gy := 0; gy < n; gy++ {
				for gx := 0; gx < n; gx++ {
					if l.at(gx, gy).kind != lEmpty || !inCity(gx, gy) || !canBuild(gx, gy) {
						continue
					}
					abuts := false
					for _, d := range nb8 {
						if nx, ny := gx+d[0], gy+d[1]; l.in(nx, ny) && occupiedBuilding(l.at(nx, ny).kind) {
							abuts = true
							break
						}
					}
					if abuts {
						// Vary the infill so a packed block isn't a grid of identical
						// cottages: try a larger house where it still fits, falling back
						// to a cottage in the tighter gaps.
						h := uint32(gx)*73856093 ^ uint32(gy)*19349663
						placed := false
						switch h % 6 {
						case 0:
							placed = placeBuilding(l, canBuild, gx, gy, btRowhouse, 0)
						case 1:
							placed = placeBuilding(l, canBuild, gx, gy, btLonghouse, 0)
						case 2:
							placed = placeBuilding(l, canBuild, gx, gy, btHouse, 0)
						case 3:
							placed = placeBuilding(l, canBuild, gx, gy, btNarrowhouse, 0)
						}
						if !placed {
							placeBuilding(l, canBuild, gx, gy, btCottage, 0)
						}
					}
				}
			}
		}
	}

	// Wall: trace the outer edge of the built-up blob. Take every settlement cell
	// (houses, lanes, square, well), fill the gaps and any enclosed water so the
	// interior is solid, then put the wall on the ring of ground just outside it.
	// This hugs the real, irregular footprint — jagged, and able to follow a
	// concave bay — instead of smoothing it into a near-circular polygon.
	// A settlement built across water (a stream or pond it surrounds) treats that
	// water as part of its interior: a narrow channel with built ground on both
	// opposite banks is "spanned", so the wall encloses it rather than running
	// along the bank, and a plank bridge later crosses it.
	const bridgeSpan = 7
	isBuilt := func(gx, gy int) bool {
		if !l.in(gx, gy) {
			return false
		}
		switch l.at(gx, gy).kind {
		case lBuildAnchor, lBuildBody, lRoad, lStreet, lGate, lWell, lPlaza, lPaved,
			lGreen, lCourtyard, lWall, lTower:
			return true
		}
		return false
	}
	isOpenWater := func(gx, gy int) bool {
		if !l.in(gx, gy) {
			return false
		}
		c := l.at(gx, gy)
		return c.kind == lEmpty && (c.biome == Water || c.biome == Deep)
	}
	// spannedAxis: built ground lies within bridgeSpan in +dir and again in −dir of
	// the water cell, with only water between — i.e. the city straddles it here.
	spannedAxis := func(gx, gy, dxs, dys int) bool {
		bank := func(sgn int) bool {
			for k := 1; k <= bridgeSpan; k++ {
				cx, cy := gx+dxs*k*sgn, gy+dys*k*sgn
				if isOpenWater(cx, cy) {
					continue
				}
				return isBuilt(cx, cy)
			}
			return false
		}
		return bank(1) && bank(-1)
	}
	spanned := make([]bool, n*n)
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			if isOpenWater(gx, gy) && (spannedAxis(gx, gy, 1, 0) || spannedAxis(gx, gy, 0, 1)) {
				spanned[gy*n+gx] = true
			}
		}
	}

	core := make([]bool, n*n)
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			switch l.at(gx, gy).kind {
			case lBuildAnchor, lBuildBody, lRoad, lStreet, lGate, lWell, lPlaza, lGreen, lPond,
				lCourtyard, lWall, lTower: // the citadel counts as built, too
				core[gy*n+gx] = true
			}
			if spanned[gy*n+gx] { // water the city straddles counts as interior
				core[gy*n+gx] = true
			}
		}
	}
	region := make([]bool, n*n) // core dilated one cell, so the wall sits just off the houses
	copy(region, core)
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			if core[gy*n+gx] {
				continue
			}
			for _, d := range nb4 {
				if nx, ny := gx+d[0], gy+d[1]; l.in(nx, ny) && core[ny*n+nx] {
					region[gy*n+gx] = true
					break
				}
			}
		}
	}
	inside := make([]bool, n*n) // fill holes: cells not reachable from the border are enclosed
	ext := make([]bool, n*n)
	var stk [][2]int
	for i := 0; i < n; i++ {
		for _, p := range [][2]int{{i, 0}, {i, n - 1}, {0, i}, {n - 1, i}} {
			if !region[p[1]*n+p[0]] && !ext[p[1]*n+p[0]] {
				ext[p[1]*n+p[0]] = true
				stk = append(stk, p)
			}
		}
	}
	for len(stk) > 0 {
		p := stk[len(stk)-1]
		stk = stk[:len(stk)-1]
		for _, d := range nb4 {
			nx, ny := p[0]+d[0], p[1]+d[1]
			if l.in(nx, ny) && !region[ny*n+nx] && !ext[ny*n+nx] {
				ext[ny*n+nx] = true
				stk = append(stk, [2]int{nx, ny})
			}
		}
	}
	for i := range inside {
		inside[i] = !ext[i]
	}
	insideAt := func(gx, gy int) bool { return l.in(gx, gy) && inside[gy*n+gx] }

	wallKind := lFence // timber palisade
	if s.town {
		wallKind = lWall // stone curtain wall
	}
	// The wall ring: ground just outside the inside region. 8-adjacency leaves no
	// diagonal gap to slip through.
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			if inside[gy*n+gx] || l.at(gx, gy).kind != lEmpty || !canBuild(gx, gy) {
				continue
			}
			for _, d := range nb8 {
				if nx, ny := gx+d[0], gy+d[1]; insideAt(nx, ny) {
					l.at(gx, gy).kind = wallKind
					break
				}
			}
		}
	}
	// Punch gates where a lane meets the wall.
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			if k := l.at(gx, gy).kind; k != lRoad && k != lStreet {
				continue
			}
			edge := false
			for _, d := range nb8 {
				if nx, ny := gx+d[0], gy+d[1]; l.in(nx, ny) && !inside[ny*n+nx] {
					edge = true
					break
				}
			}
			if !edge {
				continue
			}
			for _, d := range nb8 {
				if nx, ny := gx+d[0], gy+d[1]; l.in(nx, ny) && l.at(nx, ny).kind == wallKind {
					l.at(nx, ny).kind = lGate
				}
			}
		}
	}
	autotileFences(l)

	// A city's wall is studded with towers — flanking the gates and scattered,
	// spaced, along the curtain.
	if s.town {
		for gy := 0; gy < n; gy++ {
			for gx := 0; gx < n; gx++ {
				if l.at(gx, gy).kind != lWall {
					continue
				}
				tower := unit(hashCoord(s.id^0x70E7, l.ox+gx, l.oy+gy)) < 0.05
				for _, d := range nb8 {
					if nx, ny := gx+d[0], gy+d[1]; l.in(nx, ny) && l.at(nx, ny).kind == lGate {
						tower = true
					}
				}
				if !tower {
					continue
				}
				clash := false // keep towers from clumping
				for dy := -2; dy <= 2 && !clash; dy++ {
					for dx := -2; dx <= 2; dx++ {
						if (dx != 0 || dy != 0) && l.in(gx+dx, gy+dy) && l.at(gx+dx, gy+dy).kind == lTower {
							clash = true
							break
						}
					}
				}
				if !clash {
					l.at(gx, gy).kind = lTower
				}
			}
		}
	}

	// Bridges: where a street runs up to the bank of the water the city straddles,
	// lay a slender plank bridge straight across to walkable ground on the far
	// side, so the two banks connect. Started only from streets/roads (so crossings
	// sit at the ends of through-routes) and landed only on walkable ground (never
	// a building or the wall) so a bridge is never a stub. Each new crossing keeps
	// clear of existing bridges, so a wide channel gets a few discrete spans rather
	// than its whole bank decked over into a causeway.
	isWalkBank := func(gx, gy int) bool {
		if !l.in(gx, gy) || isOpenWater(gx, gy) {
			return false
		}
		switch l.at(gx, gy).kind {
		case lBuildAnchor, lBuildBody, lWall, lTower, lFence, lPond, lQuarryRock, lWell:
			return false // a blocker — not a bank to land on
		case lStreet, lRoad, lGate, lPlaza, lPaved, lGreen, lCourtyard, lYard:
			return true
		}
		return insideAt(gx, gy) // inside, open ground becomes walkable yard later
	}
	noBridgeNear := func(gx, gy, rad int) bool {
		for dy := -rad; dy <= rad; dy++ {
			for dx := -rad; dx <= rad; dx++ {
				if l.in(gx+dx, gy+dy) && l.at(gx+dx, gy+dy).kind == lBridge {
					return false
				}
			}
		}
		return true
	}
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			switch l.at(gx, gy).kind {
			case lStreet, lRoad, lGate, lPlaza:
			default:
				continue
			}
			for _, d := range nb4 {
				if !isOpenWater(gx+d[0], gy+d[1]) || !noBridgeNear(gx+d[0], gy+d[1], 4) {
					continue
				}
				k := 1
				for ; k <= bridgeSpan; k++ {
					if !isOpenWater(gx+d[0]*k, gy+d[1]*k) {
						break
					}
				}
				if k <= bridgeSpan && isWalkBank(gx+d[0]*k, gy+d[1]*k) {
					orient := uint8(0) // 0 = deck runs east–west, 1 = north–south
					if d[1] != 0 {
						orient = 1
					}
					for j := 1; j < k; j++ {
						c := l.at(gx+d[0]*j, gy+d[1]*j)
						c.kind, c.fv = lBridge, orient
					}
				}
			}
		}
	}

	// Outlying worksites — a quarry on nearby hills, a lumber camp at the forest
	// edge, a fishing hut on the shore — each placed in fitting terrain past the
	// wall and linked back through a gate by a spur road.
	placeOutbuildings(l, canBuild, insideAt, cgx, cgy, reachI, outpost, s.id)

	// Fields along the approach: open ground just outside the wall on the road axis.
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			if l.at(gx, gy).kind != lEmpty || !canBuild(gx, gy) || insideAt(gx, gy) {
				continue
			}
			r := math.Hypot(float64(gx-cgx), float64(gy-cgy))
			if r > reach+4 {
				continue
			}
			if dline := distToAxis(float64(gx), float64(gy), cfx, cfy, dx, dy); dline < 6 {
				l.at(gx, gy).kind = lField
			}
		}
	}

	// Everything left inside the wall becomes ground. A village is trodden grassy
	// yard throughout. A city cobbles the strip along its lanes (the street surface
	// and the little yards between the terraced houses) but leaves the depth of each
	// block — the ground enclosed behind the perimeter rows — as a grassy court, so
	// the greenery and garden passes plant it into the block's private garden.
	onStreetEdge := func(gx, gy int) bool { // directly fronting a carved lane/square
		for _, d := range nb4 {
			if nx, ny := gx+d[0], gy+d[1]; l.in(nx, ny) {
				switch l.at(nx, ny).kind {
				case lStreet, lGate, lPlaza:
					return true
				}
			}
		}
		return false
	}
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			c := l.at(gx, gy)
			if c.kind != lEmpty || !insideAt(gx, gy) || !canBuild(gx, gy) {
				continue
			}
			if s.town && onStreetEdge(gx, gy) {
				c.kind = lPaved // a thin paved shoulder along the lane
			} else {
				c.kind = lYard // everything behind it is a green court
			}
		}
	}

	// City courtyards: a city's interior isn't all bare paving. Wide-open pockets
	// of paving — the breathing space behind the terraced blocks and the green
	// strip just inside the wall — become grassy yards, so the gardens and greenery
	// passes below plant them like a village's crofts. Narrow through-lanes (which
	// always touch a building in their 3×3) stay cobbled, so this also reads the
	// circulation apart from the open ground.
	if s.town {
		openGround := func(gx, gy int) bool {
			if !l.in(gx, gy) {
				return false
			}
			switch l.at(gx, gy).kind {
			case lPaved, lYard, lPlaza, lGreen, lCourtyard:
				return true
			}
			return false
		}
		seed := make([]bool, n*n) // paved cells sitting deep in an open pocket
		for gy := 1; gy < n-1; gy++ {
			for gx := 1; gx < n-1; gx++ {
				if l.at(gx, gy).kind != lPaved {
					continue
				}
				all := true
				for _, d := range nb8 {
					if !openGround(gx+d[0], gy+d[1]) {
						all = false
						break
					}
				}
				seed[gy*n+gx] = all
			}
		}
		for gy := 0; gy < n; gy++ {
			for gx := 0; gx < n; gx++ {
				if !seed[gy*n+gx] {
					continue
				}
				if c := l.at(gx, gy); c.kind == lPaved {
					c.kind = lYard
				}
				for _, d := range nb8 { // pull the pocket's rim grassy too
					if nx, ny := gx+d[0], gy+d[1]; l.in(nx, ny) && l.at(nx, ny).kind == lPaved {
						l.at(nx, ny).kind = lYard
					}
				}
			}
		}
	}

	// Kitchen gardens: small 2×2 cultivated patches tucked into the yards (the
	// crofts behind the houses), so the inside of the village isn't bare earth.
	yard2x2 := func(gx, gy int) bool {
		for yy := gy; yy < gy+2; yy++ {
			for xx := gx; xx < gx+2; xx++ {
				if !l.in(xx, yy) || l.at(xx, yy).kind != lYard {
					return false
				}
			}
		}
		return true
	}
	for gy := 0; gy < n-1; gy++ {
		for gx := 0; gx < n-1; gx++ {
			if yard2x2(gx, gy) && unit(hashCoord(s.id^0x6A5D, l.ox+gx, l.oy+gy)) < 0.05 {
				for yy := gy; yy < gy+2; yy++ {
					for xx := gx; xx < gx+2; xx++ {
						l.at(xx, yy).kind = lGarden
					}
				}
			}
		}
	}

	// Greenery: scatter bushes, flowers and grass tufts across the remaining
	// yards so the village reads as lived-in (gardens, hedges) rather than a
	// swept dirt commons.
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			c := l.at(gx, gy)
			if c.kind != lYard {
				continue
			}
			switch h := unit(hashCoord(s.id^0xDEC04, l.ox+gx, l.oy+gy)); {
			case h < 0.03:
				c.decor = 4 // an orchard/yard tree
			case h < 0.13:
				c.decor = 1 // bush
			case h < 0.25:
				c.decor = 2 // flower
			case h < 0.43:
				c.decor = 3 // grass tuft
			}
		}
	}

	// Braziers light a city after dark — one flanking each gate, plus a scatter
	// across the market square and the citadel bailey. Placed only on open paving
	// (never a narrow street) so they never block a lane.
	if s.town {
		open := func(gx, gy int) bool {
			if !l.in(gx, gy) {
				return false
			}
			switch l.at(gx, gy).kind {
			case lPlaza, lPaved, lCourtyard:
				return true
			}
			return false
		}
		clear := func(gx, gy, rad int) bool {
			for dy := -rad; dy <= rad; dy++ {
				for dx := -rad; dx <= rad; dx++ {
					if l.in(gx+dx, gy+dy) && l.at(gx+dx, gy+dy).kind == lBrazier {
						return false
					}
				}
			}
			return true
		}
		for gy := 0; gy < n; gy++ {
			for gx := 0; gx < n; gx++ {
				if l.at(gx, gy).kind != lGate {
					continue
				}
				for _, d := range nb8 {
					if nx, ny := gx+d[0], gy+d[1]; open(nx, ny) && clear(nx, ny, 3) {
						l.at(nx, ny).kind = lBrazier
						break
					}
				}
			}
		}
		for gy := 0; gy < n; gy++ {
			for gx := 0; gx < n; gx++ {
				if k := l.at(gx, gy).kind; (k == lPlaza || k == lCourtyard) &&
					unit(hashCoord(s.id^0xB7A21E5, gx, gy)) < 0.05 && clear(gx, gy, 4) {
					l.at(gx, gy).kind = lBrazier
				}
			}
		}
		// Market stalls cluster on the square: a scatter of awninged stalls on the
		// open plaza, kept one tile apart so aisles stay walkable between them.
		noStallNear := func(gx, gy int) bool {
			for _, d := range nb8 {
				if nx, ny := gx+d[0], gy+d[1]; l.in(nx, ny) && l.at(nx, ny).kind == lStall {
					return false
				}
			}
			return true
		}
		for gy := 0; gy < n; gy++ {
			for gx := 0; gx < n; gx++ {
				if l.at(gx, gy).kind == lPlaza &&
					unit(hashCoord(s.id^0x57A11, gx, gy)) < 0.22 && noStallNear(gx, gy) {
					l.at(gx, gy).kind = lStall
				}
			}
		}
	}
	return l
}

// placeBuilding stamps a building with bottom-left anchor at (gx,gy) if its
// footprint fits on buildable, empty ground. gap is the clear margin required to
// other buildings: villages use 1 (alleys/crofts); city terraces use 0 so houses
// share walls into solid blocks.
func placeBuilding(l *layout, canBuild func(int, int) bool, gx, gy int, bt buildType, gap int) bool {
	w, h := footprint(bt)
	x0, y0 := gx, gy-(h-1)
	for yy := y0; yy <= gy; yy++ { // the footprint itself
		for xx := x0; xx <= gx+w-1; xx++ {
			if !l.in(xx, yy) || !canBuild(xx, yy) {
				return false
			}
			if k := l.at(xx, yy).kind; k != lEmpty && k != lYard {
				return false
			}
		}
	}
	for yy := y0 - gap; yy <= gy+gap; yy++ { // the gap ring around it
		for xx := x0 - gap; xx <= gx+w-1+gap; xx++ {
			inFoot := xx >= x0 && xx <= gx+w-1 && yy >= y0 && yy <= gy
			if !inFoot && l.in(xx, yy) && occupiedBuilding(l.at(xx, yy).kind) {
				return false // keep the required gap to other buildings
			}
		}
	}
	for yy := y0; yy <= gy; yy++ {
		for xx := x0; xx <= gx+w-1; xx++ {
			l.at(xx, yy).kind = lBuildBody
		}
	}
	a := l.at(gx, y0) // anchor at bottom-left
	a.kind, a.bt = lBuildAnchor, bt
	return true
}

// placeOutbuildings sites the village's resource buildings in the surrounding
// terrain and links each back to the village by a spur road. Each is optional —
// it only appears if its terrain (hills / forest / water) lies within reach.
func placeOutbuildings(l *layout, canBuild func(int, int) bool, insideAt func(int, int) bool, cgx, cgy, reach, outpost int, seed uint64) {
	n := l.n

	// find returns the matching-biome cell nearest the village but outside its wall.
	find := func(want func(Biome) bool) (int, int, bool) {
		bx, by, bd, ok := 0, 0, math.MaxFloat64, false
		for gy := 2; gy < n-2; gy++ {
			for gx := 2; gx < n-2; gx++ {
				if !want(l.at(gx, gy).biome) || insideAt(gx, gy) {
					continue
				}
				ddx, ddy := float64(gx-cgx), float64(gy-cgy)
				r := math.Hypot(ddx, ddy)
				if r > float64(outpost) {
					continue
				}
				if r < bd {
					bx, by, bd, ok = gx, gy, r, true
				}
			}
		}
		return bx, by, ok
	}
	// hutNear finds open, buildable land next to (tx,ty) for the worksite's hut.
	hutNear := func(tx, ty int) (int, int, bool) {
		for _, o := range [][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {2, 0}, {-2, 0}, {0, 2}, {0, -2}, {1, 1}, {-1, -1}} {
			gx, gy := tx+o[0], ty+o[1]
			if l.in(gx, gy) && l.at(gx, gy).kind == lEmpty && canBuild(gx, gy) {
				return gx, gy, true
			}
		}
		return 0, 0, false
	}
	// carveSpur runs a road from a worksite back to the village, opening a gate
	// where it crosses the wall; once it reaches the inside, the lanes take over.
	carveSpur := func(x0, y0 int) {
		bresenham(x0, y0, cgx, cgy, func(gx, gy int) {
			if !l.in(gx, gy) || insideAt(gx, gy) {
				return
			}
			c := l.at(gx, gy)
			switch c.kind {
			case lFence, lWall:
				c.kind = lGate
			case lEmpty, lField:
				if c.biome != Water && c.biome != Mountain && c.biome != Deep {
					c.kind = lRoad
				}
			}
		})
	}
	// stampInto converts the worksite's own ground, but only where it bites into
	// the *natural* biome (rock for a quarry, trees for a clearing) — so the site
	// hugs the real biome edge instead of stamping a synthetic patch on the grass.
	stampInto := func(tx, ty, rad int, want func(Biome) bool, to lkind) {
		for gy := ty - rad; gy <= ty+rad; gy++ {
			for gx := tx - rad; gx <= tx+rad; gx++ {
				if l.in(gx, gy) && l.at(gx, gy).kind == lEmpty && want(l.at(gx, gy).biome) {
					l.at(gx, gy).kind = to
				}
			}
		}
	}

	isRock := func(b Biome) bool { return b == Hill || b == Mountain }
	isForest := func(b Biome) bool { return b == Forest }
	isWater := func(b Biome) bool { return b == Water || b == Deep }

	// Quarry — cut into the nearest rock (hills or mountain): a stone floor with
	// boulders at the face, a hut on the meadow side, a path back.
	if tx, ty, ok := find(isRock); ok {
		hx, hy, hok := hutNear(tx, ty)
		if hok {
			placeBuilding(l, canBuild, hx, hy, btCottage, 1)
		}
		stampInto(tx, ty, 2, isRock, lQuarry)
		l.at(tx, ty).kind = lQuarryRock
		for _, o := range [][2]int{{1, 0}, {0, 1}} {
			if gx, gy := tx+o[0], ty+o[1]; l.in(gx, gy) && l.at(gx, gy).kind == lQuarry {
				l.at(gx, gy).kind = lQuarryRock
			}
		}
		spurFrom(carveSpur, hx, hy, tx, ty, hok)
	}
	// Lumber camp — a clearing bitten into the nearest forest, with felled stumps,
	// a store on the meadow side, a path back.
	if tx, ty, ok := find(isForest); ok {
		hx, hy, hok := hutNear(tx, ty)
		if hok && !placeBuilding(l, canBuild, hx, hy, btBarn, 1) {
			placeBuilding(l, canBuild, hx, hy, btCottage, 1)
		}
		stampInto(tx, ty, 2, isForest, lClearing)
		for _, o := range [][2]int{{0, 0}, {-1, -1}, {1, 0}, {0, 1}} {
			if gx, gy := tx+o[0], ty+o[1]; l.in(gx, gy) && l.at(gx, gy).kind == lClearing {
				l.at(gx, gy).kind = lStump
			}
		}
		spurFrom(carveSpur, hx, hy, tx, ty, hok)
	}
	// Fishing hut — on the shore by the nearest water, a jetty reaching out over it.
	if tx, ty, ok := find(isWater); ok {
		if hx, hy, hok := hutNear(tx, ty); hok {
			placeBuilding(l, canBuild, hx, hy, btCottage, 1)
			jetty := func(ax, ay, bx, by int) {
				bresenham(ax, ay, bx, by, func(gx, gy int) {
					if l.in(gx, gy) && isWater(l.at(gx, gy).biome) && l.at(gx, gy).kind == lEmpty {
						l.at(gx, gy).kind = lJetty
					}
				})
			}
			jetty(hx, hy, tx, ty)
			jetty(tx, ty, tx+(tx-hx), ty+(ty-hy)) // a little further out
			carveSpur(hx, hy)
		}
	}

	// Outlying farmsteads: granges scattered on the open land around the town, each
	// a barn and cottage with a patch of field and a cart track back to the nearest
	// gate. Unlike the quarry, wood and fishery these need no special terrain, so
	// every town has roads running out to its fields — not just the lucky few with
	// rock, forest or water close at hand.
	openLandNear := func(tx, ty int) (int, int, bool) {
		bx, by, bd, ok := 0, 0, math.MaxFloat64, false
		for gy := ty - 6; gy <= ty+6; gy++ {
			for gx := tx - 6; gx <= tx+6; gx++ {
				if !l.in(gx, gy) || l.at(gx, gy).kind != lEmpty || !canBuild(gx, gy) || insideAt(gx, gy) {
					continue
				}
				if d := math.Hypot(float64(gx-tx), float64(gy-ty)); d < bd {
					bx, by, bd, ok = gx, gy, d, true
				}
			}
		}
		return bx, by, ok
	}
	fr := &srng{s: seed ^ 0xFA27C0DE}
	nFarms := 2 + fr.n(2) // two or three steadings
	for i := 0; i < nFarms; i++ {
		a := fr.f() * 2 * math.Pi
		rr := float64(reach) + fr.rng(4, float64(outpost-reach))
		tx := cgx + int(math.Cos(a)*rr)
		ty := cgy + int(math.Sin(a)*rr)
		bx, by, ok := openLandNear(tx, ty)
		if !ok || !placeBuilding(l, canBuild, bx, by, btBarn, 1) {
			continue
		}
		if cx, cy, ok2 := openLandNear(bx+3, by); ok2 { // a cottage beside the barn
			placeBuilding(l, canBuild, cx, cy, btCottage, 1)
		}
		for gy := by - 2; gy <= by+2; gy++ { // a patch of field around the steading
			for gx := bx - 2; gx <= bx+3; gx++ {
				if l.in(gx, gy) && l.at(gx, gy).kind == lEmpty && canBuild(gx, gy) && !insideAt(gx, gy) && fr.f() < 0.7 {
					l.at(gx, gy).kind = lField
				}
			}
		}
		carveSpur(bx, by)
	}
}

// spurFrom carves the worksite's path from its hut if it has one, else from the
// worksite itself.
func spurFrom(carve func(int, int), hx, hy, tx, ty int, fromHut bool) {
	if fromHut {
		carve(hx, hy)
	} else {
		carve(tx, ty)
	}
}

func occupiedBuilding(k lkind) bool { return k == lBuildAnchor || k == lBuildBody }

// autotileFences picks each fence segment's orientation from its fence/gate
// neighbours, so straight runs read as rails and corners/junctions as posts.
func autotileFences(l *layout) {
	isWall := func(gx, gy int) bool {
		if !l.in(gx, gy) {
			return false
		}
		k := l.at(gx, gy).kind
		return k == lFence || k == lGate
	}
	for gy := 0; gy < l.n; gy++ {
		for gx := 0; gx < l.n; gx++ {
			if l.at(gx, gy).kind != lFence {
				continue
			}
			h := isWall(gx-1, gy) || isWall(gx+1, gy)
			v := isWall(gx, gy-1) || isWall(gx, gy+1)
			switch {
			case h && !v:
				l.at(gx, gy).fv = 0 // horizontal rail
			case v && !h:
				l.at(gx, gy).fv = 1 // vertical rail
			default:
				l.at(gx, gy).fv = 2 // corner / junction / post
			}
		}
	}
}

// ── querying ──────────────────────────────────────────────────────────────────

// settlementAt reports the cell at (x,y) if it lies within a settlement's layout
// or on a connecting road between villages.
func (g *Generator) settlementAt(x, y int) (Cell, bool) {
	mx, my := floorDiv(x, settleCell), floorDiv(y, settleCell)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			s := g.settlementFor(mx+dx, my+dy)
			if !s.valid {
				continue
			}
			l := g.layoutOf(s)
			gx, gy := x-l.ox, y-l.oy
			if !l.in(gx, gy) {
				continue
			}
			if c := l.at(gx, gy); c.kind != lEmpty {
				return cellFor(c), true
			}
		}
	}
	return g.connectingRoad(x, y)
}

// ── claimable plots ──────────────────────────────────────────────────────────

// Plot identifies a claimable building parcel inside a settlement: one of the
// town's pre-drawn buildings, keyed stably by its settlement and its anchor so a
// stored claim (docs/CLAIMS_PLAN.md) can re-find the same ground deterministically
// across runs. It reports structure only — the build-rights margin around a plot
// is the game layer's policy, not worldgen's.
type Plot struct {
	ID         string // "<settlement-hex>:<ax>,<ay>" — stable across runs
	Settlement string // settlement identity hex, for grouping / naming
	Town       bool   // a stone city (vs a timber village)
	Kind       string // building kind, e.g. "Cottage", "Townhouse", "Church"
	AX, AY     int    // anchor (north-west corner) world coords
	W, H       int    // footprint size in tiles
}

// PlotAt returns the building plot whose footprint covers (x,y), if any. It scans
// the same ±1 macro neighbourhood as settlementAt (a building can belong to a
// neighbouring macro-cell's layout) and recovers the building's anchor — so a
// query on any footprint cell, anchor or body, yields the same plot. ok is false
// for open ground, roads, walls, worksites or anywhere outside a settlement.
func (g *Generator) PlotAt(x, y int) (Plot, bool) {
	mx, my := floorDiv(x, settleCell), floorDiv(y, settleCell)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			s := g.settlementFor(mx+dx, my+dy)
			if !s.valid {
				continue
			}
			l := g.layoutOf(s)
			gx, gy := x-l.ox, y-l.oy
			if !l.in(gx, gy) {
				continue
			}
			if k := l.at(gx, gy).kind; k != lBuildAnchor && k != lBuildBody {
				continue
			}
			agx, agy, bt, ok := anchorCovering(l, gx, gy)
			if !ok {
				continue
			}
			w, h := footprint(bt)
			ax, ay := l.ox+agx, l.oy+agy
			return Plot{
				ID:         fmt.Sprintf("%x:%d,%d", s.id, ax, ay),
				Settlement: fmt.Sprintf("%x", s.id),
				Town:       s.town,
				Kind:       buildTypeName(bt),
				AX:         ax, AY: ay,
				W: w, H: h,
			}, true
		}
	}
	return Plot{}, false
}

// anchorCovering finds the building anchor whose footprint covers grid cell
// (gx,gy). The anchor is the north-west corner and footprints never overlap, so a
// small window scan (bounded by the largest footprint) finds the one match.
func anchorCovering(l *layout, gx, gy int) (ax, ay int, bt buildType, ok bool) {
	const maxW, maxH = 3, 4 // the cathedral, the biggest footprint, is 3×4
	for cy := gy - (maxH - 1); cy <= gy; cy++ {
		for cx := gx - (maxW - 1); cx <= gx; cx++ {
			if !l.in(cx, cy) {
				continue
			}
			c := l.at(cx, cy)
			if c.kind != lBuildAnchor {
				continue
			}
			w, h := footprint(c.bt)
			if gx >= cx && gx < cx+w && gy >= cy && gy < cy+h {
				return cx, cy, c.bt, true
			}
		}
	}
	return 0, 0, btNone, false
}

// buildTypeName is the display name for a building kind (for a claim's address).
func buildTypeName(bt buildType) string {
	switch bt {
	case btCottage:
		return "Cottage"
	case btHouse:
		return "House"
	case btLonghouse:
		return "Longhouse"
	case btBarn:
		return "Barn"
	case btChurch:
		return "Church"
	case btKeep:
		return "Keep"
	case btCathedral:
		return "Cathedral"
	case btTownhouse:
		return "Townhouse"
	case btMarketHall:
		return "Market Hall"
	case btSmithy:
		return "Smithy"
	case btTavern:
		return "Tavern"
	case btRowhouse:
		return "Rowhouse"
	case btNarrowhouse:
		return "Narrow House"
	case btDeephouse:
		return "Deep House"
	default:
		return "Building"
	}
}

// connectingRoad draws the road leaving a village toward its nearest neighbour;// far neighbours dwindle to a faded trail rather than a solid road.
func (g *Generator) connectingRoad(x, y int) (Cell, bool) {
	mx, my := floorDiv(x, settleCell), floorDiv(y, settleCell)
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			s := g.settlementFor(mx+dx, my+dy)
			if !s.valid {
				continue
			}
			for _, p := range g.partnersOf(s) { // every road this town sends out
				// Canonicalise the edge so the winding curve is identical whichever of
				// the two towns we happen to be rendering from (partnerships are mutual).
				ax, ay, bx, by := float64(s.cx), float64(s.cy), float64(p.cx), float64(p.cy)
				if p.cx < s.cx || (p.cx == s.cx && p.cy < s.cy) {
					ax, ay, bx, by = bx, by, ax, ay
				}
				abx, aby := bx-ax, by-ay
				L := math.Hypot(abx, aby)
				if L < 1 {
					continue
				}
				dxn, dyn := abx/L, aby/L // unit along, and its perpendicular
				pxn, pyn := -dyn, dxn
				qx, qy := float64(x)-ax, float64(y)-ay
				t := qx*dxn + qy*dyn  // distance along the baseline
				sd := qx*pxn + qy*pyn // signed perpendicular distance
				if t < 0 || t > L {
					continue
				}
				if math.Abs(sd-g.roadCurveOffset(ax, ay, dxn, dyn, t, L)) > 0.9 {
					continue
				}
				// Fade out toward the middle of a very long road (rare: network edges
				// are short enough to stay solid). along is the nearer town's distance.
				if along := math.Min(t, L-t); along > linkFade {
					if unit(hashCoord(g.seed^0xC0FFEE11, x, y)) > 0.35 {
						continue
					}
				}
				b := g.biomeAt(x, y)
				if b == Water || b == Mountain || b == Deep {
					continue
				}
				return Cell{Biome: Path, Glyph: '·', Color: "#8C7A56", Walkable: true}, true
			}
		}
	}
	return Cell{}, false
}

// roadSalt separates the road-meander noise field from the terrain noise.
const roadSalt uint64 = 0x20AD5EEDC0FFEE01

// roadCurveOffset is the winding road's perpendicular offset (in tiles) at
// distance t along a canonical A→B baseline of length L. The meander is driven
// by the world's own low-frequency noise field — sampled at the baseline point —
// so a road bends with the lie of the land rather than running ruler-straight,
// and it's windowed to zero at both ends so the road approaches each town head-on
// (meeting the gate its main street already points at) and only wanders in the
// open country between.
func (g *Generator) roadCurveOffset(ax, ay, dxn, dyn, t, L float64) float64 {
	bx, by := ax+dxn*t, ay+dyn*t                   // the baseline point at t
	n := g.fbmAt(bx, by, roadSalt, 0.022, 2)*2 - 1 // [-1,1], coherent with the map
	amp := 0.16 * L                                // longer roads wander more…
	if amp > 13 {
		amp = 13 // …but only so far
	}
	return amp * math.Sin(math.Pi*t/L) * n // window → 0 at both towns
}

// cellFor converts a generated layout cell into a worldgen Cell.
func cellFor(c *lcell) Cell {
	ground := groundColorFor(c.biome)
	switch c.kind {
	case lYard:
		y := Cell{Biome: c.biome, Glyph: '·', Color: ground, Walkable: true}
		switch c.decor { // scattered greenery — orchard trees, hedges, flowers
		case 1:
			y.Glyph, y.Color = 'o', "#3E8F57" // bush
		case 2:
			y.Glyph, y.Color = '*', "#FF6B6B" // flower
		case 3:
			y.Glyph, y.Color = ',', "#4F9460" // grass tuft
		case 4:
			y.Glyph, y.Color, y.Walkable = 'Y', "#2F7D4F", false // an orchard tree (blocks)
		}
		return y
	case lGreen:
		return Cell{Biome: Grass, Glyph: '·', Color: "#6FBE6A", Walkable: true}
	case lRoad, lGate:
		return Cell{Biome: Path, Glyph: '·', Color: "#8C7A56", Walkable: true}
	case lStreet:
		return Cell{Biome: Path, Glyph: '·', Color: "#BBB29B", Walkable: true} // pale cobbled street
	case lWell:
		return Cell{Biome: c.biome, Glyph: 'W', Color: "#9AA7B0"}
	case lFence:
		return Cell{Biome: c.biome, Glyph: '=', Color: "#5A3D22", Variant: c.fv}
	case lField:
		return Cell{Biome: Grass, Glyph: '"', Color: "#86974A", Walkable: true}
	case lGarden:
		return Cell{Biome: Grass, Glyph: '"', Color: "#7FA64B", Walkable: true}
	case lPond:
		return Cell{Biome: Water, Glyph: '~', Color: "#3F9AE0",
			AnimA: "#2E6BD0", AnimB: "#5BB0E0", Frames: []rune{'~', '≈', '~', '≋'}}
	case lQuarry:
		return Cell{Biome: Mountain, Glyph: '·', Color: "#9AA0A8", Walkable: true}
	case lQuarryRock:
		return Cell{Biome: Hill, Glyph: '▲', Color: "#8A8170"} // boulder (blocks)
	case lClearing:
		return Cell{Biome: Path, Glyph: '·', Color: "#8C7A56", Walkable: true}
	case lStump:
		return Cell{Biome: Path, Glyph: 'u', Color: "#6B4A2B", Walkable: true}
	case lJetty:
		return Cell{Biome: Path, Glyph: '·', Color: "#7A5A38", Walkable: true} // dock planks
	case lBridge:
		// A plank bridge over water; Variant carries the deck's run (0 east–west,
		// 1 north–south) so the sprite lays its planks across the span.
		return Cell{Biome: Path, Glyph: 'b', Color: "#8A5A30", Walkable: true, Variant: c.fv}
	case lWall:
		return Cell{Biome: Hill, Glyph: '#', Color: "#8E9099"} // stone curtain wall (blocks)
	case lTower:
		return Cell{Biome: Hill, Glyph: 'I', Color: "#9A9CA6"} // stone tower (blocks)
	case lPlaza:
		return Cell{Biome: Path, Glyph: '·', Color: "#A89B82", Walkable: true} // cobbled square
	case lPaved:
		return Cell{Biome: Path, Glyph: '·', Color: "#83785F", Walkable: true} // darker earth between buildings
	case lCourtyard:
		return Cell{Biome: Path, Glyph: '·', Color: "#8F8576", Walkable: true} // castle bailey
	case lBrazier:
		return Cell{Biome: Path, Glyph: 'i', Color: "#FF7A1E"} // a street brazier (blocks, glows at night)
	case lStall:
		return Cell{Biome: Path, Glyph: 's', Color: "#C24A3A"} // a market stall (blocks)
	case lBuildAnchor:
		return Cell{Biome: c.biome, Glyph: buildingGlyph(c.bt), Color: buildingColor(c.bt, c.biome), Variant: uint8(c.bt)}
	case lBuildBody:
		return Cell{Biome: c.biome, Glyph: '%', Color: groundColorFor(c.biome)}
	}
	return Cell{}
}

func buildingGlyph(bt buildType) rune {
	switch bt {
	case btCathedral, btChurch:
		return 'C'
	case btKeep:
		return 'K'
	case btMarketHall:
		return 'M'
	case btSmithy:
		return 'S'
	case btTavern:
		return 'V'
	case btTownhouse:
		return 'T'
	case btLonghouse:
		return 'L'
	case btBarn:
		return 'B'
	case btHouse:
		return 'H'
	case btRowhouse:
		return 'r'
	case btNarrowhouse:
		return 'n'
	case btDeephouse:
		return 'd'
	default:
		return 'h'
	}
}

func buildingColor(bt buildType, b Biome) string {
	switch bt {
	case btChurch:
		return "#C9CCD2" // pale stone
	case btCathedral:
		return "#D6D2C4" // pale cathedral stone
	case btKeep:
		return "#9A9CA6" // grey castle stone
	case btTownhouse:
		return "#CDBBA0" // pale plaster townhouse
	case btMarketHall:
		return "#B89A6A" // timber-framed market hall
	case btSmithy:
		return "#6E6A66" // dark soot-stained stone
	case btTavern:
		return "#C68A3E" // warm limewashed-amber tavern, brighter than the houses
	case btBarn:
		return "#7C5A38" // dark timber
	}
	return houseColor(uint64Hash(bt, b))
}

func uint64Hash(bt buildType, b Biome) float64 {
	return unit(hashCoord(uint64(bt)*2654435761, int(b), 0))
}

func groundColorFor(b Biome) string {
	switch b {
	case Grass:
		return "#5EAE63"
	case Forest:
		return "#3E7A4F"
	case Hill:
		return "#9C8D67"
	case Sand:
		return "#E6D6A0"
	case Savanna:
		return "#CDBA5C"
	case Swamp:
		return "#45533C"
	case Snow:
		return "#E8EEF5"
	default:
		return "#5EAE63"
	}
}

// ── geometry helpers ───────────────────────────────────────────────────────────

func distPointSeg(px, py, ax, ay, bx, by float64) float64 {
	vx, vy := bx-ax, by-ay
	wx, wy := px-ax, py-ay
	c1 := vx*wx + vy*wy
	if c1 <= 0 {
		return math.Hypot(px-ax, py-ay)
	}
	c2 := vx*vx + vy*vy
	if c2 <= c1 {
		return math.Hypot(px-bx, py-by)
	}
	t := c1 / c2
	return math.Hypot(px-(ax+t*vx), py-(ay+t*vy))
}

// abs is defined in worldgen.go.

// distToAxis is the perpendicular distance from (px,py) to the infinite line
// through (ox,oy) with unit direction (dx,dy).
func distToAxis(px, py, ox, oy, dx, dy float64) float64 {
	return math.Abs((px-ox)*(-dy) + (py-oy)*dx)
}

// bresenham walks a line, but is kept 4-connected: on a diagonal step it also
// plots the corner cell, so a wall drawn along it never has a diagonal-only gap
// (which would both look broken and let a body slip through).
func bresenham(x0, y0, x1, y1 int, plot func(int, int)) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		plot(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		movedX := false
		if e2 >= dy {
			err += dy
			x0 += sx
			movedX = true
		}
		if e2 <= dx {
			if movedX {
				plot(x0, y0) // fill the corner to keep the run 4-connected
			}
			err += dx
			y0 += sy
		}
	}
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

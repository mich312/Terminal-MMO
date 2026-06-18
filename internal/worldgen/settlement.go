package worldgen

import "math"

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
	settleSalt     uint64 = 0x5E771E3D0FAB12C7
	settleCell            = 80    // macro-grid cell size, in world tiles
	settleHubKeep         = 76    // keep settlement centres this far from the origin hub
	settleHalf            = 30    // layout grid half-extent (grid is 2*half+1 square)
	settleMaxReach        = 22    // a settlement's features never extend past this from centre
	linkMax               = 168.0 // longest village-to-village connecting road
	linkFade              = 96.0  // beyond this a connecting road dwindles to a trail
)

// ── settlement identity ──────────────────────────────────────────────────────

type settlement struct {
	mx, my int    // macro-cell
	id     uint64 // identity hash — seeds the whole layout
	cx, cy int    // centre, world coordinates
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

// ── layout grid ──────────────────────────────────────────────────────────────

type lkind uint8

const (
	lEmpty       lkind = iota // not part of the settlement → fall through to terrain
	lYard                     // cleared, trodden ground (walkable)
	lGreen                    // the village green (walkable)
	lRoad                     // road / lane (walkable, packed earth)
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
	lBuildAnchor              // base tile of a building (blocks) — bt names the kind
	lBuildBody                // a non-base tile of a building (blocks)
)

type buildType uint8

const (
	btNone      buildType = iota
	btCottage             // 1×1
	btHouse               // 2×2
	btLonghouse           // 3×2
	btBarn                // 2×2
	btChurch              // 2×3 (tall, central)
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
	n := 2*settleHalf + 1
	l := &layout{ox: s.cx - settleHalf, oy: s.cy - settleHalf, n: n, cells: make([]lcell, n*n)}
	cgx, cgy := settleHalf, settleHalf // centre in grid space
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
	type seg struct{ ax, ay, bx, by float64 }
	reach := float64(settleMaxReach)
	dx, dy := math.Cos(axis), math.Sin(axis)
	px, py := -dy, dx // perpendicular
	off := rng.rng(-1.5, 1.5)
	bend := rng.rng(-3, 3)
	cfx, cfy := float64(cgx)+px*off, float64(cgy)+py*off // road passes just off-centre
	mid := [2]float64{cfx + px*bend, cfy + py*bend}
	segs := []seg{
		{cfx + dx*reach, cfy + dy*reach, mid[0], mid[1]},
		{mid[0], mid[1], cfx - dx*reach, cfy - dy*reach},
	}
	// A loop lane around the core — an irregular ring of a few points.
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
		segs = append(segs, seg{lp[i][0], lp[i][1], lp[j][0], lp[j][1]})
	}
	// One or two short lanes linking the loop back to the main road.
	for i := 0; i < 2; i++ {
		if i == 1 && rng.f() < 0.4 {
			continue
		}
		v := lp[rng.n(loopK)]
		segs = append(segs, seg{v[0], v[1], cfx + dx*rng.rng(-5, 5), cfy + dy*rng.rng(-5, 5)})
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

	// Carve roads (skip non-buildable so a road meets water/rock instead of
	// bridging it). Roads can extend to the grid edge so they pass through the wall.
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			if canBuild(gx, gy) && onAnyRoad(float64(gx), float64(gy)) < 0.8 {
				l.at(gx, gy).kind = lRoad
			}
		}
	}

	// Central focus: the well, then the church fronting it from the south, then
	// the green filling the open ground around them (skipping what's taken).
	gpx, gpy := float64(cgx)-px*off, float64(cgy)-py*off // green sits off the road
	if canBuild(cgx, cgy) {
		l.at(cgx, cgy).kind = lWell
	}
	// Church: the centrepiece, fronting the green. Try a ring of spots around the
	// green so a road through the middle doesn't stop it being placed at all.
	gx0, gy0 := int(gpx), int(gpy)
	for _, o := range [][2]int{{-1, 3}, {0, 3}, {-2, 3}, {1, 3}, {-1, 4}, {-2, 2}, {1, 2}, {-3, 3}, {2, 3}} {
		if placeBuilding(l, canBuild, gx0+o[0], gy0+o[1], btChurch) {
			break
		}
	}
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			if l.at(gx, gy).kind != lEmpty || !canBuild(gx, gy) {
				continue
			}
			if math.Hypot(float64(gx)-gpx, float64(gy)-gpy) < 2.4 {
				l.at(gx, gy).kind = lGreen
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

	buildR := reach - 4 // buildings stay inside this; the wall encloses them
	density := func(r float64) float64 {
		p := 0.95 - 0.6*(r/buildR) // dense core, thinning toward the edge
		if p < 0.22 {
			p = 0.22
		}
		return p
	}
	chooseType := func(r float64, rr *srng) buildType {
		switch {
		case r > buildR*0.62: // outskirts: small cottages and the odd barn
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

	// Houses front the streets or ring the green. The front band is wide and its
	// acceptance is jittered per plot, so dwellings sit at varied setbacks (some
	// on the street, some back in their croft) rather than in a tidy line.
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			r := math.Hypot(float64(gx-cgx), float64(gy-cgy))
			if r > buildR || l.at(gx, gy).kind != lEmpty || !canBuild(gx, gy) {
				continue
			}
			roadD := onAnyRoad(float64(gx), float64(gy))
			greenD := math.Hypot(float64(gx)-gpx, float64(gy)-gpy)
			cellHash := &srng{s: s.id ^ uint64(uint32(gx*73856093^gy*19349663))}
			setback := 2.0 + cellHash.f()*1.6 // 2.0 … 3.6 tiles off the street
			if !((roadD >= 0.9 && roadD <= setback) || (greenD >= 2.6 && greenD <= 5)) {
				continue
			}
			if cellHash.f() > density(r) {
				continue
			}
			placeBuilding(l, canBuild, gx, gy, chooseType(r, cellHash))
		}
	}

	// Palisade: enclose the built-up area with an irregular polygon. For each of
	// N angular sectors, take the farthest *built* cell (houses and green, not the
	// roads that run out to the gates) + a margin as a vertex, then connect the
	// vertices with straight runs — so the wall hugs the houses and has real
	// corners, not a smooth ring ballooned out along the roads.
	const sectors = 20
	sectorR := make([]float64, sectors)
	for i := range sectorR {
		sectorR[i] = 5
	}
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			if !builtUp(l.at(gx, gy).kind) {
				continue
			}
			ddx, ddy := float64(gx-cgx), float64(gy-cgy)
			r := math.Hypot(ddx, ddy)
			ang := math.Atan2(ddy, ddx)
			si := int((ang+math.Pi)/(2*math.Pi)*sectors) % sectors
			if r+2 > sectorR[si] {
				sectorR[si] = r + 2
			}
		}
	}
	for i := range sectorR { // clamp jumps so the wall stays a simple loop
		if sectorR[i] > reach-1 {
			sectorR[i] = reach - 1
		}
	}
	vert := func(i int) (int, int) {
		a := float64(i%sectors)/sectors*2*math.Pi - math.Pi
		r := sectorR[i%sectors]
		return cgx + int(math.Round(math.Cos(a)*r)), cgy + int(math.Round(math.Sin(a)*r))
	}
	for i := 0; i < sectors; i++ {
		x0, y0 := vert(i)
		x1, y1 := vert(i + 1)
		bresenham(x0, y0, x1, y1, func(gx, gy int) {
			if !l.in(gx, gy) {
				return
			}
			c := l.at(gx, gy)
			switch c.kind {
			case lRoad, lGate:
				c.kind = lGate // a road crossing the wall is a gateway
			case lEmpty, lYard, lField:
				if canBuild(gx, gy) {
					c.kind = lFence
				}
			}
		})
	}
	autotileFences(l)

	wallRad := func(ang float64) float64 {
		si := int((ang+math.Pi)/(2*math.Pi)*sectors) % sectors
		return sectorR[si]
	}

	// Outlying worksites — a quarry on nearby hills, a lumber camp at the forest
	// edge, a fishing hut on the shore — each placed in fitting terrain just past
	// the wall and linked back through a gate by a spur road.
	placeOutbuildings(l, canBuild, cgx, cgy, wallRad)

	// Fields along the approach, just outside the wall on the road axis.
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			if l.at(gx, gy).kind != lEmpty || !canBuild(gx, gy) {
				continue
			}
			ddx, ddy := float64(gx-cgx), float64(gy-cgy)
			r := math.Hypot(ddx, ddy)
			ang := math.Atan2(ddy, ddx)
			if r < wallRad(ang)+1 || r > reach-1 {
				continue
			}
			if dline := distToAxis(float64(gx), float64(gy), cfx, cfy, dx, dy); dline < 6 {
				l.at(gx, gy).kind = lField
			}
		}
	}

	// Everything left inside the wall becomes trodden yard.
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			c := l.at(gx, gy)
			if c.kind != lEmpty {
				continue
			}
			r := math.Hypot(float64(gx-cgx), float64(gy-cgy))
			ang := math.Atan2(float64(gy-cgy), float64(gx-cgx))
			if r < wallRad(ang) && canBuild(gx, gy) {
				c.kind = lYard
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
	return l
}

// placeBuilding stamps a building with bottom-left anchor at (gx,gy) if its
// whole footprint fits on buildable, empty ground with a one-tile margin.
func placeBuilding(l *layout, canBuild func(int, int) bool, gx, gy int, bt buildType) bool {
	w, h := footprint(bt)
	x0, y0 := gx, gy-(h-1)
	for yy := y0 - 1; yy <= gy+1; yy++ { // include a 1-tile margin
		for xx := x0 - 1; xx <= gx+w; xx++ {
			if !l.in(xx, yy) {
				return false
			}
			inFoot := xx >= x0 && xx <= gx+w-1 && yy >= y0 && yy <= gy
			if inFoot && !canBuild(xx, yy) {
				return false
			}
			k := l.at(xx, yy).kind
			if inFoot && k != lEmpty && k != lYard {
				return false
			}
			if !inFoot && occupiedBuilding(k) {
				return false // keep a gap to other buildings
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
func placeOutbuildings(l *layout, canBuild func(int, int) bool, cgx, cgy int, wallRad func(float64) float64) {
	n := l.n

	// find returns the matching-biome cell nearest the village but past the wall.
	find := func(want func(Biome) bool) (int, int, bool) {
		bx, by, bd, ok := 0, 0, math.MaxFloat64, false
		for gy := 2; gy < n-2; gy++ {
			for gx := 2; gx < n-2; gx++ {
				if !want(l.at(gx, gy).biome) {
					continue
				}
				ddx, ddy := float64(gx-cgx), float64(gy-cgy)
				r := math.Hypot(ddx, ddy)
				if r < wallRad(math.Atan2(ddy, ddx))+3 || r > float64(settleHalf-4) {
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
	// where it crosses the wall; inside the wall the existing lanes take over.
	carveSpur := func(x0, y0 int) {
		bresenham(x0, y0, cgx, cgy, func(gx, gy int) {
			if !l.in(gx, gy) {
				return
			}
			ddx, ddy := float64(gx-cgx), float64(gy-cgy)
			if math.Hypot(ddx, ddy) < wallRad(math.Atan2(ddy, ddx))-0.5 {
				return
			}
			c := l.at(gx, gy)
			switch c.kind {
			case lFence:
				c.kind = lGate
			case lEmpty, lField:
				if c.biome != Water && c.biome != Mountain && c.biome != Deep {
					c.kind = lRoad
				}
			}
		})
	}
	stamp := func(tx, ty, rad int, from lkind, to lkind) {
		for gy := ty - rad; gy <= ty+rad; gy++ {
			for gx := tx - rad; gx <= tx+rad; gx++ {
				if l.in(gx, gy) && l.at(gx, gy).kind == from {
					l.at(gx, gy).kind = to
				}
			}
		}
	}

	// Quarry — cut into nearby hills: a stone floor with a couple of boulders.
	if tx, ty, ok := find(func(b Biome) bool { return b == Hill }); ok {
		stamp(tx, ty, 2, lEmpty, lQuarry)
		l.at(tx, ty).kind = lQuarryRock
		if l.in(tx+1, ty+1) && l.at(tx+1, ty+1).kind == lQuarry {
			l.at(tx+1, ty+1).kind = lQuarryRock
		}
		if hx, hy, ok := hutNear(tx-2, ty); ok {
			placeBuilding(l, canBuild, hx, hy, btCottage)
			carveSpur(hx, hy)
		} else {
			carveSpur(tx, ty)
		}
	}
	// Lumber camp — a clearing at the forest edge with felled stumps and a store.
	if tx, ty, ok := find(func(b Biome) bool { return b == Forest }); ok {
		stamp(tx, ty, 2, lEmpty, lClearing)
		for _, o := range [][2]int{{-1, -1}, {1, 0}, {0, 1}} {
			if gx, gy := tx+o[0], ty+o[1]; l.in(gx, gy) && l.at(gx, gy).kind == lClearing {
				l.at(gx, gy).kind = lStump
			}
		}
		if hx, hy, ok := hutNear(tx, ty); ok {
			placeBuilding(l, canBuild, hx, hy, btBarn)
			carveSpur(hx, hy)
		} else {
			carveSpur(tx, ty)
		}
	}
	// Fishing hut — on the shore, with a jetty reaching out over the water.
	if tx, ty, ok := find(func(b Biome) bool { return b == Water }); ok {
		if hx, hy, ok := hutNear(tx, ty); ok {
			placeBuilding(l, canBuild, hx, hy, btCottage)
			jetty := func(ax, ay, bx, by int) {
				bresenham(ax, ay, bx, by, func(gx, gy int) {
					if l.in(gx, gy) && l.at(gx, gy).biome == Water && l.at(gx, gy).kind == lEmpty {
						l.at(gx, gy).kind = lJetty
					}
				})
			}
			jetty(hx, hy, tx, ty)
			jetty(tx, ty, tx+(tx-hx), ty+(ty-hy)) // a little further out
			carveSpur(hx, hy)
		}
	}
}

func occupied(k lkind) bool {
	switch k {
	case lRoad, lGreen, lWell, lBuildAnchor, lBuildBody:
		return true
	}
	return false
}

// builtUp is the occupied set the wall hugs: houses and the green, but not the
// roads (which run out to the gates).
func builtUp(k lkind) bool {
	switch k {
	case lGreen, lWell, lBuildAnchor, lBuildBody:
		return true
	}
	return false
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

// connectingRoad draws the road leaving a village toward its nearest neighbour;
// far neighbours dwindle to a faded trail rather than a solid road.
func (g *Generator) connectingRoad(x, y int) (Cell, bool) {
	mx, my := floorDiv(x, settleCell), floorDiv(y, settleCell)
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			s := g.settlementFor(mx+dx, my+dy)
			if !s.valid {
				continue
			}
			p, ok := g.partnerOf(s)
			if !ok {
				continue
			}
			d := distPointSeg(float64(x), float64(y), float64(s.cx), float64(s.cy), float64(p.cx), float64(p.cy))
			if d > 0.7 {
				continue
			}
			// How far along from this village's centre — used to fade the road out.
			along := math.Hypot(float64(x-s.cx), float64(y-s.cy))
			if along > linkFade {
				// Dithered trail: thin to scattered packed-earth patches.
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
	return Cell{}, false
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
	case lBuildAnchor:
		return Cell{Biome: c.biome, Glyph: buildingGlyph(c.bt), Color: buildingColor(c.bt, c.biome), Variant: uint8(c.bt)}
	case lBuildBody:
		return Cell{Biome: c.biome, Glyph: '%', Color: groundColorFor(c.biome)}
	}
	return Cell{}
}

func buildingGlyph(bt buildType) rune {
	switch bt {
	case btChurch:
		return 'C'
	case btLonghouse:
		return 'L'
	case btBarn:
		return 'B'
	case btHouse:
		return 'H'
	default:
		return 'h'
	}
}

func buildingColor(bt buildType, b Biome) string {
	if bt == btChurch {
		return "#C9CCD2" // pale stone
	}
	if bt == btBarn {
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

package worldgen

import "testing"

// Same seed must produce byte-identical terrain — the property that keeps
// every multiplayer session standing on the same ground.
func TestDeterministic(t *testing.T) {
	a, b := New(42), New(42)
	for x := -40; x < 40; x++ {
		for y := -40; y < 40; y++ {
			ca, cb := a.At(x, y), b.At(x, y)
			if ca.Biome != cb.Biome || ca.Glyph != cb.Glyph ||
				ca.Walkable != cb.Walkable || ca.Color != cb.Color {
				t.Fatalf("(%d,%d): %+v != %+v", x, y, ca, cb)
			}
		}
	}
}

// The home gate returns to the lobby and is stood-on-able; the spawn just
// south of it must be walkable thanks to the forced clearing.
func TestGateAndSpawn(t *testing.T) {
	g := New(99)
	gate := g.At(GateX, GateY)
	if gate.Portal != "lobby" || !gate.Walkable {
		t.Fatalf("gate = %+v, want walkable lobby portal", gate)
	}
	if !g.Walkable(GateX, GateY+1) {
		t.Fatal("spawn tile south of the gate must be walkable")
	}
}

// Every hub landmark must sit on a walkable portal tile in a reachable
// clearing — these are the Wilds' doors to the hand-built areas.
func TestLandmarkPortals(t *testing.T) {
	g := New(1)
	for _, lm := range Landmarks {
		c := g.At(lm.X, lm.Y)
		if c.Portal != lm.Portal || !c.Walkable {
			t.Fatalf("landmark %q at (%d,%d) = %+v, want walkable portal %q",
				lm.Name, lm.X, lm.Y, c, lm.Portal)
		}
		// the tile beside it must be walkable (clearing) so a body can reach it
		if !g.Walkable(lm.X+1, lm.Y+1) {
			t.Fatalf("clearing around %q is not walkable", lm.Name)
		}
	}
}

// The forced trails must connect the spawn to every wing's door for ANY seed —
// the regression guard for "spawned boxed in by forest". A 2×2 body walks the
// overworld, so reachability is computed over footprint-walkable cells (all
// four cells of the body must be clear), exactly like the live movement code.
func TestSpawnReachesEveryLandmark(t *testing.T) {
	const body = 2 // PlayerW/PlayerH (kept local: worldgen is a leaf package)
	footprintWalkable := func(g *Generator, x, y int) bool {
		for dy := 0; dy < body; dy++ {
			for dx := 0; dx < body; dx++ {
				if !g.Walkable(x+dx, y+dy) {
					return false
				}
			}
		}
		return true
	}

	// The offsets the live wilds.spawn() tries near the gate; pick the first
	// footprint-walkable one (the clearing may hold an occasional blocking house).
	spawnOffsets := [][2]int{{2, 2}, {-3, 2}, {2, -3}, {-3, -3}, {3, 0}, {0, 3}}

	for _, seed := range []uint64{1, 2, 7, 42, 99, 1000, 31337} {
		g := New(seed)
		var start [2]int
		found := false
		for _, off := range spawnOffsets {
			if p := [2]int{GateX + off[0], GateY + off[1]}; footprintWalkable(g, p[0], p[1]) {
				start, found = p, true
				break
			}
		}
		if !found {
			t.Fatalf("seed %d: no walkable spawn footprint near the gate", seed)
		}
		// BFS the footprint-walkable graph within a box that encloses every
		// landmark plus margin.
		const lo, hi = -24, 24
		seen := map[[2]int]bool{start: true}
		queue := [][2]int{start}
		for len(queue) > 0 {
			p := queue[0]
			queue = queue[1:]
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				n := [2]int{p[0] + d[0], p[1] + d[1]}
				if n[0] < lo || n[0] > hi || n[1] < lo || n[1] > hi || seen[n] {
					continue
				}
				if footprintWalkable(g, n[0], n[1]) {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		// Every landmark and sealed gate must have a reachable footprint cell
		// touching it — the trails wire them all to spawn.
		for _, lm := range append(append([]Landmark{}, Landmarks...), Gates...) {
			ok := false
			for dy := -body; dy <= 0 && !ok; dy++ {
				for dx := -body; dx <= 0 && !ok; dx++ {
					if seen[[2]int{lm.X + dx, lm.Y + dy}] {
						ok = true
					}
				}
			}
			if !ok {
				t.Errorf("seed %d: landmark %q at (%d,%d) is unreachable from spawn",
					seed, lm.Name, lm.X, lm.Y)
			}
		}
	}
}

// The climate model (domain-warped elevation/moisture plus a temperature
// field) must actually produce its richer biomes — cold Snow, warm-dry
// Savanna and wet-low Swamp — not just the original grass/forest/water set.
func TestClimateBiomesAppear(t *testing.T) {
	for _, seed := range []uint64{1, 42} {
		g := New(seed)
		seen := map[Biome]bool{}
		for x := -200; x < 200; x += 3 {
			for y := -200; y < 200; y += 3 {
				seen[g.At(x, y).Biome] = true
			}
		}
		for _, b := range []Biome{Snow, Savanna, Swamp} {
			if !seen[b] {
				t.Errorf("seed %d: biome %d never appears across the sample", seed, b)
			}
		}
	}
}

// Different seeds must yield different worlds.
func TestSeedsDiffer(t *testing.T) {
	a, b := New(1), New(2)
	diff := 0
	for x := -40; x < 40; x++ {
		for y := -40; y < 40; y++ {
			if a.At(x, y).Biome != b.At(x, y).Biome {
				diff++
			}
		}
	}
	if diff == 0 {
		t.Fatal("two seeds produced identical biomes everywhere")
	}
}

// A healthy generator yields varied terrain, not one flat biome.
func TestBiomeVariety(t *testing.T) {
	g := New(7)
	seen := map[Biome]bool{}
	for x := -120; x < 120; x += 3 {
		for y := -120; y < 120; y += 3 {
			seen[g.At(x, y).Biome] = true
		}
	}
	if len(seen) < 3 {
		t.Fatalf("only %d biomes across the sample; want variety", len(seen))
	}
}

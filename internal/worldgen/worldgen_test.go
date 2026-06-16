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

package worldgen

import (
	"testing"
	"time"
)

// The seasonal cover: deep in January, gone in July, and strictly cosmetic —
// a cell's structure (biome, glyph, walkability, settlement layout) never
// changes with the calendar.
func TestSeasonalSnowline(t *testing.T) {
	orig := Clock
	defer func() { Clock = orig }()
	jan := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	if s := SeasonSnow(jan); s < 0.9 {
		t.Fatalf("mid-January season = %v, want deep winter (>0.9)", s)
	}
	if s := SeasonSnow(jul); s > 0.1 {
		t.Fatalf("mid-July season = %v, want high summer (<0.1)", s)
	}

	g := New(42)

	// Somewhere in the sampled band, winter dusts ground that summer leaves bare.
	Clock = func() time.Time { return jan }
	wx, wy, found := 0, 0, false
	for y := -300; y <= 300 && !found; y += 3 {
		for x := -300; x <= 300 && !found; x += 3 {
			if g.At(x, y).Snowy {
				wx, wy, found = x, y, true
			}
		}
	}
	if !found {
		t.Fatal("no winter snow cover anywhere in a 600x600 sample")
	}
	winter := g.At(wx, wy)

	Clock = func() time.Time { return jul }
	summer := g.At(wx, wy)
	if summer.Snowy {
		t.Fatal("snow cover survived into mid-July")
	}
	if winter.Biome != summer.Biome || winter.Glyph != summer.Glyph || winter.Walkable != summer.Walkable {
		t.Fatalf("season changed structure: winter %v/%q/%v vs summer %v/%q/%v",
			winter.Biome, winter.Glyph, winter.Walkable, summer.Biome, summer.Glyph, summer.Walkable)
	}
	if winter.Color == summer.Color {
		t.Fatal("a dusted cell should recolor in winter")
	}

	// Across a broad sample, the season may only touch Color / Snowy / the
	// shimmer — never biome, glyph, walkability or portals. This is what keeps
	// settlements, items, claims and collision identical year round.
	Clock = func() time.Time { return jan }
	type key struct{ x, y int }
	winterCells := map[key]Cell{}
	for y := -240; y <= 240; y += 4 {
		for x := -240; x <= 240; x += 4 {
			winterCells[key{x, y}] = g.At(x, y)
		}
	}
	Clock = func() time.Time { return jul }
	for k, wc := range winterCells {
		sc := g.At(k.x, k.y)
		if wc.Biome != sc.Biome || wc.Glyph != sc.Glyph || wc.Walkable != sc.Walkable ||
			wc.Object != sc.Object || wc.Portal != sc.Portal || wc.Variant != sc.Variant {
			t.Fatalf("cell (%d,%d) changed structurally with the season: %+v vs %+v", k.x, k.y, wc, sc)
		}
		if !wc.Snowy && wc.Color != sc.Color {
			t.Fatalf("cell (%d,%d) recolored without being snow-covered: %q vs %q", k.x, k.y, wc.Color, sc.Color)
		}
	}
}

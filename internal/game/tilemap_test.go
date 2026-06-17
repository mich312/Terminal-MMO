package game

import "testing"

func TestParseMapBaseLegend(t *testing.T) {
	tm := ParseMap([]string{"#.", " #"}, nil, nil)
	if tm.W != 2 || tm.H != 2 {
		t.Fatalf("dims = %dx%d, want 2x2", tm.W, tm.H)
	}
	if w := tm.At(0, 0); w.Kind != TileWall || w.Walkable {
		t.Errorf("'#' = %+v, want impassable wall", w)
	}
	if f := tm.At(1, 0); f.Kind != TileFloor || !f.Walkable {
		t.Errorf("'.' = %+v, want walkable floor", f)
	}
	if v := tm.At(0, 1); v.Kind != TileVoid {
		t.Errorf("' ' = %+v, want void", v)
	}
}

// Unknown runes fall through to impassable decor that renders as themselves.
func TestParseMapUnknownRune(t *testing.T) {
	tm := ParseMap([]string{"?"}, nil, nil)
	d := tm.At(0, 0)
	if d.Kind != TileDecor || d.Walkable || d.Ch != '?' {
		t.Errorf("'?' = %+v, want impassable decor drawn as '?'", d)
	}
}

// A per-map legend overrides the base legend; short rows pad with void.
func TestParseMapLegendAndPadding(t *testing.T) {
	legend := map[rune]LegendEntry{
		'g': {Kind: TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	}
	tm := ParseMap([]string{"g", "..."}, legend, nil)
	if tm.W != 3 {
		t.Fatalf("width = %d, want 3 (widest row)", tm.W)
	}
	g := tm.At(0, 0)
	if g.Kind != TilePortal || g.Portal != "lobby" || !g.Walkable {
		t.Errorf("'g' = %+v, want lobby portal", g)
	}
	if pad := tm.At(1, 0); pad.Kind != TileVoid { // short first row padded
		t.Errorf("padding tile = %+v, want void", pad)
	}
}

func TestTileMapAtOutOfBounds(t *testing.T) {
	tm := ParseMap([]string{"."}, nil, nil)
	for _, p := range [][2]int{{-1, 0}, {0, -1}, {1, 0}, {0, 1}} {
		if got := tm.At(p[0], p[1]); got.Kind != TileVoid {
			t.Errorf("At(%d,%d) = %+v, want void", p[0], p[1], got)
		}
	}
}

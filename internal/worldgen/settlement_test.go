package worldgen

import (
	"strings"
	"testing"
)

// worldSeed mirrors the seed the live Wilds runs on.
const worldSeed = 0xD0117C0FFEE5742

// findSettlement scans macro-cells outward from the origin for the first valid
// settlement.
func findSettlement(g *Generator) (settlement, bool) {
	for ring := 1; ring < 30; ring++ {
		for my := -ring; my <= ring; my++ {
			for mx := -ring; mx <= ring; mx++ {
				if abs(mx) != ring && abs(my) != ring {
					continue
				}
				if s := g.settlementFor(mx, my); s.valid {
					return s, true
				}
			}
		}
	}
	return settlement{}, false
}

func cellEq(a, b Cell) bool {
	return a.Biome == b.Biome && a.Glyph == b.Glyph && a.Color == b.Color &&
		a.Walkable == b.Walkable && a.Variant == b.Variant
}

func TestSettlementDeterminism(t *testing.T) {
	s, ok := findSettlement(New(worldSeed))
	if !ok {
		t.Fatal("no settlement found near origin")
	}
	g1, g2 := New(worldSeed), New(worldSeed)
	_, half, _ := s.dims()
	for y := s.cy - half; y <= s.cy+half; y++ {
		for x := s.cx - half; x <= s.cx+half; x++ {
			a := g1.At(x, y)
			if b := g1.At(x, y); !cellEq(a, b) {
				t.Fatalf("At(%d,%d) not deterministic within a generator", x, y)
			}
			if c := g2.At(x, y); !cellEq(a, c) {
				t.Fatalf("At(%d,%d) differs across generators", x, y)
			}
		}
	}
}

func TestNoSettlementsNearHub(t *testing.T) {
	g := New(worldSeed)
	const hubZone = 90 // no settlement cell may reach the cleared zone near origin
	for y := -hubZone; y <= hubZone; y++ {
		for x := -hubZone; x <= hubZone; x++ {
			if _, ok := g.settlementAt(x, y); ok {
				t.Fatalf("settlement intrudes on hub at (%d,%d)", x, y)
			}
		}
	}
}

// TestVillageReachable confirms a village's interior connects to the outside on
// foot — i.e. the roads punch real gates through the wall. It floods walkable
// cells from the centre and asserts some reach well past the wall.
func TestVillageReachable(t *testing.T) {
	g := New(worldSeed)
	s, ok := findSettlement(g)
	if !ok {
		t.Fatal("no settlement found")
	}
	// A guaranteed-walkable start: the green/yard right by the well.
	var start [2]int
	found := false
	for r := 0; r <= 4 && !found; r++ {
		for dy := -r; dy <= r && !found; dy++ {
			for dx := -r; dx <= r; dx++ {
				if g.Walkable(s.cx+dx, s.cy+dy) {
					start, found = [2]int{dx, dy}, true
					break
				}
			}
		}
	}
	if !found {
		t.Fatalf("village at (%d,%d) has no walkable centre", s.cx, s.cy)
	}
	reach, half, _ := s.dims()
	lo, hi := -half, half
	seen := map[[2]int]bool{}
	stack := [][2]int{start}
	escaped := false
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[p] || p[0] < lo || p[0] > hi || p[1] < lo || p[1] > hi {
			continue
		}
		seen[p] = true
		if abs(p[0]) > reach || abs(p[1]) > reach {
			escaped = true // reached well outside the wall
		}
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			if g.Walkable(s.cx+p[0]+d[0], s.cy+p[1]+d[1]) {
				stack = append(stack, [2]int{p[0] + d[0], p[1] + d[1]})
			}
		}
	}
	if !escaped {
		t.Fatalf("village at (%d,%d) interior is walled in — no gate out", s.cx, s.cy)
	}
}

// TestCityWalkable finds a stone-walled city and confirms a player can walk
// from its centre out through a gate — the tangled lanes must stay connected and
// breach the wall.
func TestCityWalkable(t *testing.T) {
	g := New(worldSeed)
	var city settlement
	found := false
	for ring := 1; ring < 40 && !found; ring++ {
		for my := -ring; my <= ring && !found; my++ {
			for mx := -ring; mx <= ring; mx++ {
				if abs(mx) != ring && abs(my) != ring {
					continue
				}
				if s := g.settlementFor(mx, my); s.valid && s.town {
					city, found = s, true
					break
				}
			}
		}
	}
	if !found {
		t.Skip("no city found near origin")
	}
	reach, half, _ := city.dims()
	var start [2]int
	ok := false
	for r := 0; r <= 6 && !ok; r++ {
		for dy := -r; dy <= r && !ok; dy++ {
			for dx := -r; dx <= r; dx++ {
				if g.Walkable(city.cx+dx, city.cy+dy) {
					start, ok = [2]int{dx, dy}, true
					break
				}
			}
		}
	}
	if !ok {
		t.Fatalf("city at (%d,%d) has no walkable centre", city.cx, city.cy)
	}
	seen := map[[2]int]bool{}
	stack := [][2]int{start}
	escaped := false
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[p] || p[0] < -half || p[0] > half || p[1] < -half || p[1] > half {
			continue
		}
		seen[p] = true
		if abs(p[0]) > reach || abs(p[1]) > reach {
			escaped = true
		}
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			if g.Walkable(city.cx+p[0]+d[0], city.cy+p[1]+d[1]) {
				stack = append(stack, [2]int{p[0] + d[0], p[1] + d[1]})
			}
		}
	}
	if !escaped {
		t.Fatalf("city at (%d,%d) centre cannot reach a gate — not walkable through", city.cx, city.cy)
	}
}

// TestPreviewVillage renders the first few settlements as ASCII so the layout
// can be eyeballed. Run with: go test ./internal/worldgen -run Preview -v
//
// Legend: W well  C church  L longhouse  H house  B barn  h cottage  % building
// = / | / + fence (h/v/corner)  : field  , road  . ground/green  T tree  ^ rock  ~ water
func TestPreviewVillage(t *testing.T) {
	g := New(worldSeed)
	shown := 0
	for ring := 1; ring < 24 && shown < 3; ring++ {
		for my := -ring; my <= ring && shown < 3; my++ {
			for mx := -ring; mx <= ring && shown < 3; mx++ {
				if abs(mx) != ring && abs(my) != ring {
					continue
				}
				s := g.settlementFor(mx, my)
				if !s.valid {
					continue
				}
				shown++
				t.Logf("settlement at (%d,%d)", s.cx, s.cy)
				_, half, _ := s.dims()
				t.Log(renderArea(g, s.cx, s.cy, half))
			}
		}
	}
}

func renderArea(g *Generator, cx, cy, rad int) string {
	var b strings.Builder
	b.WriteByte('\n')
	for y := cy - rad; y <= cy+rad; y++ {
		for x := cx - rad; x <= cx+rad; x++ {
			b.WriteRune(previewRune(g.At(x, y)))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func previewRune(c Cell) rune {
	switch c.Glyph {
	case 'W', 'C', 'L', 'H', 'B', 'h':
		return c.Glyph // well / buildings
	case '%':
		return '%' // building body
	case '=':
		switch c.Variant {
		case 1:
			return '|'
		case 2:
			return '+'
		default:
			return '='
		}
	case '"':
		return ':' // field
	case '♣', '♠', 'ϒ', 'Ψ':
		return 'T'
	case '~', '≈', '≋':
		return '~'
	case '▲', 'Δ', '°':
		return '^'
	case '·':
		if c.Biome == Path {
			return ',' // road
		}
		return '.' // ground / green
	default:
		return c.Glyph
	}
}

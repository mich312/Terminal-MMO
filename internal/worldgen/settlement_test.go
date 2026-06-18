package worldgen

import (
	"strings"
	"testing"
)

// worldSeed mirrors the seed the live Wilds runs on.
const worldSeed = 0xD0117C0FFEE5742

// findSettlement scans outward from the origin for the first valid settlement
// (optionally requiring a fenced village) and returns it.
func findSettlement(g *Generator, wantVillage bool) (settlement, bool) {
	for ring := 1; ring < 20; ring++ {
		for my := -ring; my <= ring; my++ {
			for mx := -ring; mx <= ring; mx++ {
				if abs(mx) != ring && abs(my) != ring {
					continue // only the new ring's edge
				}
				s := g.settlementFor(mx, my)
				if s.valid && (!wantVillage || s.hasFence) {
					return s, true
				}
			}
		}
	}
	return settlement{}, false
}

func TestSettlementDeterminism(t *testing.T) {
	g := New(worldSeed)
	s, ok := findSettlement(g, false)
	if !ok {
		t.Fatal("no settlement found near origin")
	}
	g2 := New(worldSeed)
	for y := s.cy - 16; y <= s.cy+16; y++ {
		for x := s.cx - 16; x <= s.cx+16; x++ {
			a := g.At(x, y)
			if b := g.At(x, y); !cellEq(a, b) {
				t.Fatalf("At(%d,%d) not deterministic", x, y)
			}
			// A second generator with the same seed must agree.
			if c := g2.At(x, y); !cellEq(a, c) {
				t.Fatalf("At(%d,%d) differs across generators", x, y)
			}
		}
	}
}

func cellEq(a, b Cell) bool {
	return a.Biome == b.Biome && a.Glyph == b.Glyph && a.Color == b.Color &&
		a.Walkable == b.Walkable && a.Object == b.Object && a.Portal == b.Portal
}

func TestNoSettlementsNearHub(t *testing.T) {
	g := New(worldSeed)
	// The functional hub (spawn plaza, landmark clearings, connecting trails)
	// lives within ~30 tiles of the origin; no settlement footprint may reach it.
	const hubZone = settleHubKeep - settleMaxReach
	for y := -hubZone; y <= hubZone; y++ {
		for x := -hubZone; x <= hubZone; x++ {
			if _, ok := g.settlementAt(x, y); ok {
				t.Fatalf("settlement intrudes on hub at (%d,%d)", x, y)
			}
		}
	}
}

// TestVillageReachable confirms a fenced village's interior is reachable from
// outside its wall — i.e. the radial roads punch real gaps in the fence ring,
// so a player can always walk in.
func TestVillageReachable(t *testing.T) {
	g := New(worldSeed)
	s, ok := findSettlement(g, true)
	if !ok {
		t.Skip("no fenced village found near origin")
	}
	// Flood-fill walkable cells from a point well outside the village.
	const margin = 6
	lo, hi := -int(s.r0)-margin, int(s.r0)+margin
	seen := map[[2]int]bool{}
	start := [2]int{lo, lo}
	if !g.Walkable(s.cx+start[0], s.cy+start[1]) {
		t.Skip("village corner not walkable; terrain-clipped edge")
	}
	stack := [][2]int{start}
	reachedCenter := false
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[p] || p[0] < lo || p[0] > hi || p[1] < lo || p[1] > hi {
			continue
		}
		seen[p] = true
		if abs(p[0]) <= 2 && abs(p[1]) <= 2 {
			reachedCenter = true
		}
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := [2]int{p[0] + d[0], p[1] + d[1]}
			if g.Walkable(s.cx+n[0], s.cy+n[1]) {
				stack = append(stack, n)
			}
		}
	}
	if !reachedCenter {
		t.Fatalf("village at (%d,%d) interior unreachable from outside", s.cx, s.cy)
	}
}

// TestPreviewVillage renders the first few settlements as ASCII so the
// generated layouts can be eyeballed. Run with:
//
//	go test ./internal/worldgen -run Preview -v
//
// Legend: H house  O well  # fence  : field  . ground/road  T tree  ^ rock  ~ water
func TestPreviewVillage(t *testing.T) {
	g := New(worldSeed)
	seen := map[[2]int]bool{}
	shown := 0
	for ring := 1; ring < 24 && shown < 4; ring++ {
		for my := -ring; my <= ring && shown < 4; my++ {
			for mx := -ring; mx <= ring && shown < 4; mx++ {
				if abs(mx) != ring && abs(my) != ring {
					continue
				}
				s := g.settlementFor(mx, my)
				if !s.valid || seen[[2]int{s.cx, s.cy}] {
					continue
				}
				seen[[2]int{s.cx, s.cy}] = true
				shown++
				kind := "hamlet"
				if s.hasFence {
					kind = "village"
				}
				t.Logf("%s at (%d,%d)  r0=%.1f  spokes=%d  fields=%v",
					kind, s.cx, s.cy, s.r0, s.spokes, s.hasFields)
				t.Log(renderArea(g, s.cx, s.cy, 15))
			}
		}
	}
}

func renderArea(g *Generator, cx, cy, rad int) string {
	var b strings.Builder
	b.WriteByte('\n')
	for y := cy - rad; y <= cy+rad; y++ {
		for x := cx - rad; x <= cx+rad; x++ {
			b.WriteRune(previewRune(g.At(x, y).Glyph))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// previewRune maps a cell glyph to a clearer ASCII stand-in for the text dump.
func previewRune(r rune) rune {
	switch r {
	case 'H':
		return 'H' // house
	case 'W':
		return 'O' // well
	case '=':
		return '#' // fence
	case '"':
		return ':' // field furrow
	case '·', ',', '∘':
		return '.' // open ground / road / yard
	case '♣', '♠', 'ϒ', 'Ψ':
		return 'T' // tree
	case '~', '≈', '≋':
		return '~' // water
	case '▲', 'Δ', '°':
		return '^' // rock / hill
	default:
		return r
	}
}

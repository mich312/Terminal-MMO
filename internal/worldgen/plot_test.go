package worldgen

import "testing"

// findBuildingCell scans a settlement's layout for the first cell PlotAt resolves
// to a plot, returning its world coords and the plot.
func findBuildingCell(g *Generator, s settlement) (int, int, Plot, bool) {
	_, half, _ := s.dims()
	for y := s.cy - half; y <= s.cy+half; y++ {
		for x := s.cx - half; x <= s.cx+half; x++ {
			if p, ok := g.PlotAt(x, y); ok {
				return x, y, p, true
			}
		}
	}
	return 0, 0, Plot{}, false
}

func TestPlotAtFindsBuildings(t *testing.T) {
	g := New(worldSeed)
	s, ok := findSettlement(g)
	if !ok {
		t.Fatal("no settlement found near origin")
	}
	x, y, p, ok := findBuildingCell(g, s)
	if !ok {
		t.Fatal("no building plot found in the settlement")
	}
	if p.ID == "" || p.Settlement == "" || p.Kind == "" {
		t.Errorf("plot is under-populated: %+v", p)
	}
	if p.W < 1 || p.H < 1 {
		t.Errorf("plot footprint = %dx%d, want positive", p.W, p.H)
	}
	// The queried cell must lie within the reported footprint.
	if x < p.AX || x >= p.AX+p.W || y < p.AY || y >= p.AY+p.H {
		t.Errorf("query (%d,%d) outside plot footprint %+v", x, y, p)
	}
}

func TestPlotAtCoversWholeFootprint(t *testing.T) {
	g := New(worldSeed)
	s, _ := findSettlement(g)
	_, _, p, ok := findBuildingCell(g, s)
	if !ok {
		t.Fatal("no building plot found")
	}
	// Every cell of the footprint — anchor and body alike — resolves to the same
	// plot id (anchor recovery from any body cell).
	for y := p.AY; y < p.AY+p.H; y++ {
		for x := p.AX; x < p.AX+p.W; x++ {
			got, ok := g.PlotAt(x, y)
			if !ok {
				t.Fatalf("PlotAt(%d,%d) within footprint returned no plot", x, y)
			}
			if got.ID != p.ID {
				t.Errorf("PlotAt(%d,%d) id = %q, want %q (same building)", x, y, got.ID, p.ID)
			}
			if got.AX != p.AX || got.AY != p.AY || got.W != p.W || got.H != p.H {
				t.Errorf("PlotAt(%d,%d) footprint %+v != anchor's %+v", x, y, got, p)
			}
		}
	}
}

func TestPlotAtDeterministic(t *testing.T) {
	g1, g2 := New(worldSeed), New(worldSeed)
	s, _ := findSettlement(g1)
	_, half, _ := s.dims()
	for y := s.cy - half; y <= s.cy+half; y++ {
		for x := s.cx - half; x <= s.cx+half; x++ {
			p1, ok1 := g1.PlotAt(x, y)
			p2, ok2 := g2.PlotAt(x, y)
			if ok1 != ok2 || p1 != p2 {
				t.Fatalf("PlotAt(%d,%d) differs across generators: (%+v,%v) vs (%+v,%v)",
					x, y, p1, ok1, p2, ok2)
			}
		}
	}
}

func TestPlotAtEmptyOffBuildings(t *testing.T) {
	g := New(worldSeed)
	// Deep in the cleared spawn hub there are no settlements at all.
	for y := -20; y <= 20; y++ {
		for x := -20; x <= 20; x++ {
			if _, ok := g.PlotAt(x, y); ok {
				t.Fatalf("PlotAt(%d,%d) found a plot in the empty hub", x, y)
			}
		}
	}
	// A walkable cell inside a settlement (its centre well/green area) is not a
	// building footprint, so it has no plot either.
	s, _ := findSettlement(g)
	if _, ok := g.PlotAt(s.cx, s.cy); ok {
		// The exact centre could occasionally host the well/church; only fail if it
		// reports a plot when the cell isn't actually a building.
		if k := g.layoutOf(s).at(s.cx-g.layoutOf(s).ox, s.cy-g.layoutOf(s).oy).kind; k != lBuildAnchor && k != lBuildBody {
			t.Errorf("PlotAt(centre) reported a plot on a non-building cell")
		}
	}
}

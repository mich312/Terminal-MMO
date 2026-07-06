package wilds

import (
	"strings"
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// The minimap must mark discovered landmark doors and gates distinctly — as
// their own glyph (glyph client) / a badge cell (HD) — and must not leak ones
// the player hasn't walked into view yet.
func TestMinimapMarksDiscoveredLandmarks(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(t.TempDir() + "/w.db"), Name: name, Theme: ui.Default}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	// Spawn reveals a circle around the origin: the HQ door (0,0) is seen, the
	// Whispering Gate (22,0) is beyond discoverR and still hidden.
	hq := worldgen.Landmarks[0]
	gate := worldgen.Gates[0]
	if !a.seen(hq.X, hq.Y) {
		t.Fatal("the HQ door should be discovered at spawn")
	}
	if a.seen(gate.X, gate.Y) {
		t.Skip("spawn already revealed the gate — landmark layout changed; re-pick a distant one")
	}

	marks := a.mapMarks()
	foundHQ, foundGate := false, false
	for _, lm := range marks {
		foundHQ = foundHQ || lm.Portal == hq.Portal
		foundGate = foundGate || lm.Name == gate.Name
	}
	if !foundHQ {
		t.Fatal("discovered HQ door missing from the minimap marks")
	}
	if foundGate {
		t.Fatal("undiscovered gate leaked onto the minimap")
	}

	// The glyph panel shows the door's glyph; the HD grid flags a Mark cell in
	// the door's color.
	a.showMap = true
	if panel := a.minimap(); !strings.ContainsRune(panel, hq.Glyph) {
		t.Fatalf("glyph minimap does not show the %q door", hq.Glyph)
	}
	_, rows, show := a.HDMinimap()
	if !show {
		t.Fatal("HDMinimap should show while the map is open")
	}
	marked := false
	for _, row := range rows {
		for _, cell := range row {
			if cell.Mark && cell.Hex == hq.Color {
				marked = true
			}
		}
	}
	if !marked {
		t.Fatal("HD minimap has no Mark cell for the discovered HQ door")
	}

	// Walking the gate into view puts it on the chart.
	a.wx, a.wy = gate.X, gate.Y
	a.reveal()
	for _, lm := range a.mapMarks() {
		if lm.Name == gate.Name {
			return
		}
	}
	t.Fatal("gate still missing from the minimap after discovering it")
}

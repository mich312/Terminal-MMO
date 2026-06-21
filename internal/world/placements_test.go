package world

import "testing"

func TestPlaceAndQuery(t *testing.T) {
	w := New()
	defer w.Close()
	name, _ := w.Join("ada")
	w.EnterArea(name, "wilds", 0, 0, "")

	if _, ok := w.PlacementAt(3, 4); ok {
		t.Fatal("empty cell should have no placement")
	}
	if !w.Place("wilds", Placement{X: 3, Y: 4, Kind: "fence", Owner: name}) {
		t.Fatal("Place into an empty cell should succeed")
	}
	p, ok := w.PlacementAt(3, 4)
	if !ok || p.Kind != "fence" || p.Owner != name {
		t.Fatalf("PlacementAt = %+v, %v; want the fence we placed", p, ok)
	}
}

func TestPlaceRejectsOccupied(t *testing.T) {
	w := New()
	defer w.Close()
	if !w.Place("wilds", Placement{X: 1, Y: 1, Kind: "fence", Owner: "ada"}) {
		t.Fatal("first Place should succeed")
	}
	if w.Place("wilds", Placement{X: 1, Y: 1, Kind: "chest", Owner: "bob"}) {
		t.Error("Place onto an occupied cell must fail")
	}
	if p, _ := w.PlacementAt(1, 1); p.Kind != "fence" {
		t.Errorf("occupant changed to %q; the original must stand", p.Kind)
	}
}

func TestUnplace(t *testing.T) {
	w := New()
	defer w.Close()
	w.Place("wilds", Placement{X: 2, Y: 2, Kind: "fence", Owner: "ada"})
	if _, ok := w.Unplace("wilds", 2, 2); !ok {
		t.Fatal("Unplace should report it removed something")
	}
	if _, ok := w.PlacementAt(2, 2); ok {
		t.Error("cell should be empty after Unplace")
	}
	if _, ok := w.Unplace("wilds", 2, 2); ok {
		t.Error("Unplace on an empty cell should report nothing removed")
	}
}

// A placement must reach everyone in the area as an EventPlaced (so the glyph
// client redraws), and persist through the wired callbacks.
func TestPlaceBroadcastsAndPersists(t *testing.T) {
	w := New()
	defer w.Close()
	var saved []Placement
	w.SetPlacementPersist(func(p Placement) { saved = append(saved, p) }, func(int, int) {})

	name, ch := w.Join("ada")
	w.EnterArea(name, "wilds", 0, 0, "")
	drain(ch) // clear the join echo

	w.Place("wilds", Placement{X: 7, Y: 8, Kind: "workbench", Owner: name})

	var got bool
	for _, ev := range drain(ch) {
		if ev.Type == EventPlaced && ev.X == 7 && ev.Y == 8 && ev.Detail == "workbench" {
			got = true
		}
	}
	if !got {
		t.Error("placing should broadcast an EventPlaced to players in the area")
	}
	if len(saved) != 1 || saved[0].Kind != "workbench" {
		t.Errorf("persist callback got %+v; want one workbench", saved)
	}
}

func TestLoadPlacementsSeedsTheSet(t *testing.T) {
	w := New()
	defer w.Close()
	w.LoadPlacements([]Placement{{X: 5, Y: 6, Kind: "chest", Owner: "ada"}})
	if p, ok := w.PlacementAt(5, 6); !ok || p.Kind != "chest" {
		t.Errorf("LoadPlacements didn't seed the cell: %+v, %v", p, ok)
	}
}

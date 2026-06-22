package game

import (
	"testing"

	"github.com/durst-group/durstworld/internal/store"
)

func TestPlaceablesReferenceRealItems(t *testing.T) {
	for _, p := range Placeables {
		if p.Prop == PropNone {
			t.Errorf("placeable %q has no prop sprite", p.ID)
		}
		for _, c := range p.Cost {
			if _, ok := ItemByID(c.Item); !ok {
				t.Errorf("placeable %q cost item %q is not in the catalog", p.ID, c.Item)
			}
		}
	}
}

func TestCanAfford(t *testing.T) {
	fence, _ := PlaceableByID("workbench") // 4 planks
	if CanAfford(fence, map[string]int{"plank": 3}) {
		t.Error("3 planks should not afford a 4-plank workbench")
	}
	if !CanAfford(fence, map[string]int{"plank": 4}) {
		t.Error("4 planks should afford a 4-plank workbench")
	}
}

func TestSpendForDeductsCost(t *testing.T) {
	ctx := &Ctx{Name: "tester", Store: store.Open(""), Inventory: map[string]int{"plank": 5}}
	wb, _ := PlaceableByID("workbench")
	if !SpendFor(ctx, wb) {
		t.Fatal("SpendFor returned false with enough materials")
	}
	if ctx.Inventory["plank"] != 1 {
		t.Errorf("plank after build = %d, want 1 (spent 4)", ctx.Inventory["plank"])
	}
}

func TestSpendForInsufficientIsNoOp(t *testing.T) {
	ctx := &Ctx{Name: "tester", Store: store.Open(""), Inventory: map[string]int{"plank": 2}}
	wb, _ := PlaceableByID("workbench")
	if SpendFor(ctx, wb) {
		t.Fatal("SpendFor should fail without enough materials")
	}
	if ctx.Inventory["plank"] != 2 {
		t.Errorf("plank = %d, want 2 (untouched on a failed build)", ctx.Inventory["plank"])
	}
}

func TestBuildPaletteGroupsAffordAndHotkeys(t *testing.T) {
	ctx := &Ctx{Name: "ada", Store: store.Open(""),
		Inventory: map[string]int{"plank": 12, "lamp": 1, "stone": 4}}
	groups := BuildPalette(ctx)
	if len(groups) < 3 {
		t.Fatalf("got %d groups, want at least Structures/Machines/Trade", len(groups))
	}
	// First group is Structures, in catalog order, and hotkeys run 1..N across all.
	if groups[0].Cat != CatStructure {
		t.Errorf("first group = %v, want Structures", groups[0].Cat)
	}
	seen := map[int]bool{}
	hot := 0
	for _, g := range groups {
		for _, e := range g.Entries {
			if e.Hotkey != 0 {
				hot++
				if seen[e.Hotkey] {
					t.Errorf("hotkey %d assigned twice", e.Hotkey)
				}
				seen[e.Hotkey] = true
			}
			// afford state matches affordMax.
			if (e.Max > 0) != e.Afford {
				t.Errorf("%s afford=%v but Max=%d", e.P.ID, e.Afford, e.Max)
			}
		}
	}
	if hot == 0 || hot > 9 {
		t.Errorf("assigned %d hotkeys, want 1..9", hot)
	}
	// The furnace needs 8 stone; with 4 it must be unaffordable (x0).
	for _, g := range groups {
		for _, e := range g.Entries {
			if e.P.ID == "furnace" && (e.Afford || e.Max != 0) {
				t.Errorf("furnace should be unaffordable with 4 stone, got afford=%v max=%d", e.Afford, e.Max)
			}
			if e.P.ID == "fence" && e.Max != 12 {
				t.Errorf("fence (1 plank) with 12 planks should be x12, got %d", e.Max)
			}
		}
	}
}

func TestPaletteHotkeyResolves(t *testing.T) {
	ctx := &Ctx{Name: "ada", Store: store.Open(""), Inventory: map[string]int{}}
	// Hotkey 1 is the first catalog placeable (fence), regardless of afford.
	idx, ok := PaletteHotkey(ctx, 1)
	if !ok || Placeables[idx].ID != "fence" {
		t.Errorf("hotkey 1 = (%d,%v), want the fence", idx, ok)
	}
	if _, ok := PaletteHotkey(ctx, 9); ok {
		// only 8 placeables today, so 9 has no entry
		t.Error("hotkey 9 should be unassigned with 8 placeables")
	}
}

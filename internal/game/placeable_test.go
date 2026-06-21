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

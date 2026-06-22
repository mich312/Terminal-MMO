package game

import (
	"testing"

	"github.com/durst-group/durstworld/internal/store"
)

// a Ctx whose store is the process noop, with a live inventory — enough to
// exercise Craft without a database.
func craftCtx(inv map[string]int) *Ctx {
	return &Ctx{Name: "tester", Store: store.Open(""), Inventory: inv}
}

var planks = Recipe{ID: "plank", Name: "Planks", In: []Ingredient{{"wood", 2}}, Out: "plank", OutN: 1}
var salve = Recipe{ID: "salve", Name: "Salve", In: []Ingredient{{"herb", 1}, {"mushroom", 1}}, Out: "salve", OutN: 1}

func TestCraftable(t *testing.T) {
	cases := []struct {
		name string
		r    Recipe
		inv  map[string]int
		want int
	}{
		{"empty pack", planks, map[string]int{}, 0},
		{"short by one", planks, map[string]int{"wood": 1}, 0},
		{"exactly one set", planks, map[string]int{"wood": 2}, 1},
		{"three sets, rounds down", planks, map[string]int{"wood": 7}, 3},
		{"multi-input limited by scarcest", salve, map[string]int{"herb": 5, "mushroom": 2}, 2},
		{"multi-input missing one", salve, map[string]int{"herb": 5}, 0},
	}
	for _, c := range cases {
		if got := Craftable(c.r, c.inv); got != c.want {
			t.Errorf("%s: Craftable = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestCraftSpendsAndYields(t *testing.T) {
	ctx := craftCtx(map[string]int{"wood": 5})
	if !Craft(ctx, planks) {
		t.Fatal("Craft returned false with enough inputs")
	}
	if got := ctx.Inventory["wood"]; got != 3 {
		t.Errorf("wood after craft = %d, want 3 (spent 2)", got)
	}
	if got := ctx.Inventory["plank"]; got != 1 {
		t.Errorf("plank after craft = %d, want 1", got)
	}
}

func TestCraftMultiInputDeletesEmptied(t *testing.T) {
	ctx := craftCtx(map[string]int{"herb": 1, "mushroom": 1})
	if !Craft(ctx, salve) {
		t.Fatal("Craft returned false with exactly one set")
	}
	if _, ok := ctx.Inventory["herb"]; ok {
		t.Error("herb should be deleted from the pack once it hits zero")
	}
	if _, ok := ctx.Inventory["mushroom"]; ok {
		t.Error("mushroom should be deleted from the pack once it hits zero")
	}
	if ctx.Inventory["salve"] != 1 {
		t.Errorf("salve = %d, want 1", ctx.Inventory["salve"])
	}
}

func TestCraftInsufficientIsNoOp(t *testing.T) {
	ctx := craftCtx(map[string]int{"wood": 1})
	if Craft(ctx, planks) {
		t.Fatal("Craft should return false when inputs are short")
	}
	if ctx.Inventory["wood"] != 1 {
		t.Errorf("wood = %d, want 1 (untouched on a failed craft)", ctx.Inventory["wood"])
	}
	if _, ok := ctx.Inventory["plank"]; ok {
		t.Error("no plank should be yielded on a failed craft")
	}
}

func TestCraftNilInventory(t *testing.T) {
	ctx := &Ctx{Name: "tester", Store: store.Open("")} // nil Inventory
	if Craft(ctx, planks) {
		t.Fatal("Craft on an empty (nil) pack should be a no-op")
	}
	if ctx.Inventory == nil {
		t.Error("Craft should initialize a nil inventory map")
	}
}

// Every recipe's inputs and output must be real catalog items, or they'd never
// show or persist.
func TestRecipesReferenceRealItems(t *testing.T) {
	for _, r := range Recipes {
		if _, ok := ItemByID(r.Out); !ok {
			t.Errorf("recipe %q output %q is not in the item catalog", r.ID, r.Out)
		}
		for _, in := range r.In {
			if _, ok := ItemByID(in.Item); !ok {
				t.Errorf("recipe %q input %q is not in the item catalog", r.ID, in.Item)
			}
		}
		if Craftable(r, map[string]int{}) != 0 {
			t.Errorf("recipe %q craftable from an empty pack", r.ID)
		}
	}
}

package game

import (
	"testing"

	"github.com/durst-group/durstworld/internal/store"
)

func TestPlaceablesReferenceRealItems(t *testing.T) {
	for _, p := range Placeables {
		if p.Prop == PropNone && !IsTool(p) {
			t.Errorf("placeable %q has no prop sprite", p.ID) // tools clear, they don't place a sprite
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
	// One hotkey past the live palette has no entry.
	n := 0
	for _, g := range BuildPalette(ctx) {
		n += len(g.Entries)
	}
	if _, ok := PaletteHotkey(ctx, n+1); ok {
		t.Errorf("hotkey %d should be unassigned past the %d palette entries", n+1, n)
	}
}

func TestToolPaletteShowsOnlyWhenOwned(t *testing.T) {
	none := &Ctx{Name: "ada", Store: store.Open(""), Inventory: map[string]int{"plank": 4}}
	for _, g := range BuildPalette(none) {
		if g.Cat == CatTool {
			t.Error("a player with no tools should see no Tools group")
		}
	}
	owner := &Ctx{Name: "ada", Store: store.Open(""), Inventory: map[string]int{"axe": 1}}
	var toolNames []string
	for _, g := range BuildPalette(owner) {
		if g.Cat == CatTool {
			for _, e := range g.Entries {
				toolNames = append(toolNames, e.P.ID)
				if !e.Afford {
					t.Errorf("owned tool %q should read as ready (afford)", e.P.ID)
				}
			}
		}
	}
	if len(toolNames) != 1 || toolNames[0] != "axe" {
		t.Errorf("Tools group = %v, want just the axe", toolNames)
	}
	if !OwnsTool(owner, "axe") || OwnsTool(owner, "pick") {
		t.Error("OwnsTool should track which tool items are in the pack")
	}
}

func TestToolRecipeGatedByFoundHead(t *testing.T) {
	var axe Recipe
	for _, r := range Recipes {
		if r.ID == "axe" {
			axe = r
		}
	}
	if axe.ID == "" {
		t.Fatal("no axe recipe")
	}
	// Without the head: not craftable even with timber.
	if n := Craftable(axe, map[string]int{"wood": 9}); n != 0 {
		t.Errorf("axe craftable=%d without a head, want 0", n)
	}
	// With the found head + timber: craftable, and Craft yields the tool.
	ctx := &Ctx{Name: "ada", Store: store.Open(""),
		Inventory: map[string]int{"axe_head": 1, "wood": 2}}
	if Craftable(axe, ctx.Inventory) != 1 {
		t.Fatal("axe should be craftable with a head and 2 timber")
	}
	Craft(ctx, axe)
	if ctx.Inventory["axe"] != 1 || ctx.Inventory["axe_head"] != 0 {
		t.Errorf("after craft: axe=%d head=%d, want 1 and 0", ctx.Inventory["axe"], ctx.Inventory["axe_head"])
	}
}

func TestClearYields(t *testing.T) {
	if it, n := ClearTree.Yield(); it != "wood" || n != 3 {
		t.Errorf("ClearTree yield = %d %s, want 3 wood", n, it)
	}
	if it, n := ClearRock.Yield(); it != "stone" || n != 3 {
		t.Errorf("ClearRock yield = %d %s, want 3 stone", n, it)
	}
}

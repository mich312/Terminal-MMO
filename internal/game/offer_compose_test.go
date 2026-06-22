package game

import (
	"testing"

	"github.com/durst-group/durstworld/internal/store"
)

func TestNewOfferDraftSeedsFromPack(t *testing.T) {
	ctx, _, _, _ := stallCtx(t, "ada", map[string]int{"plank": 4, "stone": 2})
	d, ok := NewOfferDraft(ctx)
	if !ok {
		t.Fatal("a non-empty pack should seed a draft")
	}
	// Give defaults to the first held item in catalog order (stone precedes plank).
	if d.GiveItem != "stone" {
		t.Errorf("give = %q, want the first held item (stone)", d.GiveItem)
	}
	if d.GiveItem == d.AskItem {
		t.Error("the draft must default give and ask to different items")
	}
	if d.GiveN != 1 || d.AskN != 1 {
		t.Errorf("counts = %d/%d, want 1/1", d.GiveN, d.AskN)
	}
}

func TestNewOfferDraftEmptyPack(t *testing.T) {
	ctx, _, _, _ := stallCtx(t, "ada", map[string]int{})
	if _, ok := NewOfferDraft(ctx); ok {
		t.Error("an empty pack has nothing to sell — draft should decline")
	}
}

func TestCycleGiveCountClampsToHeld(t *testing.T) {
	ctx, _, _, _ := stallCtx(t, "ada", map[string]int{"plank": 3})
	d := OfferDraft{Field: OfferFieldGiveN, GiveItem: "plank", GiveN: 1, AskItem: "stone", AskN: 1}
	for i := 0; i < 10; i++ {
		CycleOfferField(ctx, &d, +1)
	}
	if d.GiveN != 3 {
		t.Errorf("give count = %d, want it clamped to the 3 held", d.GiveN)
	}
	for i := 0; i < 10; i++ {
		CycleOfferField(ctx, &d, -1)
	}
	if d.GiveN != 1 {
		t.Errorf("give count = %d, want it clamped to a 1 floor", d.GiveN)
	}
}

func TestCycleAskCountClamps(t *testing.T) {
	ctx, _, _, _ := stallCtx(t, "ada", map[string]int{"plank": 1})
	d := OfferDraft{Field: OfferFieldAskN, GiveItem: "plank", GiveN: 1, AskItem: "stone", AskN: 1}
	CycleOfferField(ctx, &d, -1)
	if d.AskN != 1 {
		t.Errorf("ask count = %d, want a 1 floor", d.AskN)
	}
	for i := 0; i < 200; i++ {
		CycleOfferField(ctx, &d, +1)
	}
	if d.AskN != 99 {
		t.Errorf("ask count = %d, want a 99 ceiling", d.AskN)
	}
}

func TestCycleGiveNeverEqualsAsk(t *testing.T) {
	ctx, _, _, _ := stallCtx(t, "ada", map[string]int{"plank": 2, "stone": 2})
	d := OfferDraft{Field: OfferFieldGive, GiveItem: "plank", GiveN: 1, AskItem: "stone", AskN: 1}
	// Cycle the give item across every held item; ask must never collide.
	for i := 0; i < 8; i++ {
		CycleOfferField(ctx, &d, +1)
		if d.GiveItem == d.AskItem {
			t.Fatalf("give and ask collided on %q after cycling", d.GiveItem)
		}
	}
}

func TestCycleAskSkipsGiveItem(t *testing.T) {
	ctx, _, _, _ := stallCtx(t, "ada", map[string]int{"plank": 2})
	d := OfferDraft{Field: OfferFieldAsk, GiveItem: "plank", GiveN: 1, AskItem: "stone", AskN: 1}
	// Walk the ask through the whole catalog; it must never land on the give item.
	for i := 0; i < len(Items)+2; i++ {
		CycleOfferField(ctx, &d, +1)
		if d.AskItem == d.GiveItem {
			t.Fatal("ask cycled onto the give item — the two must stay distinct")
		}
	}
}

func TestDraftValidAndStock(t *testing.T) {
	ctx, _, _, _ := stallCtx(t, "ada", map[string]int{"plank": 7})
	d := OfferDraft{GiveItem: "plank", GiveN: 2, AskItem: "stone", AskN: 3}
	if !DraftValid(ctx, d) {
		t.Error("a draft for an item the owner holds enough of should be valid")
	}
	units, sales := DraftStock(ctx, d)
	if units != 7 || sales != 3 {
		t.Errorf("stock = %d units, %d sales; want 7 and 3 (7/2)", units, sales)
	}

	d.GiveN = 8 // more per sale than held
	if DraftValid(ctx, d) {
		t.Error("a draft asking more per sale than held should be invalid")
	}
	same := OfferDraft{GiveItem: "plank", GiveN: 1, AskItem: "plank", AskN: 1}
	if DraftValid(ctx, same) {
		t.Error("give == ask should be invalid")
	}
}

func TestPostDraftListsAndStocks(t *testing.T) {
	ctx, _, x, y := stallCtx(t, "ada", map[string]int{"plank": 6})
	d := OfferDraft{GiveItem: "plank", GiveN: 2, AskItem: "stone", AskN: 5}
	if n := PostDraft(ctx, x, y, d); n != 6 {
		t.Fatalf("PostDraft stocked %d, want all 6", n)
	}
	st, _ := StallSnapshot(ctx, x, y)
	if len(st.Offers) != 1 {
		t.Fatalf("offers = %d, want 1 posted", len(st.Offers))
	}
	o := st.Offers[0]
	if o.GiveItem != "plank" || o.GiveN != 2 || o.AskItem != "stone" || o.AskN != 5 || o.Stock != 6 {
		t.Errorf("offer = %+v, want a 2-plank-for-5-stone offer stocked 6", o)
	}
	if _, ok := ctx.Inventory["plank"]; ok {
		t.Error("posting should move the planks out of the pack")
	}
}

func TestPostDraftNonOwnerRejected(t *testing.T) {
	owner, w, x, y := stallCtx(t, "ada", map[string]int{"plank": 5})
	_ = owner
	intruder := &Ctx{World: w, Store: store.Open(""), Name: "mallory", Inventory: map[string]int{"plank": 5}}
	if n := PostDraft(intruder, x, y, OfferDraft{GiveItem: "plank", GiveN: 1, AskItem: "stone", AskN: 1}); n != 0 {
		t.Error("a non-owner must not be able to post at someone else's stall")
	}
}

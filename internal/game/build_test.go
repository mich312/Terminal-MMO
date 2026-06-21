package game

import (
	"testing"

	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
)

func TestDemolishReturnsStallGoods(t *testing.T) {
	owner, _, x, y := stallCtx(t, "ada", map[string]int{"plank": 10})
	AddOffer(owner, x, y, "plank", 5, "stone", 3) // moves 10 planks into the stall
	// A sale puts 3 stone in the till.
	buyer := &Ctx{World: owner.World, Store: store.Open(""), Name: "bob", Inventory: map[string]int{"stone": 3}}
	AcceptOffer(buyer, x, y, 0)

	if !Demolish(owner, x, y) {
		t.Fatal("owner should be able to demolish their own stall")
	}
	if owner.Inventory["plank"] != 5 {
		t.Errorf("got back %d planks, want 5 (the unsold stock)", owner.Inventory["plank"])
	}
	if owner.Inventory["stone"] != 3 {
		t.Errorf("got back %d stone, want 3 (the till)", owner.Inventory["stone"])
	}
	if _, ok := owner.World.PlacementAt(x, y); ok {
		t.Error("the stall should be gone after demolish")
	}
}

func TestDemolishReturnsMachineBuffers(t *testing.T) {
	w := world.New()
	defer w.Close()
	name, _ := w.Join("ada")
	w.EnterArea(name, "wilds", 0, 0, "")
	ctx := &Ctx{World: w, Store: store.Open(""), Name: name, Inventory: map[string]int{"nugget": 8}}
	w.Place("wilds", world.Placement{X: 3, Y: 3, Kind: "furnace", Owner: name})
	RefuelMachine(ctx, 3, 3) // 8 nuggets into the hopper

	if !Demolish(ctx, 3, 3) {
		t.Fatal("owner should demolish their furnace")
	}
	if ctx.Inventory["nugget"] != 8 {
		t.Errorf("got back %d nuggets, want 8 (the unspent input)", ctx.Inventory["nugget"])
	}
}

func TestDemolishOnlyOwner(t *testing.T) {
	owner, w, x, y := stallCtx(t, "ada", map[string]int{"plank": 10})
	AddOffer(owner, x, y, "plank", 5, "stone", 3)
	intruder := &Ctx{World: w, Store: store.Open(""), Name: "mallory", Inventory: map[string]int{}}
	if Demolish(intruder, x, y) {
		t.Fatal("a non-owner must not be able to demolish someone's stall")
	}
	if _, ok := w.PlacementAt(x, y); !ok {
		t.Error("the stall should still stand")
	}
	if len(intruder.Inventory) != 0 {
		t.Error("a failed demolish must not hand out any goods")
	}
}

func TestRemoveOfferRefundsStock(t *testing.T) {
	owner, _, x, y := stallCtx(t, "ada", map[string]int{"plank": 10, "stone": 6})
	AddOffer(owner, x, y, "plank", 5, "stone", 3) // offer 0: 10 planks stocked
	AddOffer(owner, x, y, "stone", 2, "grain", 1) // offer 1: 6 stone stocked

	if !RemoveOffer(owner, x, y, 0) {
		t.Fatal("owner should remove their own offer")
	}
	if owner.Inventory["plank"] != 10 {
		t.Errorf("got back %d planks, want 10 (the refunded stock)", owner.Inventory["plank"])
	}
	st, _ := StallSnapshot(owner, x, y)
	if len(st.Offers) != 1 || st.Offers[0].GiveItem != "stone" {
		t.Fatalf("offers after remove = %+v, want just the stone offer", st.Offers)
	}
}

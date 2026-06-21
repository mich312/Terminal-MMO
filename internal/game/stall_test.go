package game

import (
	"sync"
	"testing"

	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
)

// stallCtx wires a player into a world with a stall already placed, returning the
// owner's ctx and the stall coords.
func stallCtx(t *testing.T, owner string, inv map[string]int) (*Ctx, *world.World, int, int) {
	t.Helper()
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join(owner)
	w.EnterArea(name, "wilds", 5, 5, "")
	ctx := &Ctx{World: w, Store: store.Open(""), Name: name, Inventory: inv}
	w.Place("wilds", world.Placement{X: 7, Y: 7, Kind: "stall", Owner: name})
	return ctx, w, 7, 7
}

func TestAddOfferStocksFromPack(t *testing.T) {
	ctx, _, x, y := stallCtx(t, "ada", map[string]int{"plank": 10})
	if n := AddOffer(ctx, x, y, "plank", 5, "stone", 3); n != 10 {
		t.Fatalf("AddOffer stocked %d, want all 10 planks", n)
	}
	if _, ok := ctx.Inventory["plank"]; ok {
		t.Error("planks should have moved out of the pack into the stall")
	}
	st, _ := StallSnapshot(ctx, x, y)
	if len(st.Offers) != 1 || st.Offers[0].Stock != 10 || st.Offers[0].GiveN != 5 {
		t.Fatalf("offer = %+v, want one 5-for-3 offer stocked 10", st.Offers)
	}
}

func TestAcceptOfferSpendsAndReceivesAndCreditsTill(t *testing.T) {
	owner, _, x, y := stallCtx(t, "ada", map[string]int{"plank": 10})
	AddOffer(owner, x, y, "plank", 5, "stone", 3)

	// A separate buyer with stone.
	buyer := &Ctx{World: owner.World, Store: store.Open(""), Name: "bob",
		Inventory: map[string]int{"stone": 9}}
	o, ok := AcceptOffer(buyer, x, y, 0)
	if !ok || o.GiveN != 5 {
		t.Fatalf("AcceptOffer = %+v, %v; want a 5-plank sale", o, ok)
	}
	if buyer.Inventory["plank"] != 5 {
		t.Errorf("buyer got %d planks, want 5", buyer.Inventory["plank"])
	}
	if buyer.Inventory["stone"] != 6 {
		t.Errorf("buyer has %d stone, want 6 (paid 3)", buyer.Inventory["stone"])
	}
	st, _ := StallSnapshot(buyer, x, y)
	if st.Offers[0].Stock != 5 {
		t.Errorf("stall stock = %d, want 5 after one sale", st.Offers[0].Stock)
	}
	if st.Till["stone"] != 3 {
		t.Errorf("till = %d stone, want 3", st.Till["stone"])
	}

	// Owner collects the till.
	if n := CollectTill(owner, x, y); n != 3 {
		t.Fatalf("collected %d, want 3", n)
	}
	if owner.Inventory["stone"] != 3 {
		t.Errorf("owner has %d stone after collect, want 3", owner.Inventory["stone"])
	}
}

func TestAcceptOfferUnaffordableOrSoldOutIsNoOp(t *testing.T) {
	owner, w, x, y := stallCtx(t, "ada", map[string]int{"plank": 5})
	AddOffer(owner, x, y, "plank", 5, "stone", 3) // stock for exactly one sale

	poor := &Ctx{World: w, Store: store.Open(""), Name: "poor", Inventory: map[string]int{"stone": 2}}
	if _, ok := AcceptOffer(poor, x, y, 0); ok {
		t.Error("a buyer who can't pay should fail")
	}
	if poor.Inventory["stone"] != 2 {
		t.Error("a failed buy must not spend the buyer's goods")
	}

	rich := &Ctx{World: w, Store: store.Open(""), Name: "rich", Inventory: map[string]int{"stone": 99}}
	if _, ok := AcceptOffer(rich, x, y, 0); !ok {
		t.Fatal("the one stocked sale should succeed")
	}
	if _, ok := AcceptOffer(rich, x, y, 0); ok {
		t.Error("the offer is sold out; the second buy must fail")
	}
}

// The test that justifies MutatePlacement: many buyers hit one offer at once;
// exactly as many succeed as there was stock, and the till is never overcredited.
func TestConcurrentAcceptNeverOversells(t *testing.T) {
	owner, w, x, y := stallCtx(t, "ada", map[string]int{"plank": 20})
	AddOffer(owner, x, y, "plank", 1, "stone", 1) // 20 units, 1 per sale → 20 sales max

	const buyers = 60
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < buyers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			b := &Ctx{World: w, Store: store.Open(""), Name: "b", Inventory: map[string]int{"stone": 1}}
			if _, ok := AcceptOffer(b, x, y, 0); ok {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if wins != 20 {
		t.Errorf("%d buyers succeeded, want exactly 20 (the stock)", wins)
	}
	st, _ := StallSnapshot(owner, x, y)
	if st.Offers[0].Stock != 0 {
		t.Errorf("stock = %d, want 0 (sold out, never negative)", st.Offers[0].Stock)
	}
	if st.Till["stone"] != 20 {
		t.Errorf("till = %d, want exactly 20 (never overcredited)", st.Till["stone"])
	}
}

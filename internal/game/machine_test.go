package game

import (
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
)

var testMill = MachineKind{Placeable: "mill", Name: "Mill", In: "grain", Out: "flour",
	InPer: 1, OutPer: 1, Period: 20 * time.Second, Cap: 16}

func TestSettleFirstTouchStartsTheClock(t *testing.T) {
	s, out, in := Settle(testMill, MachineState{In: 10, Out: 0, Last: 0}, 1000)
	if out != 0 || in != 0 {
		t.Errorf("first touch produced %d/%d, want 0/0", out, in)
	}
	if s.Last != 1000 {
		t.Errorf("first touch Last = %d, want 1000 (clock started)", s.Last)
	}
}

func TestSettleTimeLimitedProducesAndCarriesRemainder(t *testing.T) {
	// 50s elapsed at a 20s period → 2 cycles, 10s remainder carried.
	s, out, in := Settle(testMill, MachineState{In: 10, Out: 0, Last: 1000}, 1050)
	if out != 2 || in != 2 {
		t.Fatalf("produced %d (consumed %d), want 2/2 in 50s", out, in)
	}
	if s.In != 8 || s.Out != 2 {
		t.Errorf("buffers = in %d out %d, want in 8 out 2", s.In, s.Out)
	}
	if s.Last != 1040 { // 1000 + 2*20, not 1050 — the 10s remainder carries
		t.Errorf("Last = %d, want 1040 (sub-period remainder carried)", s.Last)
	}
}

func TestSettleStarvedStopsAndDropsIdleTime(t *testing.T) {
	// Only 3 input, but 100s/20s = 5 cycles of time: produce 3, then idle.
	s, out, _ := Settle(testMill, MachineState{In: 3, Out: 0, Last: 1000}, 1100)
	if out != 3 || s.In != 0 {
		t.Fatalf("produced %d leaving %d input, want 3 leaving 0", out, s.In)
	}
	if s.Last != 1100 { // stalled → clock jumps to now (idle time is gone)
		t.Errorf("Last = %d, want 1100 (stalled to now)", s.Last)
	}
}

func TestSettleRespectsOutputCap(t *testing.T) {
	// Cap 16, already 15 out, lots of input and time → only 1 more fits.
	s, out, _ := Settle(testMill, MachineState{In: 99, Out: 15, Last: 1000}, 9000)
	if out != 1 || s.Out != 16 {
		t.Fatalf("produced %d to %d, want 1 to 16 (cap)", out, s.Out)
	}
}

func TestSettleIsDeterministic(t *testing.T) {
	in := MachineState{In: 50, Out: 0, Last: 1000}
	a, ao, ai := Settle(testMill, in, 1234)
	b, bo, bi := Settle(testMill, in, 1234)
	if a != b || ao != bo || ai != bi {
		t.Error("Settle must be a pure function of (kind, state, now)")
	}
}

// The whole offline loop end to end: build a furnace, fuel it, let real time
// pass, and confirm it smelted while "away" — collected into the pack and
// persisted in the shared placement's State.
func TestMachineOfflineLoop(t *testing.T) {
	clock := int64(1000)
	old := nowUnix
	nowUnix = func() int64 { return clock }
	defer func() { nowUnix = old }()

	w := world.New()
	defer w.Close()
	name, _ := w.Join("ada")
	w.EnterArea(name, "wilds", 0, 0, "")
	ctx := &Ctx{World: w, Store: store.Open(""), Name: name,
		Inventory: map[string]int{"nugget": 10}}

	// Build a furnace and fuel it from the pack.
	w.Place("wilds", world.Placement{X: 5, Y: 5, Kind: "furnace", Owner: name})
	if RefuelMachine(ctx, 5, 5) != 10 {
		t.Fatal("refuel should move all 10 nuggets into the furnace")
	}
	if ctx.Inventory["nugget"] != 0 {
		t.Errorf("pack still has %d nuggets after refuel", ctx.Inventory["nugget"])
	}

	// Go away for 5 minutes. Furnace: 2 nugget → 1 ingot per 45s → 6 ingots
	// (300/45 = 6 cycles, consuming 12 nuggets — but only 10 are loaded → 5).
	clock += 300
	_, gainedOut, consumedIn, ok := OpenMachine(ctx, 5, 5)
	if !ok {
		t.Fatal("OpenMachine should recognize the furnace")
	}
	if gainedOut != 5 || consumedIn != 10 {
		t.Fatalf("while away made %d ingots from %d nuggets, want 5 from 10", gainedOut, consumedIn)
	}

	if got := CollectMachine(ctx, 5, 5); got != 5 {
		t.Fatalf("collected %d ingots, want 5", got)
	}
	if ctx.Inventory["ingot"] != 5 {
		t.Errorf("pack has %d ingots after collect, want 5", ctx.Inventory["ingot"])
	}

	// The machine's state persisted into the shared placement (out drained to 0).
	pl, _ := w.PlacementAt(5, 5)
	st := decodeMachine(pl.State)
	if st.Out != 0 {
		t.Errorf("furnace output buffer = %d after collect, want 0", st.Out)
	}
}

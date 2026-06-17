package wilds

import (
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// A personal gate opens for the individual — by offering its item or by saying
// the riddle's answer at it — and then behaves like a portal for that player.
func TestPersonalGate(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(t.TempDir() + "/g.db"), Name: name, Theme: ui.Default,
		Inventory: map[string]int{"crystal": 1}, Hats: map[int]bool{}, FixedGates: map[string]bool{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	g := gates["grove"]
	a.wx, a.wy = g.X, g.Y
	if _, ok := a.sealedGateUnderBody(); !ok {
		t.Fatal("expected to stand on a sealed grove gate")
	}
	if _, ok := a.portalUnder(g.X, g.Y); ok {
		t.Fatal("a sealed gate must not transition")
	}
	a.offerToGate(g) // spend the crystal
	if !ctx.FixedGates["grove"] {
		t.Fatal("grove should be fixed for the player after offering")
	}
	if ctx.Inventory["crystal"] != 0 {
		t.Fatalf("offering should consume the crystal, have %d", ctx.Inventory["crystal"])
	}
	if dest, ok := a.portalUnder(g.X, g.Y); !ok || dest != "grove" {
		t.Fatalf("opened gate should transition to grove, got %q ok=%v", dest, ok)
	}
}

// Saying the answer in chat at a personal gate repairs it.
func TestPersonalGateRiddle(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("bob")
	ctx := &game.Ctx{World: w, Store: store.Open(t.TempDir() + "/r.db"), Name: name, Theme: ui.Default,
		Inventory: map[string]int{}, Hats: map[int]bool{}, FixedGates: map[string]bool{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	g := gates["grove"]
	a.wx, a.wy = g.X, g.Y
	a.Update(game.WorldEventMsg(world.Event{Type: world.EventChat, Player: "bob", Detail: "MAP "}))
	if !ctx.FixedGates["grove"] {
		t.Fatal("answering the riddle in chat should fix the gate")
	}
}

// A co-op gate needs the community pool to fill before it opens for everyone.
func TestCoopGate(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(t.TempDir() + "/c.db"), Name: name, Theme: ui.Default,
		Inventory: map[string]int{"nugget": 5}, Hats: map[int]bool{}, FixedGates: map[string]bool{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	v := gates["vault"]
	a.wx, a.wy = v.X, v.Y
	for i := 0; i < v.need-1; i++ {
		a.offerToGate(v)
		if w.GateFixed("vault") {
			t.Fatalf("vault opened early at %d/%d", i+1, v.need)
		}
	}
	a.offerToGate(v) // the contribution that fills the pool
	if !w.GateFixed("vault") {
		t.Fatal("vault should open once the pool is full")
	}
	if dest, ok := a.portalUnder(v.X, v.Y); !ok || dest != "vault" {
		t.Fatalf("opened co-op gate should transition to vault, got %q ok=%v", dest, ok)
	}
}

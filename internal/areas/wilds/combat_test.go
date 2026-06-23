package wilds

import (
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// newFighter spins up a wilds area for name, positioned at (x,y) in the world.
func newFighter(t *testing.T, w *world.World, name string, x, y int) *area {
	t.Helper()
	resolved, _ := w.Join(name)
	w.EnterArea(resolved, "wilds", x, y, "")
	ctx := &game.Ctx{World: w, Store: store.Open(""), Name: resolved, Theme: ui.Default,
		Inventory: map[string]int{}}
	self, _ := w.Self(resolved)
	a := game.NewArea("wilds", ctx).(*area)
	a.Init(&self)
	a.wx, a.wy = x, y // override the persisted spawn with the test position
	return a
}

// Out in the open Wilds, a bare-handed strike on an adjacent player lands.
func TestStrikePlayerInWild(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	atk := newFighter(t, w, "attacker", 100, 100)
	vicName, _ := w.Join("victim")
	w.EnterArea(vicName, "wilds", 101, 100, "")

	atk.strike() // fists: damage 1, reach 1

	vp, _ := w.Self(vicName)
	if vp.HP != world.DefaultMaxHP-1 {
		t.Fatalf("victim HP = %d, want %d after a bare-handed blow", vp.HP, world.DefaultMaxHP-1)
	}
}

// A weapon scales the blow.
func TestStrikePlayerWithSpear(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	atk := newFighter(t, w, "attacker", 100, 100)
	atk.ctx.Inventory["spear"] = 1 // damage 3
	vicName, _ := w.Join("victim")
	w.EnterArea(vicName, "wilds", 101, 100, "")

	atk.strike()

	vp, _ := w.Self(vicName)
	if vp.HP != world.DefaultMaxHP-3 {
		t.Fatalf("victim HP = %d, want %d after a spear blow", vp.HP, world.DefaultMaxHP-3)
	}
}

// In the hub's peace ward, the same strike is refused and no one is hurt.
func TestStrikePlayerRefusedInHub(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	atk := newFighter(t, w, "attacker", 0, 0) // the green, well inside the safe radius
	vicName, _ := w.Join("victim")
	w.EnterArea(vicName, "wilds", 1, 0, "")

	atk.strike()

	vp, _ := w.Self(vicName)
	if vp.HP != world.DefaultMaxHP {
		t.Fatalf("victim HP = %d, want full %d — the hub is a sanctuary", vp.HP, world.DefaultMaxHP)
	}
}

// A weapon's cooldown throttles strike spam: a second blow fired immediately is
// dropped, so only one lands.
func TestWeaponCooldownThrottlesStrikes(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	atk := newFighter(t, w, "attacker", 100, 100)
	atk.ctx.Inventory["spear"] = 1 // damage 3, cooldown 2 ticks (~1s)
	vicName, _ := w.Join("victim")
	w.EnterArea(vicName, "wilds", 101, 100, "")

	atk.strike()
	atk.strike() // immediately again — should still be cooling down

	vp, _ := w.Self(vicName)
	if vp.HP != world.DefaultMaxHP-3 {
		t.Fatalf("victim HP = %d, want one spear blow (%d) — the second was on cooldown",
			vp.HP, world.DefaultMaxHP-3)
	}
}

// Being struck flashes the screen and tells the victim who hit them, even though
// they didn't press a key.
func TestVictimFeedbackOnDamage(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	a := newFighter(t, w, "victim", 100, 100)

	a.Update(game.WorldEventMsg(world.Event{
		Type: world.EventPlayerDamaged, Player: "attacker", Target: a.ctx.Name, Detail: "spear",
	}))

	if !a.Hurt() {
		t.Fatal("victim should flash after taking a blow")
	}
	if msg, show := a.Toast(); !show || msg == "" {
		t.Fatalf("victim should get a toast, got show=%v msg=%q", show, msg)
	}
}

func TestPvPAllowedZones(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	a := newFighter(t, w, "ada", 0, 0)
	if a.pvpAllowed(0, 0) {
		t.Error("the hub heart should be a safe zone")
	}
	if !a.pvpAllowed(200, 200) {
		t.Error("the open wilds should allow PvP")
	}
}

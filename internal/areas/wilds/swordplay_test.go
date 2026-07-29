package wilds

// The wilds side of the swordplay verbs (docs/SWORDPLAY_PLAN.md): the strong
// strike's damage scaling, the dodge hop with its immunity window, and the
// stagger a parry inflicts on the attacker. The referee's own rulings (guard,
// parry window, riposte bookkeeping) are tested in internal/world.

import (
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/world"
)

// A strong blow lands ~1.8× the weapon's damage (spear 3 → 5).
func TestStrongStrikeScalesDamage(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	atk := newFighter(t, w, "attacker", 100, 100)
	atk.ctx.Inventory["spear"] = 1
	vicName, _ := w.Join("victim")
	w.EnterArea(vicName, "wilds", 101, 100, "")

	atk.strike(true)

	vp, _ := w.Self(vicName)
	if vp.HP != world.DefaultMaxHP-5 {
		t.Fatalf("victim HP = %d, want %d after a strong spear blow", vp.HP, world.DefaultMaxHP-5)
	}
}

// The swing itself broadcasts — whiff or hit — so fights are watchable.
func TestSwingBroadcastsEvenOnAWhiff(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	atk := newFighter(t, w, "attacker", 100, 100)
	watcher, ch := w.Join("watcher")
	w.EnterArea(watcher, "wilds", 103, 100, "")
	for len(ch) > 0 {
		<-ch
	}

	atk.strike(false) // nobody in reach — a pure whiff

	found := false
	for len(ch) > 0 {
		if ev := <-ch; ev.Type == world.EventPlayerActed && ev.Player == "attacker" && ev.Detail == world.ActFast {
			found = true
		}
	}
	if !found {
		t.Fatal("a whiffed swing must still broadcast EventPlayerActed")
	}
}

// A whiff has real recovery: the follow-up inside the cooldown does nothing.
func TestStrikeCooldownAfterWhiff(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	atk := newFighter(t, w, "attacker", 100, 100)
	atk.ctx.Inventory["spear"] = 1 // cooldown 2 ticks

	atk.strike(false) // whiff — starts the recovery
	vicName, _ := w.Join("victim")
	w.EnterArea(vicName, "wilds", 101, 100, "")
	atk.strike(false) // still recovering: must not land

	if vp, _ := w.Self(vicName); vp.HP != world.DefaultMaxHP {
		t.Fatalf("victim HP = %d — a strike landed inside the whiff's recovery", vp.HP)
	}
}

// The dodge hops two tiles, turns on the immunity window, and tells the area.
func TestDodgeHopsAndGrantsImmunity(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	d := newFighter(t, w, "dodger", 100, 100)

	d.dodge(world.DirE)

	if d.wx != 102 || d.wy != 100 {
		t.Fatalf("dodger at (%d,%d), want (102,100) after a two-tile hop east", d.wx, d.wy)
	}
	p, _ := w.Self(d.ctx.Name)
	if p.X != 102 || p.Y != 100 {
		t.Fatalf("world position (%d,%d) didn't follow the hop", p.X, p.Y)
	}
	if _, out := w.Strike("anyone", d.ctx.Name, "", 5, false, time.Minute); out != world.StrikeDodged {
		t.Fatalf("outcome = %v, want StrikeDodged inside the roll's window", out)
	}
}

// Dodges have a cooldown: the second hop inside it goes nowhere.
func TestDodgeCooldown(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	d := newFighter(t, w, "dodger", 100, 100)

	d.dodge(world.DirE)
	d.dodge(world.DirE)

	if d.wx != 102 {
		t.Fatalf("dodger at x=%d — the second dodge should have been refused", d.wx)
	}
}

// Swinging into a fresh guard staggers the attacker: a step back, blade locked.
func TestParryStaggersTheAttacker(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	atk := newFighter(t, w, "attacker", 100, 100)
	vicName, _ := w.Join("victim")
	w.EnterArea(vicName, "wilds", 101, 100, "")
	w.SetGuard(vicName, true) // fresh — a parry waiting to happen

	atk.strike(false)

	vp, _ := w.Self(vicName)
	if vp.HP != world.DefaultMaxHP {
		t.Fatalf("victim HP = %d, want untouched — the blow was parried", vp.HP)
	}
	if atk.wx != 99 {
		t.Fatalf("attacker at x=%d, want 99 — staggered a step back", atk.wx)
	}
	if atk.strikeCd != parryStagger {
		t.Fatalf("attacker recovery = %v, want the %v stagger", atk.strikeCd, parryStagger)
	}
}

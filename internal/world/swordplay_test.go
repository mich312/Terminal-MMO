package world

// The swordplay referee (docs/SWORDPLAY_PLAN.md): guard, parry, dodge and the
// riposte, all judged inside Strike under the one mutex. These tests drive the
// world surface directly; the wilds-side verbs (strong damage scaling, the
// dodge hop, the stagger) are covered in internal/areas/wilds.

import (
	"testing"
	"time"
)

// ageGuard back-dates a raised guard past the parry window, so a strike tests
// the block path rather than the parry path without sleeping.
func ageGuard(w *World, name string) {
	w.MutatePlayer(name, func(p *Player) bool {
		p.GuardStart = time.Now().Add(-2 * ParryWindow)
		return true
	})
}

func TestGuardHalvesDamage(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, _ := twoFighters(w)
	w.SetGuard(vic, true)
	ageGuard(w, vic)

	hp, out := w.Strike(atk, vic, "sword", 6, false, time.Minute)
	if out != StrikeGuarded {
		t.Fatalf("outcome = %v, want StrikeGuarded", out)
	}
	if hp != DefaultMaxHP-3 {
		t.Fatalf("HP = %d, want %d (6 halved to 3)", hp, DefaultMaxHP-3)
	}
}

func TestGuardedChipFloorsAtOne(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, _ := twoFighters(w)
	w.SetGuard(vic, true)
	ageGuard(w, vic)

	hp, out := w.Strike(atk, vic, "", 1, false, time.Minute)
	if out != StrikeGuarded || hp != DefaultMaxHP-1 {
		t.Fatalf("out=%v hp=%d, want a 1-damage chip through the guard", out, hp)
	}
}

func TestParryTurnsTheBlowAside(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, vicCh := twoFighters(w)
	w.SetGuard(vic, true) // fresh — inside the parry window

	hp, out := w.Strike(atk, vic, "sword", 6, false, time.Minute)
	if out != StrikeParried {
		t.Fatalf("outcome = %v, want StrikeParried", out)
	}
	if hp != DefaultMaxHP {
		t.Fatalf("HP = %d, want untouched %d", hp, DefaultMaxHP)
	}
	ev, found := has(drain(vicCh), EventPlayerActed)
	if !found {
		t.Fatal("no EventPlayerActed broadcast for the parry")
	}
	if ev.Player != vic || ev.Target != atk || ev.Detail != ActParry {
		t.Fatalf("parry event = %+v, want parrier=%s attacker=%s detail=%s", ev, vic, atk, ActParry)
	}
	if !w.TakeRiposte(vic) {
		t.Fatal("the parry should have earned a riposte")
	}
}

func TestRiposteSpendsOnce(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, _ := twoFighters(w)
	w.SetGuard(vic, true)
	w.Strike(atk, vic, "", 3, false, time.Minute) // parried

	if !w.TakeRiposte(vic) {
		t.Fatal("first TakeRiposte should succeed")
	}
	if w.TakeRiposte(vic) {
		t.Fatal("a riposte must not be double-spent")
	}
}

func TestStrongBlowBreaksTheGuard(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, _ := twoFighters(w)
	w.SetGuard(vic, true)
	ageGuard(w, vic)

	hp, out := w.Strike(atk, vic, "sword", 4, true, time.Minute)
	if out != StrikeGuardBroken {
		t.Fatalf("outcome = %v, want StrikeGuardBroken", out)
	}
	if hp != DefaultMaxHP-4 {
		t.Fatalf("HP = %d, want %d — a broken guard softens nothing", hp, DefaultMaxHP-4)
	}
	if p, _ := w.Self(vic); p.Guarding {
		t.Fatal("the smashed guard should have dropped")
	}
}

func TestStrongInsideParryWindowIsStillParried(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, _ := twoFighters(w)
	w.SetGuard(vic, true) // fresh

	if _, out := w.Strike(atk, vic, "sword", 4, true, time.Minute); out != StrikeParried {
		t.Fatalf("outcome = %v — a read is a read, even against a heavy blow", out)
	}
}

func TestReRaisingCannotKeepTheGuardFresh(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, _ := twoFighters(w)
	w.SetGuard(vic, true)
	ageGuard(w, vic)
	w.SetGuard(vic, true) // no-op while already up: GuardStart must not refresh

	if _, out := w.Strike(atk, vic, "", 2, false, time.Minute); out != StrikeGuarded {
		t.Fatalf("outcome = %v, want StrikeGuarded — the stale guard must not parry", out)
	}
}

func TestDodgeSlipsTheBlow(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, _ := twoFighters(w)
	w.BeginDodge(vic, 50*time.Millisecond)

	hp, out := w.Strike(atk, vic, "", 5, false, time.Minute)
	if out != StrikeDodged || hp != DefaultMaxHP {
		t.Fatalf("out=%v hp=%d, want the blow to find only air", out, hp)
	}

	time.Sleep(60 * time.Millisecond) // the window closes
	if _, out := w.Strike(atk, vic, "", 5, false, time.Minute); out != StrikeHit {
		t.Fatalf("outcome after the window = %v, want StrikeHit", out)
	}
}

func TestSetFacingTurnsInPlace(t *testing.T) {
	w := New()
	defer w.Close()
	name, ch := w.Join("ada")
	w.EnterArea(name, "wilds", 5, 5, "")
	drain(ch)

	w.SetFacing(name, DirW)
	p, _ := w.Self(name)
	if p.Facing != DirW {
		t.Fatalf("facing = %v, want DirW", p.Facing)
	}
	if p.X != 5 || p.Y != 5 {
		t.Fatalf("position moved to (%d,%d) — SetFacing must not step", p.X, p.Y)
	}
	if _, found := has(drain(ch), EventMoved); !found {
		t.Fatal("the turn should broadcast so other clients redraw it")
	}

	w.SetFacing(name, Dir(99)) // out of range: ignored
	if p, _ := w.Self(name); p.Facing != DirW {
		t.Fatalf("invalid facing accepted: %v", p.Facing)
	}
}

func TestActedBroadcastsTheMotion(t *testing.T) {
	w := New()
	defer w.Close()
	atk, atkCh, _, vicCh := twoFighters(w)

	w.Acted(atk, ActStrong)
	for _, ch := range []<-chan Event{atkCh, vicCh} {
		ev, found := has(drain(ch), EventPlayerActed)
		if !found {
			t.Fatal("swing not broadcast to the area")
		}
		if ev.Player != atk || ev.Detail != ActStrong {
			t.Fatalf("swing event = %+v, want player=%s detail=%s", ev, atk, ActStrong)
		}
	}
}

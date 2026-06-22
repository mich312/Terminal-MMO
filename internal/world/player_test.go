package world

import (
	"testing"
	"time"
)

func has(evs []Event, t EventType) (Event, bool) {
	for _, ev := range evs {
		if ev.Type == t {
			return ev, true
		}
	}
	return Event{}, false
}

// two players, both standing in the wilds, with their event channels.
func twoFighters(w *World) (atk string, atkCh <-chan Event, vic string, vicCh <-chan Event) {
	atk, atkCh = w.Join("attacker")
	vic, vicCh = w.Join("victim")
	w.EnterArea(atk, "wilds", 1, 1, "")
	w.EnterArea(vic, "wilds", 2, 1, "")
	drain(atkCh) // discard the join chatter
	drain(vicCh)
	return
}

func TestFreshPlayerFullHP(t *testing.T) {
	w := New()
	defer w.Close()
	name, _ := w.Join("ada")
	p, ok := w.Self(name)
	if !ok {
		t.Fatal("Self after Join: not found")
	}
	if p.HP != DefaultMaxHP || p.MaxHP != DefaultMaxHP {
		t.Fatalf("fresh player HP=%d MaxHP=%d, want %d/%d", p.HP, p.MaxHP, DefaultMaxHP, DefaultMaxHP)
	}
	if w.Downed(name) {
		t.Fatal("fresh player should not be downed")
	}
}

func TestStrikeNonLethal(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, vicCh := twoFighters(w)

	hp, downed, ok := w.Strike(atk, vic, "spear", 3, time.Second)
	if !ok || downed {
		t.Fatalf("Strike ok=%v downed=%v, want true/false", ok, downed)
	}
	if hp != DefaultMaxHP-3 {
		t.Fatalf("HP after strike = %d, want %d", hp, DefaultMaxHP-3)
	}
	ev, found := has(drain(vicCh), EventPlayerDamaged)
	if !found {
		t.Fatal("victim got no EventPlayerDamaged")
	}
	if ev.Player != atk || ev.Target != vic || ev.Detail != "spear" {
		t.Fatalf("damage event = %+v, want attacker=%s target=%s weapon=spear", ev, atk, vic)
	}
}

func TestStrikeDownsAtZero(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, vicCh := twoFighters(w)

	// Big hit empties the bar in one blow.
	hp, downed, ok := w.Strike(atk, vic, "", DefaultMaxHP+5, time.Minute)
	if !ok || !downed {
		t.Fatalf("Strike ok=%v downed=%v, want true/true", ok, downed)
	}
	if hp != 0 {
		t.Fatalf("HP floored = %d, want 0", hp)
	}
	if !w.Downed(vic) {
		t.Fatal("victim should be downed")
	}
	if _, found := has(drain(vicCh), EventPlayerDowned); !found {
		t.Fatal("victim got no EventPlayerDowned")
	}
}

func TestStrikeOnDownedIsNoop(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, _ := twoFighters(w)

	w.Strike(atk, vic, "", DefaultMaxHP, time.Minute) // down them
	_, downed, ok := w.Strike(atk, vic, "", 5, time.Minute)
	if ok || downed {
		t.Fatalf("second strike on downed: ok=%v downed=%v, want false/false", ok, downed)
	}
}

func TestStrikeUnknownTarget(t *testing.T) {
	w := New()
	defer w.Close()
	if _, _, ok := w.Strike("ghost", "nobody", "", 1, time.Second); ok {
		t.Fatal("striking an unknown player should report ok=false")
	}
}

func TestRespawnRestores(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, vicCh := twoFighters(w)
	w.Strike(atk, vic, "", DefaultMaxHP, time.Minute)
	drain(vicCh)

	w.Respawn(vic, "wilds", 50, 60)
	p, _ := w.Self(vic)
	if p.HP != p.MaxHP {
		t.Fatalf("respawn HP = %d, want full %d", p.HP, p.MaxHP)
	}
	if p.X != 50 || p.Y != 60 {
		t.Fatalf("respawn position = (%d,%d), want (50,60)", p.X, p.Y)
	}
	if w.Downed(vic) {
		t.Fatal("respawned player should not be downed")
	}
	if _, found := has(drain(vicCh), EventPlayerRespawn); !found {
		t.Fatal("no EventPlayerRespawn after Respawn")
	}
}

func TestMutatePlayer(t *testing.T) {
	w := New()
	defer w.Close()
	name, _ := w.Join("ada")

	if w.MutatePlayer("nobody", func(*Player) bool { return true }) {
		t.Fatal("MutatePlayer on unknown name should return false")
	}
	changed := w.MutatePlayer(name, func(p *Player) bool { p.HP = 3; return true })
	if !changed {
		t.Fatal("MutatePlayer reported no change")
	}
	if p, _ := w.Self(name); p.HP != 3 {
		t.Fatalf("HP after MutatePlayer = %d, want 3", p.HP)
	}
}

// A freshly respawned player is briefly immune, so a knock-out can't be chained
// into a spawn-camp.
func TestStrikeRefusedWhileImmune(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, _ := twoFighters(w)

	w.Respawn(vic, "wilds", 2, 1) // grants the post-respawn grace window
	if !w.Immune(vic) {
		t.Fatal("a just-respawned player should be immune")
	}
	if _, _, ok := w.Strike(atk, vic, "", 3, time.Second); ok {
		t.Fatal("striking an immune player should be refused")
	}
	if p, _ := w.Self(vic); p.HP != p.MaxHP {
		t.Fatalf("immune player HP = %d, want full %d", p.HP, p.MaxHP)
	}
}

// A downed player whose timer has lapsed is no longer "downed" and can be hit
// again — the gate is the clock, not a sticky flag.
func TestDownedExpires(t *testing.T) {
	w := New()
	defer w.Close()
	atk, _, vic, _ := twoFighters(w)

	w.Strike(atk, vic, "", DefaultMaxHP, time.Millisecond)
	if !w.Downed(vic) {
		t.Fatal("should be downed immediately after the blow")
	}
	time.Sleep(5 * time.Millisecond)
	if w.Downed(vic) {
		t.Fatal("downed state should lapse once DownedUntil passes")
	}
}

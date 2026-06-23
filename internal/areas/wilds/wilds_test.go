package wilds_test

import (
	"strings"
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// abs is a tiny local helper for the proximity assertions below.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Stepping out of a hall (here Kraftwerk) drops the player back in the Wilds
// beside that landmark's door — not teleported to the distant HQ gate. This is
// what makes "leave an area" return you to the open world where you entered.
func TestReturnsBesideLandmark(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ida")
	ctx := &game.Ctx{World: w, Store: store.Open(t.TempDir() + "/w.db"), Name: name,
		From: "kraftwerk", Theme: ui.Default}

	a := game.NewArea("wilds", ctx)
	self, _ := w.Self(name)
	a.Init(&self)

	var lm worldgen.Landmark
	for _, l := range worldgen.Landmarks {
		if l.Portal == "kraftwerk" {
			lm = l
		}
	}
	got, _ := w.Self(name)
	if abs(got.X-lm.X) > 2 || abs(got.Y-lm.Y) > 2 {
		t.Fatalf("returned to (%d,%d), want beside the Kraftwerk door (%d,%d)",
			got.X, got.Y, lm.X, lm.Y)
	}
	// And definitively not parked back at the HQ gate.
	if abs(got.X-worldgen.GateX) <= 2 && abs(got.Y-worldgen.GateY) <= 2 {
		t.Fatalf("returned to the HQ gate (%d,%d), not the Kraftwerk door", got.X, got.Y)
	}
}

// The Wilds samples a player-centered window, renders a full screen, and
// stamps the local player's avatar (with its "you" chevron) at the center.
func TestWildsRendersPlayer(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(t.TempDir() + "/w.db"), Name: name, Theme: ui.Default}

	a := game.NewArea("wilds", ctx)
	self, _ := w.Self(name)
	a.Init(&self)

	out := a.View(81, 21)
	if got := len(strings.Split(out, "\n")); got != 21 {
		t.Fatalf("view height = %d, want 21", got)
	}
	if !strings.ContainsRune(out, '▾') {
		t.Fatal("local player's 'you' marker (▾) should be drawn")
	}
}

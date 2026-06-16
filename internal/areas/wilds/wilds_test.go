package wilds_test

import (
	"strings"
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// The Wilds samples a player-centered window and renders a full screen with
// the home gate visible just north of the spawn.
func TestWildsRendersGate(t *testing.T) {
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
	if !strings.ContainsRune(out, '⌂') {
		t.Fatal("home gate (⌂) should be visible near spawn")
	}
}

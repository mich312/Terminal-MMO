package game

import (
	"testing"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// A FlavorArea must hand back a Transition when the player walks into its
// portal — the behaviour kraftwerk and the Demo Center rely on.
func TestFlavorAreaPortalTransition(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("p")
	ctx := &Ctx{World: w, Name: name, Theme: ui.Default}

	a := newFlavorArea(ctx, FlavorConfig{
		ID: "room", Display: "Room",
		Rows: []string{
			"#######",
			"#.....#",
			"P.....#", // portal back at (0,2)
			"#.....#",
			"#######",
		},
		Legend: map[rune]LegendEntry{'P': {Kind: TilePortal, Ch: '◊', Walkable: true, Portal: "lobby"}},
		SpawnX: 1, SpawnY: 1,
		Title: "Test Room", Body: "flavor text",
	})
	a.Init(nil)

	// Spawned beside the portal: the first step must not transition (armed latch).
	if next, _ := a.Update(key('d')); isTransition(next) {
		t.Fatal("spawning beside the portal should not transition")
	}

	// Walk back into the portal; now it should fire.
	var got Area
	for i := 0; i < 4; i++ {
		got, _ = a.Update(key('a'))
		if isTransition(got) {
			break
		}
	}
	tr, ok := got.(Transition)
	if !ok || tr.To != "lobby" {
		t.Fatalf("walking into the portal should transition to lobby, got %#v", got)
	}

	if v := a.View(80, 20); v == "" {
		t.Fatal("flavor view is empty")
	}
}

func isTransition(a Area) bool { _, ok := a.(Transition); return ok }

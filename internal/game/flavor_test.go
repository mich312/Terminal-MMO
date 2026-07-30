package game

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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

	// Spawned beside the portal: walking away must not transition (armed latch).
	if next := holdKey(a, &a.Walker, key('d'), 700*time.Millisecond); isTransition(next) {
		t.Fatal("spawning beside the portal should not transition")
	}

	// Walk back into the portal; now it should fire.
	got := holdKey(a, &a.Walker, key('a'), 2*time.Second)
	tr, ok := got.(Transition)
	if !ok || tr.To != "lobby" {
		t.Fatalf("walking into the portal should transition to lobby, got %#v", got)
	}

	if v := a.View(80, 20); v == "" {
		t.Fatal("flavor view is empty")
	}
}

func isTransition(a Area) bool { _, ok := a.(Transition); return ok }

// holdKey drives an area the way a client holding a movement key does: the key
// repeats, the clock advances between repeats, and the body covers real ground.
// It returns as soon as Update yields a Transition, else the area it ends on.
//
// A key no longer moves anybody by itself — it only steers — so a test that
// wants to walk somewhere has to supply the time as well. The clock is injected
// rather than real so this costs no wall-clock seconds.
func holdKey(a Area, w *Walker, k tea.KeyMsg, d time.Duration) Area {
	const slice = 30 * time.Millisecond
	base := time.Now()
	w.lastTick = base
	for elapsed := time.Duration(0); elapsed < d; elapsed += slice {
		at := base.Add(elapsed + slice)
		w.Clock = func() time.Time { return at }
		next, _ := a.Update(k) // the key repeats while it is held
		if isTransition(next) {
			return next
		}
		a = next
		if next, _ = a.Update(TickMsg{}); isTransition(next) {
			return next
		}
		a = next
	}
	w.Clock = nil
	return a
}

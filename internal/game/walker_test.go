package game

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/world"
)

// openMap builds a w×h room of walkable floor, optionally with a portal cell.
func openMap(w, h int, portal ...[3]any) *TileMap {
	tiles := make([][]Tile, h)
	for y := range tiles {
		row := make([]Tile, w)
		for x := range row {
			row[x] = Tile{Kind: TileFloor, Walkable: true}
		}
		tiles[y] = row
	}
	for _, p := range portal {
		x, y, to := p[0].(int), p[1].(int), p[2].(string)
		tiles[y][x] = Tile{Kind: TilePortal, Walkable: true, Portal: to}
	}
	return &TileMap{W: w, H: h, Tiles: tiles}
}

func newWalker(t *testing.T, m *TileMap) *Walker {
	t.Helper()
	wd := world.New()
	t.Cleanup(wd.Close)
	name, _ := wd.Join("ada")
	return &Walker{Ctx: &Ctx{World: wd, Name: name}, Map: m, AreaID: "test"}
}

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// A movement key no longer takes a step: it points the body, and the body walks
// while a clock runs. This is the whole shape of the change, so it is worth
// asserting directly.
func TestWalkerKeyStartsWalkingRatherThanStepping(t *testing.T) {
	w := newWalker(t, openMap(40, 20))
	w.Enter(5, 5, 0)

	if _, handled := w.HandleCommon(keyMsg("d")); !handled {
		t.Fatal("a movement key was not consumed")
	}
	if w.X != 5 {
		t.Errorf("the key alone moved the body to x=%d; it should only have steered", w.X)
	}
	if !w.Body.Moving() {
		t.Fatal("the key did not set a steering intent")
	}

	// A second of walking east at WalkSpeed covers WalkSpeed tiles.
	w.WalkFor(1, 0, false, time.Second)
	if want := 5.5 + WalkSpeed; w.Body.FX < want-0.05 || w.Body.FX > want+0.05 {
		t.Errorf("after a second: FX = %.2f, want about %.2f", w.Body.FX, want)
	}
	if w.X != int(w.Body.FX) {
		t.Errorf("cell (%d) and body (%.2f) disagree", w.X, w.Body.FX)
	}
}

// OnTile is the per-step hook every area's mechanics now hang off, so it must
// fire once per cell and carry the cell actually entered.
func TestWalkerOnTileFiresPerCell(t *testing.T) {
	w := newWalker(t, openMap(40, 20))
	w.Enter(5, 5, 0)
	var seen [][2]int
	w.OnTile = func(x, y int) { seen = append(seen, [2]int{x, y}) }

	w.WalkFor(1, 0, false, time.Second) // 5.5 → 8.5
	want := [][2]int{{6, 5}, {7, 5}, {8, 5}}
	if len(seen) != len(want) {
		t.Fatalf("OnTile fired %d times (%v), want %d", len(seen), seen, len(want))
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("crossing %d: got %v, want %v", i, seen[i], want[i])
		}
	}
}

// Standing still must not fire it, or every area's per-step work runs forever
// at the host's tick rate.
func TestWalkerIdleDoesNotFireOnTile(t *testing.T) {
	w := newWalker(t, openMap(40, 20))
	w.Enter(5, 5, 0)
	fired := 0
	w.OnTile = func(int, int) { fired++ }
	for i := 0; i < 30; i++ {
		w.HandleCommon(TickMsg{})
	}
	if fired != 0 {
		t.Errorf("OnTile fired %d times while standing still", fired)
	}
}

// Walking into a portal transitions — and the armed latch still stops you
// bouncing straight back through the one you arrived by.
func TestWalkerPortalOnArrivalAndLatch(t *testing.T) {
	w := newWalker(t, openMap(40, 20, [3]any{9, 5, "arcade"}))
	w.Enter(5, 5, 0)

	portal, _ := w.WalkFor(1, 0, false, 2*time.Second)
	if portal != "arcade" {
		t.Fatalf("walking into a portal gave %q, want %q", portal, "arcade")
	}

	// Spawn on top of one: it must not fire until we have left and come back.
	w2 := newWalker(t, openMap(40, 20, [3]any{5, 5, "arcade"}))
	w2.Enter(5, 5, 0)
	if p, _ := w2.WalkFor(1, 0, false, 400*time.Millisecond); p != "" {
		t.Errorf("spawning on a portal fired it immediately: %q", p)
	}
}

// Both tick sources have to drive the body, or a client gets a world it can
// steer but never move in: the SSH client sees only the world's EventTick, and
// the polling clients see only TickMsg.
func TestWalkerAdvancesOnBothTickSources(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  tea.Msg
	}{
		{"TickMsg", TickMsg{}},
		{"world EventTick", WorldEventMsg(world.Event{Type: world.EventTick})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWalker(t, openMap(40, 20))
			w.Enter(5, 5, 0)
			base := time.Now()
			w.Clock = func() time.Time { return base }
			w.HandleCommon(keyMsg("d"))

			at := base
			for i := 0; i < 10; i++ {
				at = at.Add(30 * time.Millisecond)
				now := at
				w.Clock = func() time.Time { return now }
				w.Body.SetIntent(1, 0, false, now) // a held key, re-asserted
				w.HandleCommon(tc.msg)
			}
			if w.Body.FX <= 5.5 {
				t.Errorf("%s did not advance the body: FX = %.3f", tc.name, w.Body.FX)
			}
		})
	}
}

// A stalled host must not lurch the body across the map when it comes back.
func TestWalkerSkipsAStalledTick(t *testing.T) {
	w := newWalker(t, openMap(40, 20))
	w.Enter(5, 5, 0)
	base := time.Now()
	w.Clock = func() time.Time { return base }
	w.HandleCommon(keyMsg("d"))
	w.HandleCommon(TickMsg{}) // establishes lastTick

	stalled := base.Add(5 * time.Second)
	w.Clock = func() time.Time { return stalled }
	w.Body.SetIntent(1, 0, false, stalled)
	w.HandleCommon(TickMsg{})
	if w.Body.FX != 5.5 {
		t.Errorf("a five-second stall moved the body to %.3f; it should have been skipped", w.Body.FX)
	}
}

// The world's copy of the position has to follow the body, since that is what
// every other client renders from.
func TestWalkerPublishesPositionToTheWorld(t *testing.T) {
	w := newWalker(t, openMap(40, 20))
	w.Enter(5, 5, 0)
	w.WalkFor(1, 0, false, time.Second)

	self, ok := w.Ctx.World.Self(w.Ctx.Name)
	if !ok {
		t.Fatal("player vanished from the world")
	}
	if self.X != w.X {
		t.Errorf("world says cell x=%d, walker says %d", self.X, w.X)
	}
	if self.FX != w.Body.FX {
		t.Errorf("world says body x=%.3f, walker says %.3f", self.FX, w.Body.FX)
	}
	if self.Facing != world.DirE {
		t.Errorf("walking east left facing %d, want DirE", self.Facing)
	}
}

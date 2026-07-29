package web

import (
	"encoding/json"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
)

// fakeArea is a minimal game.Area + game.HDViewer: a fixed tilemap the test can
// slide a window over, so the scene builder can be exercised without spinning up
// a real area (and its worldgen, persistence and spawn logic).
type fakeArea struct {
	name string
	tm   *game.TileMap
	x, y int
}

func newFakeArea(name string, w, h int) *fakeArea {
	tiles := make([][]game.Tile, h)
	for y := 0; y < h; y++ {
		row := make([]game.Tile, w)
		for x := 0; x < w; x++ {
			row[x] = game.Tile{
				Kind: game.TileFloor, Walkable: true,
				Tex: game.TexGrass, Ground: "#2E6B40",
			}
		}
		tiles[y] = row
	}
	// One landmark so props and portal labels are covered too.
	tiles[2][2] = game.Tile{Kind: game.TilePortal, Walkable: true, Portal: "lobby",
		Label: "Durst HQ", Prop: game.PropPortal, PropHex: "#2E8BFF", Ground: "#2E6B40"}
	tiles[4][4] = game.Tile{Kind: game.TileDecor, Ground: "#2E6B40",
		Prop: game.PropTree, PropHex: "#1F6B3A"}
	return &fakeArea{name: name, tm: &game.TileMap{W: w, H: h, Tiles: tiles}}
}

func (a *fakeArea) Name() string                        { return a.name }
func (a *fakeArea) Init(*world.Player) tea.Cmd          { return nil }
func (a *fakeArea) Update(tea.Msg) (game.Area, tea.Cmd) { return a, nil }
func (a *fakeArea) View(int, int) string                { return "" }

func (a *fakeArea) HDView(vw, vh int) (*game.TileMap, int, int) {
	ox, oy := a.x-vw/2, a.y-vh/2
	tiles := make([][]game.Tile, vh)
	for ly := 0; ly < vh; ly++ {
		row := make([]game.Tile, vw)
		for lx := 0; lx < vw; lx++ {
			row[lx] = a.tm.At(ox+lx, oy+ly)
		}
		tiles[ly] = row
	}
	return &game.TileMap{W: vw, H: vh, Tiles: tiles}, ox, oy
}

func buildOnce(t *testing.T, st *sceneState, w *world.World, a *fakeArea, name string) *Scene {
	t.Helper()
	return st.Build(sceneInput{
		Area: a, View: a, World: w, Name: name, AreaID: "test",
		VW: 8, VH: 8, Now: time.Now(),
	})
}

// A tile is only worth sending when it has actually changed. This is the whole
// premise of the wire format — if a standing-still frame resends the window, the
// delta scheme is broken and every player costs a screenful of JSON 15× a second.
func TestSceneResendsNothingWhenNothingChanges(t *testing.T) {
	w := world.New()
	defer w.Close()
	name, _ := w.Join("anna")
	w.EnterArea(name, "test", 4, 4, "Test")

	area := newFakeArea("Test", 12, 12)
	area.x, area.y = 4, 4
	st := newSceneState()

	first := buildOnce(t, st, w, area, name)
	if len(first.Tiles) != 8*8*TileStride {
		t.Fatalf("first frame should carry the whole window: got %d ints, want %d",
			len(first.Tiles), 8*8*TileStride)
	}
	if !first.Reset {
		t.Error("first frame should be a reset — the client holds nothing yet")
	}

	second := buildOnce(t, st, w, area, name)
	if len(second.Tiles) != 0 {
		t.Errorf("standing still resent %d tile ints, want 0", len(second.Tiles)/TileStride)
	}
	if len(second.PalAdd) != 0 {
		t.Errorf("standing still resent %d palette entries, want 0", len(second.PalAdd))
	}
}

// Walking one tile should cost roughly one row, not a whole screen: tiles are
// addressed absolutely, so the ground you already hold stays held.
func TestSceneStepSendsOnlyTheNewEdge(t *testing.T) {
	w := world.New()
	defer w.Close()
	name, _ := w.Join("anna")
	w.EnterArea(name, "test", 4, 4, "Test")

	area := newFakeArea("Test", 40, 40)
	area.x, area.y = 20, 20
	st := newSceneState()
	buildOnce(t, st, w, area, name)

	area.x++ // one step east
	step := buildOnce(t, st, w, area, name)

	got := len(step.Tiles) / TileStride
	if got == 0 {
		t.Fatal("a step sent no tiles at all — the new column is missing")
	}
	// One new column of 8. Anything approaching a full 64-tile window means the
	// client is being asked to rebuild ground it already has.
	if got > 16 {
		t.Errorf("a one-tile step sent %d tiles, want ~8 (one column)", got)
	}
}

// Changing area invalidates every coordinate, so the client must be told to
// throw away what it holds rather than draw the old world's tiles at the new
// world's positions.
func TestSceneResetsOnAreaChange(t *testing.T) {
	w := world.New()
	defer w.Close()
	name, _ := w.Join("anna")
	w.EnterArea(name, "test", 4, 4, "Test")

	area := newFakeArea("Test", 12, 12)
	area.x, area.y = 4, 4
	st := newSceneState()
	buildOnce(t, st, w, area, name)

	next := st.Build(sceneInput{
		Area: area, View: area, World: w, Name: name, AreaID: "lobby",
		VW: 8, VH: 8, Now: time.Now(),
	})
	if !next.Reset {
		t.Error("changing area must reset the client's tile cache")
	}
	if len(next.Tiles) != 8*8*TileStride {
		t.Errorf("after a reset the whole window must be resent: got %d tiles",
			len(next.Tiles)/TileStride)
	}
}

// The palette is append-only and interned: a color is sent once and referenced
// by index thereafter.
func TestPaletteInternsColorsOnce(t *testing.T) {
	st := newSceneState()
	a := st.color("#2E6B40")
	b := st.color("#2E6B40")
	if a != b {
		t.Errorf("the same color got two indices: %d and %d", a, b)
	}
	if got := st.color(""); got != 0 {
		t.Errorf("the empty color should be index 0, got %d", got)
	}
	if len(st.newPal) != 1 {
		t.Errorf("one distinct color should add one palette entry, got %d", len(st.newPal))
	}
}

// Tiles that fall well outside the window are dropped, so a long walk across an
// infinite overworld doesn't grow either side's scene without bound.
func TestSceneDropsDistantTiles(t *testing.T) {
	w := world.New()
	defer w.Close()
	name, _ := w.Join("anna")
	w.EnterArea(name, "test", 20, 20, "Test")

	area := newFakeArea("Test", 200, 200)
	area.x, area.y = 20, 20
	st := newSceneState()
	buildOnce(t, st, w, area, name)
	held := len(st.tiles)

	for i := 0; i < 40; i++ { // walk far enough that the start is long gone
		area.x++
		buildOnce(t, st, w, area, name)
	}
	if len(st.tiles) > held*3 {
		t.Errorf("walking 40 tiles grew the retained set from %d to %d — eviction isn't working",
			held, len(st.tiles))
	}
}

// The local player must be flagged, or the client has nothing to point the
// camera at.
func TestSceneMarksTheLocalPlayer(t *testing.T) {
	w := world.New()
	defer w.Close()
	me, _ := w.Join("anna")
	them, _ := w.Join("bob")
	w.EnterArea(me, "test", 4, 4, "Test")
	w.EnterArea(them, "test", 6, 4, "Test")

	area := newFakeArea("Test", 12, 12)
	area.x, area.y = 4, 4
	sc := buildOnce(t, newSceneState(), w, area, me)

	if len(sc.Players) != 2 {
		t.Fatalf("both players should be in the scene, got %d", len(sc.Players))
	}
	var selves int
	for _, p := range sc.Players {
		if p.Self {
			selves++
			if p.Name != me {
				t.Errorf("the wrong player is flagged as self: %q", p.Name)
			}
		}
	}
	if selves != 1 {
		t.Errorf("exactly one player must be flagged as self, got %d", selves)
	}
}

// The point of the browser client is that it is a third renderer on *the same*
// world, not a second world that looks like it. A player who joined the way an
// SSH session joins must appear in a browser's scene with no translation step —
// which is what this asserts: the world is the only shared thing, and it is
// enough.
func TestTerminalPlayersAppearInABrowserScene(t *testing.T) {
	w := world.New()
	defer w.Close()

	// Exactly what cmd/durstworld's SSH handlers do at connect.
	sshName, _ := w.Join("tom")
	game.SetupAvatar(w, store.Open(""), sshName)
	w.EnterArea(sshName, "test", 5, 4, "Test")

	// …and what a browser session does.
	webName, _ := w.Join("anna")
	w.EnterArea(webName, "test", 4, 4, "Test")

	area := newFakeArea("Test", 12, 12)
	area.x, area.y = 4, 4
	sc := buildOnce(t, newSceneState(), w, area, webName)

	var sawSSHPlayer bool
	for _, p := range sc.Players {
		if p.Name == sshName {
			sawSSHPlayer = true
			if p.Self {
				t.Error("the terminal player was flagged as the browser's own avatar")
			}
			if p.X != 5 || p.Y != 4 {
				t.Errorf("terminal player is at (%d,%d), want (5,4)", p.X, p.Y)
			}
		}
	}
	if !sawSSHPlayer {
		t.Error("a player who joined over SSH is invisible to the browser client")
	}
}

// A portal has to announce where it goes, so the client can float the label the
// terminal clients draw over the gate.
func TestScenePortalCarriesItsLabel(t *testing.T) {
	w := world.New()
	defer w.Close()
	name, _ := w.Join("anna")
	w.EnterArea(name, "test", 4, 4, "Test")

	area := newFakeArea("Test", 12, 12)
	area.x, area.y = 4, 4
	sc := buildOnce(t, newSceneState(), w, area, name)

	var found bool
	for _, l := range sc.Labels {
		if l.Text == "Durst HQ" && l.Kind == "portal" {
			found = true
		}
	}
	if !found {
		t.Errorf("the portal's destination label is missing from %+v", sc.Labels)
	}
}

// The scene has to survive a JSON round trip with its tile stride intact — the
// client indexes the flat array by TileStride and would silently misread every
// tile if the encoding drifted.
func TestSceneRoundTripsThroughJSON(t *testing.T) {
	w := world.New()
	defer w.Close()
	name, _ := w.Join("anna")
	w.EnterArea(name, "test", 4, 4, "Test")

	area := newFakeArea("Test", 12, 12)
	area.x, area.y = 4, 4
	sc := buildOnce(t, newSceneState(), w, area, name)

	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Scene
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Tiles)%TileStride != 0 {
		t.Errorf("tile array length %d is not a multiple of the stride %d",
			len(back.Tiles), TileStride)
	}
	if back.W != sc.W || back.H != sc.H || back.OX != sc.OX || back.OY != sc.OY {
		t.Errorf("window geometry did not survive the round trip: %+v vs %+v",
			[]int{back.W, back.H, back.OX, back.OY}, []int{sc.W, sc.H, sc.OX, sc.OY})
	}
}

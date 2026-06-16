package game

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// A real area package can't be imported here (it would import game and cycle),
// so register a minimal lobby stub for the test binary's registry.
func init() {
	Register("lobby", "Lobby", func(ctx *Ctx) Area {
		return &testArea{Walker: Walker{
			Ctx:    ctx,
			Map:    ParseMap([]string{"##########", "#........#", "##########"}, nil, nil),
			AreaID: "lobby",
		}}
	})
}

type testArea struct{ Walker }

func (a *testArea) Name() string { return "Lobby" }
func (a *testArea) Init(*world.Player) tea.Cmd {
	a.Enter(2, 1, 0)
	return nil
}
func (a *testArea) Update(msg tea.Msg) (Area, tea.Cmd) {
	if portal, handled := a.HandleCommon(msg); handled && portal != "" {
		return Transition{To: portal}, nil
	}
	return a, nil
}
func (a *testArea) View(int, int) string { return a.Render() }

// newTestModel wires a minimal in-memory model sized to a terminal, with the
// lobby built (as Init does) but the player not yet dropped in.
func newTestModel(t *testing.T) *Model {
	t.Helper()
	w := world.New()
	t.Cleanup(w.Close)
	name, events := w.Join("tester")
	ctx := &Ctx{World: w, Store: store.Open(t.TempDir() + "/t.db"), Name: name, Theme: ui.Default}
	m := NewModel(ctx, events, store.VisitInfo{FirstVisit: true})
	m.Init() // builds the lobby area for the intro pan
	m.width, m.height = 100, 30
	return m
}

// The intro must render exactly one screen (height lines) at every frame of
// the hold-then-pan cinematic, and never panic.
func TestIntroPanGeometry(t *testing.T) {
	m := newTestModel(t)
	for step := 0; step <= introHold+introPan; step++ {
		m.introStep = step
		out := m.introView()
		if got := len(strings.Split(out, "\n")); got != m.height {
			t.Fatalf("intro step %d: got %d lines, want %d", step, got, m.height)
		}
	}
}

// The final pan frame must equal the live play field, so the cut to phasePlay
// is seamless.
func TestIntroLandsOnPlayField(t *testing.T) {
	m := newTestModel(t)
	m.introStep = introHold + introPan
	landed := m.introView()
	m.phase = phasePlay
	if landed != m.playView() {
		t.Fatal("last intro frame does not match the play field it pans onto")
	}
}

// beginPlay places the player and play renders a full screen.
func TestBeginPlayGeometry(t *testing.T) {
	m := newTestModel(t)
	m.beginPlay()
	if m.phase != phasePlay {
		t.Fatalf("phase = %v, want phasePlay", m.phase)
	}
	if got := len(strings.Split(m.View(), "\n")); got != m.height {
		t.Fatalf("play view: got %d lines, want %d", got, m.height)
	}
}

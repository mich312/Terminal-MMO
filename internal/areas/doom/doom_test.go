package doom

import (
	"image"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/world"
)

func newArea(t *testing.T) *area {
	t.Helper()
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("tester")
	ctx := &game.Ctx{World: w, Name: name}
	a := &area{Walker: game.Walker{Ctx: ctx, AreaID: "doom"}}
	self, _ := w.Self(name)
	a.Init(&self)
	return a
}

func TestStartIsClear(t *testing.T) {
	a := newArea(t)
	if a.wall(int(a.px), int(a.py)) {
		t.Fatalf("player starts inside a wall at (%.1f,%.1f)", a.px, a.py)
	}
}

func TestCastReturnsWall(t *testing.T) {
	a := newArea(t)
	dist, _, cell := a.cast(0) // straight ahead
	if dist <= 0 || dist > 64 {
		t.Fatalf("forward ray distance out of range: %v", dist)
	}
	if cell != '#' && cell != '1' && cell != '2' {
		t.Fatalf("forward ray should hit a wall, got %q", cell)
	}
}

func TestReachExitCounts(t *testing.T) {
	a := newArea(t)
	a.px, a.py = float64(a.exit[0])+0.5, float64(a.exit[1])+0.5
	a.dirX, a.dirY = 1, 0
	a.step(1) // a nudge that stays inside the exit cell
	if a.wins != 1 {
		t.Fatalf("standing on the exit should score a clear, wins=%d", a.wins)
	}
}

func TestRaycastTextSized(t *testing.T) {
	a := newArea(t)
	out := a.raycastText(40, 20)
	if n := len(strings.Split(out, "\n")); n != 20 {
		t.Fatalf("raycast text has %d rows, want 20", n)
	}
}

func TestHDFrameNoPanic(t *testing.T) {
	a := newArea(t)
	img := image.NewRGBA(image.Rect(0, 0, 48, 24))
	a.HDFrame(img) // must fill the frame without panicking
	if img.RGBAAt(24, 12).A != 0xFF {
		t.Fatal("HDFrame left pixels unpainted")
	}
}

func TestLeave(t *testing.T) {
	a := newArea(t)
	next, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if tr, ok := next.(game.Transition); !ok || tr.To != "arcade" {
		t.Fatalf("x → %#v, want Transition to arcade", next)
	}
}

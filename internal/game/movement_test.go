package game

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

func TestMoveKey(t *testing.T) {
	cases := []struct {
		key           string
		dx, dy, steps int
		ok            bool
	}{
		{"w", 0, -1, 1, true},
		{"s", 0, 1, 1, true},
		{"a", -1, 0, 1, true},
		{"d", 1, 0, 1, true},
		{"left", -1, 0, 1, true},
		{"y", -1, -1, 1, true}, // diagonals
		{"u", 1, -1, 1, true},
		{"b", -1, 1, 1, true},
		{"n", 1, 1, 1, true},
		{"W", 0, -1, 2, true}, // run (uppercase)
		{"N", 1, 1, 2, true},
		{"shift+up", 0, -1, 2, true},
		{"shift+right", 1, 0, 2, true},
		{"e", 0, 0, 0, false}, // not a move key
		{"x", 0, 0, 0, false},
	}
	for _, c := range cases {
		dx, dy, steps, ok := MoveKey(c.key)
		if dx != c.dx || dy != c.dy || steps != c.steps || ok != c.ok {
			t.Errorf("MoveKey(%q) = (%d,%d,%d,%v), want (%d,%d,%d,%v)",
				c.key, dx, dy, steps, ok, c.dx, c.dy, c.steps, c.ok)
		}
	}
}

func key(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// A 2×2 body must not bounce straight back through the portal it spawned next
// to, but must trigger it after stepping away and returning.
func TestPortalArming(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("p")
	ctx := &Ctx{World: w, Name: name, Theme: ui.Default}
	rows := []string{
		"#######",
		"#.....#",
		"P.....#", // portal on the left wall at (0,2)
		"#.....#",
		"#######",
	}
	legend := map[rune]LegendEntry{'P': {Kind: TilePortal, Ch: '◊', Walkable: true, Portal: "dest"}}
	wk := &Walker{Ctx: ctx, Map: ParseMap(rows, legend, nil), AreaID: "room"}
	wk.Enter(1, 1, 0) // body covers (1,1),(2,1),(1,2),(2,2) — beside the portal

	// First move while still next to the portal must NOT trigger (latch armed).
	if p, _ := wk.HandleCommon(key('d')); p != "" {
		t.Fatalf("spawning beside a portal should not trigger it, got %q", p)
	}
	// Walk back toward the portal; now it should fire.
	got := ""
	for i := 0; i < 4 && got == ""; i++ {
		got, _ = wk.HandleCommon(key('a'))
	}
	if got != "dest" {
		t.Fatalf("walking into the portal should transition to dest, got %q", got)
	}
}

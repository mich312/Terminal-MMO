package game

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/ui"
)

// The compendium panel must stay inside the screen at the 80-column minimum:
// every body line, once rendered, has to fit within the panel's content width
// (screen minus the rounded border + horizontal padding). Long catalog prose is
// word-wrapped to make this hold.
func TestCompendiumFitsNarrowWidth(t *testing.T) {
	m := playingModel(t)
	m.width, m.height = MinWidth, MinHeight
	m.ctx.Inventory = map[string]int{"berry": 3, "spore": 1, "crystal": 2}
	m.ctx.Hats = map[int]bool{2: true}
	m.runChatLine("/compendium")
	if !m.showInfo {
		t.Fatal("/compendium should open the info panel")
	}
	max := m.width - 6 // border (2) + padding (2 each side)
	for i, line := range m.infoLines {
		if w := lipgloss.Width(line); w > max {
			t.Errorf("line %d width %d exceeds %d: %q", i, w, max, line)
		}
	}
}

// a second registered area so /goto has somewhere to go.
func init() {
	Register("void", "The Void", func(ctx *Ctx) Area {
		return &testArea{Walker: Walker{
			Ctx:    ctx,
			Map:    ParseMap([]string{"#####", "#...#", "#####"}, nil, nil),
			AreaID: "void",
		}}
	})
}

func lastChat(m *Model) string {
	if len(m.chatLog) == 0 {
		return ""
	}
	return m.chatLog[len(m.chatLog)-1]
}

func playingModel(t *testing.T) *Model {
	m := newTestModel(t)
	m.beginPlay() // place the player in the lobby
	return m
}

func TestUnknownCommand(t *testing.T) {
	m := playingModel(t)
	m.runChatLine("/nope")
	if !strings.Contains(lastChat(m), "unknown command") {
		t.Fatalf("want unknown-command feedback, got %q", lastChat(m))
	}
}

func TestHelpOpensPanel(t *testing.T) {
	m := playingModel(t)
	m.runChatLine("/help")
	if !m.showInfo || m.infoTitle != "Commands" {
		t.Fatalf("/help should open the Commands panel; showInfo=%v title=%q", m.showInfo, m.infoTitle)
	}
	if len(m.infoLines) != len(commands) {
		t.Fatalf("help lists %d commands, want %d", len(m.infoLines), len(commands))
	}
}

func TestHelpForOneCommand(t *testing.T) {
	m := playingModel(t)
	m.runChatLine("/help roll")
	if !strings.Contains(lastChat(m), "/roll") {
		t.Fatalf("want /roll usage, got %q", lastChat(m))
	}
}

func TestWhereReportsArea(t *testing.T) {
	m := playingModel(t)
	m.runChatLine("/where")
	if !strings.Contains(lastChat(m), "Lobby") {
		t.Fatalf("want area in /where output, got %q", lastChat(m))
	}
}

func TestWhisperOffline(t *testing.T) {
	m := playingModel(t)
	m.runChatLine("/w ghost hello there")
	if !strings.Contains(lastChat(m), "not online") {
		t.Fatalf("want offline notice, got %q", lastChat(m))
	}
}

func TestColorChangesAvatar(t *testing.T) {
	m := playingModel(t)
	m.runChatLine("/color 3")
	self, _ := m.ctx.World.Self(m.ctx.Name)
	if self.Color != ui.AvatarColorByIndex(3) {
		t.Fatalf("color = %v, want %v", self.Color, ui.AvatarColorByIndex(3))
	}
}

func TestGotoValidTransitions(t *testing.T) {
	m := playingModel(t)
	cmd := m.runChatLine("/goto void")
	if cmd == nil || m.phase != phaseTransition || m.pendingArea != "void" {
		t.Fatalf("/goto void should start a transition; phase=%v pending=%q", m.phase, m.pendingArea)
	}
}

func TestGotoUnknown(t *testing.T) {
	m := playingModel(t)
	m.runChatLine("/goto narnia")
	if !strings.Contains(lastChat(m), "no such area") {
		t.Fatalf("want no-such-area, got %q", lastChat(m))
	}
}

func TestClearEmptiesLog(t *testing.T) {
	m := playingModel(t)
	m.addSystemLine("something")
	m.runChatLine("/clear")
	if len(m.chatLog) != 0 {
		t.Fatalf("chat log not cleared: %v", m.chatLog)
	}
}

func TestParseDice(t *testing.T) {
	ok := []struct {
		spec    string
		n, side int
	}{
		{"2d6", 2, 6}, {"d20", 1, 20}, {"100", 1, 100}, {"1d2", 1, 2},
	}
	for _, c := range ok {
		n, s, valid := parseDice(c.spec)
		if !valid || n != c.n || s != c.side {
			t.Fatalf("parseDice(%q) = (%d,%d,%v), want (%d,%d,true)", c.spec, n, s, valid, c.n, c.side)
		}
	}
	for _, bad := range []string{"0d6", "21d6", "d1", "5000", "abc", "d", "2dd3"} {
		if _, _, valid := parseDice(bad); valid {
			t.Fatalf("parseDice(%q) should be invalid", bad)
		}
	}
}

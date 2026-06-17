package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestEditorTypeAndNewline(t *testing.T) {
	e := NewEditor("")
	e.HandleKey(runes("ab"))
	e.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	e.HandleKey(runes("cd"))
	if got := e.Value(); got != "ab\ncd" {
		t.Fatalf("Value = %q, want %q", got, "ab\\ncd")
	}
}

func TestEditorBackspaceJoinsLines(t *testing.T) {
	e := NewEditor("ab\ncd")
	e.HandleKey(tea.KeyMsg{Type: tea.KeyHome}) // start of "cd"
	e.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := e.Value(); got != "abcd" {
		t.Fatalf("Value = %q, want abcd", got)
	}
}

func TestEditorPasteMultiline(t *testing.T) {
	e := NewEditor("")
	e.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("# Title\n\n- one\n- two"), Paste: true})
	want := "# Title\n\n- one\n- two"
	if got := e.Value(); got != want {
		t.Fatalf("paste Value = %q, want %q", got, want)
	}
}

// A line longer than the panel width must scroll so the cursor stays visible
// and the rendered line never exceeds the width (or it corrupts the border).
func TestEditorRenderLineClipsToWidth(t *testing.T) {
	e := NewEditor(strings.Repeat("x", 200)) // cursor lands at the end
	line := stripANSI(e.renderLine(Default, 0, 40))
	if w := len([]rune(line)); w > 40 {
		t.Fatalf("cursor line rendered %d runes wide, want ≤ 40", w)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			in = true
		case in && r == 'm':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestEditorInsertMidLine(t *testing.T) {
	e := NewEditor("hello")
	e.HandleKey(tea.KeyMsg{Type: tea.KeyHome})
	e.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	e.HandleKey(tea.KeyMsg{Type: tea.KeyRight}) // after "he"
	e.HandleKey(runes("XYZ"))
	if got := e.Value(); got != "heXYZllo" {
		t.Fatalf("Value = %q, want heXYZllo", got)
	}
}

package ui

import (
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

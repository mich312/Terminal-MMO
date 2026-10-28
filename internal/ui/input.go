package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// TextInput is a deliberately tiny single-line input — enough for chat and
// the guestbook, no need for the full bubbles component.
type TextInput struct {
	Value   string
	Max     int
	Prompt  string
	focused bool
}

func NewTextInput(prompt string, max int) TextInput {
	return TextInput{Prompt: prompt, Max: max}
}

func (t *TextInput) Focus() { t.focused = true; t.Value = "" }
func (t *TextInput) Blur()  { t.focused = false }

func (t *TextInput) Focused() bool { return t.focused }

// HandleKey consumes a key press. Returns true if the key was used.
func (t *TextInput) HandleKey(msg tea.KeyPressMsg) bool {
	if !t.focused {
		return false
	}
	if msg.String() == "backspace" {
		if len(t.Value) > 0 {
			r := []rune(t.Value)
			t.Value = string(r[:len(r)-1])
		}
		return true
	}
	if msg.Text == "" { // a named key (arrows, ctrl+…) — not text
		return false
	}
	t.Insert(msg.Text)
	return true
}

// Insert appends text up to Max runes — typed characters, or a bracketed paste
// (its own tea.PasteMsg in Bubble Tea v2, a flagged key in v1).
func (t *TextInput) Insert(text string) {
	if !t.focused {
		return
	}
	for _, r := range text {
		if len([]rune(t.Value)) < t.Max {
			t.Value += string(r)
		}
	}
}

func (t *TextInput) View() string {
	cursor := lipgloss.NewStyle().Foreground(ColorAccent).Render("▏")
	return AccentStyle.Render(t.Prompt) + ChatTextStyle.Render(t.Value) + cursor
}

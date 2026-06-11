package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
func (t *TextInput) HandleKey(msg tea.KeyMsg) bool {
	if !t.focused {
		return false
	}
	switch msg.Type {
	case tea.KeyBackspace:
		if len(t.Value) > 0 {
			r := []rune(t.Value)
			t.Value = string(r[:len(r)-1])
		}
		return true
	case tea.KeySpace:
		if len([]rune(t.Value)) < t.Max {
			t.Value += " "
		}
		return true
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if len([]rune(t.Value)) < t.Max {
				t.Value += string(r)
			}
		}
		return true
	}
	return false
}

func (t *TextInput) View() string {
	cursor := lipgloss.NewStyle().Foreground(ColorAccent).Render("▏")
	return AccentStyle.Render(t.Prompt) + ChatTextStyle.Render(t.Value) + cursor
}

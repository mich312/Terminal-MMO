package ui

import tea "charm.land/bubbletea/v2"

// TextKey builds the key press a terminal would send for typed text s, for the
// clients that synthesize key events instead of reading them from Bubble Tea
// (the HD renderer and the browser client) and for tests. A space becomes
// tea.KeySpace, whose String() is "space" in Bubble Tea v2; any other text
// keeps its runes in Text, so String() returns s itself, as v1's KeyRunes did.
func TextKey(s string) tea.KeyPressMsg {
	r := []rune(s)
	switch {
	case s == " ":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case len(r) == 1:
		return tea.KeyPressMsg{Code: r[0], Text: s}
	default:
		return tea.KeyPressMsg{Code: tea.KeyExtended, Text: s}
	}
}

package ui

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// Theme styles render 24-bit SGR escapes (38;2), and the output layer Bubble
// Tea v2 writes each session's frames through downsamples them for the
// client's profile (38;5 for a 256-color terminal) — the contract that lets
// one Theme serve truecolor and 256-color clients alike.
func TestThemeColorProfiles(t *testing.T) {
	cases := []struct {
		name    string
		profile colorprofile.Profile
		want    string
	}{
		{"truecolor", colorprofile.TrueColor, "38;2;"},
		{"256", colorprofile.ANSI256, "38;5;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := &colorprofile.Writer{Forward: &buf, Profile: tc.profile}
			if _, err := w.WriteString(NewTheme().Accent.Render("x")); err != nil {
				t.Fatal(err)
			}
			if out := buf.String(); !strings.Contains(out, tc.want) {
				t.Fatalf("%s: %q missing %q", tc.name, out, tc.want)
			}
		})
	}
}

// HalfBlock packs two pixel rows into one terminal row using the upper-half
// block, and renders transparent pixels as blanks.
func TestHalfBlock(t *testing.T) {
	th := NewTheme()
	out := th.HalfBlock([][]Color{
		{Color("#ffffff"), Transparent},
		{Color("#000000"), Transparent},
	})
	if strings.Contains(out, "\n") {
		t.Fatalf("two pixel rows should be one text row, got %q", out)
	}
	if !strings.Contains(out, "▀") {
		t.Fatalf("expected an upper-half block, got %q", out)
	}
}

// PanelWidth keeps Lip Gloss v1's sizing — w is content plus padding, and the
// border sits outside it — so a Width(28) panel is still 30 cells wide after
// the v2 upgrade, which moved the border inside Width.
func TestPanelWidthKeepsBorderOutside(t *testing.T) {
	out := Default.PanelWidth(28).Render("hello")
	if got := lipgloss.Width(out); got != 30 {
		t.Fatalf("PanelWidth(28) rendered %d cells wide, want 30", got)
	}
}

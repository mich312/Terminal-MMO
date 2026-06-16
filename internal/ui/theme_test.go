package ui

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// A theme bound to a truecolor renderer must emit 24-bit SGR escapes (38;2),
// and one bound to a 256-color renderer must downsample (38;5) — the
// auto-detect contract the per-session renderer relies on.
func TestThemeColorProfiles(t *testing.T) {
	cases := []struct {
		name    string
		profile termenv.Profile
		want    string
	}{
		{"truecolor", termenv.TrueColor, "38;2;"},
		{"256", termenv.ANSI256, "38;5;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := lipgloss.NewRenderer(io.Discard)
			r.SetColorProfile(tc.profile)
			out := NewTheme(r).Accent.Render("x")
			if !strings.Contains(out, tc.want) {
				t.Fatalf("%s: %q missing %q", tc.name, out, tc.want)
			}
		})
	}
}

// HalfBlock packs two pixel rows into one terminal row using the upper-half
// block, and renders transparent pixels as blanks.
func TestHalfBlock(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	th := NewTheme(r)
	out := th.HalfBlock([][]lipgloss.Color{
		{lipgloss.Color("#ffffff"), Transparent},
		{lipgloss.Color("#000000"), Transparent},
	})
	if strings.Contains(out, "\n") {
		t.Fatalf("two pixel rows should be one text row, got %q", out)
	}
	if !strings.Contains(out, "▀") {
		t.Fatalf("expected an upper-half block, got %q", out)
	}
}

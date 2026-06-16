package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Transparent is the sentinel pixel color: it renders as a blank cell so a
// pixel buffer can sit over other content without a hard background box.
const Transparent = lipgloss.Color("")

// HalfBlock renders a pixel buffer as half-height blocks, doubling vertical
// resolution: each output row consumes two pixel rows, drawn with the upper-
// half block '▀' whose foreground is the top pixel and background the bottom.
// Buffers should be an even number of rows; a trailing odd row is dropped.
// Transparent pixels render as spaces (top) / no background.
func (t *Theme) HalfBlock(pix [][]lipgloss.Color) string {
	var b strings.Builder
	for y := 0; y+1 < len(pix); y += 2 {
		top, bot := pix[y], pix[y+1]
		w := len(top)
		if len(bot) > w {
			w = len(bot)
		}
		for x := 0; x < w; x++ {
			tc, bc := Transparent, Transparent
			if x < len(top) {
				tc = top[x]
			}
			if x < len(bot) {
				bc = bot[x]
			}
			switch {
			case tc == Transparent && bc == Transparent:
				b.WriteByte(' ')
			case bc == Transparent:
				b.WriteString(t.r.NewStyle().Foreground(tc).Render("▀"))
			case tc == Transparent:
				b.WriteString(t.r.NewStyle().Foreground(bc).Render("▄"))
			default:
				b.WriteString(t.r.NewStyle().Foreground(tc).Background(bc).Render("▀"))
			}
		}
		if y+3 < len(pix) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Shimmer colors each rune of every line along a moving blue→cyan→blue ramp,
// producing a gradient sweep that animates with phase. Spaces are left as-is.
// Used by the intro title; reusable for any "energized" text.
func (t *Theme) Shimmer(lines []string, a, b string, phase float64) string {
	var out strings.Builder
	for li, line := range lines {
		if li > 0 {
			out.WriteByte('\n')
		}
		runes := []rune(line)
		for ci, r := range runes {
			if r == ' ' {
				out.WriteByte(' ')
				continue
			}
			// triangle wave in [0,1] sweeping across columns over time
			u := float64(ci)*0.06 - float64(li)*0.04 + phase
			tri := u - float64(int(u)) // frac
			if tri < 0 {
				tri += 1
			}
			if tri > 0.5 {
				tri = 1 - tri
			}
			out.WriteString(t.r.NewStyle().Foreground(Blend(a, b, tri*2)).Bold(true).Render(string(r)))
		}
	}
	return out.String()
}

// VGradient colors whole lines along a top→bottom hex ramp (one color per
// row). Cheap vertical gradient for banners and panels.
func (t *Theme) VGradient(lines []string, top, bot string) string {
	n := len(lines)
	var out strings.Builder
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		f := 0.0
		if n > 1 {
			f = float64(i) / float64(n-1)
		}
		out.WriteString(t.r.NewStyle().Foreground(Blend(top, bot, f)).Bold(true).Render(line))
	}
	return out.String()
}

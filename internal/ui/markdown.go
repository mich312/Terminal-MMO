package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/markdown"
)

// RenderSlide renders a markdown slide into a width×height block for the glyph
// client: GFM parsed into styled lines (markdown.Render), each line's spans
// turned into ANSI, left-aligned and vertically centered on the screen.
func (t *Theme) RenderSlide(src string, width, height int) string {
	lines := markdown.Render(src, width)
	rendered := make([]string, 0, len(lines))
	for _, ln := range lines {
		rendered = append(rendered, t.styleLine(ln))
	}
	if len(rendered) > height {
		rendered = rendered[:height]
	}
	top := (height - len(rendered)) / 2
	if top < 0 {
		top = 0
	}
	out := make([]string, 0, height)
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	out = append(out, rendered...)
	for len(out) < height {
		out = append(out, "")
	}
	for i, l := range out {
		out[i] = lipgloss.PlaceHorizontal(width, lipgloss.Left, l)
	}
	return strings.Join(out, "\n")
}

// styleLine turns one markdown line's spans into an ANSI string.
func (t *Theme) styleLine(ln markdown.Line) string {
	var b strings.Builder
	for _, sp := range ln {
		st := t.r.NewStyle()
		if sp.Color != "" {
			st = st.Foreground(lipgloss.Color(sp.Color))
		} else {
			st = st.Foreground(lipgloss.Color(HexText))
		}
		if sp.Bold {
			st = st.Bold(true)
		}
		if sp.Italic {
			st = st.Italic(true)
		}
		if sp.Strike {
			st = st.Strikethrough(true)
		}
		if sp.Underline {
			st = st.Underline(true)
		}
		b.WriteString(st.Render(sp.Text))
	}
	return b.String()
}

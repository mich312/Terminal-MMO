package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Overlay draws fg on top of bg at column x, row y. Both are multi-line,
// possibly ANSI-styled strings. Used for floating panels (guestbook,
// slide screen, player list) over the map.
func Overlay(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fl := range fgLines {
		row := y + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		bl := bgLines[row]
		blWidth := ansi.StringWidth(bl)
		flWidth := ansi.StringWidth(fl)

		left := ansi.Truncate(bl, x, "")
		leftWidth := ansi.StringWidth(left)
		if leftWidth < x {
			left += strings.Repeat(" ", x-leftWidth)
		}
		right := ""
		if blWidth > x+flWidth {
			right = ansi.TruncateLeft(bl, x+flWidth, "")
		}
		bgLines[row] = left + fl + right
	}
	return strings.Join(bgLines, "\n")
}

// Center places content in a w×h box, centered, without lipgloss.Place's
// whitespace options (keeps it predictable for Overlay math).
func Center(content string, w, h int) string {
	lines := strings.Split(content, "\n")
	cw := 0
	for _, l := range lines {
		if lw := ansi.StringWidth(l); lw > cw {
			cw = lw
		}
	}
	padTop := (h - len(lines)) / 2
	if padTop < 0 {
		padTop = 0
	}
	padLeft := (w - cw) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	var b strings.Builder
	for i := 0; i < padTop; i++ {
		b.WriteString("\n")
	}
	prefix := strings.Repeat(" ", padLeft)
	for i, l := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(prefix + l)
	}
	out := b.String()
	// pad bottom so the block is exactly h lines
	have := len(strings.Split(out, "\n"))
	for ; have < h; have++ {
		out += "\n"
	}
	return out
}

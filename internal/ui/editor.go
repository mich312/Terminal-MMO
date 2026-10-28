package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Editor is a minimal multi-line text editor for composing a markdown deck in
// world. It supports typing, Enter for a new line, Backspace, arrow/Home/End
// navigation, Tab, and bracketed paste (so an author can paste a whole deck at
// once). Submit/cancel are the caller's job (it watches for Ctrl+S / Esc before
// forwarding keys here).
type Editor struct {
	lines   []string
	cx, cy  int
	focused bool
}

// NewEditor seeds the buffer with initial text (split on newlines).
func NewEditor(initial string) Editor {
	lines := strings.Split(strings.ReplaceAll(initial, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	e := Editor{lines: lines}
	e.cy = len(lines) - 1
	e.cx = len([]rune(lines[e.cy]))
	return e
}

func (e *Editor) Focus()        { e.focused = true }
func (e *Editor) Blur()         { e.focused = false }
func (e *Editor) Focused() bool { return e.focused }

// Value returns the buffer as a single newline-joined string.
func (e *Editor) Value() string { return strings.Join(e.lines, "\n") }

// HandleKey applies one key to the buffer. Keys are matched by name so a
// modified variant (shift+up, ctrl+left …) stays a no-op, as it was when
// Bubble Tea v1 gave those their own key types.
func (e *Editor) HandleKey(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "space":
		e.insert(" ")
	case "tab":
		e.insert("  ")
	case "enter":
		e.insert("\n")
	case "backspace":
		e.backspace()
	case "left":
		e.left()
	case "right":
		e.right()
	case "up":
		if e.cy > 0 {
			e.cy--
			e.clampX()
		}
	case "down":
		if e.cy < len(e.lines)-1 {
			e.cy++
			e.clampX()
		}
	case "home":
		e.cx = 0
	case "end":
		e.cx = len([]rune(e.lines[e.cy]))
	default:
		if msg.Text != "" {
			e.insert(msg.Text)
		}
	}
}

// Paste inserts a bracketed paste verbatim. Bubble Tea v2 delivers pastes as
// their own tea.PasteMsg rather than as a key with a Paste flag.
func (e *Editor) Paste(text string) { e.insert(text) }

// insert writes text (which may contain newlines) at the cursor.
func (e *Editor) insert(text string) {
	parts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	row := []rune(e.lines[e.cy])
	tail := string(row[e.cx:])
	e.lines[e.cy] = string(row[:e.cx]) + parts[0]
	if len(parts) == 1 {
		e.cx += len([]rune(parts[0]))
		e.lines[e.cy] += tail
		return
	}
	mid := append([]string(nil), parts[1:]...)
	last := len(mid) - 1
	e.cx = len([]rune(mid[last]))
	mid[last] += tail
	rest := append([]string(nil), e.lines[e.cy+1:]...)
	e.lines = append(e.lines[:e.cy+1], append(mid, rest...)...)
	e.cy += len(mid)
}

func (e *Editor) backspace() {
	row := []rune(e.lines[e.cy])
	if e.cx > 0 {
		e.lines[e.cy] = string(row[:e.cx-1]) + string(row[e.cx:])
		e.cx--
		return
	}
	if e.cy > 0 { // join with previous line
		prev := []rune(e.lines[e.cy-1])
		e.cx = len(prev)
		e.lines[e.cy-1] = string(prev) + string(row)
		e.lines = append(e.lines[:e.cy], e.lines[e.cy+1:]...)
		e.cy--
	}
}

func (e *Editor) left() {
	if e.cx > 0 {
		e.cx--
	} else if e.cy > 0 {
		e.cy--
		e.cx = len([]rune(e.lines[e.cy]))
	}
}

func (e *Editor) right() {
	if e.cx < len([]rune(e.lines[e.cy])) {
		e.cx++
	} else if e.cy < len(e.lines)-1 {
		e.cy++
		e.cx = 0
	}
}

func (e *Editor) clampX() {
	if n := len([]rune(e.lines[e.cy])); e.cx > n {
		e.cx = n
	}
}

// View renders the editor panel at the given content width/height with a block
// cursor and a vertical scroll that keeps the cursor in view.
func (e *Editor) View(t *Theme, title string, width, height int) string {
	bodyH := height - 2 // title + footer
	if bodyH < 1 {
		bodyH = 1
	}
	top := 0
	if e.cy >= bodyH {
		top = e.cy - bodyH + 1
	}
	var b strings.Builder
	b.WriteString(t.PanelTitle.Render(title) + "\n")
	for r := top; r < top+bodyH; r++ {
		if r < len(e.lines) {
			b.WriteString(e.renderLine(t, r, width))
		}
		b.WriteByte('\n')
	}
	b.WriteString(t.Dim.Render("Ctrl+S present · Esc cancel · --- splits slides"))
	return t.PanelWidth(width).Render(b.String())
}

func (e *Editor) renderLine(t *Theme, row, width int) string {
	runes := []rune(e.lines[row])
	if row != e.cy {
		return t.ChatText.Render(clip(string(runes), width))
	}
	// Scroll horizontally so the cursor stays inside the panel on long lines.
	start := 0
	if e.cx >= width {
		start = e.cx - width + 1
	}
	end := start + width
	if end > len(runes) {
		end = len(runes)
	}
	win := runes[start:end]
	crel := e.cx - start
	cur := ' '
	if crel < len(win) {
		cur = win[crel]
	}
	before := string(win[:crel])
	after := ""
	if crel < len(win) {
		after = string(win[crel+1:])
	}
	cursor := lipgloss.NewStyle().Reverse(true).Render(string(cur))
	return t.ChatText.Render(before) + cursor + t.ChatText.Render(after)
}

func clip(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return s
}

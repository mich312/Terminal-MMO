package markdown

import "strings"

// cell is one rune carrying its span style, for style-preserving word wrap.
type cell struct {
	r  rune
	sp Span
}

func flatten(spans []Span) []cell {
	var cells []cell
	for _, sp := range spans {
		s := sp
		s.Text = ""
		for _, rr := range sp.Text {
			cells = append(cells, cell{rr, s})
		}
	}
	return cells
}

func sameStyle(a, b Span) bool {
	return a.Color == b.Color && a.Bold == b.Bold && a.Italic == b.Italic &&
		a.Strike == b.Strike && a.Underline == b.Underline && a.Code == b.Code
}

func cellsToLine(cells []cell) Line {
	var line Line
	for _, c := range cells {
		st := c.sp
		st.Text = string(c.r)
		if n := len(line); n > 0 && sameStyle(line[n-1], st) {
			line[n-1].Text += st.Text
		} else {
			line = append(line, st)
		}
	}
	if line == nil {
		line = Line{}
	}
	return line
}

// wrapCells greedily word-wraps styled cells to width, preserving styles.
func wrapCells(cells []cell, width int) []Line {
	if len(cells) == 0 {
		return []Line{{}}
	}
	var lines []Line
	var line []cell
	ll := 0
	for i := 0; i < len(cells); {
		ws := i
		for i < len(cells) && cells[i].r != ' ' {
			i++
		}
		word := cells[ws:i]
		ss := i
		for i < len(cells) && cells[i].r == ' ' {
			i++
		}
		spaces := cells[ss:i]
		if ll > 0 && ll+len(word) > width {
			lines = append(lines, cellsToLine(line))
			line, ll = nil, 0
		}
		line = append(line, word...)
		ll += len(word)
		if ll+len(spaces) <= width {
			line = append(line, spaces...)
			ll += len(spaces)
		} else {
			ll = width // trailing spaces would overflow → wrap next word
		}
	}
	lines = append(lines, cellsToLine(line))
	return lines
}

func wrap(spans []Span, width int) []Line {
	return wrapCells(flatten(spans), width)
}

// prefixed wraps body to width under a prefix (bullet, quote bar), repeating
// the prefix on the first line and indenting continuations.
func prefixed(prefix Span, body []Span, width int) []Line {
	pw := len([]rune(prefix.Text))
	wrapped := wrapCells(flatten(body), width-pw)
	indent := Span{Text: strings.Repeat(" ", pw)}
	out := make([]Line, 0, len(wrapped))
	for i, ln := range wrapped {
		lead := prefix
		if i > 0 {
			lead = indent
		}
		out = append(out, append(Line{lead}, ln...))
	}
	if len(out) == 0 {
		out = []Line{{prefix}}
	}
	return out
}

func bullet(t string) bool {
	return strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ ")
}

// taskMark returns the "- [ ] " / "- [x] " prefix of a task-list item, or "".
func taskMark(t string) string {
	low := strings.ToLower(t)
	if strings.HasPrefix(low, "- [ ] ") || strings.HasPrefix(low, "- [x] ") {
		return t[:6]
	}
	return ""
}

// ordered returns the "N." / "N)" marker of an ordered-list item, or "".
func ordered(t string) string {
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(t) && (t[i] == '.' || t[i] == ')') && t[i+1] == ' ' {
		return t[:i+1]
	}
	return ""
}

func isRule(t string) bool {
	return len(t) >= 3 && (strings.Trim(t, "*") == "" || strings.Trim(t, "_") == "")
}

func rule(width int) Line {
	return Line{{Text: strings.Repeat("─", width), Color: colRule}}
}

// isTableSep reports whether line i is a GFM table delimiter row (|---|:-:|)
// following a header row.
func isTableSep(lines []string, i int) bool {
	if i == 0 {
		return false
	}
	t := strings.TrimSpace(lines[i])
	if !strings.Contains(t, "-") {
		return false
	}
	for _, r := range t {
		if r != '|' && r != '-' && r != ':' && r != ' ' {
			return false
		}
	}
	return strings.Contains(lines[i-1], "|")
}

func splitRow(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	parts := strings.Split(s, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// renderTable renders the table whose header is at lines[hdr] (separator at
// hdr+1). Returns the rendered lines and how many source lines from the
// separator onward were consumed.
func renderTable(lines []string, hdr, width int) ([]Line, int) {
	header := splitRow(lines[hdr])
	var rows [][]string
	n := 1 // the separator
	for j := hdr + 2; j < len(lines); j++ {
		if t := strings.TrimSpace(lines[j]); t == "" || !strings.Contains(t, "|") {
			break
		}
		rows = append(rows, splitRow(lines[j]))
		n++
	}
	cols := len(header)
	w := make([]int, cols)
	for c := 0; c < cols; c++ {
		w[c] = len([]rune(header[c]))
	}
	for _, row := range rows {
		for c := 0; c < cols && c < len(row); c++ {
			if l := len([]rune(row[c])); l > w[c] {
				w[c] = l
			}
		}
	}
	out := []Line{tableRow(header, w, true), tableDivider(w)}
	for _, row := range rows {
		out = append(out, tableRow(row, w, false))
	}
	return out, n
}

func tableRow(cells []string, w []int, header bool) Line {
	var line Line
	for c := 0; c < len(w); c++ {
		if c > 0 {
			line = append(line, Span{Text: " │ ", Color: colRule})
		}
		val := ""
		if c < len(cells) {
			val = cells[c]
		}
		sp := Span{Text: val}
		if header {
			sp.Bold, sp.Color = true, colHeading3
		}
		line = append(line, sp)
		if pad := w[c] - len([]rune(val)); pad > 0 {
			line = append(line, Span{Text: strings.Repeat(" ", pad)})
		}
	}
	return line
}

func tableDivider(w []int) Line {
	var b strings.Builder
	for c, ww := range w {
		if c > 0 {
			b.WriteString("─┼─")
		}
		b.WriteString(strings.Repeat("─", ww))
	}
	return Line{{Text: b.String(), Color: colRule}}
}

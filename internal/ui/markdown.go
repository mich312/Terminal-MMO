package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SlidePlain renders a markdown slide to plain (unstyled) wrapped lines, for
// the HD pixel renderer's bitmap-font screen. Headings keep their text, bullets
// become "• ", quotes "| ", and inline **bold**/*italic*/`code` markers are
// stripped.
func SlidePlain(src string, width int) []string {
	var out []string
	code := false
	for _, raw := range strings.Split(src, "\n") {
		t := strings.TrimSpace(raw)
		if strings.HasPrefix(t, "```") {
			code = !code
			continue
		}
		if code {
			out = append(out, "  "+raw)
			continue
		}
		switch {
		case t == "":
			out = append(out, "")
		case strings.HasPrefix(t, "### "):
			out = append(out, wrapWords(stripInline(t[4:]), width)...)
		case strings.HasPrefix(t, "## "):
			out = append(out, wrapWords(stripInline(t[3:]), width)...)
		case strings.HasPrefix(t, "# "):
			out = append(out, wrapWords(stripInline(t[2:]), width)...)
		case strings.HasPrefix(t, "> "):
			out = append(out, prefixWrap("| ", stripInline(t[2:]), width)...)
		case strings.HasPrefix(t, "- "), strings.HasPrefix(t, "* "):
			out = append(out, prefixWrap("• ", stripInline(t[2:]), width)...)
		default:
			out = append(out, wrapWords(stripInline(t), width)...)
		}
	}
	return out
}

// stripInline removes markdown emphasis markers, leaving the text.
func stripInline(s string) string {
	return strings.NewReplacer("**", "", "`", "", "*", "", "_", "").Replace(s)
}

// prefixWrap wraps body to width and indents continuation lines under prefix.
func prefixWrap(prefix, body string, width int) []string {
	indent := strings.Repeat(" ", len([]rune(prefix)))
	var out []string
	for i, l := range wrapWords(body, width-len([]rune(prefix))) {
		if i == 0 {
			out = append(out, prefix+l)
		} else {
			out = append(out, indent+l)
		}
	}
	if len(out) == 0 {
		out = []string{prefix}
	}
	return out
}

// SplitSlides splits a markdown deck into slide sources on a line of three or
// more dashes (---). Leading/trailing blank slides are kept so the author's
// structure is preserved; an empty deck yields one empty slide.
func SplitSlides(md string) []string {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	var slides []string
	var cur []string
	flush := func() { slides = append(slides, strings.Join(cur, "\n")); cur = nil }
	for _, line := range strings.Split(md, "\n") {
		if t := strings.TrimSpace(line); len(t) >= 3 && strings.Trim(t, "-") == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	if len(slides) == 0 {
		return []string{""}
	}
	return slides
}

// RenderSlide renders one markdown slide into a width×height block, centered.
// It supports headings (#, ##, ###), bullet (-, *) and quote (>) lines, fenced
// and inline code, and inline **bold**, *italic* and `code`. Long lines wrap on
// word boundaries.
func (t *Theme) RenderSlide(src string, width, height int) string {
	lines := t.markdownLines(src, width)
	// vertical-center within height
	if len(lines) > height {
		lines = lines[:height]
	}
	top := (height - len(lines)) / 2
	out := make([]string, 0, height)
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	out = append(out, lines...)
	for len(out) < height {
		out = append(out, "")
	}
	// horizontal-center each line
	for i, l := range out {
		out[i] = lipgloss.PlaceHorizontal(width, lipgloss.Center, l)
	}
	return strings.Join(out, "\n")
}

// markdownLines renders the block markdown of src into styled, width-wrapped
// lines (no centering).
func (t *Theme) markdownLines(src string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	code := false
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimRight(raw, " ")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") { // fenced code toggles
			code = !code
			continue
		}
		if code {
			out = append(out, t.Faint.Render("  "+line))
			continue
		}
		switch {
		case trimmed == "":
			out = append(out, "")
		case strings.HasPrefix(trimmed, "### "):
			out = append(out, wrapStyled(t, trimmed[4:], width, t.Bright)...)
		case strings.HasPrefix(trimmed, "## "):
			out = append(out, wrapStyled(t, trimmed[3:], width, t.Accent.Bold(true))...)
		case strings.HasPrefix(trimmed, "# "):
			out = append(out, wrapStyled(t, trimmed[2:], width, t.Bright.Underline(true))...)
		case strings.HasPrefix(trimmed, "> "):
			body := t.inline(trimmed[2:], t.Dim.Italic(true))
			out = append(out, wrapPrefixed(t.Faint.Render("┃ "), body, trimmed[2:], width)...)
		case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
			body := t.inline(trimmed[2:], t.ChatText)
			out = append(out, wrapPrefixed(t.Accent.Render("• "), body, trimmed[2:], width)...)
		default:
			out = append(out, wrapInline(t, line, width, t.ChatText)...)
		}
	}
	return out
}

// wrapStyled wraps plain words to width and renders each line in one style.
func wrapStyled(t *Theme, text string, width int, style lipgloss.Style) []string {
	var out []string
	for _, l := range wrapWords(text, width) {
		out = append(out, style.Render(l))
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// wrapInline word-wraps text (with inline markup) to width, styling each line.
func wrapInline(t *Theme, text string, width int, base lipgloss.Style) []string {
	var out []string
	for _, l := range wrapWords(strings.TrimSpace(text), width) {
		out = append(out, t.inline(l, base))
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// wrapPrefixed wraps a bullet/quote body to width, repeating the prefix on the
// first line and indenting continuations. plain is the unstyled body text used
// for measuring the wrap.
func wrapPrefixed(prefix, _styled, plain string, width int) []string {
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	wrapped := wrapWords(plain, width-lipgloss.Width(prefix))
	var out []string
	for i, l := range wrapped {
		p := prefix
		if i > 0 {
			p = indent
		}
		out = append(out, p+l)
	}
	if len(out) == 0 {
		out = []string{prefix}
	}
	return out
}

// wrapWords greedily wraps plain text to width on spaces.
func wrapWords(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len([]rune(cur))+1+len([]rune(w)) <= width {
			cur += " " + w
		} else {
			lines = append(lines, cur)
			cur = w
		}
	}
	return append(lines, cur)
}

// inline renders **bold**, *italic*/_italic_ and `code` over a base style.
func (t *Theme) inline(s string, base lipgloss.Style) string {
	codeStyle := t.Fg(lipgloss.Color(HexAccent2))
	var b strings.Builder
	r := []rune(s)
	for i := 0; i < len(r); {
		switch {
		case strings.HasPrefix(string(r[i:]), "**"):
			if j := indexOf(r, i+2, "**"); j >= 0 {
				b.WriteString(base.Bold(true).Render(string(r[i+2 : j])))
				i = j + 2
				continue
			}
		case r[i] == '`':
			if j := indexRune(r, i+1, '`'); j >= 0 {
				b.WriteString(codeStyle.Render(string(r[i+1 : j])))
				i = j + 1
				continue
			}
		case r[i] == '*' || r[i] == '_':
			if j := indexRune(r, i+1, r[i]); j >= 0 {
				b.WriteString(base.Italic(true).Render(string(r[i+1 : j])))
				i = j + 1
				continue
			}
		}
		b.WriteString(base.Render(string(r[i])))
		i++
	}
	return b.String()
}

func indexOf(r []rune, from int, sub string) int {
	s := []rune(sub)
	for i := from; i+len(s) <= len(r); i++ {
		if string(r[i:i+len(s)]) == sub {
			return i
		}
	}
	return -1
}

func indexRune(r []rune, from int, c rune) int {
	for i := from; i < len(r); i++ {
		if r[i] == c {
			return i
		}
	}
	return -1
}

// Package markdown renders GitHub-flavored Markdown (as far as makes sense on a
// terminal slide) into styled spans that two backends consume: the ui package
// draws them as ANSI for the glyph client, and the pixel package draws them as
// colored bitmap text for HD. Keeping one parser and a neutral span model means
// headings, lists, tables, links and syntax-highlighted code look the same in
// both renderers.
//
// Supported: headings, bold/italic/strikethrough, inline and fenced code (with
// chroma syntax highlighting), ordered/unordered/task lists, blockquotes,
// tables, horizontal rules, links (shown as styled text) and images (shown as
// their alt text). Deliberately skipped: deep nesting, reference links, raw
// HTML, footnotes — rare on a slide.
package markdown

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/durst-group/durstworld/internal/palette"
)

// Span is a run of text with a uniform style. Color is a "#rrggbb" hex, or ""
// to use the backend's default text color.
type Span struct {
	Text                            string
	Color                           string
	Bold, Italic, Strike, Underline bool
	Code                            bool // monospace code (inline or block)
}

// Line is one rendered output line.
type Line []Span

// IsCode reports whether a line is a fenced-code line (every non-blank span is
// code), so a backend can give it a block background. Blank lines and ordinary
// paragraphs with inline code are not code lines.
func (l Line) IsCode() bool {
	if len(l) == 0 {
		return false
	}
	code := false
	for _, sp := range l {
		if sp.Code {
			code = true
			continue
		}
		if strings.TrimSpace(sp.Text) != "" {
			return false
		}
	}
	return code
}

// Semantic colors, drawn from the shared palette so both backends agree.
const (
	colHeading1 = palette.Bright
	colHeading2 = palette.Accent2
	colHeading3 = palette.Text
	colAccent   = palette.Accent
	colLink     = palette.Link
	colCode     = palette.Accent2
	colQuote    = palette.Toast
	colRule     = palette.Dim
)

// SplitSlides splits a deck on a line of three or more dashes (---).
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

// Render parses one slide's markdown into wrapped, styled lines for width.
func Render(src string, width int) []Line {
	if width < 4 {
		width = 4
	}
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var out []Line
	for i := 0; i < len(lines); {
		raw := lines[i]
		t := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"):
			fence := t[:3]
			lang := strings.TrimSpace(t[3:])
			i++
			var code []string
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), fence) {
				code = append(code, lines[i])
				i++
			}
			if i < len(lines) {
				i++ // closing fence
			}
			out = append(out, highlight(strings.Join(code, "\n"), lang)...)
		case isTableSep(lines, i):
			tbl, n := renderTable(lines, i-1, width)
			// the header row was already emitted; replace it
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
			out = append(out, tbl...)
			i += n
		case t == "":
			out = append(out, Line{})
			i++
		case isRule(t):
			out = append(out, rule(width))
			i++
		case strings.HasPrefix(t, "#"):
			out = append(out, heading(t, width)...)
			i++
		case strings.HasPrefix(t, ">"):
			body := strings.TrimSpace(strings.TrimPrefix(t, ">"))
			out = append(out, prefixed(Span{Text: "┃ ", Color: colRule}, italicize(inline(body)), width)...)
			i++
		case taskMark(t) != "":
			mark := taskMark(t)
			rest := strings.TrimSpace(t[len(mark):])
			box := "☐ "
			if strings.HasPrefix(strings.ToLower(mark), "- [x") {
				box = "☑ "
			}
			out = append(out, prefixed(Span{Text: box, Color: colAccent}, inline(rest), width)...)
			i++
		case bullet(t):
			rest := strings.TrimSpace(t[2:])
			out = append(out, prefixed(Span{Text: "• ", Color: colAccent}, inline(rest), width)...)
			i++
		case ordered(t) != "":
			num := ordered(t)
			rest := strings.TrimSpace(t[len(num):])
			out = append(out, prefixed(Span{Text: num + " ", Color: colAccent}, inline(rest), width)...)
			i++
		default:
			out = append(out, wrap(inline(t), width)...)
			i++
		}
	}
	return out
}

func heading(t string, width int) []Line {
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	text := strings.TrimSpace(t[n:])
	col, under := colHeading3, false
	switch n {
	case 1:
		col, under = colHeading1, true
	case 2:
		col = colHeading2
	}
	sp := inline(text)
	for i := range sp {
		sp[i].Bold = true
		if sp[i].Color == "" {
			sp[i].Color = col
		}
		sp[i].Underline = sp[i].Underline || under
	}
	return wrap(sp, width)
}

func italicize(sp []Span) []Span {
	for i := range sp {
		sp[i].Italic = true
		if sp[i].Color == "" {
			sp[i].Color = colQuote
		}
	}
	return sp
}

// inline parses inline markup into styled spans by toggling active styles.
func inline(s string) []Span {
	var spans []Span
	var bold, italic, strike, code bool
	r := []rune(s)
	var buf []rune
	flush := func() {
		if len(buf) > 0 {
			sp := Span{Text: string(buf), Bold: bold, Italic: italic, Strike: strike, Code: code}
			if code {
				sp.Color = colCode
			}
			spans = append(spans, sp)
			buf = buf[:0]
		}
	}
	for i := 0; i < len(r); {
		if code { // raw until closing backtick
			if r[i] == '`' {
				flush()
				code = false
				i++
			} else {
				buf = append(buf, r[i])
				i++
			}
			continue
		}
		switch {
		case has(r, i, "**"), has(r, i, "__"):
			flush()
			bold = !bold
			i += 2
		case has(r, i, "~~"):
			flush()
			strike = !strike
			i += 2
		case r[i] == '`':
			flush()
			code = true
			i++
		case r[i] == '*' || r[i] == '_':
			flush()
			italic = !italic
			i++
		case r[i] == '!' && i+1 < len(r) && r[i+1] == '[': // image → alt text
			if alt, _, n := link(r, i+1); n > 0 {
				flush()
				spans = append(spans, Span{Text: alt, Italic: true, Color: colQuote})
				i += n + 1
				continue
			}
			buf = append(buf, r[i])
			i++
		case r[i] == '[': // link → styled text
			if text, _, n := link(r, i); n > 0 {
				flush()
				spans = append(spans, Span{Text: text, Underline: true, Color: colLink, Bold: bold, Italic: italic})
				i += n
				continue
			}
			buf = append(buf, r[i])
			i++
		default:
			buf = append(buf, r[i])
			i++
		}
	}
	flush()
	return spans
}

// highlight tokenizes a fenced code block with chroma and emits one Line per
// code line, each token a span colored by the github-dark style. Unknown
// languages fall back to a plain (uncolored) render.
func highlight(code, lang string) []Line {
	lang = strings.ToLower(strings.TrimSpace(lang))
	var lx chroma.Lexer
	if lang != "" {
		lx = lexers.Get(lang)
	}
	if lx == nil {
		lx = lexers.Fallback
	}
	it, err := lx.Tokenise(nil, code)
	if err != nil {
		return codePlain(code)
	}
	style := styles.Get("github-dark")
	if style == nil {
		style = styles.Fallback
	}
	var lines []Line
	cur := Line{{Text: "  ", Code: true}} // 2-space gutter
	for _, tok := range it.Tokens() {
		hex := ""
		if c := style.Get(tok.Type).Colour; c != 0 {
			hex = c.String()
		}
		for k, part := range strings.Split(tok.Value, "\n") {
			if k > 0 {
				lines = append(lines, cur)
				cur = Line{{Text: "  ", Code: true}}
			}
			if part != "" {
				cur = append(cur, Span{Text: part, Color: hex, Code: true})
			}
		}
	}
	lines = append(lines, cur)
	if n := len(lines); n > 0 && len(lines[n-1]) == 1 && lines[n-1][0].Text == "  " {
		lines = lines[:n-1] // drop the trailing blank from a final newline
	}
	return lines
}

func codePlain(code string) []Line {
	var out []Line
	for _, l := range strings.Split(code, "\n") {
		out = append(out, Line{{Text: "  " + l, Code: true, Color: colCode}})
	}
	return out
}

func has(r []rune, i int, s string) bool {
	rs := []rune(s)
	if i+len(rs) > len(r) {
		return false
	}
	return string(r[i:i+len(rs)]) == s
}

// link parses [text](url) starting at r[i]=='['; returns text, url and the
// number of runes consumed (0 if not a link).
func link(r []rune, i int) (string, string, int) {
	close := -1
	for j := i + 1; j < len(r); j++ {
		if r[j] == ']' {
			close = j
			break
		}
	}
	if close < 0 || close+1 >= len(r) || r[close+1] != '(' {
		return "", "", 0
	}
	end := -1
	for j := close + 2; j < len(r); j++ {
		if r[j] == ')' {
			end = j
			break
		}
	}
	if end < 0 {
		return "", "", 0
	}
	return string(r[i+1 : close]), string(r[close+2 : end]), end - i + 1
}

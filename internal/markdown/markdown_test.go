package markdown

import (
	"strings"
	"testing"
)

func plain(lines []Line) string {
	var b strings.Builder
	for _, ln := range lines {
		for _, sp := range ln {
			b.WriteString(sp.Text)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestSplitSlides(t *testing.T) {
	if got := SplitSlides("a\n---\nb\n----\nc"); len(got) != 3 {
		t.Fatalf("got %d slides, want 3", len(got))
	}
	if got := SplitSlides("only"); len(got) != 1 {
		t.Errorf("no separator → %d slides", len(got))
	}
}

func TestInlineStripsMarkers(t *testing.T) {
	lines := Render("# Title\n\n**bold** _it_ ~~no~~ `code` and [link](http://x)", 80)
	txt := plain(lines)
	for _, m := range []string{"**", "~~", "`", "](http"} {
		if strings.Contains(txt, m) {
			t.Errorf("marker %q leaked: %q", m, txt)
		}
	}
	if !strings.Contains(txt, "Title") || !strings.Contains(txt, "bold") || !strings.Contains(txt, "link") {
		t.Errorf("content lost: %q", txt)
	}
	// the heading span should be bold
	bold := false
	for _, sp := range lines[0] {
		if sp.Bold {
			bold = true
		}
	}
	if !bold {
		t.Error("heading not bold")
	}
}

func TestCodeHighlight(t *testing.T) {
	lines := Render("```go\nfunc main() { x := 1 }\n```", 80)
	colored := false
	for _, ln := range lines {
		for _, sp := range ln {
			if sp.Code && sp.Color != "" {
				colored = true
			}
		}
	}
	if !colored {
		t.Error("code block produced no highlighted (colored) spans")
	}
}

func TestTable(t *testing.T) {
	lines := Render("| Name | Qty |\n|------|-----|\n| Ink | 2 |\n| Paper | 5 |", 80)
	txt := plain(lines)
	if !strings.Contains(txt, "Name") || !strings.Contains(txt, "Paper") {
		t.Errorf("table content missing: %q", txt)
	}
	if !strings.Contains(txt, "┼") && !strings.Contains(txt, "─") {
		t.Errorf("table divider missing: %q", txt)
	}
}

func TestWrap(t *testing.T) {
	for _, ln := range Render(strings.Repeat("word ", 40), 20) {
		w := 0
		for _, sp := range ln {
			w += len([]rune(sp.Text))
		}
		if w > 20 {
			t.Errorf("line width %d > 20", w)
		}
	}
}

func TestTaskAndOrdered(t *testing.T) {
	txt := plain(Render("- [ ] todo\n- [x] done\n1. first\n2. second", 80))
	if !strings.Contains(txt, "☐") || !strings.Contains(txt, "☑") {
		t.Errorf("task boxes missing: %q", txt)
	}
	if !strings.Contains(txt, "1.") {
		t.Errorf("ordered marker missing: %q", txt)
	}
}

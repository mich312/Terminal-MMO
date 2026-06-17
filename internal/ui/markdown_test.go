package ui

import (
	"strings"
	"testing"
)

func TestSplitSlides(t *testing.T) {
	got := SplitSlides("# A\nintro\n---\n## B\nbody\n----\nlast")
	if len(got) != 3 {
		t.Fatalf("got %d slides, want 3: %#v", len(got), got)
	}
	if !strings.Contains(got[0], "# A") || !strings.Contains(got[1], "## B") || !strings.Contains(got[2], "last") {
		t.Errorf("slides split wrong: %#v", got)
	}
	if s := SplitSlides("just one slide"); len(s) != 1 {
		t.Errorf("no separator should be one slide, got %d", len(s))
	}
	if s := SplitSlides(""); len(s) != 1 {
		t.Errorf("empty deck should be one empty slide, got %d", len(s))
	}
}

func TestRenderSlideShape(t *testing.T) {
	out := Default.RenderSlide("# Hello\n\nworld with **bold** text", 30, 8)
	lines := strings.Split(out, "\n")
	if len(lines) != 8 {
		t.Fatalf("RenderSlide should fill height 8, got %d lines", len(lines))
	}
	if !strings.Contains(out, "Hello") || !strings.Contains(out, "world") {
		t.Errorf("rendered slide missing content: %q", out)
	}
}

func TestMarkdownWraps(t *testing.T) {
	long := strings.Repeat("word ", 20)
	lines := Default.markdownLines(long, 20)
	for _, l := range lines {
		if w := len([]rune(stripANSITest(l))); w > 20 {
			t.Errorf("line exceeds width 20: %d (%q)", w, l)
		}
	}
}

// stripANSITest removes SGR codes so we can measure visible width in tests.
func stripANSITest(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			in = true
		case in && r == 'm':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

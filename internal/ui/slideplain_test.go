package ui

import (
	"strings"
	"testing"
)

func TestSlidePlain(t *testing.T) {
	lines := SlidePlain("# Title\n\n- **bold** point\n- second\n\n> a quote", 40)
	j := strings.Join(lines, "\n")
	if !strings.Contains(j, "Title") {
		t.Error("heading text dropped")
	}
	if strings.Contains(j, "**") || strings.Contains(j, "`") {
		t.Errorf("inline markers not stripped: %q", j)
	}
	if !strings.Contains(j, "• ") || !strings.Contains(j, "| ") {
		t.Errorf("bullet/quote prefixes missing: %q", j)
	}
}

func TestSlidePlainWraps(t *testing.T) {
	for _, l := range SlidePlain(strings.Repeat("word ", 30), 20) {
		if len([]rune(l)) > 20 {
			t.Errorf("line over width 20: %q", l)
		}
	}
}

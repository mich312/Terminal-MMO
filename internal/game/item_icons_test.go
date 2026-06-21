package game

import "testing"

// Every catalog item needs a compendium portrait, and each icon must be a clean
// itemIconN×itemIconN grid (the renderer crops and scales from that). A new
// collectible without art, or a mis-sized bitmap, fails here rather than drawing
// a fallback gem or a skewed sprite.
func TestItemIconsWellFormed(t *testing.T) {
	for _, it := range Items {
		art, ok := itemIcons[it.ID]
		if !ok {
			t.Errorf("item %q has no compendium icon", it.ID)
			continue
		}
		if len(art) != itemIconN {
			t.Errorf("%s icon has %d rows, want %d", it.ID, len(art), itemIconN)
		}
		for r, row := range art {
			if n := len([]rune(row)); n != itemIconN {
				t.Errorf("%s row %d is %d wide, want %d: %q", it.ID, r, n, itemIconN, row)
			}
		}
	}
}

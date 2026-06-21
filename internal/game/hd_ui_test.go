package game

import (
	"image"
	"strings"
	"testing"

	"github.com/durst-group/durstworld/internal/pixel"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
)

// Customization is shared between the glyph and HD panels, so it must cycle the
// body and gate hats to the ones unlocked.
func TestCycleAvatarFieldGatesHats(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &Ctx{World: w, Store: store.Open(t.TempDir() + "/x.db"), Name: name, Hats: map[int]bool{}}

	before, _ := w.Self(name)
	CycleAvatarField(ctx, 0, 1)
	if after, _ := w.Self(name); NumAvatarStyles() > 1 && after.Style == before.Style {
		t.Error("style should change when cycled")
	}
	// No hats owned → the hat field can't move off "none".
	CycleAvatarField(ctx, 2, 1)
	if c, _ := w.Self(name); c.Accessory != 0 {
		t.Errorf("hat became %d with none unlocked", c.Accessory)
	}
	// Unlock one and it becomes selectable.
	ctx.Hats[2] = true
	CycleAvatarField(ctx, 2, 1)
	if c, _ := w.Self(name); c.Accessory != 2 {
		t.Errorf("hat = %d, want 2 once unlocked", c.Accessory)
	}
}

func TestAsciiOnly(t *testing.T) {
	if got := asciiOnly("ab—cd＋"); got != "abcd" {
		t.Errorf("asciiOnly = %q, want \"abcd\"", got)
	}
}

func TestTruncToWidth(t *testing.T) {
	full := "hello world"
	if got := truncToWidth(full, 2, pixel.TextWidth(full, 2)); got != full {
		t.Errorf("a string that fits must pass through unchanged, got %q", got)
	}
	// Width for ~5 glyphs forces a cut, and the result must end in ".." and fit.
	narrow := pixel.TextWidth("xxxxx", 2)
	got := truncToWidth(full, 2, narrow)
	if got == full || !strings.HasSuffix(got, "..") {
		t.Errorf("truncated = %q, want a shortened string ending in ..", got)
	}
	if pixel.TextWidth(got, 2) > narrow {
		t.Errorf("truncated %q is wider (%d) than the budget (%d)", got, pixel.TextWidth(got, 2), narrow)
	}
	if truncToWidth(full, 2, 0) != "" {
		t.Error("zero width must yield empty string")
	}
}

// The on-frame chrome must lay out without panicking — and actually draw —
// across a tiny frame and a contextual prompt long enough to need truncation,
// so the prompt never runs off the frame.
func TestHDChromeLayouts(t *testing.T) {
	for _, c := range []struct{ w, h int }{{200, 120}, {820, 480}, {1600, 900}} {
		img := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
		DrawAreaTitle(img, "Presentation Wing", 1)
		DrawAreaTitle(img, "Presentation Wing", 0) // settled state
		DrawTopLegend(img)
		DrawActionPrompt(img,
			"e — sign the guestbook before you head off and greet everyone here")
		DrawMenuPanel(img, 1)
		drawn := false
		for _, b := range img.Pix {
			if b != 0 {
				drawn = true
				break
			}
		}
		if !drawn {
			t.Errorf("%dx%d: chrome drew nothing", c.w, c.h)
		}
	}
}

// The HD overlays draw straight onto an RGBA frame; they must handle a live
// player + inventory without panicking (basicfont/ASCII, bounds-checked).
func TestHDPanelsRender(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &Ctx{World: w, Store: store.Open(t.TempDir() + "/y.db"), Name: name,
		Inventory: map[string]int{"berry": 2}, Hats: map[int]bool{2: true}}
	img := image.NewRGBA(image.Rect(0, 0, 600, 360))
	DrawAreaTitle(img, "The Wilds", 0.5)
	DrawToast(img, "+ Sweet Berry")
	DrawCharPanel(img, ctx, 0)
	scroll := 0
	DrawCompendiumPanel(img, ctx, &scroll)
}

// The compendium panel lists the whole catalog and must lay out without
// panicking across frame sizes and inventory states: empty, full, and a single
// item. It scrolls, so an over-large offset must clamp (not index out of range)
// and the top of the list must always draw something.
func TestCompendiumPanelLayouts(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	st := store.Open(t.TempDir() + "/z.db")

	full := map[string]int{}
	for _, it := range Items {
		full[it.ID] = 3
	}
	manyHats := map[int]bool{}
	for i := 1; i <= 10; i++ {
		manyHats[i] = true
	}

	cases := []struct {
		name string
		w, h int
		ctx  *Ctx
	}{
		{"empty-small", 600, 360, &Ctx{World: w, Store: st, Name: name, Inventory: map[string]int{}, Hats: map[int]bool{}}},
		{"full-large", 1900, 1200, &Ctx{World: w, Store: st, Name: name, Inventory: full, Hats: manyHats}},
		{"one-item", 1000, 700, &Ctx{World: w, Store: st, Name: name, Inventory: map[string]int{"berry": 1}, Hats: map[int]bool{2: true}}},
	}
	for _, c := range cases {
		for _, scroll := range []int{0, 9999} {
			img := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
			sc := scroll
			DrawCompendiumPanel(img, c.ctx, &sc)
			if sc < 0 {
				t.Errorf("%s scroll=%d: clamped negative (%d)", c.name, scroll, sc)
			}
			drawn := false
			for _, b := range img.Pix {
				if b != 0 {
					drawn = true
					break
				}
			}
			if !drawn {
				t.Errorf("%s scroll=%d: panel drew nothing", c.name, scroll)
			}
		}
	}
}

package game

import (
	"image"
	"testing"

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

// The HD overlays draw straight onto an RGBA frame; they must handle a live
// player + inventory without panicking (basicfont/ASCII, bounds-checked).
func TestHDPanelsRender(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &Ctx{World: w, Store: store.Open(t.TempDir() + "/y.db"), Name: name,
		Inventory: map[string]int{"berry": 2}, Hats: map[int]bool{2: true}}
	img := image.NewRGBA(image.Rect(0, 0, 600, 360))
	DrawHUD(img, "The Wilds", "e - take Sweet Berry")
	DrawToast(img, "+ Sweet Berry")
	DrawCharPanel(img, ctx, 0)
	DrawInventoryPanel(img, ctx)
}

// The redesigned inventory panel includes a screen-scaled avatar and must lay
// out without panicking across frame sizes and inventory states: empty, full,
// and more hats than fit (the "+N more" overflow). It draws onto the frame, so
// each render must touch some pixels.
func TestInventoryPanelLayouts(t *testing.T) {
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
		img := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
		DrawInventoryPanel(img, c.ctx)
		drawn := false
		for _, b := range img.Pix {
			if b != 0 {
				drawn = true
				break
			}
		}
		if !drawn {
			t.Errorf("%s: panel drew nothing", c.name)
		}
	}
}

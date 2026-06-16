package game

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/durst-group/durstworld/internal/ui"
)

// trueColorTheme renders 24-bit color into the void, so tests can assert on
// the SGR escapes lighting/animation produce.
func trueColorTheme() *ui.Theme {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	return ui.NewTheme(r)
}

// An animated tile cycles its glyph with the frame counter.
func TestAnimatedTileGlyphCycles(t *testing.T) {
	legend := map[rune]LegendEntry{
		'~': {Kind: TileFloor, Ch: '~', Walkable: true, Anim: &TileAnim{
			Frames: []rune{'~', '≈', '≋'}, Speed: 1}},
	}
	tm := ParseMap([]string{"~"}, legend, nil)
	th := trueColorTheme()
	for frame, want := range []rune{'~', '≈', '≋', '~'} {
		out := RenderMap(th, tm, nil, "", frame)
		if !strings.ContainsRune(out, want) {
			t.Fatalf("frame %d: %q missing glyph %q", frame, out, want)
		}
	}
}

// A radial light leaves the source bright and darkens tiles past its radius,
// so a lit render differs from an unlit one.
func TestLightingDarkensDistance(t *testing.T) {
	tm := ParseMap([]string{".........."}, nil, nil)
	th := trueColorTheme()
	cam := Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	unlit := RenderViewport(th, tm, nil, "", 0, cam)
	lit := RenderLitViewport(th, tm, nil, "", 0, cam, Light{X: 0, Y: 0, Radius: 4})
	if unlit == lit {
		t.Fatal("expected lighting to change the rendered colors")
	}
}

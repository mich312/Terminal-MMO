package game

import (
	"io"
	"strings"
	"testing"
	"time"

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

// DayFadedLight's falloff should bite at night but vanish by midday, so the same
// light renders identically to an unlit frame in full daylight and differently
// after dark.
func TestDayFadedLightFadesByDay(t *testing.T) {
	defer func() { ui.Now = time.Now }()
	tm := ParseMap([]string{".........."}, nil, nil)
	th := trueColorTheme()
	cam := Camera{X: 0, Y: 0, W: tm.W, H: tm.H}

	// The cycle is compressed into one real hour, so the minute-of-hour drives
	// the time of day: minute 0 is midnight, minute 30 is noon.
	at := func(cycleHour int) (unlit, lit string) {
		min := cycleHour * 60 / 24
		ui.Now = func() time.Time { return time.Date(2026, 1, 1, 0, min, 0, 0, time.UTC) }
		unlit = RenderViewport(th, tm, nil, "", 0, cam)
		lit = RenderLitViewport(th, tm, nil, "", 0, cam, DayFadedLight(Light{X: 0, Y: 0, Radius: 4}))
		return
	}

	if unlit, lit := at(12); unlit != lit {
		t.Error("midday: day-faded light should leave the frame fully lit (no darkening)")
	}
	if unlit, lit := at(0); unlit == lit {
		t.Error("midnight: day-faded light should darken tiles past its radius")
	}
}

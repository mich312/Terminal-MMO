package game

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/ui"
)

// findWeather scans forward from a fixed instant until the storm field at
// (wx,wy) satisfies pred — deterministic, so every run lands on the same
// moment. Fatals rather than looping forever if the field never gets there.
func findWeather(t *testing.T, wx, wy int, pred func(float64) bool) time.Time {
	t.Helper()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for probe := base; probe.Before(base.Add(48 * time.Hour)); probe = probe.Add(3 * time.Minute) {
		if pred(StormAt(probe, wx, wy)) {
			return probe
		}
	}
	t.Fatal("storm field never satisfied the predicate in 48h — the coverage rhythm is broken")
	return base
}

// TestStormDeterministic: the field is a pure function of (time, cell) — two
// sessions sampling the same instant agree exactly, and the bucket quantization
// keeps every read within a bucket identical.
func TestStormDeterministic(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 1, 0, time.UTC)
	for _, cell := range [][2]int{{0, 0}, {123, -456}, {-7, 9000}} {
		a := StormAt(at, cell[0], cell[1])
		b := StormAt(at, cell[0], cell[1])
		if a != b {
			t.Fatalf("StormAt not deterministic at %v: %v vs %v", cell, a, b)
		}
		// Any instant within the same bucket must read identically.
		c := StormAt(at.Add(weatherBucket/2), cell[0], cell[1])
		if got := StormAt(at.Truncate(weatherBucket), cell[0], cell[1]); got != a || c != a {
			t.Fatalf("bucket quantization broken at %v: %v / %v / %v", cell, a, c, got)
		}
	}
}

// TestStormRangeAndVariety: intensity stays in [0,1], and over a couple of days
// the world sees both dry spells and real weather (the coverage rhythm works).
func TestStormRangeAndVariety(t *testing.T) {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	wet, dry := false, false
	for h := 0; h < 48*20; h++ {
		at := base.Add(time.Duration(h) * 3 * time.Minute)
		i := StormAt(at, 40, -25)
		if i < 0 || i > 1 {
			t.Fatalf("intensity out of range at +%dm: %v", h*3, i)
		}
		if i > 0.5 {
			wet = true
		}
		if i == 0 {
			dry = true
		}
	}
	if !wet || !dry {
		t.Fatalf("expected both weather and clear spells over 48h (wet=%v dry=%v)", wet, dry)
	}
}

// TestStormIsAFront: at a rainy moment the field is spatial — somewhere within
// a few front-wavelengths it is dry, so weather is a place you can walk out of,
// not a global switch.
func TestStormIsAFront(t *testing.T) {
	at := findWeather(t, 0, 0, func(i float64) bool { return i > 0.7 })
	for r := 40; r <= 2000; r += 40 {
		if StormAt(at, r, 0) == 0 || StormAt(at, 0, r) == 0 || StormAt(at, -r, -r) == 0 {
			return
		}
	}
	t.Fatal("storm covered everything within 2000 tiles — the front has no edge")
}

// TestPrecipHostSparse: the per-cell host roll stays within its budget — the
// incremental renderer's frame cost rides on this.
func TestPrecipHostSparse(t *testing.T) {
	at := findWeather(t, 0, 0, func(i float64) bool { return i > 0.9 })
	n, hosts := 0, 0
	for y := -40; y < 40; y++ {
		for x := -40; x < 40; x++ {
			n++
			if _, ok := precipHost(at, x, y); ok {
				hosts++
			}
		}
	}
	if hosts == 0 {
		t.Fatal("no host cells in the middle of a heavy storm")
	}
	if frac := float64(hosts) / float64(n); frac > precipDensity*1.3 {
		t.Fatalf("host fraction %.3f exceeds the precipDensity budget %.3f", frac, precipDensity)
	}
}

// TestGlyphRainOverlay: at a rainy moment the glyph renderer shows streak runes
// on open ground under a Sky light — and shows none when the same scene renders
// without one (an interior never rains).
func TestGlyphRainOverlay(t *testing.T) {
	orig := ui.Now
	defer func() { ui.Now = orig }()
	at := findWeather(t, 10, 10, func(i float64) bool { return i > 0.9 })
	ui.Now = func() time.Time { return at }

	const n = 20
	tiles := make([][]Tile, n)
	for y := 0; y < n; y++ {
		tiles[y] = make([]Tile, n)
		for x := 0; x < n; x++ {
			tiles[y][x] = Tile{Kind: TileFloor, Walkable: true, Tex: TexGrass, Ground: "#5EAE63"}
		}
	}
	tm := &TileMap{W: n, H: n, Tiles: tiles}
	cam := Camera{W: n, H: n}

	streaks := func(light Light) int {
		grid := buildGrid(nil, tm, cam, light, 0, 0, 0)
		c := 0
		for _, row := range grid {
			for _, cell := range row {
				if cell.ch == '\'' || cell.ch == ',' || cell.ch == '.' {
					c++
				}
			}
		}
		return c
	}
	if got := streaks(Light{Sky: true}); got == 0 {
		t.Fatal("no rain glyphs on open ground in a heavy storm")
	}
	if got := streaks(Light{}); got != 0 {
		t.Fatalf("%d rain glyphs indoors — weather must be Sky-gated", got)
	}
}

// TestHDWeatherDraws: the HD frame changes when the weather layer is switched
// on over a rainy scene, and rain in one frame animates against the next.
func TestHDWeatherDraws(t *testing.T) {
	orig := ui.Now
	defer func() { ui.Now = orig }()
	at := findWeather(t, 10, 10, func(i float64) bool { return i > 0.9 })
	ui.Now = func() time.Time { return at }

	const n, scale = 16, 12
	tiles := make([][]Tile, n)
	for y := 0; y < n; y++ {
		tiles[y] = make([]Tile, n)
		for x := 0; x < n; x++ {
			tiles[y][x] = Tile{Kind: TileFloor, Walkable: true, Tex: TexGrass, Ground: "#5EAE63"}
		}
	}
	tm := &TileMap{W: n, H: n, Tiles: tiles}
	cam := Camera{W: n, H: n}

	dry := RenderRGBA(nil, tm, nil, "", 0, cam, Light{}, 0, 0, scale, false, nil)
	wet := RenderRGBA(nil, tm, nil, "", 0, cam, Light{Sky: true}, 0, 0, scale, false, nil)
	if bytes.Equal(dry.Pix, wet.Pix) {
		t.Fatal("Sky light added no weather pixels in a heavy storm")
	}
	wet2 := RenderRGBA(nil, tm, nil, "", 1, cam, Light{Sky: true}, 0, 0, scale, false, nil)
	if bytes.Equal(wet.Pix, wet2.Pix) {
		t.Fatal("rain did not animate between frames")
	}
}

// TestSkyReport: the /where flavor line tracks the intensity bands.
func TestSkyReport(t *testing.T) {
	for _, c := range []struct {
		i    float64
		want string
	}{
		{0, "clear"}, {0.2, "drizzle"}, {0.5, "rain"}, {0.9, "storm"},
	} {
		if got := SkyReport(c.i); !strings.Contains(got, c.want) {
			t.Fatalf("SkyReport(%v) = %q, want it to mention %q", c.i, got, c.want)
		}
	}
}

// TestWhereReportsSky: /where appends the weather in the Wilds and stays
// weatherless indoors.
func TestWhereReportsSky(t *testing.T) {
	orig := ui.Now
	defer func() { ui.Now = orig }()
	m := playingModel(t)
	m.ctx.World.EnterArea(m.ctx.Name, "wilds", 4, 4, "The Wilds")
	self, _ := m.ctx.World.Self(m.ctx.Name)
	at := findWeather(t, self.X, self.Y, func(i float64) bool { return i > 0.75 })
	ui.Now = func() time.Time { return at }
	m.runChatLine("/where")
	if got := lastChat(m); !strings.Contains(got, "storm") {
		t.Fatalf("/where in a Wilds storm = %q, want the sky mentioned", got)
	}
	m.ctx.World.EnterArea(m.ctx.Name, "lobby", 2, 2, "Durst HQ")
	m.runChatLine("/where")
	if got := lastChat(m); strings.Contains(got, "storm") || strings.Contains(got, "skies") {
		t.Fatalf("/where indoors = %q, want no weather report", got)
	}
}

// TestOvercastAmbient: a storm's wash only ever greys and deepens the ambient —
// zero overcast is a no-op and full overcast never lightens the night.
func TestOvercastAmbient(t *testing.T) {
	hex, str := "#FFC08A", 0.2
	if h, s := overcastAmbient(hex, str, 0); h != hex || s != str {
		t.Fatalf("zero overcast changed the ambient: %s %v", h, s)
	}
	h, s := overcastAmbient(hex, str, 1)
	if s <= str {
		t.Fatalf("full overcast did not deepen the wash: %v -> %v", str, s)
	}
	if !strings.HasPrefix(h, "#") || h == hex {
		t.Fatalf("full overcast did not grey the tint: %s -> %s", hex, h)
	}
	if _, s2 := overcastAmbient("#0A0E1A", 0.66, 1); s2 < 0.66 {
		t.Fatalf("overcast lightened the night: %v", s2)
	}
}

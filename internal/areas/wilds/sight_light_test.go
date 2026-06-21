package wilds

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// meanLum returns the average perceived luminance of a tile-sized region of the
// rendered frame, addressed by tile coordinate (tx,ty) at the given pixel scale.
func meanLum(img *image.RGBA, tx, ty, scale int) float64 {
	var sum float64
	var n int
	for py := ty * scale; py < (ty+1)*scale; py++ {
		for px := tx * scale; px < (tx+1)*scale; px++ {
			c := img.RGBAAt(px, py)
			sum += 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// The Wilds' discovery light must stay pooled on the player no matter where in
// the world they stand. Regression guard for the bug where the light center was
// passed in absolute world coordinates while the window renderer indexed tiles
// cam-relative (cam.X==0), so the bright circle drifted off the player and only
// landed on-screen near the world origin (spawn).
//
// We render each window twice — lit and flat (no light) — and compare the
// lit/flat luminance ratio under the player against a point well outside the
// sight radius. Dividing by the flat frame cancels out the terrain, leaving
// just the radial falloff: it must be bright at the player and dim away from
// them, at spawn AND far from it.
func TestSightLightStaysCenteredOnPlayer(t *testing.T) {
	const vw, vh, scale = 40, 24, 8

	// Pin the day/night clock to deep night: the sight light's falloff now fades
	// out by day (see game.DayFadedLight), so this falloff assertion only holds
	// while it's dark. Keep the test deterministic regardless of when it runs.
	ui.Now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	defer func() { ui.Now = time.Now }()

	w := world.New()
	defer w.Close()
	name, _ := w.Join("you")
	st := store.Open(filepath.Join(t.TempDir(), "x.db"))
	defer st.Close()
	ctx := &game.Ctx{World: w, Store: st, Name: name}

	a := &area{ctx: ctx, gen: worldgen.New(worldSeed), discovered: map[[2]int]uint64{}, dirty: map[[2]int]bool{}}
	self, _ := w.Self(name)
	a.Init(&self)
	startX, startY := a.wx, a.wy
	style := game.DefaultStyle()

	// off==0 is at spawn (small world coords); off==80 is far away, where the
	// old bug pushed the light entirely off-screen.
	for _, off := range []int{0, 80} {
		a.wx, a.wy = startX+off, startY
		a.reveal() // uncover a discoverR disc around the player so terrain is visible

		tm, ox, oy := a.sample(vw, vh)
		light := a.sightLight()
		lit := game.RenderRGBA(nil, tm, nil, "", 0, game.Camera{W: vw, H: vh}, light, ox, oy, scale, false, style)
		flat := game.RenderRGBA(nil, tm, nil, "", 0, game.Camera{W: vw, H: vh}, game.Light{}, ox, oy, scale, false, style)

		// The lit circle's center, in window-tile coordinates (where the player is).
		cx, cy := light.X-ox, light.Y-oy
		// A point just inside the revealed disc (discoverR=9) but beyond the lit
		// radius (sightR=7), so it should be clearly darkened.
		ex, ey := cx, cy+8

		centerRatio := meanLum(lit, cx, cy, scale) / max1(meanLum(flat, cx, cy, scale))
		edgeRatio := meanLum(lit, ex, ey, scale) / max1(meanLum(flat, ex, ey, scale))

		if centerRatio < 0.85 {
			t.Errorf("off=%d: player tile is darkened (lit/flat=%.2f); light is not on the player", off, centerRatio)
		}
		if edgeRatio > 0.8 {
			t.Errorf("off=%d: tile beyond sight radius is not darkened (lit/flat=%.2f); no falloff", off, edgeRatio)
		}
		if centerRatio-edgeRatio < 0.2 {
			t.Errorf("off=%d: light not centered on player (center %.2f vs edge %.2f)", off, centerRatio, edgeRatio)
		}
	}
}

func max1(v float64) float64 {
	if v < 1 {
		return 1
	}
	return v
}

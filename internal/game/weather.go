package game

// Weather: drifting precipitation fronts over the open sky (docs/ROADMAP.md's
// parked "particles / weather layer").
//
// The field is a pure function of (wall-clock time, world cell) — like the
// terrain itself, nothing is stored and every session computes the same answer,
// so two players standing together always share the same shower. Fronts are
// low-frequency value noise drifting with a fixed wind, gated by a slow
// coverage rhythm so the world cycles through clear spells and long rains. Over
// the frozen biomes the same front falls as snow.
//
// Rendering is deliberately cheap in both clients. Only a sparse subset of
// covered cells "hosts" a falling drop or flake at a time (like the firefly
// motes), and everything a host draws stays strictly inside its own tile —
// that's what lets the incremental HD renderer treat a raining cell as a
// zero-dilation dirty point instead of full-repainting the frame (see
// precipOnlyTile). The glyph client overlays a streak rune on the same host
// cells, so both renderers agree on where it rains.

import (
	"image"
	"math"
	"time"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
)

// weatherBucket quantizes the clock the storm field reads. Within a frame the
// renderer samples the field several times (signature pass, draw pass); the
// bucket makes those reads agree, and it is what the incremental contract test
// pins. Fronts still drift visibly — a bucket step is well under a tile of wind.
const weatherBucket = 2 * time.Second

// Wind: how fast fronts travel, in tiles per second, and the direction rain
// streaks lean. Brisk enough that a front noticeably crosses the hub within a
// few minutes, slow enough that a shower doesn't strobe.
const (
	windX = 0.22
	windY = 0.08
)

// stormScale is a front's wavelength in tiles — systems span whole biomes, so
// walking out from under a shower is a real journey, not three steps.
const stormScale = 110.0

// precipDensity is the fraction of cells under a full-strength front that host
// a falling drop/flake at once. Kept sparse for the same reason the fireflies
// are: each host is a per-frame dirty tile in the incremental HD renderer.
const precipDensity = 0.12

// hostShuffle reshuffles which covered cells host a drop every few seconds, so
// a steady rain doesn't loop the same streaks in the same places forever.
const hostShuffle = 10 // seconds

// StormAt returns the precipitation intensity 0..1 at world cell (wx,wy) at t:
// 0 is dry, 1 the heart of a front. Deterministic — all sessions agree.
func StormAt(t time.Time, wx, wy int) float64 {
	sec := float64(t.Truncate(weatherBucket).Unix())
	// Fronts drift with the wind: sample the noise field upwind of the cell.
	u := float64(wx) - sec*windX
	v := float64(wy) - sec*windY
	n := 0.65*smoothNoise(u/stormScale, v/stormScale, 0xA1) +
		0.35*smoothNoise(u*3/stormScale, v*3/stormScale, 0xC3)
	// A slow global rhythm sets how much of the world is under weather at all.
	// The period is incommensurate with the one-hour day cycle, so storms don't
	// learn to always arrive at dusk.
	cover := 0.5 + 0.5*math.Sin(2*math.Pi*sec/8820) // ≈2.45 h swing
	thr := 0.68 - 0.20*cover
	i := (n - thr) / 0.16
	if i < 0 {
		return 0
	}
	if i > 1 {
		return 1
	}
	return i
}

// smoothNoise is one octave of smoothstep-interpolated value noise sampled at
// float coordinates — latticeNoise's continuous twin, needed because the storm
// field drifts by fractional tiles per bucket.
func smoothNoise(fx, fy float64, seed int) float64 {
	x0, y0 := math.Floor(fx), math.Floor(fy)
	tx, ty := fx-x0, fy-y0
	tx = tx * tx * (3 - 2*tx)
	ty = ty * ty * (3 - 2*ty)
	xi, yi := int(x0), int(y0)
	return bilerp(
		hashNoise(xi, yi, seed),
		hashNoise(xi+1, yi, seed),
		hashNoise(xi, yi+1, seed),
		hashNoise(xi+1, yi+1, seed),
		tx, ty)
}

// precipCell reports whether a surface can show falling weather at all: any
// real outdoor ground. TexFlat is excluded on purpose — it is both the indoor
// default and the Wilds' undiscovered fog, and rain sheeting over fog would
// trace the storm across ground the player hasn't earned yet.
func precipCell(tex TileTex) bool {
	switch tex {
	case TexFlat, TexFloor, TexBrick, TexMetal:
		return false
	}
	return true
}

// precipHost reports whether cell (wx,wy) carries a falling drop/flake right
// now, and the storm intensity there. Sparse (≤ precipDensity of covered
// cells), deterministic per bucket, reshuffled every hostShuffle seconds. It is
// the single source of truth for "which cells rain this frame", shared by both
// draw passes and the incremental renderer's tileAnimated — exactly the
// fireflyHost pattern.
func precipHost(t time.Time, wx, wy int) (float64, bool) {
	i := StormAt(t, wx, wy)
	if i <= 0 {
		return 0, false
	}
	epoch := int(t.Truncate(weatherBucket).Unix() / hostShuffle)
	if hashNoise(wx, wy, epoch, 0x5EED) >= i*precipDensity {
		return 0, false
	}
	return i, true
}

// Overcast colors: under a front the ambient pulls toward a flat grey-slate
// wash, so a rainy noon reads dimmer and cooler than a clear one (the effect
// colors live beside the other atmosphere constants, like lightfx's mist).
const overcastHex = "#78838F"

// overcastAmbient folds a storm's grey wash into the scene ambient: the tint
// blends toward slate and the wash deepens (never lightens — a stormy night
// stays a night), scaled by how hard it is raining where the player stands.
func overcastAmbient(hex string, strength, overcast float64) (string, float64) {
	if overcast <= 0 {
		return hex, strength
	}
	if overcast > 1 {
		overcast = 1
	}
	hex = string(ui.Blend(hex, overcastHex, 0.45*overcast))
	strength += 0.25 * overcast * (1 - strength)
	return hex, strength
}

// Precipitation colors: a pale cool streak for rain, near-white for snow.
// Blended over whatever is already on the ground (opaque mix, not additive), so
// drops read on a bright beach at noon and over a dark wood at night alike.
var (
	rainColor = colorful.Color{R: 0.62, G: 0.72, B: 0.90}
	snowColor = colorful.Color{R: 0.93, G: 0.96, B: 1.00}
)

// snowTex reports whether precipitation falls frozen on this surface — the
// snowfields keep their weather as snow; everywhere else the front is rain.
func snowTex(tex TileTex) bool { return tex == TexSnow }

// drawWeather lays the falling precipitation over an HD frame: on each host
// cell a slanted rain streak (with a splash blink as it lands) or a pair of
// drifting snowflakes. Everything stays strictly inside the cell's own
// scale×scale rect — the zero-dilation guarantee the incremental renderer
// relies on (precipOnlyTile) — and every decision derives from (world cell,
// frame, bucketed clock), so a re-rendered subregion reproduces it exactly.
func drawWeather(img *image.RGBA, texs [][]TileTex, cam Camera, scale, frame, originX, originY int, light Light) {
	if !light.Sky {
		return
	}
	now := ui.Now()
	apx := scale / tileArtN
	if apx < 1 {
		apx = 1
	}
	for vy := 0; vy < cam.H; vy++ {
		for vx := 0; vx < cam.W; vx++ {
			tex := texs[vy][vx]
			if !precipCell(tex) {
				continue
			}
			wx, wy := originX+vx, originY+vy
			i, ok := precipHost(now, wx, wy)
			if !ok {
				continue
			}
			ph := hashNoise(wx, wy, 0x7A11)
			x0, y0 := vx*scale, vy*scale
			if snowTex(tex) {
				drawSnowCell(img, x0, y0, scale, apx, frame, ph, i)
			} else {
				drawRainCell(img, x0, y0, scale, apx, frame, ph, i)
			}
		}
	}
}

// drawRainCell draws one cell's falling streak: a three-block slanted dash
// sweeping down the tile, then a brief splash pair on the ground. All offsets
// are clamped inside the cell.
func drawRainCell(img *image.RGBA, x0, y0, scale, apx, frame int, ph, i float64) {
	w := 0.55 + 0.35*i // how hard the drop reads
	// The fall cycle runs a little past 1: the tail is the splash window.
	cyc := math.Mod(float64(frame)*0.22+ph*7, 1.3)
	// Column drifts per cell (from the hash), leaning with the wind.
	col := int(math.Mod(ph*13, 1) * float64(scale-2*apx))
	if cyc <= 1 {
		head := int(cyc * float64(scale-apx))
		// Three stacked blocks stepping a pixel against the wind as they rise —
		// a slanted streak fading toward its tail, not a vertical bar.
		blendRect(img, x0+col, y0+head, apx, apx, rainColor, w)
		for k := 1; k <= 2; k++ {
			if head < k*apx {
				break
			}
			up := col + (k+1)/2*apx // 0, +1, +1 art-px of lean
			if up > scale-apx {
				up = scale - apx
			}
			blendRect(img, x0+up, y0+head-k*apx, apx, apx, rainColor, w*(1-0.3*float64(k)))
		}
		return
	}
	// Splash: two fading blips widening at the foot of the fall column.
	k := (cyc - 1) / 0.3
	spread := apx + int(k*float64(apx))
	yb := y0 + scale - apx
	fade := w * (1 - k) * 0.8
	if l := col - spread; l >= 0 {
		blendRect(img, x0+l, yb, apx, apx, rainColor, fade)
	}
	if r := col + spread; r <= scale-apx {
		blendRect(img, x0+r, yb, apx, apx, rainColor, fade)
	}
}

// drawSnowCell draws one cell's snowfall: two flakes half a cycle apart,
// sinking slowly and swaying sideways, clamped inside the cell.
func drawSnowCell(img *image.RGBA, x0, y0, scale, apx, frame int, ph, i float64) {
	w := 0.55 + 0.35*i
	for f := 0; f < 2; f++ {
		cyc := math.Mod(float64(frame)*0.05+ph*7+float64(f)*0.5, 1)
		fy := int(cyc * float64(scale-apx))
		sway := 0.5 + 0.42*math.Sin(float64(frame)*0.09+ph*17+float64(f)*2.4)
		fx := int(sway * float64(scale-apx))
		blendRect(img, x0+fx, y0+fy, apx, apx, snowColor, w)
	}
}

// blendRect mixes a w×h block toward col by weight (0..1) — an opaque-ish wash
// that stays legible over any ground brightness, unlike a pure additive glow.
func blendRect(img *image.RGBA, x0, y0, w, h int, col colorful.Color, weight float64) {
	if weight <= 0 {
		return
	}
	if weight > 1 {
		weight = 1
	}
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			or, og, ob, ok := getPixel(img, x, y)
			if !ok {
				continue
			}
			setPixel8(img, x, y,
				float64(or)+(col.R*255-float64(or))*weight,
				float64(og)+(col.G*255-float64(og))*weight,
				float64(ob)+(col.B*255-float64(ob))*weight)
		}
	}
}

// precipGlyph resolves a host cell's overlay for the glyph client: a streak
// rune alternating with the frame so the rain visibly falls, colored toward the
// precipitation over the tile's own lit color so it dims with night like
// everything else. Mirrors drawWeather's host set exactly.
func precipGlyph(tex TileTex, frame, wx, wy int, i float64, under colorful.Color) (rune, colorful.Color) {
	ph := int(hashNoise(wx, wy, 0x7A11) * 4)
	c := snowColor
	var frames []rune
	if snowTex(tex) {
		frames = []rune{'*', '·', '·'} // a flake sinking into the drift
	} else {
		c = rainColor
		frames = []rune{'\'', ',', '.'} // a drop falling through the cell
	}
	ch := frames[((frame/2)+ph)%len(frames)]
	return ch, under.BlendLab(c, 0.55+0.30*i).Clamped()
}

package game

import (
	"image"
	"math"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
)

// sunState derives lighting from the time-of-day clock (ui.Now):
//   elev  — sun height: 1 at noon, 0 at the horizon (06:00 / 18:00), <0 at night.
//   azX   — horizontal sun direction: −1 at dawn (east) … +1 at dusk (west).
//   night — 0 in full day, ramping to 1 after dusk; gates night-only effects.
func sunState() (elev, azX, night float64) {
	t := ui.Now()
	h := float64(t.Hour()) + float64(t.Minute())/60
	elev = math.Sin(math.Pi * (h - 6) / 12)
	azX = math.Max(-1, math.Min(1, (h-12)/6))
	night = math.Max(0, math.Min(1, 1-elev*1.3))
	return
}

// drawGlow adds a warm/cool pool of light: an additive radial falloff snapped to
// a px block grid and banded into a few steps, so it reads as retro light rather
// than a smooth gradient. col is 0..1 linear-ish; intensity scales it.
func drawGlow(img *image.RGBA, cx, cy int, radius float64, col colorful.Color, intensity float64, px int) {
	if radius < 1 || px < 1 || intensity <= 0 {
		return
	}
	r := int(radius)
	x0 := int(math.Floor(float64(cx-r)/float64(px))) * px
	y0 := int(math.Floor(float64(cy-r)/float64(px))) * px
	for by := y0; by <= cy+r; by += px {
		for bx := x0; bx <= cx+r; bx += px {
			d := math.Hypot(float64(bx+px/2-cx), float64(by+px/2-cy)) / radius
			if d >= 1 {
				continue
			}
			w := math.Ceil((1-d)*3) / 3 * intensity * 0.6 // 3 bands, softened so centres don't clip
			ar, ag, ab := col.R*255*w, col.G*255*w, col.B*255*w
			for yy := by; yy < by+px; yy++ {
				for xx := bx; xx < bx+px; xx++ {
					or, og, ob, ok := getPixel(img, xx, yy)
					if !ok {
						continue
					}
					setPixel8(img, xx, yy,
						math.Min(255, float64(or)+ar),
						math.Min(255, float64(og)+ag),
						math.Min(255, float64(ob)+ab))
				}
			}
		}
	}
}

// emitterGlow returns a prop's glow color, radius (in tiles), an intensity
// multiplier (a campfire floods, a gem only twinkles) and whether it emits at
// all. propCol is the prop's day/night-tinted color; frame drives flame flicker.
func emitterGlow(p TileProp, propCol colorful.Color, frame, wx, wy int) (col colorful.Color, radius, intensity float64, ok bool) {
	flicker := 0.85 + 0.15*math.Sin(float64(frame)*0.6+float64(wx*7+wy*3))
	switch p {
	case PropCampfire:
		return colorful.Color{R: 1, G: 0.55, B: 0.2}, 3.0 * flicker, 1.0, true
	case PropPortal:
		return propCol.BlendLab(colorful.Color{R: 0.8, G: 0.95, B: 1}, 0.4), 2.8, 0.6, true
	case PropCore, PropFountain:
		return colorful.Color{R: 0.75, G: 0.92, B: 1}, 3.4 * flicker, 0.7, true
	case PropLamp:
		return colorful.Color{R: 1, G: 0.84, B: 0.5}, 2.4 * flicker, 0.7, true
	case PropTurbine, PropScreen:
		return colorful.Color{R: 0.5, G: 0.8, B: 1}, 2.0, 0.6, true
	case PropGem:
		return propCol.BlendLab(colorful.Color{R: 1, G: 1, B: 1}, 0.25), 0.9, 0.35, true
	}
	return colorful.Color{}, 0, 0, false
}

// waterGlint lays a drifting specular sparkle over water tiles: warm gold when
// the sun is high, cool moon-glitter at night, the crest band sweeping along the
// sun's azimuth. Additive and banded, so it shimmers without going smooth.
func waterGlint(img *image.RGBA, texs [][]TileTex, cam Camera, scale, frame, originX, originY int) {
	elev, azX, night := sunState()
	gl := colorful.Color{R: 1, G: 0.93, B: 0.6}.BlendLab(colorful.Color{R: 0.72, G: 0.84, B: 1}, night).Clamped()
	bright := math.Max(0.3, math.Min(1, elev+0.3)) // night still glitters faintly
	apx := scale / tileArtN
	if apx < 1 {
		apx = 1
	}
	for vy := 0; vy < cam.H; vy++ {
		for vx := 0; vx < cam.W; vx++ {
			if texs[vy][vx] != TexWater {
				continue
			}
			wx, wy := originX+vx, originY+vy
			for ay := 0; ay < tileArtN; ay++ {
				for ax := 0; ax < tileArtN; ax++ {
					gx, gy := wx*tileArtN+ax, wy*tileArtN+ay
					phase := float64(gx)*(0.55+0.25*azX) + float64(gy)*0.4 - float64(frame)*0.5
					s := math.Sin(phase)
					if s < 0.86 || valueNoise(gx, gy+frame/3) < 0.5 {
						continue // sparse, broken-up crests
					}
					w := (s - 0.86) / 0.14 * bright * 0.9
					px0, py0 := vx*scale+ax*apx, vy*scale+ay*apx
					for yy := py0; yy < py0+apx; yy++ {
						for xx := px0; xx < px0+apx; xx++ {
							or, og, ob, ok := getPixel(img, xx, yy)
							if !ok {
								continue
							}
							setPixel8(img, xx, yy,
								math.Min(255, float64(or)+gl.R*255*w),
								math.Min(255, float64(og)+gl.G*255*w),
								math.Min(255, float64(ob)+gl.B*255*w))
						}
					}
				}
			}
		}
	}
}

// drawFireflies scatters drifting, blinking glow motes over forest and swamp
// tiles after dusk — warm fireflies in the woods, cool bioluminescence in the
// swamp. A sparse, deterministic set per tile, floating and pulsing on the frame.
func drawFireflies(img *image.RGBA, texs [][]TileTex, cam Camera, scale, frame, originX, originY int) {
	_, _, night := sunState()
	if night < 0.3 {
		return
	}
	apx := scale / tileArtN
	if apx < 1 {
		apx = 1
	}
	warm := colorful.Color{R: 1, G: 0.92, B: 0.45}
	cool := colorful.Color{R: 0.45, G: 1, B: 0.7}
	for vy := 0; vy < cam.H; vy++ {
		for vx := 0; vx < cam.W; vx++ {
			tex := texs[vy][vx]
			if tex != TexForest && tex != TexSwamp {
				continue
			}
			wx, wy := originX+vx, originY+vy
			if hashNoise(wx, wy, 0x1357) > 0.05 { // ~5% of tiles host a mote
				continue
			}
			ph := float64(wx*13 + wy*7)
			if blink := 0.5 + 0.5*math.Sin(float64(frame)*0.3+ph); blink < 0.35 {
				continue // motes wink in and out
			}
			dx := 0.5 + 0.4*math.Sin(float64(frame)*0.14+ph)
			dy := 0.5 + 0.4*math.Cos(float64(frame)*0.11+ph*1.3)
			cx := vx*scale + int(dx*float64(scale))
			cy := vy*scale + int(dy*float64(scale)) - scale/3 // float above the ground
			col := warm
			if tex == TexSwamp {
				col = cool
			}
			drawGlow(img, cx, cy, float64(apx)*2, col, night*0.8, apx)
			fillRect(img, cx, cy, max(1, apx/2), max(1, apx/2), colorfulToRGBA(col))
		}
	}
}

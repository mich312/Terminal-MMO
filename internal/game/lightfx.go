package game

import (
	"image"
	"math"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
)

// sunState derives lighting from the time-of-day clock (ui.Now):
//
//	elev  — sun height: 1 at noon, 0 at the horizon (06:00 / 18:00), <0 at night.
//	azX   — horizontal sun direction: −1 at dawn (east) … +1 at dusk (west).
//	night — 0 in full day, ramping to 1 after dusk; gates night-only effects.
func sunState() (elev, azX, night float64) {
	t := ui.Now()
	h := float64(t.Hour()) + float64(t.Minute())/60
	elev = math.Sin(math.Pi * (h - 6) / 12)
	azX = math.Max(-1, math.Min(1, (h-12)/6))
	night = math.Max(0, math.Min(1, 1-elev*1.3))
	return
}

// Glow lights a pool by multiplying the underlying night-dark pixels back up
// (revealing the terrain's own colors near the source — that reads as light,
// not a white halo) plus a small colored add for the light's hue.
const (
	glowGain = 2.2  // how hard the light brightens the dark ground it reveals
	glowHue  = 0.30 // how much of the light's own color it adds on top
)

// drawGlow paints one light pool: a radial falloff snapped to a px block grid
// and banded into steps (retro), brightening the ground it covers. col is 0..1;
// intensity scales the whole effect.
func drawGlow(img *image.RGBA, cx, cy int, radius float64, col colorful.Color, intensity float64, px int) {
	if radius < 1 || px < 1 || intensity <= 0 {
		return
	}
	// Per-channel brighten biased by the light's color, normalised so the
	// brightest channel reveals fully — a warm campfire warms the ground it
	// lights, a cool portal cools it, rather than just exposing the raw terrain.
	mx := math.Max(col.R, math.Max(col.G, col.B))
	if mx < 1e-3 {
		mx = 1
	}
	cr, cg, cb := col.R/mx, col.G/mx, col.B/mx
	r := int(radius)
	x0 := int(math.Floor(float64(cx-r)/float64(px))) * px
	y0 := int(math.Floor(float64(cy-r)/float64(px))) * px
	for by := y0; by <= cy+r; by += px {
		for bx := x0; bx <= cx+r; bx += px {
			d := math.Hypot(float64(bx+px/2-cx), float64(by+px/2-cy)) / radius
			if d >= 1 {
				continue
			}
			w := math.Ceil((1-d)*3) / 3 * intensity * 0.6 // 3 bands
			g := w * glowGain
			for yy := by; yy < by+px; yy++ {
				for xx := bx; xx < bx+px; xx++ {
					or, og, ob, ok := getPixel(img, xx, yy)
					if !ok {
						continue
					}
					setPixel8(img, xx, yy,
						math.Min(255, float64(or)*(1+g*cr)+col.R*255*w*glowHue),
						math.Min(255, float64(og)*(1+g*cg)+col.G*255*w*glowHue),
						math.Min(255, float64(ob)*(1+g*cb)+col.B*255*w*glowHue))
				}
			}
		}
	}
}

// emitterGlow returns a prop's glow color, radius (in tiles), an intensity
// multiplier (a campfire floods, loot only twinkles) and whether it emits at
// all. propCol is the prop's day/night-tinted color. Flames and fixtures
// flicker on the frame (two summed waves, so it's not a clean pulse); portals
// and luminous loot are steady. Mundane forage and hats don't emit at all.
func emitterGlow(p TileProp, propCol colorful.Color, frame, wx, wy int) (col colorful.Color, radius, intensity float64, ok bool) {
	h := float64(wx*7 + wy*3)
	flame := 0.78 + 0.16*math.Sin(float64(frame)*0.7+h) + 0.06*math.Sin(float64(frame)*1.9+h*1.7)
	gentle := 0.9 + 0.1*math.Sin(float64(frame)*0.5+h)
	white := colorful.Color{R: 1, G: 1, B: 1}
	whiten := func(c colorful.Color, k float64) colorful.Color { return c.BlendLab(white, k).Clamped() }
	switch p {
	case PropCampfire:
		// Saturated warm light so the fire genuinely warms the ground it reveals,
		// flickering in both reach and brightness.
		return whiten(colorful.Color{R: 1, G: 0.5, B: 0.16}, 0.1), 3.0 * flame, flame, true
	case PropBrazier:
		// A street brazier: a warm, flickering pool of firelight at the gates and
		// squares — a touch smaller and steadier than a campfire.
		return whiten(colorful.Color{R: 1, G: 0.55, B: 0.2}, 0.12), 2.6 * flame, 0.9 * flame, true
	case PropBldSmithy:
		// A blacksmith's forge: a strong, warm glow spilling from the forge mouth,
		// flickering as the fire is worked — the brightest window on a night street.
		return whiten(colorful.Color{R: 1, G: 0.45, B: 0.13}, 0.1), 2.4 * flame, 0.85 * flame, true
	case PropBldTavern:
		// A tavern: cosy lamplight from its windows, steady and inviting.
		return whiten(colorful.Color{R: 1, G: 0.78, B: 0.42}, 0.3), 2.0 * gentle, 0.6 * gentle, true
	case PropPortal:
		return whiten(propCol, 0.6), 2.8, 0.6, true
	case PropCore, PropFountain:
		return whiten(colorful.Color{R: 0.75, G: 0.92, B: 1}, 0.55), 3.4 * gentle, 0.7 * gentle, true
	case PropLamp:
		return whiten(colorful.Color{R: 1, G: 0.82, B: 0.45}, 0.4), 2.4 * gentle, 0.7 * gentle, true
	case PropTurbine, PropScreen:
		return whiten(colorful.Color{R: 0.5, G: 0.8, B: 1}, 0.6), 2.0, 0.6, true
	case PropGemGlow:
		// Only luminous loot (crystals, mushrooms) twinkles; mundane forage and
		// hats stay dark.
		return whiten(propCol, 0.5), 1.1, 0.4, true
	case PropCaveShroom:
		// Bioluminescent fungi: a soft, steady blue-green wash over the rock.
		return whiten(propCol, 0.4), 1.6 * gentle, 0.5 * gentle, true
	case PropGlowPool:
		// A still pool lit from within by glowing algae — a wider, cooler pool of
		// light than the mushrooms.
		return whiten(propCol, 0.45), 2.2 * gentle, 0.55 * gentle, true
	case PropRelic:
		// A buried relic with a faint, steady inner light — a beacon in the deep.
		return whiten(propCol, 0.5), 1.6 * gentle, 0.5 * gentle, true
	case PropLightShaft:
		// Daylight falling through thin rock: the brightest, widest pool in the
		// cave by day, fading to a cool wash of moonlight at night — it tracks the
		// surface sun, so a shaft is a clock as much as a landmark.
		_, _, night := sunState()
		day := 1 - night
		warm := colorful.Color{R: 1, G: 0.95, B: 0.82}
		cool := colorful.Color{R: 0.7, G: 0.82, B: 1}
		col := warm.BlendLab(cool, night)
		return whiten(col, 0.25), 2.4 + 1.4*day, 0.45 + 0.55*day, true
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

// drawCaveFauna animates the cave's living things — bats wheeling at the mouths,
// fish darting in the glow-pools, glow-worms drifting over the mushroom groves.
// Like the fireflies it's procedural (position from frame + cell hash, no state),
// keyed off the cave props so it only fires underground, and it runs day or night
// since the cave is always dark.
func drawCaveFauna(img *image.RGBA, props [][]TileProp, cam Camera, scale, frame, originX, originY int) {
	apx := scale / tileArtN
	if apx < 1 {
		apx = 1
	}
	dot := func(cx, cy, n int, col colorful.Color) {
		fillRect(img, cx, cy, max(1, n), max(1, n), colorfulToRGBA(col))
	}
	bat := colorful.Color{R: 0.30, G: 0.26, B: 0.34}
	fish := colorful.Color{R: 0.55, G: 0.92, B: 1}
	worm := colorful.Color{R: 0.5, G: 1, B: 0.74}
	for vy := 0; vy < cam.H; vy++ {
		for vx := 0; vx < cam.W; vx++ {
			wx, wy := originX+vx, originY+vy
			ph := float64(wx*13 + wy*7)
			cx0, cy0 := vx*scale+scale/2, vy*scale+scale/2
			switch props[vy][vx] {
			case PropCaveMouth:
				// A few bats wheel on erratic orbits over the mouth — dark, flitting
				// silhouettes that read as a draught of wings.
				for b := 0; b < 3; b++ {
					bp := ph + float64(b)*2.1
					a := float64(frame)*0.16 + bp
					rad := (0.8 + 0.5*math.Sin(float64(frame)*0.21+bp)) * float64(scale)
					bx := cx0 + int(math.Cos(a)*rad)
					by := cy0 + int(math.Sin(a)*rad*0.6) - scale/2
					dot(bx-apx, by, apx, bat) // two angled wings
					dot(bx+apx, by, apx, bat)
					dot(bx, by+max(1, apx/2), max(1, apx/2), bat)
				}
			case PropGlowPool:
				// A fish surfaces and darts across the pool, winking as it turns.
				if blink := math.Sin(float64(frame)*0.12 + ph); blink > 0.3 {
					t := math.Sin(float64(frame)*0.18 + ph)
					fx := cx0 + int(t*float64(scale)*0.3)
					fy := cy0 + int(math.Cos(float64(frame)*0.1+ph)*float64(scale)*0.18)
					drawGlow(img, fx, fy, float64(apx)*1.6, fish, 0.5, apx)
					dot(fx, fy, max(1, apx/2), fish)
				}
			case PropCaveShroom:
				// Glow-worms drift just above the fungi — slow cool motes.
				for g := 0; g < 2; g++ {
					gp := ph + float64(g)*3.3
					if math.Sin(float64(frame)*0.25+gp) < 0 {
						continue // wink in and out
					}
					gx := cx0 + int(math.Sin(float64(frame)*0.09+gp)*float64(scale)*0.4)
					gy := cy0 + int(math.Cos(float64(frame)*0.07+gp)*float64(scale)*0.3) - scale/3
					drawGlow(img, gx, gy, float64(apx)*1.4, worm, 0.45, apx)
					dot(gx, gy, max(1, apx/2), worm)
				}
			}
		}
	}
}

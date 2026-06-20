package game

import (
	"image"
	"math"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
)

// sunState derives lighting from the time-of-day clock (ui.Now). It reads the
// shared ui.SolarHour, so the sun rises and sets with Brixen's seasonal daylight
// (canonical hour 6 = sunrise, 18 = sunset, whatever real time those fall at):
//   elev  — sun height: 1 at noon, 0 at the horizon (sunrise / sunset), <0 at night.
//   azX   — horizontal sun direction: −1 at dawn (east) … +1 at dusk (west).
//   night — 0 in full day, ramping to 1 after dusk; gates night-only effects.
func sunState() (elev, azX, night float64) {
	h := ui.SolarHour(ui.Now())
	elev = math.Sin(math.Pi * (h - 6) / 12)
	azX = math.Max(-1, math.Min(1, (h-12)/6))
	// Raw ramp: 0 while the sun is comfortably up, reaching 1 a little before the
	// horizon. Smoothstep shapes it so midday stays cleanly lit, the fade rolls in
	// gently through dusk, and the effects only bloom to full strength in true
	// night — a softer toe and shoulder than the old linear ramp.
	n := math.Max(0, math.Min(1, 1-elev*1.5))
	night = n * n * (3 - 2*n)
	return
}

// Glow lights a pool by multiplying the underlying night-dark pixels back up
// (revealing the terrain's own colors near the source — that reads as light,
// not a white halo) plus a small colored add for the light's hue.
const (
	glowGain = 2.5  // how hard the light brightens the dark ground it reveals
	glowHue  = 0.32 // how much of the light's own color it adds on top
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
		return whiten(colorful.Color{R: 1, G: 0.5, B: 0.16}, 0.1), 3.4 * flame, flame, true
	case PropPortal:
		return whiten(propCol, 0.6), 2.9, 0.65, true
	case PropCore, PropFountain:
		return whiten(colorful.Color{R: 0.75, G: 0.92, B: 1}, 0.55), 3.6 * gentle, 0.75 * gentle, true
	case PropLamp:
		return whiten(colorful.Color{R: 1, G: 0.82, B: 0.45}, 0.4), 2.7 * gentle, 0.75 * gentle, true
	case PropHouse:
		// Windows lit from within: a small, steady warm spill at the doorstep, so a
		// cottage reads as occupied after dark rather than a dark box.
		return whiten(colorful.Color{R: 1, G: 0.74, B: 0.38}, 0.3), 2.0, 0.55 * gentle, true
	case PropTurbine, PropScreen:
		return whiten(colorful.Color{R: 0.5, G: 0.8, B: 1}, 0.6), 2.0, 0.6, true
	case PropGemGlow:
		// Only luminous loot (crystals, mushrooms) twinkles; mundane forage and
		// hats stay dark.
		return whiten(propCol, 0.5), 1.1, 0.4, true
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

// mistTex reports whether a biome breathes low ground mist at night. It is kept
// to the wet lowland — open water and swamp — both of which the incremental
// renderer already treats as animated (water every frame, swamp after dusk), so
// the mist pass rides along for free without forcing the dominant dry biomes to
// re-render. (Broad meadow mist would mean marking all grass animated — a full
// repaint every night frame — so it is deliberately left out for now.)
func mistTex(t TileTex) bool { return t == TexWater || t == TexSwamp }

// drawMist lays drifting low fog over the wet lowland after dusk: sparse, soft,
// horizontally-stretched pale banks anchored to a deterministic set of water and
// swamp tiles and edging sideways on the frame, so the ground breathes a thin
// veil at night without washing the whole scene out. Like the fireflies pass it
// is world-coordinate deterministic and stays within the renderer's overhang, so
// the incremental renderer reproduces it exactly.
func drawMist(img *image.RGBA, texs [][]TileTex, cam Camera, scale, frame, originX, originY int) {
	_, _, night := sunState()
	if night < 0.3 {
		return
	}
	apx := scale / tileArtN
	if apx < 1 {
		apx = 1
	}
	col := colorful.Color{R: 0.76, G: 0.82, B: 0.90} // pale, faintly cool grey
	for vy := 0; vy < cam.H; vy++ {
		for vx := 0; vx < cam.W; vx++ {
			if !mistTex(texs[vy][vx]) {
				continue
			}
			wx, wy := originX+vx, originY+vy
			if hashNoise(wx, wy, 0x2BCD) > 0.6 { // most wet tiles carry a wisp
				continue
			}
			ph := float64(wx*5 + wy*11)
			// Slow sideways drift (well under a tile) and a gentle breathing fade, so
			// the bank rolls and thins rather than sitting as a static blob.
			drift := 0.45 * math.Sin(float64(frame)*0.05+ph) * float64(scale)
			breathe := 0.55 + 0.45*math.Sin(float64(frame)*0.07+ph*0.7)
			cxf := float64(vx*scale+scale/2) + drift
			cyf := float64(vy*scale + scale/2 + scale/5) // hugs the lower half of the tile
			rx := float64(scale) * 1.5                   // wide, flat bank (reach stays within the overhang)
			ry := float64(scale) * 0.6
			bx0 := int(math.Floor((cxf-rx)/float64(apx))) * apx
			by0 := int(math.Floor((cyf-ry)/float64(apx))) * apx
			for by := by0; float64(by) <= cyf+ry; by += apx {
				for bx := bx0; float64(bx) <= cxf+rx; bx += apx {
					nx := (float64(bx) + float64(apx)/2 - cxf) / rx
					ny := (float64(by) + float64(apx)/2 - cyf) / ry
					d := nx*nx + ny*ny
					if d >= 1 {
						continue
					}
					w := math.Ceil((1-d)*3) / 3 * night * breathe * 0.16 // 3 bands, faint veil
					for yy := by; yy < by+apx; yy++ {
						for xx := bx; xx < bx+apx; xx++ {
							or, og, ob, ok := getPixel(img, xx, yy)
							if !ok {
								continue
							}
							setPixel8(img, xx, yy,
								float64(or)+col.R*255*w,
								float64(og)+col.G*255*w,
								float64(ob)+col.B*255*w)
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

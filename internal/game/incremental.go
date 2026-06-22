package game

import (
	"image"
	"math"

	"github.com/durst-group/durstworld/internal/world"
)

// IncrementalRenderer rasterizes the HD scene by reusing the previous frame and
// only re-drawing tiles whose appearance actually changed — animated tiles
// (water, portals, emissive props, fireflies), tiles a light band swept across,
// and tiles an avatar moved over. The rest is kept from the last frame (shifted
// when the camera pans). The contract is strict: the buffer it returns is
// byte-identical to a fresh RenderRGBA of the same scene (validated by
// incremental_test.go), so the downstream FrameWriter delta — and therefore the
// pixels the player sees — are unchanged. It is purely a server-CPU optimization
// that replaces a full ~width·height-pixel rasterize with a small dirty subset.
//
// Not safe for concurrent use; each HD session owns one.
type IncrementalRenderer struct {
	buf, scratch *image.RGBA // persistent terrain buffer + ping-pong for pan shifts
	sig          []uint64    // per-tile appearance signature, row-major vw*vh
	prevFeet     []tileXY    // last frame's avatar footprint tiles (world coords)
	vw, vh       int
	ox, oy       int // world coord of buf tile (0,0)
	scale        int
	style        *Style
	lightRad     int
	have         bool
}

type tileXY struct{ x, y int }

// Reset forces the next Render to rebuild from scratch — call it when the scene
// changes wholesale (entering a new area), so no stale pixels carry over.
func (r *IncrementalRenderer) Reset() { r.have = false }

// overhangTiles bounds how far outside its own tile any draw pass reaches: the
// widest emitter glow (~3.4 tiles), tall canopies/avatars (~3 tiles up) and their
// stretched shadows. A dirty tile is grown by this much so a changed caster's
// overhang is redrawn, and each re-rendered region is rendered with this margin
// so an unchanged neighbour drooping into it is composited correctly. Validated
// against a full render by TestIncrementalMatchesFull — raise it if that fails.
const overhangTiles = 4

// ambientOverhang bounds how far a drifting firefly / mist mote reaches from its
// own cell (drift + glow ≈ 2 tiles), used to dilate those small-reach ambient
// tiles instead of the much wider overhangTiles. Keeping it tight is what lets a
// still, fully-explored night wood stay incremental rather than full-repainting
// every frame. Validated against a full render by TestIncrementalMatchesFull at
// night — raise it if that fails.
const ambientOverhang = 2

// fullRedrawFrac forces a plain full rasterize once the dirty set covers more of
// the frame than this — past it the bookkeeping costs more than it saves.
const fullRedrawFrac = 0.6

// smallReachTile reports whether a dirty tile's only frame-to-frame change is a
// drifting firefly / mist mote — ambient effects that stay within ambientOverhang
// tiles of the cell. Such tiles can be dilated with a tighter halo than the wide
// casters (campfires, portals, avatars). Only natural forest/swamp ground (bare or
// under a static canopy) qualifies; anything carrying a portal or a glowing emitter
// can spill much further and stays in the wide bucket.
func smallReachTile(t Tile) bool {
	if t.Tex != TexForest && t.Tex != TexSwamp {
		return false
	}
	switch t.Prop {
	case PropNone, PropTree, PropAcacia, PropPalm, PropFir, PropCrag:
		return true
	}
	return false
}

// Render returns the terrain frame for a vw×vh tile window (tm), reusing the
// previous frame where it can. tm's tile (0,0) is world (ox,oy); cam is implicitly
// the whole window. forceFull (e.g. the periodic full repaint) rebuilds from
// scratch, which also re-syncs any slow day/night drift. The returned image is
// owned by the renderer and must not be mutated by the caller (copy it before
// drawing UI overlays).
func (r *IncrementalRenderer) Render(tm *TileMap, players []world.Player, self string, frame int, light Light, ox, oy, scale int, style *Style, forceFull bool) *image.RGBA {
	if style == nil {
		style = DefaultStyle()
	}
	vw, vh := tm.W, tm.H
	full := forceFull || !r.have || r.vw != vw || r.vh != vh || r.scale != scale ||
		r.style != style || r.lightRad != light.Radius

	feet := footprints(players, ox, oy, vw, vh)
	if full {
		r.buf = RenderRGBA(nil, tm, players, self, frame, Camera{W: vw, H: vh}, light, ox, oy, scale, false, style)
		r.scratch = image.NewRGBA(r.buf.Bounds())
		r.sig = signatureGrid(tm, frame, light, ox, oy, style)
		r.vw, r.vh, r.ox, r.oy, r.scale, r.style, r.lightRad = vw, vh, ox, oy, scale, style, light.Radius
		r.prevFeet, r.have = feet, true
		return r.buf
	}

	newSig := signatureGrid(tm, frame, light, ox, oy, style)

	// Pan: slide the kept pixels to their new screen position; freshly exposed
	// tiles fall outside the copied overlap and are marked dirty below.
	dx, dy := ox-r.ox, oy-r.oy
	if dx != 0 || dy != 0 {
		shiftRGBA(r.scratch, r.buf, -dx*scale, -dy*scale)
		r.buf, r.scratch = r.scratch, r.buf
	}

	dirty := make([]bool, vw*vh)
	mark := func(tx, ty int) {
		if tx >= 0 && tx < vw && ty >= 0 && ty < vh {
			dirty[ty*vw+tx] = true
		}
	}
	for ty := 0; ty < vh; ty++ {
		for tx := 0; tx < vw; tx++ {
			// Compare against the same world tile in the previous (pre-shift) grid.
			osx, osy := ox+tx-r.ox, oy+ty-r.oy
			if osx < 0 || osx >= r.vw || osy < 0 || osy >= r.vh {
				mark(tx, ty) // newly revealed by the pan
				continue
			}
			if newSig[ty*vw+tx] != r.sig[osy*r.vw+osx] {
				mark(tx, ty)
			}
		}
	}
	// Avatars aren't in the signature: redraw every current footprint (sprites bob
	// each frame) and every footprint vacated since last frame (to erase a mover).
	for _, f := range feet {
		mark(f.x-ox, f.y-oy)
	}
	for _, f := range r.prevFeet {
		mark(f.x-ox, f.y-oy)
	}

	// Grow the dirty set by the overhang, then — if it now covers most of the
	// frame — a plain full rasterize is cheaper than many overlapping regions.
	//
	// Split by spill distance first. Most night churn is drifting fireflies / mist
	// motes that stay within ambientOverhang tiles of their own cell — far less than
	// the campfire-glow / canopy / avatar overhang the rest needs. Dilating a sparse
	// scatter of motes by the full overhang re-covers a whole explored wood (each
	// mote's 9×9 halo overlaps its neighbours), tipping a still night forest into a
	// full repaint every frame; the tighter halo keeps it incremental. Wide casters
	// and avatar footprints stay in the big bucket.
	big, small := make([]bool, vw*vh), make([]bool, vw*vh)
	for ty := 0; ty < vh; ty++ {
		for tx := 0; tx < vw; tx++ {
			i := ty*vw + tx
			if !dirty[i] {
				continue
			}
			if smallReachTile(tm.At(tx, ty)) {
				small[i] = true
			} else {
				big[i] = true
			}
		}
	}
	forceBig := func(f tileXY) {
		if tx, ty := f.x-ox, f.y-oy; tx >= 0 && tx < vw && ty >= 0 && ty < vh {
			big[ty*vw+tx], small[ty*vw+tx] = true, false
		}
	}
	for _, f := range feet {
		forceBig(f)
	}
	for _, f := range r.prevFeet {
		forceBig(f)
	}
	dil := dilate(big, vw, vh, overhangTiles)
	for i, d := range dilate(small, vw, vh, ambientOverhang) {
		if d {
			dil[i] = true
		}
	}
	nDil := 0
	for _, d := range dil {
		if d {
			nDil++
		}
	}
	if nDil > int(float64(vw*vh)*fullRedrawFrac) {
		r.buf = RenderRGBA(nil, tm, players, self, frame, Camera{W: vw, H: vh}, light, ox, oy, scale, false, style)
		r.sig, r.ox, r.oy, r.prevFeet = newSig, ox, oy, feet
		return r.buf
	}

	for _, rc := range dirtyRects(dil, vw, vh) {
		r.renderRegion(tm, players, self, frame, light, ox, oy, scale, style, rc)
	}
	r.sig, r.ox, r.oy, r.prevFeet = newSig, ox, oy, feet
	return r.buf
}

// renderRegion re-rasterizes the tile rectangle rc (window coords) by rendering
// it with an overhang margin — so casters just outside still composite in — and
// copying just the rc core into the persistent buffer.
func (r *IncrementalRenderer) renderRegion(tm *TileMap, players []world.Player, self string, frame int, light Light, ox, oy, scale int, style *Style, rc image.Rectangle) {
	mx0 := maxi(0, rc.Min.X-overhangTiles)
	my0 := maxi(0, rc.Min.Y-overhangTiles)
	mx1 := mini(tm.W, rc.Max.X+overhangTiles)
	my1 := mini(tm.H, rc.Max.Y+overhangTiles)
	sub := RenderRGBA(nil, tm, players, self, frame,
		Camera{X: mx0, Y: my0, W: mx1 - mx0, H: my1 - my0}, light, ox+mx0, oy+my0, scale, false, style)

	// Copy the core (rc) out of sub into buf, row by row.
	rowBytes := (rc.Max.X - rc.Min.X) * scale * 4
	for ty := rc.Min.Y * scale; ty < rc.Max.Y*scale; ty++ {
		so := sub.PixOffset((rc.Min.X-mx0)*scale, ty-my0*scale)
		do := r.buf.PixOffset(rc.Min.X*scale, ty)
		copy(r.buf.Pix[do:do+rowBytes], sub.Pix[so:so+rowBytes])
	}
}

// footprints returns the visible tiles every player's body covers (world coords).
func footprints(players []world.Player, ox, oy, vw, vh int) []tileXY {
	var out []tileXY
	for _, p := range players {
		for dy := 0; dy < PlayerH; dy++ {
			for dx := 0; dx < PlayerW; dx++ {
				x, y := p.X+dx, p.Y+dy
				if x >= ox && x < ox+vw && y >= oy && y < oy+vh {
					out = append(out, tileXY{x, y})
				}
			}
		}
	}
	return out
}

// signatureGrid hashes each visible tile's appearance inputs: its own tex / kind
// / ground / prop / light band, the same for its 8 neighbours (the seam dither
// reads them), and the frame for animated tiles (so they always re-render). Two
// tiles with the same signature rasterize to the same pixels, so a changed
// signature is exactly when a tile must be redrawn. The slow day/night tint is
// deliberately excluded — it is sub-quantum frame-to-frame and re-synced by the
// periodic full repaint.
func signatureGrid(tm *TileMap, frame int, light Light, ox, oy int, style *Style) []uint64 {
	vw, vh := tm.W, tm.H
	_, _, night := sunState()
	band := make([]int, vw*vh)
	for ty := 0; ty < vh; ty++ {
		for tx := 0; tx < vw; tx++ {
			band[ty*vw+tx] = lightBand(ox+tx, oy+ty, light)
		}
	}
	tileSig := func(tx, ty int) uint64 {
		t := tm.At(tx, ty)
		b := lightBands
		if tx >= 0 && tx < vw && ty >= 0 && ty < vh {
			b = band[ty*vw+tx]
		}
		h := fnv1a(uint64(t.Kind), uint64(t.Tex), uint64(t.Prop), uint64(b))
		h = fnv1aStr(h, string(t.Ground))
		h = fnv1aStr(h, string(t.Color))
		h = fnv1aStr(h, string(t.PropHex))
		return h
	}
	out := make([]uint64, vw*vh)
	for ty := 0; ty < vh; ty++ {
		for tx := 0; tx < vw; tx++ {
			h := tileSig(tx, ty)
			h = fnv1a(h, tileSig(tx, ty-1), tileSig(tx+1, ty-1), tileSig(tx+1, ty), tileSig(tx+1, ty+1))
			h = fnv1a(h, tileSig(tx, ty+1), tileSig(tx-1, ty+1), tileSig(tx-1, ty), tileSig(tx-1, ty-1))
			if tileAnimated(tm.At(tx, ty), ox+tx, oy+ty, night, style) {
				h = fnv1a(h, uint64(frame))
			}
			out[ty*vw+tx] = h
		}
	}
	return out
}

// tileAnimated reports whether a tile's pixels change frame-to-frame: water
// (ripples + glint), portals and emissive props (pulse/flicker, day or night),
// and — after dusk — the forest/swamp cells that actually host a drifting
// firefly or mist wisp. (wx,wy) is the cell's world position, needed for those
// per-cell night effects.
//
// The night branch is deliberately per-cell rather than per-biome: marking the
// whole wood animated meant a fully-explored forest re-rasterized every tile
// every night frame, which is the bulk of the "explored world" cost. Only the
// cells fireflyHost/mistHost light actually change, and their sub-tile drift and
// glow stay within the renderer's overhang, so the neighbours a mote spills onto
// are still redrawn via the dirty-set dilation (the same way a campfire's glow
// reaches its neighbours). Validated against a full render by
// TestIncrementalMatchesFull at day, dusk and night.
func tileAnimated(t Tile, wx, wy int, night float64, style *Style) bool {
	if t.Tex == TexWater || t.Prop == PropPortal {
		return true
	}
	if _, _, _, ok := emitterGlow(t.Prop, mustHex("#808080"), 0, 0, 0); ok {
		return true
	}
	switch t.Prop {
	case PropCaveMouth, // bats wheel over the mouth every frame
		PropLightShaft,             // dust drifts down the daylight beam
		PropStalagmite, PropColumn: // a waterdrop falls onto the formation
		return true
	}
	if propHasGlowArt(style, t.Prop) {
		return true
	}
	if night >= 0.3 {
		switch t.Tex {
		case TexForest:
			return fireflyHost(wx, wy)
		case TexSwamp:
			return fireflyHost(wx, wy) || mistHost(wx, wy)
		}
	}
	return false
}

// propHasGlowArt reports whether a prop's tile art contains an animated emissive
// ('G') pixel — lamps, screens, the reactor core — which pulse every frame.
func propHasGlowArt(style *Style, p TileProp) bool {
	for _, row := range style.Props[p] {
		for i := 0; i < len(row); i++ {
			if row[i] == 'G' {
				return true
			}
		}
	}
	return false
}

// lightBand returns the discrete brightness ring a tile sits in, matching the
// quantization in applyLight — so the band changes exactly when the lit color does.
func lightBand(wx, wy int, light Light) int {
	if light.Radius <= 0 {
		return lightBands
	}
	floor := light.floor()
	if floor >= 1 {
		return lightBands
	}
	d := math.Hypot(float64(wx-light.X), float64(wy-light.Y))
	f := 1 - d/float64(light.Radius)
	if f < floor {
		f = floor
	}
	if f > 1 {
		f = 1
	}
	t := (f - floor) / (1 - floor)
	return int(math.Round(t * lightBands))
}

// dilate grows the dirty set by r tiles (Chebyshev), so a changed caster's
// overhang tiles are redrawn too.
func dilate(dirty []bool, vw, vh, r int) []bool {
	out := make([]bool, vw*vh)
	for ty := 0; ty < vh; ty++ {
		for tx := 0; tx < vw; tx++ {
			if !dirty[ty*vw+tx] {
				continue
			}
			for ny := maxi(0, ty-r); ny <= mini(vh-1, ty+r); ny++ {
				for nx := maxi(0, tx-r); nx <= mini(vw-1, tx+r); nx++ {
					out[ny*vw+nx] = true
				}
			}
		}
	}
	return out
}

// dirtyRects coalesces a dirty tile mask into a few cover rectangles: horizontal
// runs per row, extended downward while the row below has the identical column
// span (so vertical bands merge into one rect instead of one per row).
func dirtyRects(dirty []bool, vw, vh int) []image.Rectangle {
	type run struct{ c0, c1, r0, r1 int }
	var done, open []run
	for r := 0; r < vh; r++ {
		var runs [][2]int
		for c := 0; c < vw; {
			if !dirty[r*vw+c] {
				c++
				continue
			}
			c0 := c
			for c < vw && dirty[r*vw+c] {
				c++
			}
			runs = append(runs, [2]int{c0, c - 1})
		}
		var next []run
		used := make([]bool, len(open))
		for _, rn := range runs {
			ext := -1
			for i, o := range open {
				if !used[i] && o.c0 == rn[0] && o.c1 == rn[1] {
					ext = i
					break
				}
			}
			if ext >= 0 {
				used[ext] = true
				o := open[ext]
				o.r1 = r
				next = append(next, o)
			} else {
				next = append(next, run{rn[0], rn[1], r, r})
			}
		}
		for i, o := range open {
			if !used[i] {
				done = append(done, o)
			}
		}
		open = next
	}
	done = append(done, open...)
	rects := make([]image.Rectangle, len(done))
	for i, d := range done {
		rects[i] = image.Rect(d.c0, d.r0, d.c1+1, d.r1+1)
	}
	return rects
}

// shiftRGBA writes src shifted by (sx,sy) px into dst (same bounds); pixels that
// land outside are dropped and uncovered pixels are left as-is (the caller marks
// them dirty for a fresh draw).
func shiftRGBA(dst, src *image.RGBA, sx, sy int) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		dy := y + sy
		if dy < 0 || dy >= h {
			continue
		}
		x0, dx0 := 0, sx
		if dx0 < 0 {
			x0, dx0 = -sx, 0
		}
		n := w - dx0
		if rem := w - x0; rem < n {
			n = rem
		}
		if n <= 0 {
			continue
		}
		so := src.PixOffset(b.Min.X+x0, b.Min.Y+y)
		do := dst.PixOffset(b.Min.X+dx0, b.Min.Y+dy)
		copy(dst.Pix[do:do+n*4], src.Pix[so:so+n*4])
	}
}

// fnv1a folds several 64-bit values into an FNV-1a hash.
func fnv1a(vals ...uint64) uint64 {
	h := uint64(14695981039346656037)
	for _, v := range vals {
		for i := 0; i < 8; i++ {
			h ^= (v >> (8 * i)) & 0xff
			h *= 1099511628211
		}
	}
	return h
}

func fnv1aStr(h uint64, s string) uint64 {
	if h == 0 {
		h = 14695981039346656037
	}
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}

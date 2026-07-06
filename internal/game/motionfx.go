package game

// Motion feedback for a dash: a sprinting avatar leans into its run and kicks
// a brief dust puff up from the tile it vacated. Both cues read the trail the
// world records on Move (Player.PrevX/PrevY/Ran) plus LastMoved, so they are
// pure functions of world state — no renderer-side bookkeeping. Like the walk
// bob they run on the wall clock: every tile they touch is redrawn each frame
// while they're live (footprints includes the vacated tile), so the
// incremental renderer stays byte-true.

import (
	"image"
	"time"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/world"
)

// puffWindow is how long run dust hangs behind a dash; puffBands is how many
// discrete fade steps it crosses (chunky retro steps, not a smooth fade).
const (
	puffWindow = 350 * time.Millisecond
	puffBands  = 3
)

// DustPuff reports whether p trails run dust right now and which fade band
// it's in (0 = fresh … puffBands-1 = nearly gone).
func DustPuff(p world.Player) (band int, ok bool) {
	if !p.Ran || (p.PrevX == p.X && p.PrevY == p.Y) {
		return 0, false
	}
	age := time.Since(p.LastMoved)
	if age < 0 || age >= puffWindow {
		return 0, false
	}
	band = int(age * puffBands / puffWindow)
	if band >= puffBands {
		band = puffBands - 1
	}
	return band, true
}

// runLean is the sideways art-pixel shift a sprinting avatar's upper body
// takes toward its heading: +1 running east-ish, -1 west-ish, 0 for straight
// north/south (and 0 whenever not mid-dash).
func runLean(p world.Player) int {
	if !p.Ran || time.Since(p.LastMoved) > 300*time.Millisecond {
		return 0
	}
	switch p.Facing {
	case world.DirE, world.DirNE, world.DirSE:
		return 1
	case world.DirW, world.DirNW, world.DirSW:
		return -1
	}
	return 0
}

// dustColor is a dry, pale earth — legible on grass and dirt alike without
// reading as smoke or snow.
var dustColor = colorful.Color{R: 0.66, G: 0.62, B: 0.54}

// drawDustRGBA kicks up one player's run dust in the HD frame: a few low
// blocks in the vacated tile that spread and thin by fade band.
func drawDustRGBA(img *image.RGBA, p world.Player, scale, originX, originY int) {
	band, ok := DustPuff(p)
	if !ok {
		return
	}
	apx := scale / tileArtN
	if apx < 1 {
		apx = 1
	}
	cx := (p.PrevX-originX)*scale + scale/2
	by := (p.PrevY-originY+1)*scale - apx // the vacated tile's foot line
	w := 0.65 * (1 - float64(band)/puffBands)
	rise := band * apx / 2
	spread := apx * (1 + band)
	blendRect(img, cx-spread, by-rise, apx, apx, dustColor, w)
	blendRect(img, cx+spread-apx, by-rise, apx, apx, dustColor, w)
	if band == 0 {
		blendRect(img, cx-apx/2, by, apx, apx, dustColor, w*0.9)
	}
}

// puffRunes is the glyph client's dust: one thinning mote per fade band.
var puffRunes = [puffBands]rune{'∘', '·', '.'}

// stampDust drops one player's run dust into the glyph grid — the single-cell
// degradation of the HD puff.
func stampDust(grid [][]rcell, p world.Player, originX, originY int) {
	band, ok := DustPuff(p)
	if !ok {
		return
	}
	putCell(grid, p.PrevY-originY, p.PrevX-originX, rcell{ch: puffRunes[band], fg: dustColor})
}

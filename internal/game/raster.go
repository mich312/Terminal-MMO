package game

import (
	"image"
	"image/color"
	"sort"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// RenderRGBA rasterizes the same scene the glyph renderer draws — terrain
// (day/night tint + radial light), then players — into a real RGBA image, for
// the experimental "real pixel" path (kitty graphics / sixel). Each tile
// becomes a scale×scale block; avatars are blitted from their bitmap as true
// pixels (sprite pixel = scale/2, so an avatar is ~3 tiles wide × 4 tall, the
// same proportions as the half-block renderer). This shares buildGrid so the
// palette, animation and lighting match the text renderer exactly.
func RenderRGBA(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame int, cam Camera, light Light, originX, originY, scale int) *image.RGBA {
	if th == nil {
		th = ui.Default
	}
	if scale < 1 {
		scale = 1
	}
	if cam.W <= 0 || cam.H <= 0 {
		cam = Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	}

	grid := buildGrid(th, tm, cam, light, frame)
	img := image.NewRGBA(image.Rect(0, 0, cam.W*scale, cam.H*scale))
	bg := colorfulToRGBA(shadowColor)

	for vy := 0; vy < cam.H && vy < len(grid); vy++ {
		row := grid[vy]
		for vx := 0; vx < cam.W && vx < len(row); vx++ {
			c := row[vx]
			fill := bg
			if !c.blank {
				fill = colorfulToRGBA(c.fg)
			}
			fillRect(img, vx*scale, vy*scale, scale, scale, fill)
		}
	}

	stampSpritesRGBA(img, players, self, scale, originX, originY)
	return img
}

// stampSpritesRGBA draws every player's avatar as real pixels, oldest movers
// first and self last, mirroring the glyph renderer's ordering.
func stampSpritesRGBA(img *image.RGBA, players []world.Player, self string, scale, originX, originY int) {
	sorted := make([]world.Player, len(players))
	copy(sorted, players)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name == self {
			return false
		}
		if sorted[j].Name == self {
			return true
		}
		return sorted[i].LastMoved.Before(sorted[j].LastMoved)
	})
	for _, p := range sorted {
		blitAvatar(img, p, p.Name == self, scale, p.X-originX, p.Y-originY)
	}
}

// blitAvatar renders one player's bitmap into the image. (fc,fr) is the
// top-left of the PlayerW×PlayerH footprint in camera-cell coordinates; the
// sprite is centered horizontally and bottom-aligned on the footprint, then
// overhangs upward — matching stampSprite's geometry.
func blitAvatar(img *image.RGBA, p world.Player, isSelf bool, scale, fc, fr int) {
	body, _ := colorful.MakeColor(p.Color)
	spx := scale / 2
	if spx < 1 {
		spx = 1
	}
	bmpW, bmpH := len(avatarBitmap[0]), len(avatarBitmap)

	centerX := (fc + PlayerW/2) * scale
	left := centerX - (bmpW*spx)/2
	bottomEdge := (fr + PlayerH) * scale
	top := bottomEdge - bmpH*spx

	for sy := 0; sy < bmpH; sy++ {
		runes := []rune(avatarBitmap[sy])
		for sx := 0; sx < bmpW; sx++ {
			col, opaque := spritePixel(runes[sx], body, isSelf)
			if !opaque {
				continue
			}
			fillRect(img, left+sx*spx, top+sy*spx, spx, spx, colorfulToRGBA(col))
		}
	}
}

// fillRect paints a w×h block at (x0,y0), clipped to the image bounds.
func fillRect(img *image.RGBA, x0, y0, w, h int, c color.RGBA) {
	b := img.Bounds()
	for y := y0; y < y0+h; y++ {
		if y < b.Min.Y || y >= b.Max.Y {
			continue
		}
		for x := x0; x < x0+w; x++ {
			if x < b.Min.X || x >= b.Max.X {
				continue
			}
			img.SetRGBA(x, y, c)
		}
	}
}

func colorfulToRGBA(c colorful.Color) color.RGBA {
	c = c.Clamped()
	return color.RGBA{
		R: uint8(c.R*255 + 0.5),
		G: uint8(c.G*255 + 0.5),
		B: uint8(c.B*255 + 0.5),
		A: 255,
	}
}

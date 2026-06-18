package game

import (
	"image"
	"image/color"
)

// DebugWalkable toggles a developer overlay that tints every blocked
// (non-walkable) tile red, so you can see exactly where a player can and can't
// go — trees, water and walls light up while open ground stays clear. Off by
// default; the HD client flips it with a key.
var DebugWalkable bool

// OverlayWalkable washes blocked tiles red when DebugWalkable is set. tm is the
// on-screen tile window; scale is pixels per tile. No-op otherwise.
func OverlayWalkable(img *image.RGBA, tm *TileMap, scale int) {
	if !DebugWalkable || tm == nil {
		return
	}
	const a = 0.40
	for ty := 0; ty < tm.H; ty++ {
		for tx := 0; tx < tm.W; tx++ {
			if tm.Tiles[ty][tx].Walkable {
				continue
			}
			for yy := ty * scale; yy < (ty+1)*scale; yy++ {
				for xx := tx * scale; xx < (tx+1)*scale; xx++ {
					if _, _, _, ok := getPixel(img, xx, yy); !ok {
						continue
					}
					o := img.RGBAAt(xx, yy)
					img.SetRGBA(xx, yy, color.RGBA{
						uint8(float64(o.R)*(1-a) + 0xE0*a),
						uint8(float64(o.G)*(1-a) + 0x33*a),
						uint8(float64(o.B)*(1-a) + 0x33*a),
						0xFF,
					})
				}
			}
		}
	}
}

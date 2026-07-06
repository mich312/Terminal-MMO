// Command seasonpreview renders one real Wilds window in mid-January and
// mid-July, so the seasonal snowline can be reviewed as pictures. Like the
// other *preview commands it is a throwaway art tool, not part of the server.
package main

import (
	"fmt"
	"image/png"
	"os"
	"time"

	"github.com/durst-group/durstworld/internal/areas/wilds"
	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/worldgen"
)

const (
	vw, vh = 36, 20
	scale  = 22
)

// window samples a vw×vh window of the live overworld through the same
// CellTile mapping the game uses.
func window(g *worldgen.Generator, ox, oy int) *game.TileMap {
	tiles := make([][]game.Tile, vh)
	for y := 0; y < vh; y++ {
		tiles[y] = make([]game.Tile, vw)
		for x := 0; x < vw; x++ {
			tiles[y][x] = wilds.CellTile(g.At(ox+x, oy+y))
		}
	}
	return &game.TileMap{W: vw, H: vh, Tiles: tiles}
}

func main() {
	g := worldgen.New(wilds.Seed)
	jan := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	// Pin the day/night cycle to noon so both shots differ only by season.
	noon := time.Date(2026, 6, 20, 10, 30, 0, 0, time.UTC)
	ui.Now = func() time.Time { return noon }

	// Find a window that straddles the winter snowline: plenty of cells dusted
	// in January, bare in July.
	worldgen.Clock = func() time.Time { return jan }
	ox, oy, found := 0, 0, false
	for r := 0; r <= 600 && !found; r += 6 {
		for _, cand := range [][2]int{{r, 0}, {-r, 0}, {0, r}, {0, -r}, {r, r}, {-r, -r}} {
			dusted := 0
			for y := 0; y < vh; y += 2 {
				for x := 0; x < vw; x += 2 {
					if g.At(cand[0]+x, cand[1]+y).Snowy {
						dusted++
					}
				}
			}
			if dusted > vw*vh/16 && dusted < vw*vh/5 { // a snowline, not a snowfield
				ox, oy, found = cand[0], cand[1], true
				break
			}
		}
	}
	fmt.Printf("window at (%d,%d)\n", ox, oy)

	if err := os.MkdirAll("seasonshots", 0o755); err != nil {
		panic(err)
	}
	for name, at := range map[string]time.Time{"january": jan, "july": jul} {
		worldgen.Clock = func() time.Time { return at }
		tm := window(g, ox, oy)
		img := game.RenderRGBA(nil, tm, nil, "", 7, game.Camera{W: vw, H: vh}, game.Light{}, ox, oy, scale, false, nil)
		f, err := os.Create("seasonshots/" + name + ".png")
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		f.Close()
		fmt.Println("wrote seasonshots/" + name + ".png")
	}
}

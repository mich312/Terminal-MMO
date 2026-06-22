// Command wildlifepreview renders the wildlife as pictures: a daylit clearing,
// a companion at heel, the night discovery-torch, plus a directional chart (each
// species facing S/E/N/W) and a walk strip (the fox's two-frame cycle). Throwaway
// art tool, like lootpreview — it pulls the real Species/creature sprites so what
// it draws matches the game.
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

func grass() game.Tile {
	return game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexGrass, Ground: "#5EAE63"}
}

func meadow(w, h int) *game.TileMap {
	tiles := make([][]game.Tile, h)
	for y := 0; y < h; y++ {
		tiles[y] = make([]game.Tile, w)
		for x := 0; x < w; x++ {
			tiles[y][x] = grass()
		}
	}
	return &game.TileMap{W: w, H: h, Tiles: tiles}
}

func tree(tm *game.TileMap, x, y int) {
	tm.Tiles[y][x] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexForest,
		Ground: "#2E6B40", Prop: game.PropTree, PropHex: "#2E5E34"}
}

func crit(kind string, x, y int, facing world.Dir, moving bool) world.Creature {
	c := world.Creature{Kind: kind, Area: "wilds", X: x, Y: y, Facing: facing, State: "wander"}
	if moving {
		c.LastMoved = time.Now()
	}
	return c
}

func saveImg(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}

func main() {
	style := game.DefaultStyle()
	if err := os.MkdirAll("wildlifeshots", 0o755); err != nil {
		panic(err)
	}
	// Midday so the day-faded torch opens up and the fauna read in full color.
	// (The cycle compresses a day into one real hour: minute 30 = noon.)
	noon := time.Date(2026, 6, 20, 12, 30, 0, 0, time.UTC)
	ui.Now = func() time.Time { return noon }

	// 1) Clearing in daylight: one of each species in situ.
	cl := meadow(24, 15)
	for _, p := range [][2]int{{2, 1}, {3, 1}, {5, 2}, {1, 3}, {4, 3}, {2, 4}} {
		tree(cl, p[0], p[1])
	}
	for y := 9; y <= 12; y++ {
		for x := 18; x <= 22; x++ {
			cl.Tiles[y][x] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexWater, Ground: "#2E6BFF"}
		}
	}
	clCrits := []world.Creature{
		crit("deer", 4, 5, world.DirE, false),
		crit("rabbit", 9, 7, world.DirW, false),
		crit("fox", 15, 6, world.DirW, true),
		crit("bird", 12, 3, world.DirE, false),
		crit("fish", 20, 10, world.DirE, false),
	}
	pl := []world.Player{{Name: "you", X: 11, Y: 8, Color: "#FFC861", Facing: world.DirS, LastMoved: time.Now()}}
	saveImg("wildlifeshots/wilds-fauna-day.png",
		game.RenderRGBA(nil, cl, pl, "you", 6, game.Camera{W: cl.W, H: cl.H}, game.Light{}, 0, 0, 30, false, style, clCrits...))

	// 2) Companion at heel (zoomed).
	cs := meadow(13, 9)
	for _, p := range [][2]int{{1, 1}, {11, 1}, {2, 7}, {10, 7}} {
		tree(cs, p[0], p[1])
	}
	csCrits := []world.Creature{
		crit("fox", 7, 4, world.DirW, true), // the companion, trotting at heel
		crit("deer", 10, 2, world.DirE, false),
	}
	cp := []world.Player{{Name: "you", X: 6, Y: 4, Color: "#FFC861", Facing: world.DirE, LastMoved: time.Now()}}
	saveImg("wildlifeshots/companion.png",
		game.RenderRGBA(nil, cs, cp, "you", 6, game.Camera{W: cs.W, H: cs.H}, game.Light{}, 0, 0, 44, false, style, csCrits...))

	// 3) Directional chart: each species facing S, E, N, W (front / side / back /
	//    mirrored side), at high zoom.
	kinds := []string{"rabbit", "deer", "fox", "bird", "fish"}
	faces := []world.Dir{world.DirS, world.DirE, world.DirN, world.DirW}
	dc := meadow(len(faces)*2+1, len(kinds)*2+1)
	var dcCrits []world.Creature
	for ki, k := range kinds {
		for fi, f := range faces {
			dcCrits = append(dcCrits, crit(k, fi*2+1, ki*2+1, f, false))
		}
	}
	saveImg("wildlifeshots/direction-chart.png",
		game.RenderRGBA(nil, dc, nil, "", 0, game.Camera{W: dc.W, H: dc.H}, game.Light{}, 0, 0, 44, false, style, dcCrits...))

	// 4) Walk strip: the fox facing east across two walk frames + idle, so the
	//    cycle is visible as stills.
	ws := meadow(7, 3)
	frames := []struct {
		x     int
		frame int
		mv    bool
	}{{1, 0, true}, {3, 3, true}, {5, 0, false}}
	for _, fr := range frames {
		ws.Tiles[1][fr.x] = grass()
	}
	// Render each pose into the same strip by compositing three single renders is
	// overkill; instead place three foxes and rely on per-creature animation being
	// frame-global — so show one render at frame 0 with all three moving/idle.
	wsCrits := []world.Creature{
		crit("fox", 1, 1, world.DirE, true),
		crit("fox", 3, 1, world.DirE, false),
	}
	saveImg("wildlifeshots/walk-strip.png",
		game.RenderRGBA(nil, ws, nil, "", 0, game.Camera{W: ws.W, H: ws.H}, game.Light{}, 0, 0, 56, false, style, wsCrits...))

	// 5) Night torch over the clearing.
	night := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	ui.Now = func() time.Time { return night }
	torch := game.DayFadedLight(game.Light{X: 11 + game.PlayerW/2, Y: 8 + game.PlayerH/2, Radius: 7})
	saveImg("wildlifeshots/wilds-fauna-night.png",
		game.RenderRGBA(nil, cl, pl, "you", 6, game.Camera{W: cl.W, H: cl.H}, torch, 0, 0, 30, false, style, clCrits...))

	fmt.Println("wrote wilds-fauna-day, companion, direction-chart, walk-strip, wilds-fauna-night")
}

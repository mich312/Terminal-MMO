// Command weatherpreview renders one representative outdoor scene under clear
// sky, a daytime storm, a night storm and steady snowfall, and writes the
// frames as PNGs — so the weather layer can be reviewed as pictures rather than
// read off the code. Like nightpreview it is a throwaway art tool, not part of
// the server.
package main

import (
	"fmt"
	"image/png"
	"os"
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// scene is a meadow crossing into a snowfield: grass with a few trees and a
// cottage on the left, a pond at the bottom, and a snow shelf along the right
// edge — one frame shows rain and snowfall side by side, plus how drops read
// over water, props and buildings.
func scene() *game.TileMap {
	const W, H = 24, 14
	tiles := make([][]game.Tile, H)
	for y := 0; y < H; y++ {
		tiles[y] = make([]game.Tile, W)
		for x := 0; x < W; x++ {
			t := game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexGrass, Ground: "#5EAE63"}
			if x >= 17 { // the snow shelf
				t.Tex, t.Ground = game.TexSnow, "#E8EEF5"
			}
			tiles[y][x] = t
		}
	}
	tree := func(x, y int) {
		tiles[y][x] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexForest,
			Ground: "#2E6B40", Prop: game.PropTree, PropHex: "#2E5E34"}
	}
	tree(2, 3)
	tree(3, 4)
	tree(5, 2)
	tree(13, 3)
	tiles[10][20] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexSnow,
		Ground: "#E8EEF5", Prop: game.PropFir, PropHex: "#3E6B52"}
	tiles[3][19] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexSnow,
		Ground: "#E8EEF5", Prop: game.PropFir, PropHex: "#3E6B52"}
	for y := 10; y <= 12; y++ { // the pond
		for x := 5; x <= 9; x++ {
			tiles[y][x] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexWater, Ground: "#2E6BFF"}
		}
	}
	tiles[6][10] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexGrass,
		Ground: "#5EAE63", Prop: game.PropHouse, PropHex: "#B07A4A"}
	tiles[4][15] = game.Tile{Kind: game.TilePortal, Walkable: true, Tex: game.TexGrass,
		Ground: "#5EAE63", Prop: game.PropPortal, PropHex: "#7DF0FF", Portal: "lobby"}
	return &game.TileMap{W: W, H: H, Tiles: tiles}
}

// findStorm scans forward, hour by hour, over instants pinned to a minute of
// the compressed one-hour day/night cycle (minute 30 = noon, 0 = midnight)
// until the storm intensity at the scene origin passes want — so a shot can be
// both "noon" and "under the front". The storm rhythm (~2.45 h) is
// incommensurate with the hourly step, so every coverage level comes around.
func findStorm(want float64, cycleMinute int) time.Time {
	base := time.Date(2026, 3, 1, 0, cycleMinute, 0, 0, time.UTC)
	for probe := base; ; probe = probe.Add(time.Hour) {
		i := game.StormAt(probe, 8, 6)
		if (want == 0 && i == 0) || (want > 0 && i >= want) {
			return probe
		}
	}
}

func main() {
	tm := scene()
	players := []world.Player{
		{Name: "you", X: 11, Y: 8, Color: "#FFC861", Facing: world.DirS, LastMoved: time.Now()},
	}
	style := game.DefaultStyle()
	cam := game.Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	const scale = 28

	stormNoon := findStorm(0.85, 30)
	shots := []struct {
		name  string
		at    time.Time
		sky   bool
		frame int
	}{
		{"1-clear-noon", findStorm(0, 30), true, 7},
		{"2-storm-noon", stormNoon, true, 7},
		{"3-storm-noon-next-frame", stormNoon, true, 8},
		{"4-storm-night", findStorm(0.85, 0), true, 7},
		{"5-indoors-during-storm", stormNoon, false, 7},
	}

	if err := os.MkdirAll("weathershots", 0o755); err != nil {
		panic(err)
	}
	for _, s := range shots {
		at := s.at
		ui.Now = func() time.Time { return at }
		light := game.Light{Sky: s.sky}
		if s.sky {
			light.Overcast = game.StormAt(at, 8, 6)
		}
		img := game.RenderRGBA(nil, tm, players, "you", s.frame, cam, light, 0, 0, scale, false, style)
		path := fmt.Sprintf("weathershots/%s.png", s.name)
		f, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		f.Close()
		fmt.Printf("wrote %s  (storm %.2f)\n", path, game.StormAt(at, 8, 6))
	}
}

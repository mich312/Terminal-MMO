// Command mechpreview renders a representative "cozy-frontier Workspace" scene —
// a fenced homestead with machines, a glowing forge, storage, a trade stall and
// the player's avatar — as real HD PNG frames, at a few points of the day/night
// cycle. It exists to answer "show me HQ frames of the new mechanics" with the
// actual pixel renderer, not ASCII. It is a throwaway art tool (not the server),
// and it composes the scene from the engine's *current* sprites so the frame is
// genuinely what the live sixel/kitty client would paint; the final mechanics
// would get dedicated workbench/sawmill/furnace sprites (see docs/MOCKUPS.md).
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

func main() {
	const W, H = 24, 15

	grass := func() game.Tile {
		return game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexGrass, Ground: "#3A7D44"}
	}
	tiles := make([][]game.Tile, H)
	for y := 0; y < H; y++ {
		tiles[y] = make([]game.Tile, W)
		for x := 0; x < W; x++ {
			tiles[y][x] = grass()
		}
	}

	// prop stamps a decoration onto an existing ground tile.
	prop := func(x, y int, p game.TileProp, hex string) {
		t := tiles[y][x]
		t.Prop, t.PropHex = p, hex
		tiles[y][x] = t
	}
	// dirt swaps the ground texture to a packed-earth yard under the buildings.
	dirt := func(x, y int) {
		tiles[y][x] = game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexField, Ground: "#6B5A3A"}
	}

	// A few trees ring the plot so it reads as a clearing in the Wilds.
	for x := 0; x < W; x++ {
		if x%3 != 1 {
			prop(x, 0, game.PropTree, "#2E5E34")
		}
	}
	prop(1, 1, game.PropTree, "#2E5E34")
	prop(22, 1, game.PropTree, "#2E5E34")
	prop(0, 6, game.PropTree, "#2E5E34")
	prop(23, 8, game.PropTree, "#2E5E34")

	// The Workspace fence: an enclosure with corner posts and a gate gap at the
	// bottom (the lane in). Rails run along the four sides.
	fx0, fy0, fx1, fy1 := 3, 3, 20, 13
	for x := fx0; x <= fx1; x++ {
		prop(x, fy0, game.PropFenceH, "#8A6E3C")
		if x < 11 || x > 12 { // leave a 2-tile gate at the bottom
			prop(x, fy1, game.PropFenceH, "#8A6E3C")
		}
	}
	for y := fy0; y <= fy1; y++ {
		prop(fx0, y, game.PropFenceV, "#8A6E3C")
		prop(fx1, y, game.PropFenceV, "#8A6E3C")
	}
	prop(fx0, fy0, game.PropFencePost, "#A8854C")
	prop(fx1, fy0, game.PropFencePost, "#A8854C")
	prop(fx0, fy1, game.PropFencePost, "#A8854C")
	prop(fx1, fy1, game.PropFencePost, "#A8854C")

	// Pack the yard under the work area with packed earth.
	for y := 5; y <= 11; y++ {
		for x := 5; x <= 18; x++ {
			dirt(x, y)
		}
	}

	// The cottage (home) up in the corner of the plot.
	prop(5, 5, game.PropHouse, "#B07A4A")

	// The production line, left to right: workbench, two machines, the forge.
	prop(8, 5, game.PropWorkbench, "#B8924E") // Crafting (Self-Service) workbench
	prop(10, 5, game.PropSawmill, "#8FB7FF")  // Sawmill: timber -> planks
	prop(12, 5, game.PropMill, "#C2A06A")     // Mill: grain -> flour
	prop(14, 5, game.PropFurnace, "#C46A3A")  // Ingot Synergy Furnace (glows warm)

	// Storage and the trade post.
	prop(8, 10, game.PropChest, "#9C7A45")  // Cold Storage chest
	prop(10, 10, game.PropChest, "#9C7A45") // Cold Storage chest
	prop(17, 10, game.PropStall, "#C98A4A") // Durst Group Concession (trade stall)

	// A well as the yard's centrepiece, with lamps and a brazier for night light.
	prop(13, 8, game.PropWell, "#B8BEC6")
	prop(7, 8, game.PropLamp, "#FFD27A")
	prop(16, 6, game.PropLamp, "#FFD27A")
	prop(17, 12, game.PropBrazier, "#FF8A3C")

	// Yields lying about: the machines' output and gathered stock.
	prop(11, 7, game.PropLog, "#9C6B3F")   // planks/timber from the sawmill
	prop(15, 7, game.PropStone, "#B8BEC6") // cut stone for building
	prop(9, 8, game.PropCrop, "#E6C84B")   // flour/grain by the mill
	prop(15, 10, game.PropCrop, "#E6C84B")

	tm := &game.TileMap{W: W, H: H, Tiles: tiles}

	// The owner, standing by the forge facing the line.
	players := []world.Player{
		{Name: "steurer", X: 13, Y: 6, Color: "#FFC861", Facing: world.DirS, LastMoved: time.Now()},
	}

	style := game.DefaultStyle()
	cam := game.Camera{X: 0, Y: 0, W: W, H: H}
	const scale = 32
	const frame = 7 // mid-animation so the flames/machines aren't caught flat

	shots := []struct {
		name string
		min  int
	}{
		{"1-noon", 30},
		{"2-golden-dusk", 44},
		{"3-night", 0},
	}

	if err := os.MkdirAll("mechshots", 0o755); err != nil {
		panic(err)
	}
	for _, s := range shots {
		at := time.Date(2026, 6, 20, 10, s.min, 0, 0, time.UTC)
		ui.Now = func() time.Time { return at }

		img := game.RenderRGBA(nil, tm, players, "steurer", frame, cam, game.Light{}, 0, 0, scale, false, style)
		path := fmt.Sprintf("mechshots/%s.png", s.name)
		f, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		f.Close()
		fmt.Printf("wrote %s\n", path)
	}
}

// Command uipreview renders the proposed cozy-frontier *interface* panels —
// Crafting (Self-Service), a machine's offline-production readout, build mode,
// and a trade Concession — as real HD frames, drawn over a live homestead scene
// with the same pixel primitives (pixel.DrawPanel/DrawText/Shade/Frame and
// game.DrawActionPrompt) the live sixel/kitty client uses. Like cmd/mechpreview
// it's a throwaway art tool, not the server: it shows what the panels would look
// like before they're wired into the game. Copy is corporate × medieval.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/pixel"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// ── palette (mirrors hd_ui.go's HUD colors) ────────────────────────────────
var (
	white  = color.RGBA{0xF2, 0xF5, 0xFA, 0xFF}
	dim    = color.RGBA{0x9A, 0xA3, 0xAD, 0xFF}
	accent = color.RGBA{0x2E, 0x8B, 0xFF, 0xFF}
	bright = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	warn   = color.RGBA{0xFF, 0xB4, 0x54, 0xFF}
	good   = color.RGBA{0x7B, 0xD8, 0x8F, 0xFF}
)

const (
	s  = 2      // text scale at this frame width
	lh = 16 * s // line height
)

func main() {
	if err := os.MkdirAll("uishots", 0o755); err != nil {
		panic(err)
	}
	at := time.Date(2026, 6, 20, 10, 30, 0, 0, time.UTC) // noon, calm light
	ui.Now = func() time.Time { return at }

	write("uishots/1-crafting.png", panelOver(craftPanel))
	write("uishots/2-machine.png", panelOver(machinePanel))
	write("uishots/3-build.png", buildFrame())
	write("uishots/4-trade.png", panelOver(tradePanel))
}

func write(path string, img *image.RGBA) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
	fmt.Println("wrote", path)
}

// panelOver renders the homestead, dims it as a modal backdrop, then runs draw.
func panelOver(draw func(*image.RGBA)) *image.RGBA {
	img := homestead()
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	pixel.Shade(img, 0, 0, W, H, 0.5)
	draw(img)
	return img
}

// ── small drawing helpers ──────────────────────────────────────────────────

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	r := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
	for j := r.Min.Y; j < r.Max.Y; j++ {
		for i := r.Min.X; i < r.Max.X; i++ {
			img.SetRGBA(i, j, c)
		}
	}
}

// swatch draws a small color chip — stands in for an item's gem icon.
func swatch(img *image.RGBA, x, y int, hex string) int {
	box := 9 * s
	c := hexRGBA(hex)
	fillRect(img, x, y+2*s, box, box, c)
	fillRect(img, x+box-2*s, y+2*s, 2*s, 2*s, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}) // glint
	return box
}

func hexRGBA(h string) color.RGBA {
	var r, g, b uint8
	fmt.Sscanf(h, "#%02x%02x%02x", &r, &g, &b)
	return color.RGBA{r, g, b, 0xFF}
}

// progressBar draws a filled meter (filled/total) at (x,y).
func progressBar(img *image.RGBA, x, y, w, filled, total int, c color.RGBA) {
	h := 8 * s
	fillRect(img, x, y, w, h, color.RGBA{0x2A, 0x30, 0x38, 0xFF})
	if total > 0 {
		fillRect(img, x, y, w*filled/total, h, c)
	}
	pixel.Frame(img, x, y, w, h)
}

func center(img *image.RGBA, pw, ph int) (ox, oy int) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	return (W - pw) / 2, (H - ph) / 2
}

// rightText draws s right-aligned so it ends at xRight.
func rightText(img *image.RGBA, xRight, y int, str string, c color.RGBA) {
	pixel.DrawText(img, xRight-pixel.TextWidth(str, s), y, s, str, c)
}

// ── 1 · Crafting (Self-Service) ────────────────────────────────────────────

func craftPanel(img *image.RGBA) {
	pw, ph := 720, 448
	ox, oy := center(img, pw, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)
	pad := 11 * s
	x, y := ox+pad, oy+pad

	pixel.DrawText(img, x, y, s, "CRAFTING", accent)
	pixel.DrawText(img, x+pixel.TextWidth("CRAFTING  ", s), y, s, "(Self-Service)", dim)
	y += lh + lh/3

	type recipe struct {
		name, needs, count string
		sel                bool
	}
	rows := []recipe{
		{"Wall Block", "2 Stone + 1 Plank", "x3", true},
		{"Planks", "2 Timber", "x8", false},
		{"Wrought Lamp", "1 Nugget + 1 Amber", "x1", false},
		{"Field Salve", "1 Herb + 1 Mushroom", "x5", false},
		{"Sawmill", "6 Plank + 4 Lamp", "--", false},
	}
	// Measure the name column so the needs column never collides with it.
	nameCol := 0
	for _, r := range rows {
		if w := pixel.TextWidth(r.name, s); w > nameCol {
			nameCol = w
		}
	}
	needsX := x + 8*s + nameCol + 12*s
	for _, r := range rows {
		col := white
		if r.sel {
			pixel.Shade(img, x-4*s, y-2*s, pw-2*pad+8*s, lh, 0.3)
			pixel.DrawText(img, x-3*s, y, s, ">", accent)
			col = bright
		}
		pixel.DrawText(img, x+8*s, y, s, r.name, col)
		pixel.DrawText(img, needsX, y, s, r.needs, dim)
		cc := good
		if r.count == "--" {
			cc = dim
		}
		rightText(img, ox+pw-pad, y, "["+r.count+"]", cc)
		y += lh
	}

	// divider
	y += lh / 4
	fillRect(img, x, y, pw-2*pad, s, color.RGBA{0x3A, 0x42, 0x4C, 0xFF})
	y += lh / 2

	// selected detail
	pixel.DrawText(img, x, y, s, "Wall Block", bright)
	y += lh
	pixel.DrawText(img, x, y, s, "\"Load-bearing. Meets Durst facility code.\"", dim)
	y += lh + lh/4
	pixel.DrawText(img, x, y, s, "needs:", dim)
	nx := x + pixel.TextWidth("needs:  ", s)
	nx += swatch(img, nx, y, "#B8BEC6") + 3*s
	pixel.DrawText(img, nx, y, s, "Cut Stone", white)
	nx += pixel.TextWidth("Cut Stone ", s)
	pixel.DrawText(img, nx, y, s, "OK", good)
	nx += pixel.TextWidth("OK    ", s)
	nx += swatch(img, nx, y, "#9C6B3F") + 3*s
	pixel.DrawText(img, nx, y, s, "Planks", white)
	nx += pixel.TextWidth("Planks ", s)
	pixel.DrawText(img, nx, y, s, "OK", good)
	y += lh
	pixel.DrawText(img, x, y, s, "yields:", dim)
	yx := x + pixel.TextWidth("yields:  ", s)
	yx += swatch(img, yx, y, "#C9A86A") + 3*s
	pixel.DrawText(img, yx, y, s, "Wall Block x1", white)

	// footer + craft button
	fy := oy + ph - pad - lh + lh/4
	pixel.DrawText(img, x, fy, s, "up/down choose    q close", dim)
	btn := "[ e  CRAFT x3 ]"
	bw := pixel.TextWidth(btn, s) + 8*s
	bx := ox + pw - pad - bw
	pixel.Shade(img, bx, fy-3*s, bw, lh, 0.4)
	pixel.Frame(img, bx, fy-3*s, bw, lh)
	pixel.DrawText(img, bx+4*s, fy, s, btn, good)
}

// ── 2 · Machine: Ingot Synergy Furnace (offline production) ─────────────────

func machinePanel(img *image.RGBA) {
	pw, ph := 640, 392
	ox, oy := center(img, pw, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)
	pad := 11 * s
	x, y := ox+pad, oy+pad
	right := ox + pw - pad

	pixel.DrawText(img, x, y, s, "INGOT SYNERGY FURNACE", warn)
	rightText(img, right, y, "owner: steurer", dim)
	y += lh + lh/3
	pixel.DrawText(img, x, y, s, "status:", dim)
	pixel.DrawText(img, x+pixel.TextWidth("status:  ", s), y, s, "SMELTING", good)
	y += lh + lh/3

	// Each bar runs from after its label to where the right-hand readout begins.
	barX := x + pixel.TextWidth("output  ", s)
	inLabel, outLabel := "Gold Nugget x14", "Gold Ingot x7  (cap 8)"
	readoutW := pixel.TextWidth(outLabel, s)
	if w := pixel.TextWidth(inLabel, s); w > readoutW {
		readoutW = w
	}
	barW := right - readoutW - 8*s - barX
	pixel.DrawText(img, x, y+s, s, "input", dim)
	progressBar(img, barX, y, barW, 5, 8, accent)
	rightText(img, right, y+s, inLabel, white)
	y += lh + lh/4
	pixel.DrawText(img, x, y+s, s, "output", dim)
	progressBar(img, barX, y, barW, 7, 8, warn)
	rightText(img, right, y+s, outLabel, white)
	y += lh + lh/2

	fillRect(img, x, y, pw-2*pad, s, color.RGBA{0x3A, 0x42, 0x4C, 0xFF})
	y += lh / 2
	pixel.DrawText(img, x, y, s, "While you were away  (3h 41m):", accent)
	y += lh
	gained := "+ 5 Gold Ingot"
	pixel.DrawText(img, x+8*s, y, s, gained, good)
	pixel.DrawText(img, x+8*s+pixel.TextWidth(gained+"     ", s), y, s, "- 4 Gold Nugget", dim)
	y += lh + lh/4
	pixel.DrawText(img, x, y, s, "next ingot ~6m", dim)
	rightText(img, right, y, "rate 1 / 20m", dim)

	fy := oy + ph - pad - lh + lh/4
	for i, b := range []struct {
		txt string
		c   color.RGBA
	}{{"[ e  COLLECT 7 ]", good}, {"[ f  REFUEL ]", warn}} {
		bw := pixel.TextWidth(b.txt, s) + 8*s
		bx := x + i*(bw+10*s)
		pixel.Shade(img, bx, fy-3*s, bw, lh, 0.4)
		pixel.Frame(img, bx, fy-3*s, bw, lh)
		pixel.DrawText(img, bx+4*s, fy, s, b.txt, b.c)
	}
}

// ── 3 · Build mode (placement ghost over the live world) ────────────────────

func buildFrame() *image.RGBA {
	img := homestead()
	game.DrawAreaTitle(img, "Build  -  placing: Sawmill", 1)
	// A green placement ghost on an open yard tile (world tile 16,8 at scale 32).
	const scale = 32
	gx, gy := 16*scale, 8*scale
	ghost := color.RGBA{0x7B, 0xD8, 0x8F, 0xFF}
	// translucent green fill
	r := image.Rect(gx, gy, gx+scale, gy+scale).Intersect(img.Bounds())
	for j := r.Min.Y; j < r.Max.Y; j++ {
		for i := r.Min.X; i < r.Max.X; i++ {
			o := img.RGBAAt(i, j)
			img.SetRGBA(i, j, color.RGBA{
				uint8((int(o.R) + int(ghost.R)) / 2),
				uint8((int(o.G)*1 + int(ghost.G)*3) / 4),
				uint8((int(o.B) + int(ghost.B)) / 2), 0xFF})
		}
	}
	pixel.Frame(img, gx, gy, scale, scale)
	game.DrawActionPrompt(img, "e place (6 Planks  4 Lamp)   r rotate   q cancel")
	return img
}

// ── 4 · Trade: a Durst Group Concession ─────────────────────────────────────

func tradePanel(img *image.RGBA) {
	pw, ph := 688, 312
	ox, oy := center(img, pw, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)
	pad := 11 * s
	x, y := ox+pad, oy+pad
	right := ox + pw - pad
	askX := x + 168*s // the "she asks" column

	pixel.DrawText(img, x, y, s, "DURST GROUP CONCESSION", accent)
	rightText(img, right, y, "anna's stall", dim)
	y += lh + lh/3
	pixel.DrawText(img, x, y, s, "she offers", dim)
	pixel.DrawText(img, askX, y, s, "she asks", dim)
	y += lh + lh/4

	type swap struct {
		offHex, off, askHex, ask string
		sel                      bool
	}
	rows := []swap{
		{"#9C6B3F", "Planks x10", "#B8BEC6", "Cut Stone x6", true},
		{"#FFC861", "Gold Ingot x2", "#9CE0FF", "Geode x1", false},
		{"#7BD88F", "Field Salve x5", "#E6C84B", "Grain x8", false},
	}
	for _, r := range rows {
		col := white
		if r.sel {
			pixel.Shade(img, x-4*s, y-2*s, pw-2*pad+8*s, lh, 0.3)
			pixel.DrawText(img, x-3*s, y, s, ">", accent)
			col = bright
		}
		cx := x + 8*s
		cx += swatch(img, cx, y, r.offHex) + 3*s
		pixel.DrawText(img, cx, y, s, r.off, col)
		pixel.DrawText(img, askX-pixel.TextWidth("<->  ", s), y, s, "<->", dim)
		ax := askX
		ax += swatch(img, ax, y, r.askHex) + 3*s
		pixel.DrawText(img, ax, y, s, r.ask, col)
		y += lh
	}
	y += lh / 3
	fillRect(img, x, y, pw-2*pad, s, color.RGBA{0x3A, 0x42, 0x4C, 0xFF})
	y += lh / 2
	pixel.DrawText(img, x, y, s, "your pack: Cut Stone x11", dim)
	rightText(img, right, y, "you can afford row 1", good)

	fy := oy + ph - pad - lh + lh/4
	pixel.DrawText(img, x, fy, s, "up/down choose", dim)
	btn := "[ e  ACCEPT TRADE ]"
	bw := pixel.TextWidth(btn, s) + 8*s
	bx := ox + pw - pad - bw
	pixel.Shade(img, bx, fy-3*s, bw, lh, 0.4)
	pixel.Frame(img, bx, fy-3*s, bw, lh)
	pixel.DrawText(img, bx+4*s, fy, s, btn, good)
}

// ── the shared homestead backdrop (same composition as cmd/mechpreview) ──────

func homestead() *image.RGBA {
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
	prop := func(x, y int, p game.TileProp, hex string) {
		t := tiles[y][x]
		t.Prop, t.PropHex = p, hex
		tiles[y][x] = t
	}
	dirt := func(x, y int) {
		tiles[y][x] = game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexField, Ground: "#6B5A3A"}
	}
	for x := 0; x < W; x++ {
		if x%3 != 1 {
			prop(x, 0, game.PropTree, "#2E5E34")
		}
	}
	fx0, fy0, fx1, fy1 := 3, 3, 20, 13
	for x := fx0; x <= fx1; x++ {
		prop(x, fy0, game.PropFenceH, "#8A6E3C")
		if x < 11 || x > 12 {
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
	for y := 5; y <= 11; y++ {
		for x := 5; x <= 18; x++ {
			dirt(x, y)
		}
	}
	prop(5, 5, game.PropHouse, "#B07A4A")
	prop(8, 5, game.PropCrate, "#A8854C")
	prop(10, 5, game.PropMachine, "#8FB7FF")
	prop(12, 5, game.PropMachine, "#8FB7FF")
	prop(14, 5, game.PropCampfire, "#FF7A2C")
	prop(8, 10, game.PropCrate, "#9C7A45")
	prop(10, 10, game.PropCrate, "#9C7A45")
	prop(17, 10, game.PropStall, "#C98A4A")
	prop(13, 8, game.PropWell, "#B8BEC6")
	prop(7, 8, game.PropLamp, "#FFD27A")
	prop(16, 6, game.PropLamp, "#FFD27A")
	prop(17, 12, game.PropBrazier, "#FF8A3C")
	prop(11, 7, game.PropLog, "#9C6B3F")
	prop(15, 7, game.PropStone, "#B8BEC6")
	prop(9, 8, game.PropCrop, "#E6C84B")
	prop(15, 10, game.PropCrop, "#E6C84B")

	tm := &game.TileMap{W: W, H: H, Tiles: tiles}
	players := []world.Player{{Name: "steurer", X: 13, Y: 6, Color: "#FFC861", Facing: world.DirS, LastMoved: time.Now()}}
	cam := game.Camera{X: 0, Y: 0, W: W, H: H}
	return game.RenderRGBA(nil, tm, players, "steurer", 7, cam, game.Light{}, 0, 0, 32, false, game.DefaultStyle())
}

// Package vault is The Vault — the chamber beyond the Sunken Gate, a
// cooperative sealed gate the community opens by pooling offerings. A small
// reward room built as a game.FlavorArea; a portal returns to the Wilds.
package vault

import "github.com/durst-group/durstworld/internal/game"

var rows = []string{
	"###################",
	"#.................#",
	"#.CC.gg.....gg.CC.#",
	"#.C...........C...#",
	"#.....g.G.g.......#",
	"#.C...........C...#",
	"#.CC.gg.....gg.CC.#",
	"#........#0#......#",
	"##########.########",
}

var legend = map[rune]game.LegendEntry{
	'.': {Kind: game.TileFloor, Ch: '·', Walkable: true, Color: "#9C8D67", Tex: game.TexDirt, Ground: "#3A3530"},
	'C': {Kind: game.TileWall, Ch: '█', Tex: game.TexBrick, Ground: "#4A4030"},
	'g': {Kind: game.TileDecor, Ch: '◆', Walkable: true, Color: "#FFC861", Tex: game.TexDirt, Ground: "#3A3530", Prop: game.PropGem, PropHex: "#FFC861"},
	'G': {Kind: game.TileObject, Ch: '◉', Walkable: true, Color: "#7DF0FF", Tex: game.TexDirt, Ground: "#3A3530", Prop: game.PropCore, PropHex: "#7DF0FF"},
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "wilds", Label: "The Wilds"},
}

func init() {
	game.RegisterFlavor(game.FlavorConfig{
		ID: "vault", Display: "The Vault",
		Rows: rows, Legend: legend,
		SpawnX: 9, SpawnY: 7,
		Title: "◆ The Sunken Vault",
		Body:  "Opened by the whole community's offerings.\n\nGold glints in the alcoves and a core hums at the center — proof of what cooperation unlocks.",
		Light: 9,
	})
}

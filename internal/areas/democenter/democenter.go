// Package democenter is a placeholder area: a small showroom with
// decorative printers and a portal back to the lobby, built as a
// game.FlavorArea (map data plus one RegisterFlavor call).
package democenter

import "github.com/durst-group/durstworld/internal/game"

var rows = []string{
	"##############################",
	"#............................#",
	"#...pppppp........pppppp.....#",
	"#...p....p........p....p.....#",
	"#...pppppp........pppppp.....#",
	"#............................0",
	"#............................#",
	"#.........pppppppp...........#",
	"#.........p......p...........#",
	"#.........pppppppp...........#",
	"#............................#",
	"##############################",
}

var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	'p': {Kind: game.TileDecor, Ch: '▚'},
}

func init() {
	game.RegisterFlavor(game.FlavorConfig{
		ID: "democenter", Display: "Demo Center",
		Rows: rows, Legend: legend,
		SpawnX: 27, SpawnY: 5, Jitter: 1,
		Title:     "🖨 Demo Center",
		Body:      "The printers are warming up.\n\nCome back for the full tour.",
		PanelLeft: true,
	})
}

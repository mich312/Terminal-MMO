// Package grove is The Grove — the hidden glade beyond the Whispering Gate, a
// personal sealed gate in the Wilds. A small reward room built as a
// game.FlavorArea; a portal returns to the Wilds.
package grove

import "github.com/durst-group/durstworld/internal/game"

var rows = []string{
	"###################",
	"#.................#",
	"#..ttt.......ttt..#",
	"#.t..............t#",
	"#.t...*.....*....t#",
	"#........***......#",
	"#.t...*.....*....t#",
	"#.t..............t#",
	"#..ttt..#0#..ttt..#",
	"#########.#########",
}

var legend = map[rune]game.LegendEntry{
	'.': {Kind: game.TileFloor, Ch: '·', Walkable: true, Color: "#3F8A5A", Tex: game.TexGrass, Ground: "#3F8A5A"},
	't': {Kind: game.TileDecor, Ch: '♣', Color: "#2F7D4F", Tex: game.TexForest, Ground: "#3F8A5A", Prop: game.PropTree, PropHex: "#2F7D4F"},
	'*': {Kind: game.TileDecor, Ch: '✿', Walkable: true, Color: "#C792EA", Tex: game.TexGrass, Ground: "#3F8A5A", Prop: game.PropFlower, PropHex: "#C792EA"},
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "wilds", Label: "The Wilds"},
}

func init() {
	game.RegisterFlavor(game.FlavorConfig{
		ID: "grove", Display: "The Grove",
		Rows: rows, Legend: legend,
		SpawnX: 9, SpawnY: 8,
		Title: "❀ The Whispering Grove",
		Body:  "A hidden glade beyond the sealed gate.\n\nThe trees murmur as you pass — whatever riddle you answered, the grove remembers.",
	})
}

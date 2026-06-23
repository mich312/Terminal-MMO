// Package kraftwerk is the Durst power hall: a turbine gallery with raised
// machine alcoves, a console bank and a coolant sump, opening through a corridor
// into a round reactor chamber whose core pulses with energy. A portal returns
// to the lobby. Built as a game.FlavorArea (map data plus one RegisterFlavor
// call); the legend drives both the glyph and HD pixel renderers.
package kraftwerk

import "github.com/durst-group/durstworld/internal/game"

var rows = []string{
	"######################################################",
	"#######.....###.....###.....##########################",
	"#######.mmm.###.mmm.###.mmm.##########################",
	"#######.mmm.###.mmm.###.mmm.##########################",
	"######.........................#######################",
	"#####...........................######################",
	"####.=.........................=.#####################",
	"###........o........o.......o.....######CCC.o.CCC#####",
	"###...............................#####CC.......CC####",
	"###..=.........................=..####C...........C###",
	"###...............................###C....rrrrr....C##",
	"#....................................C...rr...rr...C##",
	"#....=.........................=.........r.RRR.r...C##",
	"0.......................................rrRRRRRrr..CC#",
	"#........................................r.RRR.r...C##",
	"#....=.........................=.....C...rr...rr...C##",
	"###.................o.......o.....###C....rrrrr....C##",
	"###...............................####C...........C###",
	"###..=.........................=..#####CC.......CC####",
	"####..kkkkkkkkkk.................#######CCC.o.CCC#####",
	"#####.kkkkkkkkkk................######################",
	"######.........................#######################",
	"##########.~~~~~~~~~.#################################",
	"##########.~~~~~~~~~.#################################",
	"##########...........#################################",
	"######################################################",
}

var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◈', Walkable: true, Portal: "wilds", Label: "The Wilds"},
	// Metal-plate floor and walls.
	'.': {Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexMetal, Ground: "#23272E"},
	'#': {Kind: game.TileWall, Ch: '█', Tex: game.TexMetal, Ground: "#3A424C"},
	// Reactor core: a white-hot orb that pulses (HD: PropCore glow).
	'R': {Kind: game.TileDecor, Ch: '◉', Tex: game.TexMetal, Ground: "#16345E", Prop: game.PropCore, PropHex: "#7DF0FF", Anim: &game.TileAnim{
		Frames: []rune{'◉', '◎', '●', '◎'}, ColorA: "#2E6BFF", ColorB: "#EAFBFF", Speed: 1}},
	// Reactor casing shell and inner ring.
	'C': {Kind: game.TileDecor, Ch: '▒', Color: "#566372", Tex: game.TexMetal, Ground: "#566372"},
	'r': {Kind: game.TileDecor, Ch: '▚', Color: "#6B7480", Tex: game.TexMetal, Ground: "#6B7480"},
	// Turbine units in the raised alcoves (HD: PropTurbine glow band).
	'm': {Kind: game.TileDecor, Ch: '▓', Tex: game.TexMetal, Ground: "#46566B", Prop: game.PropTurbine, PropHex: "#6FA8D8", Anim: &game.TileAnim{
		Frames: []rune{'▓', '▒', '░', '▒'}, ColorA: "#3A4654", ColorB: "#7DF0FF", Speed: 2}},
	// Console / machine bank.
	'k': {Kind: game.TileDecor, Ch: '▦', Tex: game.TexMetal, Ground: "#2A2F38", Prop: game.PropMachine, PropHex: "#7BD88F", Anim: &game.TileAnim{
		Frames: []rune{'▦', '▩', '▦', '▣'}, ColorA: "#7BD88F", ColorB: "#FFC861", Speed: 3}},
	// Steam pipes with glowing valves.
	'=': {Kind: game.TileDecor, Ch: '═', Color: "#8A93A0", Tex: game.TexMetal, Ground: "#2A2F38", Prop: game.PropPipe, PropHex: "#9AA3AD"},
	// Coolant sump: real flowing water in HD.
	'~': {Kind: game.TileDecor, Ch: '~', Tex: game.TexWater, Ground: "#2E6BFF", Anim: &game.TileAnim{
		Frames: []rune{'~', '≈', '~', '≋'}, ColorA: "#2E6BFF", ColorB: "#56E1FF", Speed: 3}},
	// Catwalk lamps: warm flicker (HD: PropLamp glow).
	'o': {Kind: game.TileObject, Ch: '◉', Tex: game.TexMetal, Ground: "#2A2F38", Prop: game.PropLamp, PropHex: "#FFC861", Anim: &game.TileAnim{
		ColorA: "#FF8A4C", ColorB: "#FFC861", Speed: 2}},
}

func init() {
	game.RegisterFlavor(game.FlavorConfig{
		ID: "kraftwerk", Display: "Kraftwerk",
		Rows: rows, Legend: legend,
		SpawnX: 6, SpawnY: 12, Jitter: 0,
		Title: "⚡ Durst Kraftwerk",
		Body:  "Home of spin-offs and experiments.\n\nFollow the corridor to the reactor — mind the coolant.",
		// The hall sits in shadow; the player's lamp reaches into it.
		Light: 13,
	})
}

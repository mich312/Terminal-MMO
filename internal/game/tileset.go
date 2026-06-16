package game

// Tileset: hand-authored 6×6 pixel-art tiles, matching the avatar's pixel
// density (the 12×12 avatar spans 2×2 tiles → 6 art-pixels per tile). Ground
// tiles are shade patterns (B base, L light, D dark, ' ' base) colored live by
// each cell's color, so day/night tint still works. Prop sprites overlay the
// ground (P prop, p prop-shade, T trunk, '.' transparent). The HD renderer
// nearest-upscales these to the on-screen tile size, so edges stay crisp.

// groundArt holds variants per surface. For water the variants are animation
// frames; otherwise one is picked per tile by a coordinate hash to break up
// obvious repetition. Edge pixels stay base so same-type tiles join seamlessly.
var groundArt = map[TileTex][][]string{
	TexGrass: {
		{"BBBBBB", "BBLBBB", "BBBBBB", "BDBBBB", "BBBBLB", "BBBBBB"},
		{"BBBBBB", "BLBBBB", "BBBDBB", "BBBBBB", "BBLBBB", "BBBBBB"},
		{"BBBBBB", "BBBLBB", "BBBBBB", "BDBBDB", "BBBBBB", "BBLBBB"},
	},
	TexSand: {
		{"BBBBBB", "BBDBBB", "BBBBBB", "DBBBDB", "BBBBBB", "BBDBBB"},
		{"BBBBBB", "BDBBBB", "BBBBDB", "BBBBBB", "BBDBBB", "DBBBBB"},
	},
	TexWater: {
		{"BBBBBB", "LBBLBB", "BBBBBB", "BBBBBB", "BBLBBL", "BBBBBB"},
		{"BBBBBB", "BBBBBB", "BLBBLB", "BBBBBB", "BBBBBB", "LBBLBB"},
		{"BBBBBB", "BBLBBL", "BBBBBB", "LBBLBB", "BBBBBB", "BBBBBB"},
	},
	TexDirt: {
		{"BBBBBB", "BBLBBB", "DBBBBB", "BBBBDB", "BLBBBB", "BBBBBB"},
		{"BBBBBB", "LBBBBB", "BBBDBB", "BBBBBB", "BBLBBD", "BBBBBB"},
	},
	TexForest: {
		{"BBBBBB", "BLBBBB", "BBBBDB", "BBBBBB", "BDBBLB", "BBBBBB"},
		{"BBBBBB", "BBBLBB", "DBBBBB", "BBBBDB", "BBBBBB", "BLBBBB"},
	},
	TexRock: {
		{"BLBBBB", "BBBBDB", "DDBBBB", "BBBDBB", "BBBBBB", "BBLBBB"},
		{"BBBLBB", "DBBBBB", "BBBBDD", "BBBBBB", "BLBBBB", "BBBBDB"},
	},
}

var propArt = map[TileProp][]string{
	PropFlower: {
		"......",
		".P.P..",
		"..p...",
		".P.P..",
		"......",
		"......",
	},
	PropTuft: {
		"......",
		"......",
		"P.P.P.",
		"PpPpp.",
		"......",
		"......",
	},
	PropTree: {
		"..PP..",
		".PpPP.",
		"PPPPPP",
		".PpPP.",
		"..TT..",
		"..TT..",
	},
	PropBoulder: {
		"......",
		".PPPp.",
		"PPPpp.",
		"PppppP",
		".pppp.",
		"......",
	},
	PropBush: {
		"..PP..",
		".PPPPp",
		"PPPPpp",
		"PpPPpp",
		".pPpp.",
		"......",
	},
	PropRock: {
		"......",
		"......",
		".PPp..",
		"PPppP.",
		".pppp.",
		"......",
	},
	PropStump: {
		"......",
		"......",
		".PPPP.",
		"PpppPP",
		".PppP.",
		"......",
	},
	// PropHouse / PropPortal are drawn by a separate multi-tile pass, not here.
}

// houseArt is a 2-wide × 3-tall decorative building (12×18 art-pixels): pitched
// roof (p), walls (P) with windows (L), a solid wooden door and stone base (D).
// A house — not a portal.
var houseArt = []string{
	"....pppp....",
	"...pppppp...",
	"..pppppppp..",
	".pppppppppp.",
	"pppppppppppp",
	"pPPPPPPPPPPp",
	"PPPPPPPPPPPP",
	"PPLLPPPPLLPP",
	"PPLLPPPPLLPP",
	"PPPPPPPPPPPP",
	"PPPPDDDDPPPP",
	"PPPDDDDDDPPP",
	"PPPDDDDDDPPP",
	"PPPDDDDDDPPP",
	"PPPDDDDDDPPP",
	"PPPDDDDDDPPP",
	"PPPPPPPPPPPP",
	"DDDDDDDDDDDD",
}

// portalArt is a freestanding 2×2-tile gate (12×12 art-pixels): a ring (R) in
// the destination's color enclosing a swirling energy field (@) that animates.
// A portal — not a house.
var portalArt = []string{
	"...RRRRRR...",
	"..R@@@@@@R..",
	".R@@@@@@@@R.",
	".R@@@@@@@@R.",
	"R@@@@@@@@@@R",
	"R@@@@@@@@@@R",
	"R@@@@@@@@@@R",
	"R@@@@@@@@@@R",
	".R@@@@@@@@R.",
	".R@@@@@@@@R.",
	"..R@@@@@@R..",
	"...RRRRRR...",
}

// trunkColor is the fixed wood color for tree trunks (prop code 'T').
var trunkColor = mustHex("#6B4A2B")

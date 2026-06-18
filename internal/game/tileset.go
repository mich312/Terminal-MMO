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
	// Forest floor: dense leaf litter — clumped dark flecks (D) with the odd
	// dapple of light (L) through the canopy. Busier than open grass.
	TexForest: {
		{"BBBBBB", "BDBBDB", "BBLBBB", "BBBDBB", "BDBBDB", "BBBBBB"},
		{"BBBBBB", "BBDBLB", "BDBBBB", "BBBDDB", "BBLBBB", "BBBBBB"},
		{"BBBBBB", "BLBBDB", "BBBDBB", "BDBBBB", "BBBDLB", "BBBBBB"},
	},
	TexRock: {
		{"BLBBBB", "BBBBDB", "DDBBBB", "BBBDBB", "BBBBBB", "BBLBBB"},
		{"BBBLBB", "DBBBBB", "BBBBDD", "BBBBBB", "BLBBBB", "BBBBDB"},
	},
	// Snow: a clean bright field with a few glinting sparkles (L) and the odd
	// wind-packed drift shadow (D). Interior-only marks keep drifts seamless.
	TexSnow: {
		{"BBBBBB", "BBLBBB", "BBBBBB", "BLBBLB", "BBBBBB", "BBBBBB"},
		{"BBBBBB", "BBBBLB", "BBLBBB", "BBBBBB", "BBDLBB", "BBBBBB"},
		{"BBBBBB", "BLBBBB", "BBBBBB", "BBBLDB", "BBLBBB", "BBBBBB"},
	},
	// Savanna: dry, sun-bleached grass lying flat — short horizontal dashes of
	// parched tussock (DD pairs), a distinct grain from upright lush grass.
	TexSavanna: {
		{"BBBBBB", "BDDBBB", "BBBBDB", "BBBBBB", "BBDDBB", "BBBBBB"},
		{"BBBBBB", "BBBDDB", "BDDBBB", "BBBBBB", "BBBDDB", "BBBBBB"},
		{"BBBBBB", "BBDDBB", "BBBBBB", "BDDBDB", "BBBBBB", "BBBBBB"},
	},
	// Swamp: waterlogged muck — clumped dark mud blotches (D) with the
	// occasional algae sheen (L) catching the light.
	TexSwamp: {
		{"BBBBBB", "BDDBBB", "BDDBBB", "BBBBLB", "BBBBBB", "BBBBBB"},
		{"BBBBBB", "BBBDDB", "BBBDDB", "BLBBBB", "BBBBBB", "BBBBBB"},
		{"BBBBBB", "BBLBBB", "DDBBBB", "DDBBDB", "BBBBDB", "BBBBBB"},
	},
	// Indoor floor: a faint speckle so the surface reads as hand-pixeled.
	TexFloor: {
		{"BBBBBB", "BBBBBB", "BBLBBB", "BBBBBB", "BBBBBL", "BBBBBB"},
		{"BBBBBB", "LBBBBB", "BBBBBB", "BBBLBB", "BBBBBB", "BBBBBB"},
	},
	// Brick wall: staggered courses with darker mortar.
	TexBrick: {
		{"BBBBBD", "BBBBBD", "DDDDDD", "DBBBBB", "DBBBBB", "DDDDDD"},
	},
	// Metal plate: riveted panels for the machine hall.
	TexMetal: {
		{"LBBBBB", "BBBBBB", "BBBBBB", "LBBBBB", "BBBBBB", "BBBBBB"},
		{"BBBBBL", "BBBBBB", "BBBBBB", "BBBBBL", "BBBBBB", "BBBBBB"},
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
	// Cattail reeds: thin vertical stalks with darker seed-heads. Walkable.
	PropReed: {
		"..P.P.",
		".PPPP.",
		".PpPp.",
		".PpPp.",
		".pp.p.",
		"......",
	},
	// A traveler's campfire: an animated flame (G, pulses) over charred logs.
	PropCampfire: {
		"..G...",
		".GGG..",
		"GGWGG.",
		".GGG..",
		"DppD..",
		".DD...",
	},
	// PropHouse is a single-tile cottage: roof (p), walls (P), a dark door (p).
	PropHouse: {
		"..PP..",
		".pppp.",
		"pppppp",
		"PPPPPP",
		"PPppPP",
		"PPppPP",
	},
	// PropHat is a wearable lying on the ground — a little brimmed hat that
	// glints (W) so it reads as special loot, not terrain.
	PropHat: {
		"......",
		"..WP..",
		".PPPP.",
		"PPPPPP",
		".pppp.",
		"......",
	},
	// PropSealed is a broken stone gate arch — a dormant portal. Cracked ring
	// (P stone, p shade) with a dark, empty centre; reads as "sealed" vs the
	// glowing portal swirl it becomes once repaired.
	PropSealed: {
		".pPPp.",
		"pP..Pp",
		"P....P",
		"P.  .P",
		"pp..pp",
		"..pp..",
	},
	// PropPortal is drawn by the multi-tile structure pass, not here.

	// Indoor furniture, drawn with the richer prop palette: o outline, p/D shade,
	// L/W highlight, G animated glow. Authored for the hand-built rooms.
	PropMachine: {
		"oPPPPo",
		"PLPPLP",
		"PPGGPP",
		"PpPPpP",
		"PLLLLP",
		".oPPo.",
	},
	PropScreen: {
		"oPPPPo",
		"PGGGGP",
		"PGGGGP",
		"PGGGGP",
		"oPPPPo",
		"..pp..",
	},
	PropPlinth: {
		".LPPL.",
		"..PP..",
		".oPPo.",
		".PppP.",
		"oPPPPo",
		"pPPPPp",
	},
	// A collectible gem: mostly the item's own color (so a red berry reads red,
	// a purple mushroom purple) with light facets and a single white glint — not
	// the all-white sparkle that used to look like snow.
	PropGem: {
		"..L...",
		".LPL..",
		"LPPPW.",
		".LPPL.",
		"..p...",
		"......",
	},
	PropGemGlow: { // same sprite as PropGem; the glow is added by the light pass
		"..L...",
		".LPL..",
		"LPPPW.",
		".LPPL.",
		"..p...",
		"......",
	},
	PropLamp: {
		"..GG..",
		".GWWG.",
		".GGGG.",
		"..pp..",
		"..PP..",
		".oPPo.",
	},
	PropCrate: {
		"oPPPPo",
		"PLppLP",
		"PpPPpP",
		"PpPPpP",
		"PLppLP",
		"oPPPPo",
	},
	// Reactor core: a white-hot energy orb that pulses.
	PropCore: {
		".oGGo.",
		"oGWWGo",
		"GWWWWG",
		"GWWWWG",
		"oGWWGo",
		".oGGo.",
	},
	// Turbine/generator with a glowing core band.
	PropTurbine: {
		"oPPPPo",
		"PLppLP",
		"PGGGGP",
		"PGGGGP",
		"PLppLP",
		"oPPPPo",
	},
	// Pipe segment with a glowing valve.
	PropPipe: {
		".PPPP.",
		".PppP.",
		".PGGP.",
		".PGGP.",
		".PppP.",
		".PPPP.",
	},
	// Fountain/water feature with a bright glowing basin.
	PropFountain: {
		".oPPo.",
		".PWWP.",
		"oGWWGo",
		"GWWWWG",
		".pGGp.",
		"oPPPPo",
	},
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

// treeArt holds the canopy sprites for forest trees. Unlike single-tile props
// these are taller than one tile and drawn in a back-to-front structure pass so
// they overhang upward and overlap their neighbors — a stand of them reads as a
// continuous canopy rather than a grid of identical stamps. Codes: P canopy
// (the tree's color), p canopy shade, L sun-dapple highlight, T trunk. Variants
// give size/shape variety; the live color (incl. autumn) adds the rest.
var treeArt = [][]string{
	{ // broad oak
		"..ppp..",
		".pPPPp.",
		"pPPPPPp",
		"pPPLPPp",
		"pLPPPPp",
		"pPPPPPp",
		"pPPPPpp",
		".pPPPp.",
		"...T...",
		"...T...",
	},
	{ // young/small tree
		".ppp.",
		"pPPPp",
		"pPLPp",
		"pPPPp",
		".ppp.",
		"..T..",
		"..T..",
	},
	{ // conifer / pine
		"...P...",
		"..ppp..",
		"..PPP..",
		".pPPPp.",
		".PPLPP.",
		"pPPPPPp",
		".PPPPP.",
		"pPPPPPp",
		"PPPPPPP",
		"...T...",
		"...T...",
	},
}

// Signature biome canopies, drawn through the same back-to-front overhang path
// as treeArt (P body, p rim, L dapple, W glint/snow, T trunk). One bold,
// symmetric silhouette each, on the same 6-px grid so they stay retro.
var (
	acaciaArt = []string{ // flat-topped umbrella
		"..PPPPP..",
		".PPPPPPP.",
		"PPPPPPPPP",
		".pp.P.pp.",
		"....T....",
		"....T....",
		"....T....",
	}
	palmArt = []string{ // fronds radiating from a leaning trunk
		"P..P..P",
		".PpPpP.",
		"..PPP..",
		"...T...",
		"...T...",
		"...T...",
		"..T....",
	}
	firArt = []string{ // snow-tipped conifer (W = snow)
		"...W...",
		"...P...",
		"..WPW..",
		"..PPP..",
		".WPPPW.",
		".PPPPP.",
		"WPPPPPW",
		"PPPPPPP",
		"...T...",
		"...T...",
	}
	// A jagged rock spire that overhangs upward, lit on the left (L), body (P),
	// shaded right face (D). No 'p', so the canopy rim-dither leaves it solid —
	// hard rock, not feathered foliage — and it reads as a crag, not a boulder.
	cragArt = []string{
		"...P...",
		"..LPD..",
		"..LPD..",
		".LLPDD.",
		".LLPDD.",
		"LLLPDDD",
		"LLPPDDD",
		"LLPPPDD",
		"LLPPPDD",
	}
)

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
	// Field: plowed earth in straight furrows — full-width dark rows so adjacent
	// field tiles join into continuous plowlines across a farm.
	TexField: {
		{"BBBBBB", "DDDDDD", "BBBBBB", "DDDDDD", "BBBBBB", "DDDDDD"},
		{"DDDDDD", "BBBBBB", "DDDDDD", "BBBBBB", "DDDDDD", "BBBBBB"},
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
	// PropWell is a round stone village well: a light-rimmed stone ring (P/L)
	// around dark water (D), the centrepiece of a settlement.
	PropWell: {
		".PPPP.",
		"PLppLP",
		"PpDDpP",
		"PpDDpP",
		"PLppLP",
		".pPPp.",
	},
	// Ripe grain: a little stand of golden stalks with heavy seed-heads (W),
	// standing in a field — reads as a crop ready to harvest.
	PropCrop: {
		"W.W.W.",
		"PWPWPW",
		"PPPPPP",
		".PPPP.",
		".PPPP.",
		"..pp..",
	},
	// Cut-stone rubble: a little heap of squared blocks at the quarry.
	PropStone: {
		"......",
		".PPp..",
		"PPpp..",
		".PPpP.",
		"PppppP",
		"......",
	},
	// A stack of logs, seen end-on (dark bark rings around pale cores).
	PropLog: {
		"......",
		"DPPDP.",
		"DppDp.",
		"DPPDP.",
		"DppDp.",
		"......",
	},
	// A fish lying by the jetty: body, tail and a glint of an eye (W).
	PropFish: {
		"......",
		"..PPp.",
		".PPPPp",
		"WPPPp.",
		".PPp..",
		"......",
	},
	// Stone curtain wall: a thick coursed-block wall with battlement notches
	// along the top. Reads as solid masonry whichever way it runs.
	PropStoneWall: {
		"P.PP.P",
		"PPPPPP",
		"PppPpP",
		"PPPPPP",
		"PppPpP",
		"PPPPPP",
	},
	// Palisade rails: rough timber stakes. The horizontal run shows a long rail
	// across two posts; the vertical run shows stakes along the line of travel;
	// the post is a stout corner/junction upright. Autotiling picks between them.
	PropFenceH: {
		"......",
		"P....P",
		"PPPPPP",
		"pPPPPp",
		"P....P",
		"P....P",
	},
	PropFenceV: {
		".P..P.",
		".P..P.",
		".PppP.",
		".PppP.",
		".P..P.",
		".P..P.",
	},
	PropFencePost: {
		"PP..PP",
		"PP..PP",
		"PPppPP",
		"PPppPP",
		"PP..PP",
		"PP..PP",
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

// buildingArt holds the multi-tile sprites for village buildings, generated once
// from their footprint. Each is (h·6) rows × (w·6) cols of art-pixels, drawn
// bottom-left-anchored so it rises up (north) and extends right (east) from its
// base tile. Codes: P wall, p roof, D base/door shade, L window, R trim/cross.
// Each building type has a few variants (different roofs, window rows, door
// placement, half-timbering, steeple offset). The renderer picks one per
// instance by world position, so a street isn't lined with identical copies.
var buildingArt = map[TileProp][][]string{
	PropBldCottage:    bldVariants(1, 1, bldDwelling),
	PropBldHouse:      bldVariants(2, 2, bldDwelling),
	PropBldLonghouse:  bldVariants(3, 2, bldDwelling),
	PropBldBarn:       bldVariants(2, 2, bldBarn),
	PropBldChurch:     bldVariants(2, 3, bldChurch),
	PropBldCathedral:  bldVariants(3, 4, bldChurch),
	PropBldTownhouse:  bldVariants(2, 3, bldDwelling),
	PropBldMarketHall: bldVariants(3, 3, bldDwelling),
	PropBldKeep:       {genKeep(3, 3)},
	PropTower:         {towerArt},
}

// genKeep builds a castle keep: a solid stone block with a flat battlemented
// crown (merlons), corner turrets a touch taller, arrow slits and a gate.
func genKeep(wt, ht int) []string {
	w, h := wt*tileArtN, ht*tileArtN
	g := make([][]byte, h)
	for y := range g {
		g[y] = make([]byte, w)
		for x := range g[y] {
			g[y][x] = 'P'
		}
	}
	for x := 0; x < w; x++ {
		g[h-1][x] = 'D' // base course
		if x%2 == 1 {
			g[0][x] = '.' // battlement gaps (merlons between)
		} else {
			g[0][x] = 'L' // lit merlon caps
		}
	}
	for y := 2; y < h-2; y += 3 { // arrow slits
		for x := 2; x < w-2; x += 4 {
			g[y][x] = 'D'
		}
	}
	dcx := w / 2 // a gate at the base
	for y := h - 3; y < h; y++ {
		for x := dcx - 1; x <= dcx+1; x++ {
			g[y][x] = 'D'
		}
	}
	out := make([]string, h)
	for y := range g {
		out[y] = string(g[y])
	}
	return out
}

// towerArt is a stone wall tower: one tile wide, two tall, with a battlemented
// crown and an arrow slit, drawn bottom-anchored so it rises above the wall.
var towerArt = []string{
	"P.PP.P",
	"PPPPPP",
	"PPPPPP",
	"PPDDPP",
	"PPPPPP",
	"PpPPpP",
	"PPPPPP",
	"PpPPpP",
	"PPPPPP",
	"PpPPpP",
	"PPPPPP",
	"DDDDDD",
}

type bldKind uint8

const (
	bldDwelling bldKind = iota
	bldBarn
	bldChurch
)

func bldVariants(wt, ht int, k bldKind) [][]string {
	return [][]string{
		genBuilding(wt, ht, k, 0),
		genBuilding(wt, ht, k, 1),
		genBuilding(wt, ht, k, 2),
	}
}

func genBuilding(wt, ht int, k bldKind, style int) []string {
	w, h := wt*tileArtN, ht*tileArtN
	g := make([][]byte, h)
	for y := range g {
		g[y] = make([]byte, w)
		for x := range g[y] {
			g[y][x] = '.'
		}
	}
	roofH := h * []int{42, 50, 38}[style%3] / 100
	if roofH < 2 {
		roofH = 2
	}
	wallTop, baseY := roofH, h-1
	for y := wallTop; y <= baseY; y++ {
		for x := 0; x < w; x++ {
			g[y][x] = 'P'
		}
	}
	for x := 0; x < w; x++ {
		g[baseY][x] = 'D' // base course
	}
	// Roof: a pitched ridge, or (style 1) a hipped roof with a flat-ish top.
	topInset := 0
	if style == 1 {
		topInset = w / 4
	}
	for ry := 0; ry < roofH; ry++ {
		m := (roofH - 1 - ry) * (w/2 - topInset) / roofH
		for x := m; x < w-m; x++ {
			g[ry][x] = 'p'
		}
	}
	// Half-timbering (style 2 dwellings): dark vertical studs in the walls.
	if style == 2 && k == bldDwelling {
		for y := wallTop + 1; y < baseY; y++ {
			for x := 0; x < w; x++ {
				if x%4 == 1 {
					g[y][x] = 'D'
				}
			}
		}
	}
	// Windows: one or two rows, phase shifted per style.
	for r := 0; r < []int{1, 2, 1}[style%3]; r++ {
		wy := wallTop + 1 + r*2
		if wy >= baseY {
			break
		}
		for x := 1; x < w-1; x++ {
			if (x+style)%3 == 1 {
				g[wy][x] = 'L'
			}
		}
	}
	// Door, offset per style.
	dcx := w/2 + []int{0, -w / 6, w / 6}[style%3]
	dw := w / 4
	if dw < 1 {
		dw = 1
	}
	for y := baseY - max(1, (baseY-wallTop)/2); y <= baseY; y++ {
		for x := dcx - dw/2; x <= dcx+dw/2; x++ {
			if x >= 0 && x < w {
				g[y][x] = 'D'
			}
		}
	}
	if k == bldBarn { // big central wagon doors
		for y := wallTop + (baseY-wallTop)/3; y <= baseY; y++ {
			for x := w / 4; x < w-w/4; x++ {
				g[y][x] = 'D'
			}
		}
	}
	if k == bldChurch { // a steeple rising above the roof, topped with a cross
		towW := tileArtN/2 + style%2
		tx0 := w/2 - towW/2 + []int{0, -1, 1}[style%3]
		if tx0 < 0 {
			tx0 = 0
		}
		for y := 0; y < wallTop; y++ {
			for x := tx0; x < tx0+towW && x < w; x++ {
				g[y][x] = 'P'
			}
		}
		cx := tx0 + towW/2
		if cx >= w {
			cx = w - 1
		}
		g[0][cx], g[1][cx], g[2][cx] = 'R', 'R', 'R'
		if cx-1 >= 0 {
			g[1][cx-1] = 'R'
		}
		if cx+1 < w {
			g[1][cx+1] = 'R'
		}
	}
	out := make([]string, h)
	for y := range g {
		out[y] = string(g[y])
	}
	return out
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

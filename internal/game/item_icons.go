package game

import (
	"image"
	"image/color"
)

// Per-item pixel-art icons for the compendium. In the world a forage find is a
// small recolored gem (it has to read at one tile); the compendium has room for
// a proper little portrait, so each collectible gets its own sprite here. They
// share the prop-art palette idea — a base color shaded into light/dark — but
// add fixed leaf/stem/bone tones so a berry can have a green leaf and a mushroom
// a pale stalk regardless of the find's own color.
//
// Codes (everything else is transparent):
//
//	#  the item's own color        +  lit             *  bright glint
//	-  shade                       =  dark shade      o  outline
//	g  leaf green   G  leaf light  s  stem brown      w  bone/white
const itemIconN = 12

var itemIcons = map[string][]string{
	"berry": {
		"......g.....",
		".....gGs....",
		"......g.....",
		"...oo.oo....",
		"..o++#-o....",
		".o+####-o...",
		".o*####=o...",
		".o#####=o...",
		".o-####=o...",
		"..o-##=o....",
		"...oo=o.....",
		"....oo......",
	},
	"herb": {
		"....+.+.....",
		"...+#oo#+...",
		"..+##oo##+..",
		"..o#oooo#o..",
		"..+##oo##+..",
		"...+#oo#+...",
		"....o##o....",
		".....ss.....",
		".....ss.....",
		".....ss.....",
		".....ss.....",
		"............",
	},
	"mushroom": {
		"...oooooo...",
		"..o++##--o..",
		".o+##ww##-o.",
		"o+##w##w##=o",
		"o*########=o",
		"o##ww####=-o",
		".o########o.",
		"..oWWWWWWo..",
		"....w##w....",
		"....w##w....",
		"...ow##wo...",
		"....oooo....",
	},
	"shell": {
		"...o...o....",
		"..o#o.o#o...",
		".o#+#o#+#o..",
		"o#+#+#+#+#o.",
		"o+#+#+#+#+o.",
		"o#+#+#+#+#o.",
		".o+#+#+#+o..",
		".o-#####-o..",
		"..o-###-o...",
		"...o#=#o....",
		"....o#o.....",
		".....o......",
	},
	"crystal": {
		".....*......",
		"....+#-.....",
		"...++#=.....",
		"..++##=.....",
		".++###=..*..",
		"o+####=.+#-.",
		"o*####=o+#=.",
		"o+####=o##=.",
		"o-###==o#==.",
		".o-##=oo#=o.",
		"..o-=o.oo o.",
		"...ooo......",
	},
	"nugget": {
		"............",
		"....oooo....",
		"..oo+##-oo..",
		".o++#*##-=o.",
		"o++####-#=-o",
		"o#*####=##=o",
		"o-#####-#=-o",
		".o-####=#=o.",
		"..oo-##=oo..",
		"....oooo....",
		"............",
		"............",
	},
	"grain": {
		"...*..*..*..",
		"..+#-+#-+#-.",
		"..o#-o#-o#-.",
		"..+#-+#-+#-.",
		"..o#-o#-o#-.",
		"...s..s..s..",
		"...ss.ss.s..",
		"....ssss....",
		".....ss.....",
		".....ss.....",
		".....ss.....",
		"............",
	},
	"stone": {
		"............",
		"..oooooo....",
		".o++##-=o...",
		".o+###-=o...",
		".oooooooo...",
		"..o++##-=o..",
		"..o+###-=o..",
		"..o-###==o..",
		"..oooooooo..",
		"............",
		"............",
		"............",
	},
	"wood": {
		"............",
		"..oooooooo..",
		".o=o==o==o=.",
		".o#wo#wo#wo.",
		".o=o==o==o=.",
		".o#wo#wo#wo.",
		".o=o==o==o=.",
		"..oooooooo..",
		"............",
		"............",
		"............",
		"............",
	},
	"fish": {
		"............",
		"............",
		"....oooo..o.",
		"..o++##-o#o.",
		".o+#####o#=o",
		"o+#w#####=#o",
		"o++####o#=#o",
		".o-####o#=o.",
		"..o-##-o#o..",
		"....oooo.o..",
		"............",
		"............",
	},
	"geode": {
		"....oooo....",
		"..oo=--=oo..",
		".o==-++-==o.",
		"o=-+#**#+-=o",
		"o=+#*##*#+=o",
		"o=+##**##+=o",
		"o=-+#**#+-=o",
		".o==-++-==o.",
		"..oo=--=oo..",
		"....oooo....",
		"............",
		"............",
	},
	"relic": {
		".....oo.....",
		"....o++o....",
		"....o##o....",
		"...o#++#o...",
		"..o#+##+#o..",
		".o#+#**#+#o.",
		".o#+####+#o.",
		".o+######+o.",
		".o-######-o.",
		"..o-####-o..",
		"...oo--oo...",
		"....oooo....",
	},
	"spore": {
		".....*......",
		"....+#+.....",
		"...++#++....",
		"..++#*#++...",
		".o++###++o..",
		".oo#####oo..",
		"..oo+++oo...",
		"....w##w....",
		"....w##w....",
		"...ow##wo...",
		"....oooo....",
		"............",
	},
	"amber": {
		".....oo.....",
		"....o++-o...",
		"...o++#*-o..",
		"..o++###-o..",
		"..o+####-o..",
		".o++#**##-o.",
		".o+##**##=o.",
		".o+######=o.",
		".o-#####=o..",
		"..o-###=o...",
		"...oo==oo...",
		"....oooo....",
	},
	// Crafted goods — a stack of boards, a flour sack, a gold bar, a salve pot,
	// and a glowing lamp. Made at a workbench/machine, never foraged.
	"plank": {
		"............",
		"............",
		".oooooooooo.",
		".o++####+#o.",
		".o-######=o.",
		".oooooooooo.",
		".o++####+#o.",
		".o-######=o.",
		".oooooooooo.",
		".o++####+#o.",
		".o-######=o.",
		".oooooooooo.",
	},
	"flour": {
		"....oooo....",
		"...o=##=o...",
		"...o+##+o...",
		"..o######o..",
		".o++####+o..",
		".o##*##*#o..",
		".o######=o..",
		".o##****#o..",
		".o######=o..",
		".o-####==o..",
		"..oo====oo..",
		"...oooooo...",
	},
	"ingot": {
		"............",
		"............",
		"............",
		"....oooo....",
		"...o+##+o...",
		"..o++##+#o..",
		".o*######=o.",
		".o########o.",
		".o-######=o.",
		"..oooooooo..",
		"............",
		"............",
	},
	"salve": {
		"............",
		"....oooo....",
		"...o====o...",
		"...o+##+o...",
		"..oo####oo..",
		".o++####+#o.",
		".o##*###=#o.",
		".o######=#o.",
		".o######=#o.",
		".o-#####==o.",
		"..oooooooo..",
		"............",
	},
	"lamp": {
		".....oo.....",
		"....o++o....",
		"...o####o...",
		"..o##**##o..",
		".o##****##o.",
		".o#******#o.",
		".o##****##o.",
		"..o##**##o..",
		"...o+##o....",
		"....o#o.....",
		"...oo#oo....",
		"..oooooooo..",
	},
	"meat": {
		"............",
		".......ww...",
		"......w==w..",
		"...ooo.w.w..",
		"..o++#oo....",
		".o+###-o....",
		".o*###=o....",
		".o####=o....",
		".o-##==o....",
		"..o-#=o.....",
		"...ooo......",
		"............",
	},
	"hide": {
		"............",
		"....oooo....",
		"...o++##o...",
		"..o+####-o..",
		".o+######-o.",
		"o+########=o",
		"o-########=o",
		".o-######=o.",
		"..o-####=o..",
		"...o-##=o...",
		"....oooo....",
		"............",
	},
	"pelt": {
		"............",
		"...oooo.....",
		"..o++##o....",
		".o+####-o...",
		".o######=o..",
		".o######=o..",
		".o-####=o...",
		"..o-##=o....",
		"...o##o.....",
		"....o#o.....",
		".....o#o....",
		"......oo....",
	},
	"feather": {
		"......o.....",
		".....o#o....",
		"....o+#-o...",
		"...o+##-o...",
		"...o+##=o...",
		"...o+##=o...",
		"....o##=o...",
		".....s#o....",
		".....s......",
		".....s......",
		".....s......",
		".....s......",
	},
}

// itemIconPalette maps the icon codes to concrete colors for one item: its own
// hue shaded into light and dark, plus the fixed leaf/stem/bone tones.
func itemIconPalette(it Item) map[byte]color.RGBA {
	main := mustHex(it.Hex)
	return map[byte]color.RGBA{
		'#': colorfulToRGBA(main),
		'+': colorfulToRGBA(main.BlendLab(spriteWhite, 0.42).Clamped()),
		'*': colorfulToRGBA(main.BlendLab(spriteWhite, 0.80).Clamped()),
		'-': colorfulToRGBA(main.BlendLab(shadowColor, 0.38).Clamped()),
		'=': colorfulToRGBA(main.BlendLab(shadowColor, 0.60).Clamped()),
		'o': colorfulToRGBA(main.BlendLab(shadowColor, 0.80).Clamped()),
		'W': colorfulToRGBA(main.BlendLab(spriteWhite, 0.62).Clamped()),
		'g': colorfulToRGBA(mustHex("#5BA85A")),
		'G': colorfulToRGBA(mustHex("#86D98C")),
		's': colorfulToRGBA(mustHex("#8C6A3C")),
		'w': colorfulToRGBA(mustHex("#ECEFF4")),
	}
}

// drawItemIcon renders an item's compendium portrait, cropped to its pixels and
// centered in a box-sized cell. Items without a bespoke icon fall back to the
// recolored gem the world uses, so a new collectible still shows something.
func drawItemIcon(img *image.RGBA, x, y, box int, it Item) {
	art, ok := itemIcons[it.ID]
	if !ok {
		c := mustHex(it.Hex)
		drawGem(img, x, y, box, colorfulToRGBA(c),
			colorfulToRGBA(c.BlendLab(spriteWhite, 0.55).Clamped()))
		return
	}
	minX, minY, maxX, maxY := 1<<30, 1<<30, -1, -1
	for r, row := range art {
		for c, ch := range row {
			if ch != ' ' && ch != '.' {
				minX, maxX = min(minX, c), max(maxX, c)
				minY, maxY = min(minY, r), max(maxY, r)
			}
		}
	}
	if maxX < 0 {
		return
	}
	bw, bh := maxX-minX+1, maxY-minY+1
	sc := min(box/bw, box/bh)
	if sc < 1 {
		sc = 1
	}
	pal := itemIconPalette(it)
	offX := x + (box-bw*sc)/2
	offY := y + (box-bh*sc)/2
	for r := minY; r <= maxY; r++ {
		row := []byte(art[r])
		for c := minX; c <= maxX && c < len(row); c++ {
			if col, ok := pal[row[c]]; ok {
				fillRect(img, offX+(c-minX)*sc, offY+(r-minY)*sc, sc, sc, col)
			}
		}
	}
}

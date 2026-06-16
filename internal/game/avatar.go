package game

import (
	"time"

	"github.com/durst-group/durstworld/internal/world"
)

// Avatars are 12×12 pixel sprites with 8-way facing, a 2-frame walk cycle, a
// few body styles and optional accessories. The same sprite data drives both
// renderers: the HD pixel renderer (raster.go) draws them crisp (sharp,
// nearest-neighbor), and the half-block glyph renderer (sprite.go) downsamples
// them to fit the cell grid. Each renderer turns the rune codes into colors via
// spritePixel.
//
// Codes: B body, L light shade, D dark shade, E eye, W eye highlight, m mouth,
// H/h accessory, '.' / ' ' transparent. The side view faces right and is
// mirrored for left, so 8 facings render from three authored views.

type avatarView int

const (
	vFront avatarView = iota // facing the camera (down)
	vBack                    // facing away (up)
	vSide                    // facing right (mirrored for left)
)

type avatarStyle struct {
	Name  string
	front [2][]string // [walk frame]
	back  [2][]string
	side  [2][]string
}

// avatarStyles are the selectable body shapes. Index 0 ("claude") is the
// default: a rounded, warm-coral critter inspired by the Claude mark.
var avatarStyles = []avatarStyle{
	{
		Name: "claude",
		front: [2][]string{
			{
				"....LLLL....",
				"..LLLLLLLL..",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBWEBBBBWEBB",
				"BBEEBBBBEEBB",
				"BBBBBBBBBBBB",
				"BBBBmmmmBBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"..D......D..",
			},
			{
				"....LLLL....",
				"..LLLLLLLL..",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBWEBBBBWEBB",
				"BBEEBBBBEEBB",
				"BBBBBBBBBBBB",
				"BBBBmmmmBBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"...D....D...",
			},
		},
		back: [2][]string{
			{
				"....LLLL....",
				"..LLLLLLLL..",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"..D......D..",
			},
			{
				"....LLLL....",
				"..LLLLLLLL..",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"...D....D...",
			},
		},
		side: [2][]string{
			{
				"....LLLL....",
				"..LLLLLLLL..",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBBBBBBWEBBB",
				"BBBBBBBEEBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBmmBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"..D......D..",
			},
			{
				"....LLLL....",
				"..LLLLLLLL..",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBBBBBBWEBBB",
				"BBBBBBBEEBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBmmBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"...D....D...",
			},
		},
	},
	{
		Name: "cat",
		front: [2][]string{
			{
				"..LL....LL..",
				".LLLL..LLLL.",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBWEBBBBWEBB",
				"BBEEBBBBEEBB",
				"BBBBBBBBBBBB",
				"BBBBmmmmBBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"..D......D..",
			},
			{
				"..LL....LL..",
				".LLLL..LLLL.",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBWEBBBBWEBB",
				"BBEEBBBBEEBB",
				"BBBBBBBBBBBB",
				"BBBBmmmmBBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"...D....D...",
			},
		},
		back: [2][]string{
			{
				"..LL....LL..",
				".LLLL..LLLL.",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"..D......D..",
			},
			{
				"..LL....LL..",
				".LLLL..LLLL.",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"...D....D...",
			},
		},
		side: [2][]string{
			{
				"..LL....LL..",
				".LLLL..LLLL.",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBBBBBBWEBBB",
				"BBBBBBBEEBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBmmBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"..D......D..",
			},
			{
				"..LL....LL..",
				".LLLL..LLLL.",
				".LLLLLLLLLL.",
				".LBBBBBBBBL.",
				"BBBBBBBBBBBB",
				"BBBBBBBWEBBB",
				"BBBBBBBEEBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBmmBBB",
				"BDBBBBBBBBDB",
				".DDBBBBBBDD.",
				"...D....D...",
			},
		},
	},
	{
		Name: "bot",
		front: [2][]string{
			{
				".....LL.....",
				".....BB.....",
				"LLLLLLLLLLLL",
				"LBBBBBBBBBBL",
				"BBBBBBBBBBBB",
				"BEEBBBBBBEEB",
				"BEEBBBBBBEEB",
				"BBBBBBBBBBBB",
				"BBBBmmmmBBBB",
				"BBBBBBBBBBBB",
				"DDDBBBBBBDDD",
				"DD......DD..",
			},
			{
				".....LL.....",
				".....BB.....",
				"LLLLLLLLLLLL",
				"LBBBBBBBBBBL",
				"BBBBBBBBBBBB",
				"BEEBBBBBBEEB",
				"BEEBBBBBBEEB",
				"BBBBBBBBBBBB",
				"BBBBmmmmBBBB",
				"BBBBBBBBBBBB",
				"DDDBBBBBBDDD",
				".DD......DD.",
			},
		},
		back: [2][]string{
			{
				".....LL.....",
				".....BB.....",
				"LLLLLLLLLLLL",
				"LBBBBBBBBBBL",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"DDDBBBBBBDDD",
				"DD......DD..",
			},
			{
				".....LL.....",
				".....BB.....",
				"LLLLLLLLLLLL",
				"LBBBBBBBBBBL",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"BBBBBBBBBBBB",
				"DDDBBBBBBDDD",
				".DD......DD.",
			},
		},
		side: [2][]string{
			{
				".....LL.....",
				".....BB.....",
				"LLLLLLLLLLLL",
				"LBBBBBBBBBBL",
				"BBBBBBBBBBBB",
				"BBBBBBBBEEBB",
				"BBBBBBBBEEBB",
				"BBBBBBBBBBBB",
				"BBBBBBBmmBBB",
				"BBBBBBBBBBBB",
				"DDDBBBBBBDDD",
				"DD......DD..",
			},
			{
				".....LL.....",
				".....BB.....",
				"LLLLLLLLLLLL",
				"LBBBBBBBBBBL",
				"BBBBBBBBBBBB",
				"BBBBBBBBEEBB",
				"BBBBBBBBEEBB",
				"BBBBBBBBBBBB",
				"BBBBBBBmmBBB",
				"BBBBBBBBBBBB",
				"DDDBBBBBBDDD",
				".DD......DD.",
			},
		},
	},
}

// accessories overlay the head's top rows (H accessory, H shade h). Index 0 is
// "none". Overlays are 12 wide; spaces leave the underlying pixel untouched.
var accessories = []struct {
	Name    string
	overlay []string
}{
	{"none", nil},
	{"cap", []string{
		"            ",
		"..HHHHHHHH..",
		".hhhhhhhhhh.",
	}},
	{"crown", []string{
		"H..HH..HH..H",
		".HHHHHHHHHH.",
	}},
	{"band", []string{
		"            ",
		"            ",
		".HHHHHHHHHH.",
	}},
}

// NumAvatarStyles / NumAccessories and the name lookups back the /avatar command.
func NumAvatarStyles() int         { return len(avatarStyles) }
func NumAccessories() int          { return len(accessories) }
func AvatarStyleName(i int) string { return avatarStyles[wrapIdx(i, len(avatarStyles))].Name }
func AccessoryName(i int) string   { return accessories[wrapIdx(i, len(accessories))].Name }

// facingView maps an 8-way facing to an authored view plus whether to mirror.
// Any eastward component shows the right profile; westward, the mirrored one;
// pure vertical shows front/back.
func facingView(d world.Dir) (v avatarView, mirror bool) {
	switch d {
	case world.DirN:
		return vBack, false
	case world.DirNE, world.DirE, world.DirSE:
		return vSide, false
	case world.DirNW, world.DirW, world.DirSW:
		return vSide, true
	default: // DirS
		return vFront, false
	}
}

// AvatarBitmap returns the sprite rows for a player's customization, facing and
// walk frame, with the accessory overlaid and mirrored as needed.
func AvatarBitmap(style, accessory int, facing world.Dir, walkFrame int) []string {
	st := avatarStyles[wrapIdx(style, len(avatarStyles))]
	v, mirror := facingView(facing)
	var frames [2][]string
	switch v {
	case vBack:
		frames = st.back
	case vSide:
		frames = st.side
	default:
		frames = st.front
	}
	rows := append([]string(nil), frames[walkFrame%2]...)
	rows = overlayAccessory(rows, accessory)
	if mirror {
		rows = mirrorRows(rows)
	}
	return rows
}

// AvatarWalkFrame picks the walk-cycle frame for a player at a global tick:
// alternating while they moved recently, the neutral stance when idle.
func AvatarWalkFrame(lastMoved time.Time, frame int) int {
	if time.Since(lastMoved) > 300*time.Millisecond {
		return 0
	}
	return (frame / 3) % 2
}

func overlayAccessory(rows []string, accessory int) []string {
	ov := accessories[wrapIdx(accessory, len(accessories))].overlay
	if len(ov) == 0 {
		return rows
	}
	out := make([]string, len(rows))
	copy(out, rows)
	for r := 0; r < len(ov) && r < len(out); r++ {
		dst := []rune(out[r])
		src := []rune(ov[r])
		for c := 0; c < len(src) && c < len(dst); c++ {
			if src[c] != ' ' {
				dst[c] = src[c]
			}
		}
		out[r] = string(dst)
	}
	return out
}

func mirrorRows(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		rs := []rune(r)
		for l, h := 0, len(rs)-1; l < h; l, h = l+1, h-1 {
			rs[l], rs[h] = rs[h], rs[l]
		}
		out[i] = string(rs)
	}
	return out
}

func wrapIdx(i, n int) int {
	if n <= 0 {
		return 0
	}
	return ((i % n) + n) % n
}

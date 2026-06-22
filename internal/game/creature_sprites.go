package game

import (
	"time"

	"github.com/durst-group/durstworld/internal/world"
)

// Creature sprites: animals get the same treatment as avatars — three authored
// views (front = toward camera, back = away, side = right profile, mirrored for
// left) each with two walk frames, so all eight facings render from a handful of
// poses. The HD renderer (raster.go) blits them; the glyph client shows the
// species letter instead. Codes match creaturePixel:
//
//	P body   p shade   D dark   o outline   L light   W highlight   e eye   n nose
//
// All sprites are 12 wide × 10 tall (validated by TestCreatureSpritesWellFormed),
// bottom-aligned on the tile so feet sit on the ground.
type creatureSprite struct {
	front [2][]string
	back  [2][]string
	side  [2][]string
}

// dup makes a [2] of one pose (a view that doesn't animate its own pose; the
// renderer's bob still gives it motion while walking).
func dup(rows []string) [2][]string { return [2][]string{rows, rows} }

var creatureSprites = map[string]creatureSprite{
	"rabbit": {
		side: [2][]string{
			{
				".......o....",
				"......oPo...",
				".o....oPo...",
				"oPo...oPo...",
				"oPo..oPPo...",
				"oPPooPPPo...",
				".oPPPPPPLo..",
				"..oPPPPePo..",
				"..oPpppPPo..",
				"...oo..ooo..",
			},
			{
				".......o....",
				"......oPo...",
				".o....oPo...",
				"oPo...oPo...",
				"oPo..oPPo...",
				"oPPooPPPo...",
				".oPPPPPPLo..",
				"..oPPPPePo..",
				"..oPpppPPo..",
				"..oo..oo.o..",
			},
		},
		front: dup([]string{
			"....o..o....",
			"...oLooLo...",
			"...oPooPo...",
			"..oPPPPPPo..",
			"..oPePPePo..",
			"..oPPnnPPo..",
			"..oPPPPPPo..",
			"...oPPPPo...",
			"...D.DD.D...",
			"............",
		}),
		back: dup([]string{
			"....o..o....",
			"...oLooLo...",
			"...oPooPo...",
			"..oPPPPPPo..",
			"..oPPPPPPo..",
			"..oPpppPPo..",
			"..oPPPPPPo..",
			"...oPWWPo...",
			"...D.DD.D...",
			"............",
		}),
	},
	"deer": {
		side: [2][]string{
			{
				"..o......o..",
				".oLo....oLo.",
				"o.Lo.o.oL.o.",
				".oLooLooLo..",
				"..ooPPPoo...",
				"....oPPo....",
				"..ooPPPPoo..",
				".oPPPPPPPPo.",
				".oPepPPPnPo.",
				".o.oo..oo.o.",
			},
			{
				"..o......o..",
				".oLo....oLo.",
				"o.Lo.o.oL.o.",
				".oLooLooLo..",
				"..ooPPPoo...",
				"....oPPo....",
				"..ooPPPPoo..",
				".oPPPPPPPPo.",
				".oPepPPPnPo.",
				".oo.o..o.oo.",
			},
		},
		front: dup([]string{
			"o.o....o.o..",
			".oLo...oLo..",
			"..ooLooLoo..",
			"...oPPPPo...",
			"..oPePPePo..",
			"..oPPnnPPo..",
			"..oPPPPPPo..",
			"...oPPPPo...",
			"...D.DD.D...",
			"............",
		}),
		back: dup([]string{
			"o.o....o.o..",
			".oLo...oLo..",
			"..ooooooo...",
			"...oPPPPo...",
			"..oPPPPPPo..",
			"..oPpppPPo..",
			"..oPPWPPPo..",
			"...oPPPPo...",
			"...D.DD.D...",
			"............",
		}),
	},
	"fox": {
		side: [2][]string{
			{
				"............",
				".o........o.",
				"oLo......oLo",
				"oLPo....oPLo",
				".oPPoooooPPo",
				"..oPPPPPPPPW",
				"..oPePPPPpPo",
				"..oPPPPPPpo.",
				"...oo.oo.oo.",
				"............",
			},
			{
				"............",
				".o........o.",
				"oLo......oLo",
				"oLPo....oPLo",
				".oPPoooooPPo",
				"..oPPPPPPPPW",
				"..oPePPPPpPo",
				"..oPPPPPPpo.",
				"..oo.oo.oo..",
				"............",
			},
		},
		front: dup([]string{
			"..o......o..",
			".oLo....oLo.",
			".oPo....oPo.",
			"..oPPPPPPo..",
			"..oPePPePo..",
			"..oPPnnPPo..",
			"..oPPPPPPo..",
			"...oPnnPo...",
			"...D.DD.D...",
			"............",
		}),
		back: dup([]string{
			"..o......o..",
			".oLo....oLo.",
			".oPo....oPo.",
			"..oPPPPPPo..",
			"..oPPPPPPoWo",
			"..oPPPPPPWWo",
			"..oPpppPPWo.",
			"...oPPPPo...",
			"...D.DD.D...",
			"............",
		}),
	},
	"bird": {
		side: [2][]string{
			{
				"............",
				"............",
				".......oo...",
				"......oPPo..",
				"..oooPPPePo.",
				".oPPPPPPLn..",
				".oPpppPPo...",
				"...oo.npo...",
				"......oo....",
				"............",
			},
			{
				"............",
				".....ooo....",
				"....oPPPo...",
				"...oPPPPo...",
				"..oooPPPePo.",
				".oPPPPPPLn..",
				".oPPPPPPo...",
				"...oo.npo...",
				"......oo....",
				"............",
			},
		},
		front: dup([]string{
			"............",
			"............",
			"....oooo....",
			"...oLPPLo...",
			"..oPePPePo..",
			"..oPPnnPPo..",
			"..oPPPPPPo..",
			"...oPPPPo...",
			"...D..D.....",
			"............",
		}),
		back: dup([]string{
			"............",
			"............",
			"....oooo....",
			"...oPPPPo...",
			"..oPPPPPPo..",
			"..oPpppPPo..",
			"..oPPPPPPo..",
			"...oPPPPo...",
			"...D..D.....",
			"............",
		}),
	},
	"fish": {
		// A fish only ever reads as a side profile, so every facing uses it.
		side: [2][]string{
			{
				"............",
				"............",
				"...oooo...o.",
				".ooPPPPoooLo",
				"oPPLPPPPPoLo",
				"oPePPPPPPPLo",
				"oPPLPPPPPoLo",
				".ooPPPPoooLo",
				"...oooo...o.",
				"............",
			},
			{
				"............",
				"............",
				"...oooo..oo.",
				".ooPPPPooLLo",
				"oPPLPPPPPLLo",
				"oPePPPPPPPLo",
				"oPPLPPPPPLLo",
				".ooPPPPooLLo",
				"...oooo..oo.",
				"............",
			},
		},
	},
}

func init() {
	// Fish use the side profile for every facing.
	f := creatureSprites["fish"]
	f.front, f.back = f.side, f.side
	creatureSprites["fish"] = f
}

// CreatureBitmap returns a species' sprite rows for a facing and walk frame,
// mirrored as needed. ok is false for an unknown species.
func CreatureBitmap(kind string, facing world.Dir, walkFrame int) ([]string, bool) {
	cs, ok := creatureSprites[kind]
	if !ok {
		return nil, false
	}
	v, mirror := facingView(facing)
	var frames [2][]string
	switch v {
	case vBack:
		frames = cs.back
	case vSide:
		frames = cs.side
	default:
		frames = cs.front
	}
	rows := frames[walkFrame%2]
	if mirror {
		rows = mirrorRows(rows)
	}
	return rows, true
}

// creatureMoveWindow is how long after its last step an animal still reads as
// "walking" (brisk pose cycle + bob); after that it idles with a slow sway.
const creatureMoveWindow = 600 * time.Millisecond

// CreatureWalkFrame picks the pose frame and reports whether the animal is
// actively moving (so the renderer can add a walking bob). Idle animals sway
// slowly between poses; movers cycle briskly.
func CreatureWalkFrame(lastMoved time.Time, frame int) (wf int, moving bool) {
	if time.Since(lastMoved) <= creatureMoveWindow {
		return (frame / 3) % 2, true
	}
	return (frame / 10) % 2, false
}

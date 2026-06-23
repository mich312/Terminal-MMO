package game

import (
	"time"

	"github.com/durst-group/durstworld/internal/world"
)

// Creature sprites: animals get the same treatment as avatars — three authored
// views (front = toward camera, back = away, side = right profile, mirrored for
// left), each with two frames. Frame 0 is the resting pose; frame 1 is a small
// characterful change (an ear-twitch, a wing-flap, a tail-swish, a leg-step)
// that doubles as the walk pose (fast cycle + bob while moving) and the idle
// flourish (an occasional flick while still). All eight facings render from
// these via facingView + mirror. The HD renderer (raster.go) blits them; the
// glyph client shows the species letter instead. Codes match creaturePixel:
//
//	P body   p shade   D dark/leg   o outline   L light   W highlight/tail
//	e eye    n nose/beak   . clear
//
// Every sprite is 12 wide × 10 tall (TestCreatureSpritesWellFormed).
type creatureSprite struct {
	front [2][]string
	back  [2][]string
	side  [2][]string
}

var creatureSprites = map[string]creatureSprite{
	"rabbit": {
		side: [2][]string{
			{
				"......oo....",
				".....oPo....",
				".....oPo....",
				".....oPPo...",
				"....oPPPPo..",
				"...oPPPPePo.",
				"..oPPPPPPno.",
				"..oPPPPPPpo.",
				"..oPpppppo..",
				"...DD..DD...",
			},
			{ // ear flicks forward, hind foot kicks back
				"............",
				".....ooo....",
				".....oPPo...",
				".....oPPo...",
				"....oPPPPo..",
				"...oPPPPePo.",
				"..oPPPPPPno.",
				"..oPPPPPPpo.",
				"..oPpppppo..",
				"...D.DD.D...",
			},
		},
		front: [2][]string{
			{
				"...P....P...",
				"..oPo..oPo..",
				"..oPo..oPo..",
				"..oPPPPPPo..",
				".oPPePPePPo.",
				".oPPPnnPPPo.",
				".oPPPPPPPPo.",
				"..oPPPPPPo..",
				"..oPppppPo..",
				"...DD..DD...",
			},
			{ // one ear flicks aside, feet shift
				"...P....P...",
				"..oPo...Po..",
				"..oPo..oPo..",
				"..oPPPPPPo..",
				".oPPePPePPo.",
				".oPPPnnPPPo.",
				".oPPPPPPPPo.",
				"..oPPPPPPo..",
				"..oPppppPo..",
				"...D.DD.D...",
			},
		},
		back: [2][]string{
			{
				"...P....P...",
				"..oPo..oPo..",
				"..oPo..oPo..",
				"..oPPPPPPo..",
				".oPPPPPPPPo.",
				".oPPPPPPPPo.",
				".oPpppppPo..",
				"..oPPWWPPo..",
				"..oPPPPPPo..",
				"...DD..DD...",
			},
			{ // ear + cotton-tail flick
				"...P....P...",
				"..Po...oPo..",
				"..oPo..oPo..",
				"..oPPPPPPo..",
				".oPPPPPPPPo.",
				".oPPPPPPPPo.",
				".oPpppppPo..",
				"..oPWWWPPo..",
				"..oPPPPPPo..",
				"...D.DD.D...",
			},
		},
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
			{ // legs stride, tail flicks
				"..o......o..",
				".oLo....oLo.",
				"o.Lo.o.oL.o.",
				".oLooLooLo..",
				"..ooPPPoo...",
				"....oPPo....",
				"W.ooPPPPoo..",
				".oPPPPPPPPo.",
				".oPepPPPnPo.",
				".oo.o..o.oo.",
			},
		},
		front: [2][]string{
			{
				"o.o....o.o..",
				".oLo..oLo...",
				"..ooLooLoo..",
				"...oPPPPo...",
				"..oPePPePo..",
				"..oPPnnPPo..",
				"..oPPPPPPo..",
				"..oPPPPPPo..",
				"..oPPPPPPo..",
				"..D.D.D.D...",
			},
			{ // ears twitch, legs shift
				"o.o....o.o..",
				".oLo..oLo...",
				"..oLooooLo..",
				"...oPPPPo...",
				"..oPePPePo..",
				"..oPPnnPPo..",
				"..oPPPPPPo..",
				"..oPPPPPPo..",
				"..oPPPPPPo..",
				"...DD.DD....",
			},
		},
		back: [2][]string{
			{
				"o.o....o.o..",
				".oLo..oLo...",
				"..ooooooo...",
				"...oPPPPo...",
				"..oPPPPPPo..",
				"..oPPPPPPo..",
				"..oPPWPPPo..",
				"..oPPPPPPo..",
				"..D.D.D.D...",
				"............",
			},
			{ // tail flick, legs shift
				"o.o....o.o..",
				".oLo..oLo...",
				"..ooooooo...",
				"...oPPPPo...",
				"..oPPPPPPo..",
				"..oPPPPPPo..",
				"..oPWWPPPo..",
				"..oPPPPPPo..",
				"...DD.DD....",
				"............",
			},
		},
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
			{ // brush tail lifts, legs stride
				"...........W",
				".o.......oWW",
				"oLo......oLW",
				"oLPo....oPLo",
				".oPPoooooPPo",
				"..oPPPPPPPPo",
				"..oPePPPPpPo",
				"..oPPPPPPpo.",
				"..oo.oo.oo..",
				"............",
			},
		},
		front: [2][]string{
			{
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
			},
			{ // ear flicks, feet shift
				"..o......o..",
				".oLo....oL..",
				".oPo....oPo.",
				"..oPPPPPPo..",
				"..oPePPePo..",
				"..oPPnnPPo..",
				"..oPPPPPPo..",
				"...oPnnPo...",
				"...DD..DD...",
				"............",
			},
		},
		back: [2][]string{
			{
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
			},
			{ // tail swishes up
				"..o......o..",
				".oLo....oLo.",
				".oPo....oPo.",
				"..oPPPPPPoW.",
				"..oPPPPPPWWo",
				"..oPPPPPPWo.",
				"..oPpppPPo..",
				"...oPPPPo...",
				"...DD..DD...",
				"............",
			},
		},
	},
	"bird": {
		side: [2][]string{
			{
				"............",
				".....ooo....",
				"....oPPePn..",
				"..ooPPPPo...",
				"oLPPPPpPo...",
				".oPPPppPo...",
				".oPPPPPo....",
				"...D.D......",
				"............",
				"............",
			},
			{ // wing lifts in a flap, feet hop
				"............",
				".....ooo....",
				"....oPPePn..",
				"..LoPPPPo...",
				".LPoPPpPo...",
				".oPPPppPo...",
				".oPPPPPo....",
				"...D..D.....",
				"............",
				"............",
			},
		},
		front: [2][]string{
			{
				"............",
				"............",
				"....oooo....",
				"...oPePeo...",
				"...oPnnPo...",
				"..oPPPPPPo..",
				"..pPPPPPPp..",
				"..oPPPPPPo..",
				"...oPPPo....",
				"....D.D.....",
			},
			{ // both wings flare up
				"............",
				"............",
				"....oooo....",
				"...oPePeo...",
				"...oPnnPo...",
				".poPPPPPPop.",
				"..oPPPPPPo..",
				"..oPPPPPPo..",
				"...oPPPo....",
				"....D.D.....",
			},
		},
		back: [2][]string{
			{
				"............",
				"............",
				"....oooo....",
				"...oPPPPo...",
				"..oPpPPpPo..",
				"..oPPPPPPo..",
				"..oPPPPPPo..",
				"..oPpppPo...",
				"...oPPPo....",
				"....D.D.....",
			},
			{ // wings flare
				"............",
				"............",
				"....oooo....",
				"...oPPPPo...",
				".poPpPPpPop.",
				"..oPPPPPPo..",
				"..oPPPPPPo..",
				"..oPpppPo...",
				"...oPPPo....",
				"....D.D.....",
			},
		},
	},
	"fish": {
		// A fish only ever reads as a side profile, so every facing uses it
		// (front/back are filled from side in init).
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
			{ // tail fin swishes
				"............",
				"...........o",
				"...oooo..ooL",
				".ooPPPPooLLo",
				"oPPLPPPPPLLo",
				"oPePPPPPPPoo",
				"oPPLPPPPPLLo",
				".ooPPPPooLLo",
				"...oooo..ooL",
				"...........o",
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
	rows := frames[((walkFrame%2)+2)%2] // floor-mod so negative frames stay in range
	if mirror {
		rows = mirrorRows(rows)
	}
	return rows, true
}

// creatureMoveWindow is how long after its last step an animal still reads as
// "walking" (brisk pose cycle + bob); after that it idles.
const creatureMoveWindow = 600 * time.Millisecond

// CreatureWalkFrame picks the pose frame and reports whether the animal is
// actively moving (so the renderer can add a walking bob). A mover cycles
// briskly between its two poses; an idle animal mostly holds pose 0 and now and
// then flicks to pose 1 — the species' idle flourish (an ear-twitch, a tail
// swish, a wing-flap). phase (a per-creature constant) desyncs the herd so they
// don't all twitch in lockstep.
func CreatureWalkFrame(lastMoved time.Time, frame, phase int) (wf int, moving bool) {
	if time.Since(lastMoved) <= creatureMoveWindow {
		// Floor-mod: phase derives from world coords, which can be negative.
		return (((frame/3+phase)%2)+2)%2, true
	}
	if (((frame+phase*7)%70)+70)%70 < 5 { // a brief flourish roughly every ~70 frames
		return 1, false
	}
	return 0, false
}

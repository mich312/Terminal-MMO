package game

import (
	"math"
	"strings"
	"unicode"
)

// MoveKey maps a key to a movement intent: direction (dx,dy ∈ {-1,0,1}) and a
// step count (1 walk, 2 run). Cardinals are WASD / arrows; diagonals are the
// roguelike Y U B N (↖ ↗ ↙ ↘) — chosen to avoid q (quit) and e (interact).
// Running is an uppercase letter (Shift+key) or Shift+arrow.
func MoveKey(key string) (dx, dy, steps int, ok bool) {
	switch key {
	case "shift+up":
		return 0, -1, 2, true
	case "shift+down":
		return 0, 1, 2, true
	case "shift+left":
		return -1, 0, 2, true
	case "shift+right":
		return 1, 0, 2, true
	}

	run := false
	if len(key) == 1 {
		if r := rune(key[0]); unicode.IsUpper(r) {
			run = true
			key = strings.ToLower(key)
		}
	}
	switch key {
	case "up", "w":
		dy = -1
	case "down", "s":
		dy = 1
	case "left", "a":
		dx = -1
	case "right", "d":
		dx = 1
	case "y":
		dx, dy = -1, -1
	case "u":
		dx, dy = 1, -1
	case "b":
		dx, dy = -1, 1
	case "n":
		dx, dy = 1, 1
	default:
		return 0, 0, 0, false
	}
	steps = 1
	if run {
		steps = 2
	}
	return dx, dy, steps, true
}

// CanStep reports whether a body at (x,y) may move by (dx,dy). The destination
// footprint must fit, and a diagonal step additionally requires at least one of
// the two adjacent orthogonal cells to be open — so you can't slip through the
// corner where two blockers (e.g. two tree canopies) meet. This keeps movement
// matching what's drawn: you go where there's a visible gap.
func CanStep(walk func(x, y int) bool, x, y, dx, dy int) bool {
	if !footprintWalkable(walk, x+dx, y+dy) {
		return false
	}
	if dx != 0 && dy != 0 && !footprintWalkable(walk, x+dx, y) && !footprintWalkable(walk, x, y+dy) {
		return false // both orthogonal cells blocked — would cut the corner
	}
	return true
}

// footprintWalkable reports whether a PlayerW×PlayerH body with its top-left
// at (x,y) fits — every covered tile must be walkable.
func footprintWalkable(walk func(x, y int) bool, x, y int) bool {
	for dy := 0; dy < PlayerH; dy++ {
		for dx := 0; dx < PlayerW; dx++ {
			if !walk(x+dx, y+dy) {
				return false
			}
		}
	}
	return true
}

// BodyRadius is half a body's width in tiles, for continuous movement. Narrower
// than the tile it stands on, so a one-tile gap is a doorway you walk through
// rather than a slot you have to hit exactly — but wide enough that you can
// never be half inside a wall.
const BodyRadius = 0.35

// Slide moves a body of the given radius from (fx,fy) by (dx,dy) and returns
// where it ends up, stopping against blocked tiles but sliding along them.
//
// The two axes are resolved separately, which is the whole trick: walk at an
// angle into a wall and the component along the wall survives while the one
// into it is dropped, so you glide along the face instead of sticking to it.
// Without that, free-angle movement feels like walking into flypaper — every
// approach that isn't perpendicular to the gap stops dead. The Doom area has
// done it this way since it was written (internal/areas/doom/doom.go step);
// this is that, generalized over an arbitrary walkability predicate.
//
// walk is the same "is this cell open" predicate CanStep takes, so an area's
// existing collision rules carry over untouched.
func Slide(walk func(x, y int) bool, fx, fy, dx, dy, radius float64) (float64, float64) {
	if dx != 0 && bodyFits(walk, fx+dx, fy, radius) {
		fx += dx
	}
	if dy != 0 && bodyFits(walk, fx, fy+dy, radius) {
		fy += dy
	}
	return fx, fy
}

// bodyFits reports whether a body centred at (fx,fy) rests entirely on open
// cells. The body is treated as a square rather than a circle: against a grid of
// square tiles the two differ only at the corners, by less than the radius is
// accurate to anyway, and a square is four lookups with no arithmetic to get
// wrong.
func bodyFits(walk func(x, y int) bool, fx, fy, radius float64) bool {
	x0, x1 := int(math.Floor(fx-radius)), int(math.Floor(fx+radius))
	y0, y1 := int(math.Floor(fy-radius)), int(math.Floor(fy+radius))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if !walk(x, y) {
				return false
			}
		}
	}
	return true
}

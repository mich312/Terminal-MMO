package game

import (
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

package ui

import "time"

// Now is the clock the renderer reads for the day/night cycle. It defaults to
// the wall clock; tests and art tools override it to sample a fixed hour.
var Now = time.Now

// dayKey is one anchor in the 24-hour ambient cycle: at hour H the world is
// tinted toward Hex by Strength (0 = untinted, 1 = fully the tint color).
// Values between anchors are interpolated, and the ring wraps midnight.
type dayKey struct {
	Hour     float64
	Hex      string
	Strength float64
}

var dayCycle = []dayKey{
	{0, "#0E1B3A", 0.38},  // deep night — cool blue
	{6, "#FFC08A", 0.20},  // dawn — warm amber
	{9, "#EAF2FF", 0.06},  // morning — faint cool wash
	{13, "#FFFFFF", 0.03}, // midday — essentially neutral
	{18, "#FF8A4C", 0.24}, // dusk — amber
	{21, "#16244A", 0.34}, // evening — blue
}

// Ambient returns the time-of-day tint and its strength for t's local clock.
// Tiles blend their color toward this tint by the strength; player glyphs are
// left untouched so avatars stay readable at night.
func Ambient(t time.Time) (hex string, strength float64) {
	h := float64(t.Hour()) + float64(t.Minute())/60 + float64(t.Second())/3600

	n := len(dayCycle)
	for i := 0; i < n; i++ {
		a := dayCycle[i]
		b := dayCycle[(i+1)%n]
		hi := b.Hour
		if hi <= a.Hour { // wraps past midnight
			hi += 24
		}
		hh := h
		if hh < a.Hour {
			hh += 24
		}
		if hh >= a.Hour && hh < hi {
			f := (hh - a.Hour) / (hi - a.Hour)
			tint := Blend(a.Hex, b.Hex, f)
			return string(tint), a.Strength + (b.Strength-a.Strength)*f
		}
	}
	return dayCycle[0].Hex, dayCycle[0].Strength
}

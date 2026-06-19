package ui

import "time"

// Now is the clock the renderer reads for the day/night cycle. It defaults to
// the wall clock; tests and art tools override it to sample a fixed hour.
var Now = time.Now

// CyclePeriod is how long one full day/night cycle takes in real time. The
// 24-hour ambient ring is compressed into this span, so dawn → noon → dusk →
// night all pass within a single real hour rather than a real day.
const CyclePeriod = time.Hour

// CycleHour maps a wall-clock instant onto the 0..24 day/night ring, compressed
// so one full cycle elapses every CyclePeriod. Both the ambient tint and the
// sun/lighting model read it, so the tint and the shadows stay in lockstep.
func CycleHour(t time.Time) float64 {
	elapsed := t.Sub(t.Truncate(CyclePeriod)).Seconds()
	return elapsed / CyclePeriod.Seconds() * 24
}

// dayKey is one anchor in the 24-hour ambient cycle: at hour H the world is
// tinted toward Hex by Strength (0 = untinted, 1 = fully the tint color).
// Values between anchors are interpolated, and the ring wraps midnight.
type dayKey struct {
	Hour     float64
	Hex      string
	Strength float64
}

var dayCycle = []dayKey{
	{0, "#0A1228", 0.55},  // deep night — dark, cool blue (lights pop against it)
	{6, "#FFC08A", 0.20},  // dawn — warm amber
	{9, "#EAF2FF", 0.06},  // morning — faint cool wash
	{13, "#FFFFFF", 0.03}, // midday — essentially neutral
	{18, "#FF8A4C", 0.24}, // dusk — amber
	{21, "#14203F", 0.46}, // evening — blue
}

// Ambient returns the time-of-day tint and its strength for t's local clock.
// Tiles blend their color toward this tint by the strength; player glyphs are
// left untouched so avatars stay readable at night.
func Ambient(t time.Time) (hex string, strength float64) {
	h := CycleHour(t)

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

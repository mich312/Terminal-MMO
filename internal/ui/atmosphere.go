package ui

import (
	"math"
	"time"
)

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

// brixenLatitude is the Durst HQ at Brixen (Bressanone), South Tyrol. The
// cycle's daylight length tracks this latitude's real seasons.
const brixenLatitude = 46.715

// DayLength returns the hours of daylight at Brixen for t's date, derived from
// the solar declination: ~8.4h around the winter solstice, ~15.7h at the summer
// solstice, 12h at the equinoxes.
func DayLength(t time.Time) float64 {
	lat := brixenLatitude * math.Pi / 180
	// Solar declination: ~0 at the equinoxes (≈ day 81), ±23.44° at the solstices.
	doy := float64(t.YearDay())
	decl := 23.44 * math.Pi / 180 * math.Sin(2*math.Pi*(doy-81)/365)
	// Sunrise hour angle; clamp keeps it finite (no polar day/night at Brixen).
	x := math.Max(-1, math.Min(1, -math.Tan(lat)*math.Tan(decl)))
	return 24 * math.Acos(x) / math.Pi
}

// SunWindow returns the cycle's sunrise and sunset hours, centered on noon, so
// the daylight span widens in summer and narrows in winter.
func SunWindow(t time.Time) (sunrise, sunset float64) {
	d := DayLength(t)
	return 12 - d/2, 12 + d/2
}

// SolarHour maps the wall clock onto the canonical 0..24 day/night ring that the
// tint and lighting share. It (a) compresses 24h into one real hour (CycleHour),
// then (b) warps the real daylight span onto the fixed dawn..dusk anchors (6..18)
// for t's date — stretching summer days and squeezing winter ones, while keeping
// midnight and noon pinned. One in-game day is still one real hour; only the
// proportion of it spent in daylight changes with the season.
func SolarHour(t time.Time) float64 {
	h := CycleHour(t)
	sr, ss := SunWindow(t)
	switch {
	case h < sr:
		return h / sr * 6 // pre-dawn night → [0,6)
	case h < 12:
		return 6 + (h-sr)/(12-sr)*6 // morning → [6,12)
	case h < ss:
		return 12 + (h-12)/(ss-12)*6 // afternoon → [12,18)
	default:
		return 18 + (h-ss)/(24-ss)*6 // post-dusk night → [18,24)
	}
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
	{0, "#0A0E1A", 0.66},  // deep night — dark, gently cool (desaturated vs. the old electric blue, but darker so it still reads as night, not dim daytime)
	{6, "#FFC08A", 0.20},  // dawn — warm amber
	{9, "#EAF2FF", 0.06},  // morning — faint cool wash
	{13, "#FFFFFF", 0.03}, // midday — essentially neutral
	{18, "#FF8A4C", 0.24}, // dusk — amber
	{21, "#151A28", 0.50}, // evening — muted blue-grey
}

// Ambient returns the time-of-day tint and its strength for t's local clock.
// Tiles blend their color toward this tint by the strength; player glyphs are
// left untouched so avatars stay readable at night.
func Ambient(t time.Time) (hex string, strength float64) {
	h := SolarHour(t)

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

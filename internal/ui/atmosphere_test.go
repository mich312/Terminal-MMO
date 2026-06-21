package ui

import (
	"math"
	"testing"
	"time"

	"github.com/lucasb-eyer/go-colorful"
)

// Ambient must return a parseable hex tint and a strength in [0,1] across the
// whole cycle, including the midnight wrap. The cycle is compressed into one
// real hour, so sweeping the minutes-of-the-hour sweeps the full day/night ring.
func TestAmbientCycle(t *testing.T) {
	for m := 0; m < 60; m++ {
		at := time.Date(2026, 6, 16, 10, m, 0, 0, time.UTC)
		hex, str := Ambient(at)
		if _, err := colorful.Hex(hex); err != nil {
			t.Fatalf("minute %d: bad hex %q: %v", m, hex, err)
		}
		if str < 0 || str > 1 {
			t.Fatalf("minute %d: strength %v out of [0,1]", m, str)
		}
	}
}

// MoonIllumination must stay in [0,1], read ~0 at the reference new moon and ~1
// a half-synodic-month later (full moon), so it can drive moonlight strength.
func TestMoonIllumination(t *testing.T) {
	full := newMoonEpoch.Add(time.Duration(synodicMonth / 2 * 24 * float64(time.Hour)))
	if il := MoonIllumination(newMoonEpoch); il > 0.02 {
		t.Errorf("new moon illumination = %v, want ~0", il)
	}
	if il := MoonIllumination(full); il < 0.98 {
		t.Errorf("full moon illumination = %v, want ~1", il)
	}
	for d := 0; d < 30; d++ { // sweep a month: always within [0,1]
		if il := MoonIllumination(newMoonEpoch.AddDate(0, 0, d)); il < 0 || il > 1 {
			t.Fatalf("day %d: illumination %v out of [0,1]", d, il)
		}
	}
}

// Night should be tinted noticeably more than midday. With the one-hour cycle,
// minute 0 is midnight and minute 30 is noon.
func TestAmbientNightDarkerThanNoon(t *testing.T) {
	_, night := Ambient(time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC))
	_, noon := Ambient(time.Date(2026, 6, 16, 10, 30, 0, 0, time.UTC))
	if night <= noon {
		t.Fatalf("night tint %v should exceed midday %v", night, noon)
	}
}

// One full day/night cycle is compressed into a single real hour: the
// minute-of-hour drives the ring, with minute 0 at midnight and minute 30 noon.
func TestCycleHourCompressedToOneHour(t *testing.T) {
	cases := []struct {
		min, sec int
		want     float64
	}{
		{0, 0, 0},   // midnight
		{15, 0, 6},  // dawn
		{30, 0, 12}, // noon
		{45, 0, 18}, // dusk
	}
	for _, c := range cases {
		got := CycleHour(time.Date(2026, 6, 16, 10, c.min, c.sec, 0, time.UTC))
		if got != c.want {
			t.Errorf("CycleHour(min=%d sec=%d) = %v, want %v", c.min, c.sec, got, c.want)
		}
	}
}

// Daylight length tracks Brixen's real seasons: long summer days, short winter
// ones, ~12h at the equinoxes — while one in-game day stays one real hour.
func TestSeasonalDayLength(t *testing.T) {
	summer := DayLength(time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC))
	winter := DayLength(time.Date(2026, 12, 21, 0, 0, 0, 0, time.UTC))
	equinox := DayLength(time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC))
	t.Logf("Brixen daylight — summer %.2fh, equinox %.2fh, winter %.2fh", summer, equinox, winter)

	if summer <= winter {
		t.Errorf("summer day (%.2fh) should be longer than winter (%.2fh)", summer, winter)
	}
	if summer < 15 || summer > 16.5 {
		t.Errorf("summer solstice daylight %.2fh outside the plausible Brixen range", summer)
	}
	if winter < 7.5 || winter > 9.5 {
		t.Errorf("winter solstice daylight %.2fh outside the plausible Brixen range", winter)
	}
	if equinox < 11.5 || equinox > 12.5 {
		t.Errorf("equinox daylight %.2fh should be near 12h", equinox)
	}
}

// SolarHour warps the daylight span by season but keeps midnight and noon pinned,
// and a longer daylight span means more of the in-game hour reads as "lit": the
// canonical hour at a fixed mid-morning clock position is further along in summer.
func TestSolarHourSeasonalWarp(t *testing.T) {
	// minute 15 of the hour == CycleHour 6 (a fixed clock position partway through
	// the hour). In summer the sun is already well up; in winter it's barely dawn.
	atMin15 := func(month time.Month) float64 {
		return SolarHour(time.Date(2026, month, 21, 10, 15, 0, 0, time.UTC))
	}
	summer, winter := atMin15(6), atMin15(12)
	t.Logf("canonical hour at clock-minute 15 — summer %.2f, winter %.2f", summer, winter)
	if summer <= winter {
		t.Errorf("at a fixed mid-morning clock position the sun should be higher in summer (%.2f) than winter (%.2f)", summer, winter)
	}
	// Noon (minute 30) stays pinned to canonical 12 year-round.
	for _, m := range []time.Month{time.June, time.December} {
		if h := SolarHour(time.Date(2026, m, 21, 10, 30, 0, 0, time.UTC)); math.Abs(h-12) > 1e-9 {
			t.Errorf("%v: noon should stay pinned at canonical 12, got %.4f", m, h)
		}
	}
}

// The Now seam lets the renderer (and art tools) sample a fixed hour instead of
// the wall clock, and defaults to time.Now.
func TestNowOverride(t *testing.T) {
	orig := Now
	defer func() { Now = orig }()
	Now = func() time.Time { return time.Date(2026, 6, 16, 3, 0, 0, 0, time.UTC) }
	hex, str := Ambient(Now())
	wantHex, wantStr := Ambient(time.Date(2026, 6, 16, 3, 0, 0, 0, time.UTC))
	if hex != wantHex || str != wantStr {
		t.Fatalf("Now override not honored: (%s,%v) != (%s,%v)", hex, str, wantHex, wantStr)
	}
}

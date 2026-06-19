package ui

import (
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

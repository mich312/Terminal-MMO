package ui

import (
	"testing"
	"time"

	"github.com/lucasb-eyer/go-colorful"
)

// Ambient must return a parseable hex tint and a strength in [0,1] across the
// whole 24-hour clock, including the midnight wrap.
func TestAmbientCycle(t *testing.T) {
	for h := 0; h < 24; h++ {
		at := time.Date(2026, 6, 16, h, 30, 0, 0, time.UTC)
		hex, str := Ambient(at)
		if _, err := colorful.Hex(hex); err != nil {
			t.Fatalf("hour %d: bad hex %q: %v", h, hex, err)
		}
		if str < 0 || str > 1 {
			t.Fatalf("hour %d: strength %v out of [0,1]", h, str)
		}
	}
}

// Night should be tinted noticeably more than midday.
func TestAmbientNightDarkerThanNoon(t *testing.T) {
	_, night := Ambient(time.Date(2026, 6, 16, 1, 0, 0, 0, time.UTC))
	_, noon := Ambient(time.Date(2026, 6, 16, 13, 0, 0, 0, time.UTC))
	if night <= noon {
		t.Fatalf("night tint %v should exceed midday %v", night, noon)
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

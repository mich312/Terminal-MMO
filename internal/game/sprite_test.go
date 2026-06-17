package game

import (
	"testing"

	"github.com/lucasb-eyer/go-colorful"
)

func TestSpritePixelOpacity(t *testing.T) {
	body := mustHex("#D97757")
	opaque := []rune{'B', 'L', 'D', 'E', 'm', 'W', 'H', 'h'}
	for _, code := range opaque {
		if _, ok := spritePixel(code, body, false); !ok {
			t.Errorf("code %q reported transparent, want opaque", code)
		}
	}
	for _, code := range []rune{'.', ' ', 'x'} {
		if _, ok := spritePixel(code, body, false); ok {
			t.Errorf("code %q reported opaque, want transparent", code)
		}
	}
}

func TestSpritePixelBodyAndFixedColors(t *testing.T) {
	body := mustHex("#3366CC")
	if c, _ := spritePixel('B', body, false); c != body {
		t.Errorf("'B' = %v, want the body color %v", c, body)
	}
	// 'E' (eye) and 'W' (highlight) are fixed, independent of body color.
	eye, _ := spritePixel('E', body, false)
	if eye != spriteBlack {
		t.Errorf("'E' = %v, want spriteBlack", eye)
	}
	hi, _ := spritePixel('W', body, false)
	if hi != spriteWhite {
		t.Errorf("'W' = %v, want spriteWhite", hi)
	}
}

// isSelf tints the body toward white, so 'B' differs from the plain body.
func TestSpritePixelSelfTint(t *testing.T) {
	body := mustHex("#3366CC")
	plain, _ := spritePixel('B', body, false)
	self, _ := spritePixel('B', body, true)
	if near(plain, self) {
		t.Errorf("self body %v not distinguished from plain %v", self, plain)
	}
}

func near(a, b colorful.Color) bool { return a.DistanceLab(b) < 0.001 }

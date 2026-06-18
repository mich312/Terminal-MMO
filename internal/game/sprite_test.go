package game

import (
	"testing"

	"github.com/lucasb-eyer/go-colorful"
)

func TestSpritePixelOpacity(t *testing.T) {
	body := mustHex("#D97757")
	opaque := []rune{'B', 'L', 'D', 'E', 'm', 'W', 'H', 'h'}
	for _, code := range opaque {
		if _, ok := spritePixel(code, body, hatMain, hatShade, false); !ok {
			t.Errorf("code %q reported transparent, want opaque", code)
		}
	}
	for _, code := range []rune{'.', ' ', 'x'} {
		if _, ok := spritePixel(code, body, hatMain, hatShade, false); ok {
			t.Errorf("code %q reported opaque, want transparent", code)
		}
	}
}

func TestSpritePixelBodyAndFixedColors(t *testing.T) {
	body := mustHex("#3366CC")
	if c, _ := spritePixel('B', body, hatMain, hatShade, false); c != body {
		t.Errorf("'B' = %v, want the body color %v", c, body)
	}
	// 'E' (eye) and 'W' (highlight) are fixed, independent of body color.
	eye, _ := spritePixel('E', body, hatMain, hatShade, false)
	if eye != spriteBlack {
		t.Errorf("'E' = %v, want spriteBlack", eye)
	}
	hi, _ := spritePixel('W', body, hatMain, hatShade, false)
	if hi != spriteWhite {
		t.Errorf("'W' = %v, want spriteWhite", hi)
	}
}

// isSelf tints the body toward white, so 'B' differs from the plain body.
func TestSpritePixelSelfTint(t *testing.T) {
	body := mustHex("#3366CC")
	plain, _ := spritePixel('B', body, hatMain, hatShade, false)
	self, _ := spritePixel('B', body, hatMain, hatShade, true)
	if near(plain, self) {
		t.Errorf("self body %v not distinguished from plain %v", self, plain)
	}
}

// Accessory pixels (H/h) take the passed accessory colors, and each named
// accessory resolves to its own hue (so hats aren't all gold anymore).
func TestSpritePixelAccessoryColor(t *testing.T) {
	body := mustHex("#3366CC")
	pink := mustHex("#FF7AA8")
	if c, _ := spritePixel('H', body, pink, pink, false); !near(c, pink) {
		t.Errorf("'H' = %v, want the accessory color %v", c, pink)
	}
	flower, ok := AccessoryIndex("flower")
	if !ok {
		t.Fatal("flower accessory should exist")
	}
	crown, _ := AccessoryIndex("crown")
	fm, _ := accessoryColors(flower)
	cm, _ := accessoryColors(crown)
	if near(fm, cm) {
		t.Errorf("flower and crown share a color %v; accessories should differ", fm)
	}
}

func near(a, b colorful.Color) bool { return a.DistanceLab(b) < 0.001 }

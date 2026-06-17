package pixel

import (
	"image"
	"testing"
)

func TestTextWidthScales(t *testing.T) {
	if w1, w2 := TextWidth("Hello", 1), TextWidth("Hello", 2); w2 != 2*w1 || w1 <= 0 {
		t.Errorf("TextWidth scale: 1x=%d 2x=%d", w1, w2)
	}
}

func TestDrawSlidePanelDrawsSomething(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 520, 360))
	DrawSlidePanel(img, "# Hello\n\n- one\n- two\n\n```go\nx := 1\n```", "Deck · slide 1/2 · anna")
	lit := false
	for i := 0; i+3 < len(img.Pix); i += 4 {
		if img.Pix[i]|img.Pix[i+1]|img.Pix[i+2] != 0 {
			lit = true
			break
		}
	}
	if !lit {
		t.Error("DrawSlidePanel drew nothing")
	}
}

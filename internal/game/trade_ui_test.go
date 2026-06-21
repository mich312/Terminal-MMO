package game

import (
	"image"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// The trade panel draws straight onto an RGBA frame; it must lay out without
// panicking across frame sizes and trade states — empty table, a full table
// with more offered rows than fit, and a pack picker — and actually paint
// something.
func TestTradePanelRenders(t *testing.T) {
	it := func(id string) Item { i, _ := ItemByID(id); return i }
	cases := []struct {
		name string
		v    TradeView
		draw bool // expect pixels
	}{
		{"empty", TradeView{}, true},
		{"full", TradeView{
			You: TradeParty{Name: "ada", Accessory: mustAccTest("shroom"),
				Color: lipgloss.Color("#7DF0FF"), Ready: true,
				Offer: []TradeRow{{it("crystal"), 2}, {it("mushroom"), 1}}},
			Them: TradeParty{Name: "brix", Color: lipgloss.Color("#FFC861"),
				// more rows than maxTradeRows, to exercise the cap
				Offer: []TradeRow{{it("nugget"), 3}, {it("wood"), 5}, {it("amber"), 1}, {it("berry"), 9}, {it("grain"), 2}}},
			Pack: []TradeRow{{it("berry"), 8}, {it("crystal"), 4}, {it("grain"), 9}},
			Sel:  1,
		}, true},
	}
	for _, c := range cases {
		for _, sz := range [][2]int{{900, 560}, {1600, 1000}} {
			img := image.NewRGBA(image.Rect(0, 0, sz[0], sz[1]))
			DrawTradePanel(img, c.v) // must not panic
			drawn := false
			for _, b := range img.Pix {
				if b != 0 {
					drawn = true
					break
				}
			}
			if c.draw && !drawn {
				t.Errorf("%s @ %dx%d: drew nothing", c.name, sz[0], sz[1])
			}
		}
	}
}

func mustAccTest(name string) int { i, _ := AccessoryIndex(name); return i }

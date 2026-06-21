package game

import "testing"

// TestAccessoryPowerWiring guards the wearable-power discoverability: anything
// that grants a light or a documented ability must describe itself (so the
// /avatar listing, the equip line and the character panel can tell the player),
// while a plain cosmetic stays silent.
func TestAccessoryPowerWiring(t *testing.T) {
	for i := 1; i < NumAccessories(); i++ {
		if _, _, lit := AccessoryLight(i); lit && AccessoryPower(i) == "" {
			t.Errorf("%s glows but has no power description", AccessoryName(i))
		}
	}
	powered := []string{"crown", "diadem", "glowcap", "flower", "circlet", "ambergem", "shroom"}
	for _, name := range powered {
		idx, ok := AccessoryIndex(name)
		if !ok {
			t.Fatalf("missing accessory %q", name)
		}
		if AccessoryPower(idx) == "" {
			t.Errorf("%s should describe its power", name)
		}
	}
	if idx, _ := AccessoryIndex("band"); AccessoryPower(idx) != "" {
		t.Errorf("band is a plain cosmetic, should have no power text")
	}
}

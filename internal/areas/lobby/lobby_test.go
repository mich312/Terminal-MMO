package lobby

import "testing"

func TestMapIsRectangular(t *testing.T) {
	for i, r := range rows {
		if n := len([]rune(r)); n != 60 {
			t.Errorf("row %d: width %d, want 60", i, n)
		}
	}
}

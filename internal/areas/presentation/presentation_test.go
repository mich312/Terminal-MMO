package presentation

import (
	"testing"

	"github.com/durst-group/durstworld/internal/game"
)

func parsed() *game.TileMap { return game.ParseMap(rows, legend, texts()) }

func TestMapIsRectangular(t *testing.T) {
	for i, r := range rows {
		if n := len([]rune(r)); n != 60 {
			t.Errorf("row %d: width %d, want 60", i, n)
		}
	}
}

func TestPresenterTilesAreInsideRooms(t *testing.T) {
	tm := parsed()
	xs, ys := tm.FindObject("presenter")
	if len(xs) != 4 {
		t.Fatalf("expected 4 presenter tiles, got %d", len(xs))
	}
	for i := range xs {
		if _, ok := roomAt(xs[i], ys[i]); !ok {
			t.Errorf("presenter tile (%d,%d) is outside every room", xs[i], ys[i])
		}
	}
}

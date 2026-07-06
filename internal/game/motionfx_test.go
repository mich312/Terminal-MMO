package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/world"
)

// DustPuff and runLean read the world's motion trail: dust only behind a fresh
// dash, lean only mid-dash and only sideways.
func TestDustPuffAndRunLean(t *testing.T) {
	now := time.Now()
	dash := world.Player{X: 6, Y: 4, PrevX: 4, PrevY: 4, Ran: true, Facing: world.DirE, LastMoved: now}
	if band, ok := DustPuff(dash); !ok || band != 0 {
		t.Fatalf("fresh dash: band=%d ok=%v, want 0 true", band, ok)
	}
	if runLean(dash) != 1 {
		t.Fatalf("eastward dash should lean +1, got %d", runLean(dash))
	}

	walk := dash
	walk.Ran = false
	if _, ok := DustPuff(walk); ok {
		t.Fatal("a walk kicks no dust")
	}
	if runLean(walk) != 0 {
		t.Fatal("a walk doesn't lean")
	}

	west := dash
	west.Facing = world.DirW
	if runLean(west) != -1 {
		t.Fatalf("westward dash should lean -1, got %d", runLean(west))
	}
	north := dash
	north.Facing = world.DirN
	if runLean(north) != 0 {
		t.Fatal("a straight north dash has no sideways lean")
	}

	settled := dash
	settled.LastMoved = now.Add(-time.Second)
	if _, ok := DustPuff(settled); ok {
		t.Fatal("dust settles after the puff window")
	}
	if runLean(settled) != 0 {
		t.Fatal("the lean relaxes once the dash is over")
	}
}

// A dashing avatar renders differently from the same avatar mid-walk: the dust
// in the vacated tile and the lean are both visible pixels.
func TestDashRendersDustAndLean(t *testing.T) {
	const n, scale = 12, 18
	tiles := make([][]Tile, n)
	for y := 0; y < n; y++ {
		tiles[y] = make([]Tile, n)
		for x := 0; x < n; x++ {
			tiles[y][x] = Tile{Kind: TileFloor, Walkable: true, Tex: TexGrass, Ground: "#5EAE63"}
		}
	}
	tm := &TileMap{W: n, H: n, Tiles: tiles}
	cam := Camera{W: n, H: n}

	dash := world.Player{Name: "ada", X: 6, Y: 4, PrevX: 4, PrevY: 4, Ran: true,
		Facing: world.DirE, Color: "#FFC861", LastMoved: time.Now()}
	walk := dash
	walk.Ran = false

	a := RenderRGBA(nil, tm, []world.Player{dash}, "ada", 0, cam, Light{}, 0, 0, scale, false, nil)
	b := RenderRGBA(nil, tm, []world.Player{walk}, "ada", 0, cam, Light{}, 0, 0, scale, false, nil)
	if bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("a dash should render dust + lean that a walk doesn't")
	}
}

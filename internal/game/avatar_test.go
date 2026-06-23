package game

import (
	"testing"

	"github.com/durst-group/durstworld/internal/world"
)

func TestWrapIdx(t *testing.T) {
	cases := []struct{ i, n, want int }{
		{0, 5, 0}, {4, 5, 4}, {5, 5, 0}, {6, 5, 1},
		{-1, 5, 4}, {-5, 5, 0}, {-6, 5, 4},
		{3, 0, 0}, // empty domain never indexes
	}
	for _, c := range cases {
		if got := wrapIdx(c.i, c.n); got != c.want {
			t.Errorf("wrapIdx(%d,%d) = %d, want %d", c.i, c.n, got, c.want)
		}
	}
}

// Every style/facing/frame must yield a square 12×12 bitmap, since both
// renderers index it by fixed coordinates.
func TestAvatarBitmapDimensions(t *testing.T) {
	dirs := []world.Dir{world.DirN, world.DirNE, world.DirE, world.DirSE, world.DirS, world.DirSW, world.DirW, world.DirNW}
	for style := 0; style < NumAvatarStyles(); style++ {
		for acc := 0; acc < NumAccessories(); acc++ {
			for _, d := range dirs {
				for f := 0; f < 2; f++ {
					bmp := AvatarBitmap(style, acc, "", d, f)
					if len(bmp) != 12 {
						t.Fatalf("style %d acc %d dir %v frame %d: %d rows, want 12", style, acc, d, f, len(bmp))
					}
					for r, row := range bmp {
						if n := len([]rune(row)); n != 12 {
							t.Fatalf("style %d dir %v row %d: width %d, want 12", style, d, r, n)
						}
					}
				}
			}
		}
	}
}

// West is the east profile mirrored: AvatarBitmap(...DirW) must equal each
// row of AvatarBitmap(...DirE) reversed.
func TestAvatarBitmapWestMirrorsEast(t *testing.T) {
	east := AvatarBitmap(0, 0, "", world.DirE, 0)
	west := AvatarBitmap(0, 0, "", world.DirW, 0)
	if len(east) != len(west) {
		t.Fatalf("row count mismatch: east %d, west %d", len(east), len(west))
	}
	for i := range east {
		er := []rune(east[i])
		rev := make([]rune, len(er))
		for j, r := range er {
			rev[len(er)-1-j] = r
		}
		if string(rev) != west[i] {
			t.Errorf("row %d: west %q is not the mirror of east %q", i, west[i], east[i])
		}
	}
}

// The "none" accessory (index 0) leaves the body untouched; a real accessory
// changes the top rows.
func TestAvatarAccessoryOverlay(t *testing.T) {
	plain := AvatarBitmap(0, 0, "", world.DirS, 0)
	if got := overlayAccessory(append([]string(nil), plain...), 0); !equalRows(got, plain) {
		t.Errorf("accessory 0 (none) altered the bitmap")
	}
	hatted := AvatarBitmap(0, 1, "", world.DirS, 0) // cap
	if equalRows(hatted, plain) {
		t.Errorf("accessory 1 (cap) left the bitmap unchanged")
	}
}

// A wielded weapon overlays the body (it draws something the unarmed sprite
// doesn't), every weapon shape stays a clean 12×12, and an unknown/empty weapon
// leaves the figure untouched.
func TestAvatarWeaponOverlay(t *testing.T) {
	plain := AvatarBitmap(0, 0, "", world.DirS, 0)
	if armed := AvatarBitmap(0, 0, "sword", world.DirS, 0); equalRows(armed, plain) {
		t.Error("wielding a sword left the avatar unchanged")
	}
	if got := AvatarBitmap(0, 0, "not-a-weapon", world.DirS, 0); !equalRows(got, plain) {
		t.Error("an unknown weapon should not alter the avatar")
	}
	for _, wp := range Weapons() {
		bmp := AvatarBitmap(0, 0, wp.Item, world.DirN, 0)
		if len(bmp) != 12 {
			t.Fatalf("weapon %q: %d rows, want 12", wp.Item, len(bmp))
		}
		for r, row := range bmp {
			if n := len([]rune(row)); n != 12 {
				t.Fatalf("weapon %q row %d: width %d, want 12", wp.Item, r, n)
			}
		}
		if weaponShapeOf(wp.Item) == "" {
			t.Errorf("weapon %q has no in-hand sprite shape", wp.Item)
		}
	}
}

func equalRows(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

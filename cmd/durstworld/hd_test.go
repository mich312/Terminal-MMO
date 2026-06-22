package main

import (
	"path/filepath"
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
)

// moveKeyMsg accepts every movement key and rejects everything else, and the
// KeyMsg it builds must drive the areas' MoveKey the same way the raw name does
// — otherwise HD movement would silently diverge from the glyph client.
func TestMoveKeyMsg(t *testing.T) {
	moves := []string{"w", "a", "s", "d", "W", "up", "down", "left", "right",
		"shift+up", "shift+down", "shift+left", "shift+right", "y", "u", "b", "n"}
	for _, k := range moves {
		km, ok := moveKeyMsg(k)
		if !ok {
			t.Errorf("movement key %q rejected", k)
			continue
		}
		wantDx, wantDy, wantSteps, _ := game.MoveKey(k)
		gotDx, gotDy, gotSteps, gotOK := game.MoveKey(km.String())
		if !gotOK || gotDx != wantDx || gotDy != wantDy || gotSteps != wantSteps {
			t.Errorf("%q → KeyMsg %q drives MoveKey (%d,%d,%d) want (%d,%d,%d)",
				k, km.String(), gotDx, gotDy, gotSteps, wantDx, wantDy, wantSteps)
		}
	}
	for _, k := range []string{"e", "enter", "x", " ", "q", ""} {
		if _, ok := moveKeyMsg(k); ok {
			t.Errorf("non-movement key %q accepted", k)
		}
	}
}

// HD is the default: only an explicit `glyph` command routes to the bubbletea
// client; a plain connection or any other args stays HD.
func TestCmdWantsClassic(t *testing.T) {
	for _, cmd := range [][]string{{"glyph"}, {"glyph", "extra"}} {
		if !cmdWantsClassic(cmd) {
			t.Errorf("%v should opt into the classic client", cmd)
		}
	}
	for _, cmd := range [][]string{nil, {}, {"hd"}, {"GLYPH"}, {"foo"}} {
		if cmdWantsClassic(cmd) {
			t.Errorf("%v should stay in default HD", cmd)
		}
	}
}

// The Arcade and its minigames are walkable, HD-renderable rooms, so entering
// them in HD keeps the requested area (rather than falling back to the lobby,
// which is what enterHD does only for an area that can't draw in pixels).
func TestEnterHDArcadeAndGames(t *testing.T) {
	for _, id := range []string{"arcade", "sokoban", "maze", "snake", "tetris", "pong", "breakout", "bomberman", "2048", "chess"} {
		t.Run(id, func(t *testing.T) {
			w := world.New()
			defer w.Close()
			name, _ := w.Join("p")
			st := store.Open(filepath.Join(t.TempDir(), "x.db"))
			defer st.Close()
			ctx := &game.Ctx{World: w, Store: st, Name: name}

			got, area, hv := enterHD(ctx, "lobby", id)
			if got != id {
				t.Fatalf("entering %q in HD landed in %q", id, got)
			}
			if area == nil || hv == nil {
				t.Fatal("enterHD returned nil area/viewer")
			}
		})
	}
}

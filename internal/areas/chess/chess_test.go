package chess

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/world"
)

func newArea(t *testing.T) *area {
	t.Helper()
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("tester")
	ctx := &game.Ctx{World: w, Name: name}
	a := &area{Walker: game.Walker{Ctx: ctx, AreaID: "chess"}}
	self, _ := w.Self(name)
	a.Init(&self)
	return a
}

// The opening position has exactly 20 legal moves for White (16 pawn, 4 knight).
func TestOpeningMoveCount(t *testing.T) {
	a := newArea(t)
	if n := len(a.legalMoves(white)); n != 20 {
		t.Fatalf("opening legal moves = %d, want 20", n)
	}
}

func TestCheckDetection(t *testing.T) {
	a := newArea(t)
	a.board = [8][8]int{}
	a.board[7][4] = white * king
	a.board[0][4] = black * rook // same file as the king, nothing between
	if !a.inCheck(white) {
		t.Fatal("king should be in check from the rook down the file")
	}
	a.board[0][4] = 0
	a.board[0][3] = black * rook
	if a.inCheck(white) {
		t.Fatal("king should be safe once the rook is off the file")
	}
}

// apply followed by undo restores the board, castling rights and ep target.
func TestApplyUndoRoundtrip(t *testing.T) {
	a := newArea(t)
	before := a.board
	cr, ep := a.cr, a.ep
	u := a.apply(move{4, 6, 4, 4, false, 0, false}) // 1. e4 (a double push)
	if a.board == before {
		t.Fatal("apply did not change the board")
	}
	if a.ep != [2]int{4, 5} {
		t.Fatalf("double push should set the ep target, got %v", a.ep)
	}
	a.undo(u)
	if a.board != before || a.cr != cr || a.ep != ep {
		t.Fatal("undo did not fully restore the position")
	}
}

// A back-rank mate is recognised as game over with White (to move) checkmated.
func TestBackRankMate(t *testing.T) {
	a := newArea(t)
	a.board = [8][8]int{}
	a.board[7][6] = white * king // g1
	a.board[6][5] = white * pawn // f2
	a.board[6][6] = white * pawn // g2
	a.board[6][7] = white * pawn // h2
	a.board[7][0] = black * rook // a1, checking along the back rank
	a.board[0][4] = black * king // give Black a king so the position is legal
	a.turn = white
	if !a.finishIfDone(white) {
		t.Fatal("a back-rank mate should end the game")
	}
	if !a.over {
		t.Fatal("game should be over")
	}
}

func TestLeave(t *testing.T) {
	a := newArea(t)
	next, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if tr, ok := next.(game.Transition); !ok || tr.To != "arcade" {
		t.Fatalf("x → %#v, want Transition to arcade", next)
	}
}

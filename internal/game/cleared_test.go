package game

import (
	"testing"

	"github.com/durst-group/durstworld/internal/store"
)

func TestClearGroundAndQuery(t *testing.T) {
	ctx := claimCtx(t, "ada", 1000)
	if IsCleared(ctx, 50, 50) {
		t.Fatal("nothing cleared yet")
	}
	if !ClearGround(ctx, 50, 50) {
		t.Fatal("clearing open frontier should succeed")
	}
	if !IsCleared(ctx, 50, 50) {
		t.Error("a just-cleared cell should read as cleared")
	}
	if set := ActiveClearedSet(ctx, 40, 40, 60, 60); !set[[2]int{50, 50}] {
		t.Error("the cleared cell should be in the window set")
	}
}

func TestClearedRegrowsAfterLease(t *testing.T) {
	ctx := claimCtx(t, "ada", 1000)
	ClearGround(ctx, 50, 50)

	// Past the lease with no touch → regrown (treated as original terrain).
	nowUnix = func() int64 { return 1000 + clearLease + 1 }
	if IsCleared(ctx, 50, 50) {
		t.Error("a cleared cell should regrow once the lease lapses")
	}
	if set := ActiveClearedSet(ctx, 40, 40, 60, 60); set[[2]int{50, 50}] {
		t.Error("a lapsed cell must not appear in the active set")
	}
}

func TestTendKeepsClearingAlive(t *testing.T) {
	ctx := claimCtx(t, "ada", 1000)
	ClearGround(ctx, 50, 50)
	// Just before lapse, the owner tends it; then past the original lapse it lives.
	nowUnix = func() int64 { return 1000 + clearLease - 5 }
	TouchCleared(ctx, 50, 50)
	nowUnix = func() int64 { return 1000 + clearLease + 5 }
	if !IsCleared(ctx, 50, 50) {
		t.Error("a tended clearing should outlast its original lease")
	}
}

func TestClearGroundRespectsBuildRight(t *testing.T) {
	owner := claimCtx(t, "ada", 1000)
	ClaimWorkspace(owner, "plotA", 100, 100, 2, 2)
	intruder := &Ctx{World: owner.World, Store: store.Open(""), Name: "mallory"}
	if ClearGround(intruder, 100, 100) {
		t.Error("a stranger must not clear inside someone's claim")
	}
	if _, ok := owner.World.ClearedAt(100, 100); ok {
		t.Error("the blocked clear must not have written a record")
	}
	// The owner can clear their own parcel.
	if !ClearGround(owner, 100, 100) {
		t.Error("the claim holder should be able to clear their own ground")
	}
}

func TestRegrowRemovesRecord(t *testing.T) {
	ctx := claimCtx(t, "ada", 1000)
	ClearGround(ctx, 7, 8)
	ctx.World.Regrow(7, 8)
	if _, ok := ctx.World.ClearedAt(7, 8); ok {
		t.Error("Regrow should delete the cleared record")
	}
}

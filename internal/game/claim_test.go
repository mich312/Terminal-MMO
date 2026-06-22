package game

import (
	"testing"

	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
)

// claimCtx wires a player into a fresh world. fixedNow pins the machine/claim
// clock for the test and restores it after.
func claimCtx(t *testing.T, name string, fixedNow int64) *Ctx {
	t.Helper()
	w := world.New()
	t.Cleanup(w.Close)
	pn, _ := w.Join(name)
	w.EnterArea(pn, "wilds", 0, 0, "")
	old := nowUnix
	nowUnix = func() int64 { return fixedNow }
	t.Cleanup(func() { nowUnix = old })
	return &Ctx{World: w, Store: store.Open(""), Name: pn}
}

func TestParcelBox(t *testing.T) {
	// A 2×2 building anchored at (10,10) with margin 2 → (8,8)..(13,13).
	minX, minY, maxX, maxY := ParcelBox(10, 10, 2, 2)
	if minX != 8 || minY != 8 || maxX != 13 || maxY != 13 {
		t.Errorf("parcel = (%d,%d)..(%d,%d), want (8,8)..(13,13)", minX, minY, maxX, maxY)
	}
}

func TestBuildRightOpenGround(t *testing.T) {
	ctx := claimCtx(t, "ada", 1000)
	if ok, _ := BuildRight(ctx, 500, 500); !ok {
		t.Error("open, unclaimed ground should be buildable by anyone")
	}
}

func TestBuildRightTownClaim(t *testing.T) {
	owner := claimCtx(t, "ada", 1000)
	if !ClaimWorkspace(owner, "plotA", 100, 100, 2, 2) {
		t.Fatal("owner should be able to claim an unowned plot")
	}
	// Inside the parcel: owner may build, a stranger may not.
	if ok, _ := BuildRight(owner, 100, 100); !ok {
		t.Error("the holder should be able to build on their own parcel")
	}
	stranger := &Ctx{World: owner.World, Store: store.Open(""), Name: "mallory"}
	ok, who := BuildRight(stranger, 100, 100)
	if ok || who != "ada" {
		t.Errorf("BuildRight for a stranger = (%v,%q), want blocked by ada", ok, who)
	}
	// Just outside the parcel margin, anyone may build again.
	if ok, _ := BuildRight(stranger, 110, 110); !ok {
		t.Error("ground outside the parcel should be open")
	}
}

func TestBuildRightLapsedClaimOpens(t *testing.T) {
	owner := claimCtx(t, "ada", 100) // claimed at t=100
	ClaimWorkspace(owner, "plotA", 100, 100, 2, 2)

	// Much later, the lease has lapsed; the parcel is open and re-deedable.
	nowUnix = func() int64 { return 100 + leasePeriod + 1 }
	stranger := &Ctx{World: owner.World, Store: store.Open(""), Name: "mallory"}
	if ok, _ := BuildRight(stranger, 100, 100); !ok {
		t.Error("a lapsed claim should leave its parcel open to others")
	}
	if !ClaimWorkspace(stranger, "plotA", 100, 100, 2, 2) {
		t.Error("a lapsed plot should be re-deedable")
	}
	if _, mine, ok := WorkspaceAt(stranger, 100, 100); !ok || !mine {
		t.Error("after re-deeding, the new owner should hold the live claim")
	}
}

func TestBuildRightWildsBuffer(t *testing.T) {
	owner := claimCtx(t, "ada", 1000)
	owner.World.Place("wilds", world.Placement{X: 200, Y: 200, Kind: "sawmill", Owner: "ada"})
	stranger := &Ctx{World: owner.World, Store: store.Open(""), Name: "mallory"}

	// Within the buffer of ada's machine, mallory can't build; ada can.
	if ok, who := BuildRight(stranger, 202, 201); ok || who != "ada" {
		t.Errorf("buffer check = (%v,%q), want blocked by ada", ok, who)
	}
	if ok, _ := BuildRight(owner, 202, 201); !ok {
		t.Error("the structure's owner should build freely within their own buffer")
	}
	// Beyond the buffer, open again.
	if ok, _ := BuildRight(stranger, 210, 210); !ok {
		t.Error("ground beyond the buffer should be open")
	}
}

func TestTouchWorkspaceRefreshesLease(t *testing.T) {
	owner := claimCtx(t, "ada", 100)
	ClaimWorkspace(owner, "plotA", 100, 100, 2, 2)
	// Advance to just before lapse, then touch to push the lease forward.
	nowUnix = func() int64 { return 100 + leasePeriod - 10 }
	TouchWorkspace(owner, "plotA")
	// Now past the original lapse, but the touch kept it live.
	nowUnix = func() int64 { return 100 + leasePeriod + 5 }
	if _, _, ok := WorkspaceAt(owner, 100, 100); !ok {
		t.Error("a touched claim should still be live past its original lease")
	}
}

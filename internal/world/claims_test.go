package world

import "testing"

func mkClaim(plot, owner string, touch int64) Claim {
	return Claim{PlotID: plot, Owner: owner, MinX: 0, MinY: 0, MaxX: 5, MaxY: 5, LastTouch: touch}
}

func TestClaimPlotArbitration(t *testing.T) {
	w := New()
	defer w.Close()
	const now = 1000
	const expireBefore = 0 // nothing lapses in this test

	if !w.ClaimPlot(mkClaim("p1", "ada", now), expireBefore) {
		t.Fatal("first claim on an unowned plot should succeed")
	}
	// A second player can't take a live claim.
	if w.ClaimPlot(mkClaim("p1", "bob", now), expireBefore) {
		t.Error("a live claim must block another player")
	}
	c, ok := w.ClaimForPlot("p1")
	if !ok || c.Owner != "ada" {
		t.Fatalf("claim owner = %+v, want ada still holds it", c)
	}
	// The owner re-deeding their own live claim is allowed (a refresh).
	if !w.ClaimPlot(mkClaim("p1", "ada", now+5), expireBefore) {
		t.Error("the owner should be able to re-deed their own claim")
	}
}

func TestClaimLapsedIsReDeedable(t *testing.T) {
	w := New()
	defer w.Close()
	// ada claimed long ago at t=100.
	w.ClaimPlot(mkClaim("p1", "ada", 100), 0)
	// Now it's t=10000 and the lease cutoff is 9000 — ada's claim (100) has lapsed.
	if !w.ClaimPlot(mkClaim("p1", "bob", 10000), 9000) {
		t.Fatal("a lapsed claim should be re-deedable by another player")
	}
	c, _ := w.ClaimForPlot("p1")
	if c.Owner != "bob" {
		t.Errorf("owner = %q, want bob took the lapsed plot", c.Owner)
	}
}

func TestClaimAtCoversParcel(t *testing.T) {
	w := New()
	defer w.Close()
	w.ClaimPlot(Claim{PlotID: "p1", Owner: "ada", MinX: 10, MinY: 10, MaxX: 14, MaxY: 14, LastTouch: 1}, 0)
	if _, ok := w.ClaimAt(12, 12); !ok {
		t.Error("a cell inside the parcel should resolve to the claim")
	}
	if _, ok := w.ClaimAt(20, 20); ok {
		t.Error("a cell outside the parcel must not resolve to the claim")
	}
}

func TestTouchAndRelease(t *testing.T) {
	w := New()
	defer w.Close()
	w.ClaimPlot(mkClaim("p1", "ada", 100), 0)

	w.TouchClaim("p1", "bob", 500) // not bob's — no-op
	if c, _ := w.ClaimForPlot("p1"); c.LastTouch != 100 {
		t.Error("a non-owner touch must not refresh the lease")
	}
	w.TouchClaim("p1", "ada", 500)
	if c, _ := w.ClaimForPlot("p1"); c.LastTouch != 500 {
		t.Error("the owner's touch should refresh the lease")
	}

	if w.ReleaseClaim("p1", "bob") {
		t.Error("a non-owner can't release a claim")
	}
	if !w.ReleaseClaim("p1", "ada") {
		t.Error("the owner should be able to release their claim")
	}
	if _, ok := w.ClaimForPlot("p1"); ok {
		t.Error("a released plot should be unclaimed")
	}
}

func TestPlacementsNear(t *testing.T) {
	w := New()
	defer w.Close()
	w.Place("wilds", Placement{X: 0, Y: 0, Kind: "fence", Owner: "ada"})
	w.Place("wilds", Placement{X: 2, Y: 1, Kind: "fence", Owner: "ada"})
	w.Place("wilds", Placement{X: 10, Y: 10, Kind: "fence", Owner: "bob"})
	near := w.PlacementsNear(0, 0, 3)
	if len(near) != 2 {
		t.Errorf("found %d placements within r=3 of origin, want 2", len(near))
	}
}

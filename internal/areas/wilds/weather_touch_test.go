package wilds

import (
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// findSky scans forward from a fixed instant until the storm at (x,y) satisfies
// pred — the same deterministic scan the game package's weather tests use.
func findSky(t *testing.T, x, y int, pred func(float64) bool) time.Time {
	t.Helper()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for probe := base; probe.Before(base.Add(48 * time.Hour)); probe = probe.Add(3 * time.Minute) {
		if pred(game.StormAt(probe, x, y)) {
			return probe
		}
	}
	t.Fatal("storm field never satisfied the predicate in 48h")
	return base
}

// The fish bite in the rain: a jetty haul yields double under a storm and the
// usual single fish under a clear sky.
func TestFishBiteInTheRain(t *testing.T) {
	wet := findSky(t, 100, 100, func(i float64) bool { return i >= 0.9 })
	dry := findSky(t, 100, 100, func(i float64) bool { return i == 0 })
	if got := fishHaul(100, 100, wet); got != 2 {
		t.Fatalf("heavy rain haul = %d fish, want 2", got)
	}
	if got := fishHaul(100, 100, dry); got != 1 {
		t.Fatalf("clear-sky haul = %d fish, want 1", got)
	}
}

// Distant thunder rolls now and then under a storm — deterministically per
// wall-clock window, never twice in one window, and never under a clear sky.
func TestThunderRolls(t *testing.T) {
	orig := ui.Now
	defer func() { ui.Now = orig }()

	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(t.TempDir() + "/w.db"), Name: name, Theme: ui.Default}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	// Under a heavy storm, some window within a stretch must thunder.
	stormy := findSky(t, a.wx, a.wy, func(i float64) bool { return i >= 0.9 })
	fired := 0
	for k := 0; k < 60; k++ { // 60 windows ≈ 20 simulated minutes
		at := stormy.Add(time.Duration(k*thunderWindow) * time.Second)
		ui.Now = func() time.Time { return at }
		before := a.lastThunder
		a.rollThunder()
		if a.lastThunder != before {
			fired++
			// A second roll in the same window must not re-fire.
			mark := a.toastUntil
			a.rollThunder()
			if a.toastUntil != mark {
				t.Fatal("thunder fired twice in one window")
			}
		}
	}
	if fired == 0 {
		t.Fatal("no thunder in 20 minutes of heavy storm")
	}
	if fired > 45 {
		t.Fatalf("thunder fired %d/60 windows — it should mutter, not drone", fired)
	}

	// Under a clear sky, no window thunders.
	clear := findSky(t, a.wx, a.wy, func(i float64) bool { return i == 0 })
	a.lastThunder, a.toast, a.toastUntil = 0, "", time.Time{}
	for k := 0; k < 60; k++ {
		at := clear.Add(time.Duration(k*thunderWindow) * time.Second)
		// Skip windows where a front happens to drift back in mid-scan.
		if game.StormAt(at, a.wx, a.wy) >= 0.35 {
			continue
		}
		ui.Now = func() time.Time { return at }
		a.rollThunder()
	}
	if a.toast != "" {
		t.Fatalf("thunder %q under a clear sky", a.toast)
	}
}

package world

import (
	"sync"
	"testing"
)

// The test that justifies the atomic contribution path, mirroring the stall's
// TestConcurrentAcceptNeverOversells: many contributors hit one phase at once;
// exactly as many succeed as there was room, the phase is never overfilled, and
// the completing contribution advances the build exactly once.
func TestContributeToProjectAtomic(t *testing.T) {
	w := New()
	defer w.Close()

	const need = 20
	req := map[string]int{"timber": need}

	const contributors = 60
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted, advances := 0, 0
	for i := 0; i < contributors; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, got, advanced := w.ContributeToProject("hall", "ada", 0, "timber", 1, req, 1)
			mu.Lock()
			accepted += got
			if advanced {
				advances++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if accepted != need {
		t.Errorf("accepted %d timber, want exactly %d (the phase requirement)", accepted, need)
	}
	if advances != 1 {
		t.Errorf("%d contributions reported advancing the phase, want exactly 1", advances)
	}
	st, ok := w.ProjectState("hall")
	if !ok || !st.Done {
		t.Errorf("project state = %+v ok=%v, want a finished single-phase build", st, ok)
	}
	if st.Phase != 1 {
		t.Errorf("phase = %d, want 1 (advanced past the only phase)", st.Phase)
	}
}

// A multi-resource, multi-phase build advances phase by phase, resets its pool
// between phases, and finishes only when the last phase fills.
func TestContributeToProjectAdvancesPhases(t *testing.T) {
	w := New()
	defer w.Close()

	phases := []map[string]int{
		{"timber": 2},             // phase 0
		{"stone": 1, "planks": 1}, // phase 1
	}
	contribute := func(item string, n int) (Project, int, bool) {
		st, _ := w.ProjectState("hall")
		return w.ContributeToProject("hall", "ada", st.Phase, item, n, phases[st.Phase], len(phases))
	}

	// Phase 0 needs 2 timber; one short doesn't advance.
	if _, n, adv := contribute("timber", 1); n != 1 || adv {
		t.Fatalf("first timber: accepted=%d advanced=%v, want 1/false", n, adv)
	}
	if _, n, adv := contribute("timber", 1); n != 1 || !adv {
		t.Fatalf("completing timber: accepted=%d advanced=%v, want 1/true", n, adv)
	}

	st, _ := w.ProjectState("hall")
	if st.Phase != 1 || st.Done || len(st.Pool) != 0 {
		t.Fatalf("after phase 0: %+v, want phase 1, not done, empty pool", st)
	}

	// Phase 1 needs one each of stone and planks; both must land to finish.
	if _, _, adv := contribute("stone", 5); adv {
		t.Fatalf("stone alone advanced the multi-resource phase; want false")
	}
	if _, _, adv := contribute("planks", 1); !adv {
		t.Fatalf("planks should complete the final phase; want true")
	}
	st, _ = w.ProjectState("hall")
	if !st.Done || st.Phase != 2 {
		t.Fatalf("after phase 1: %+v, want done at phase 2", st)
	}
}

// Contributions are clamped to the phase requirement (never overfilled and the
// surplus is never spent), unwanted items are refused, and a stale phase guard
// — a contributor working from a phase a concurrent build already left — banks
// nothing rather than spilling into the new phase.
func TestContributeToProjectClampsAndGuards(t *testing.T) {
	w := New()
	defer w.Close()
	req0 := map[string]int{"timber": 3}

	// Overshoot the requirement: only the remaining room is accepted.
	if _, n, adv := w.ContributeToProject("hall", "ada", 0, "timber", 10, req0, 2); n != 3 || !adv {
		t.Fatalf("clamped contribution: accepted=%d advanced=%v, want 3/true (room, then complete)", n, adv)
	}

	// An item the current phase doesn't want is refused.
	req1 := map[string]int{"stone": 2}
	if _, n, _ := w.ContributeToProject("hall", "ada", 1, "berries", 5, req1, 2); n != 0 {
		t.Errorf("unwanted item accepted=%d, want 0", n)
	}

	// Stale guard: a contributor still thinks it's phase 0; nothing is banked,
	// so phase-0 goods never leak into phase 1.
	if _, n, _ := w.ContributeToProject("hall", "ada", 0, "timber", 1, req0, 2); n != 0 {
		t.Errorf("stale-phase contribution accepted=%d, want 0", n)
	}
	st, _ := w.ProjectState("hall")
	if st.Phase != 1 || len(st.Pool) != 0 {
		t.Errorf("after guards: %+v, want phase 1 with an empty pool", st)
	}
}

// A persist callback fires with a snapshot on every accepted contribution, and
// the snapshot owns its own Pool map (mutating the live project later doesn't
// rewrite what was handed to the store).
func TestContributeToProjectPersists(t *testing.T) {
	w := New()
	defer w.Close()
	var last Project
	saves := 0
	w.SetProjectPersist(func(p Project) { last = p; saves++ })

	req := map[string]int{"timber": 3}
	w.ContributeToProject("hall", "ada", 0, "timber", 1, req, 1)
	if saves != 1 || last.Pool["timber"] != 1 {
		t.Fatalf("after one contribution: saves=%d snapshot=%+v, want 1 save with 1 timber", saves, last)
	}
	// A refused contribution doesn't persist.
	w.ContributeToProject("hall", "ada", 0, "berries", 1, req, 1)
	if saves != 1 {
		t.Errorf("refused contribution persisted (saves=%d), want still 1", saves)
	}
}

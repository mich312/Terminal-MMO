package game

import (
	"strings"
	"testing"
)

// Every project phase costs real, gatherable goods, and the catalog's lookups
// stay in sync — the foundation the world's ContributeToProject and the wilds
// progress hint both rely on.
func TestProjectsCatalogValid(t *testing.T) {
	if len(Projects) == 0 {
		t.Fatal("no community projects defined")
	}
	for _, p := range Projects {
		if p.ID == "" || p.Name == "" || len(p.Phases) == 0 {
			t.Errorf("project %q is missing id/name/phases", p.ID)
		}
		if got, ok := ProjectByID(p.ID); !ok || got.ID != p.ID {
			t.Errorf("ProjectByID(%q) didn't round-trip", p.ID)
		}
		for i, ph := range p.Phases {
			if len(ph.Need) == 0 {
				t.Errorf("%s phase %d (%s) costs nothing", p.ID, i, ph.Name)
			}
			req := p.PhaseReq(i)
			for _, ing := range ph.Need {
				if _, ok := ItemByID(ing.Item); !ok {
					t.Errorf("%s phase %d needs unknown item %q", p.ID, i, ing.Item)
				}
				if ing.N <= 0 {
					t.Errorf("%s phase %d needs %d of %q, want > 0", p.ID, i, ing.N, ing.Item)
				}
				if req[ing.Item] != ing.N {
					t.Errorf("PhaseReq(%d)[%q] = %d, want %d", i, ing.Item, req[ing.Item], ing.N)
				}
			}
		}
		// A phase past the last one has no requirement (the build is finished).
		if p.PhaseReq(len(p.Phases)) != nil {
			t.Errorf("%s: PhaseReq past the last phase should be nil", p.ID)
		}
	}
}

// The progress line shows banked vs. needed for the current phase, and reads as
// complete once the build is done.
func TestProjectStatusLine(t *testing.T) {
	p, ok := ProjectByID("all-hands-hall")
	if !ok {
		t.Fatal("all-hands-hall missing from the catalog")
	}
	line := p.ProjectStatus(0, map[string]int{"stone": 5}, false)
	if !strings.Contains(line, "Foundation") || !strings.Contains(line, "5/30") {
		t.Errorf("phase-0 status = %q, want it to show Foundation and 5/30", line)
	}
	if done := p.ProjectStatus(len(p.Phases), nil, true); !strings.Contains(done, "complete") {
		t.Errorf("done status = %q, want it to read complete", done)
	}
}

package game

import (
	"fmt"
	"strings"
)

// Community projects (docs/COMMUNITY_PLAN.md): the staged communal builds the
// whole player base raises together — the co-op gate pool generalized into an
// ongoing, multi-resource build. This is the catalog: pure data plus the phase
// lookups the world's atomic ContributeToProject needs. The world owns the live
// pool and persistence and stays schema-agnostic; this owns what a build is,
// what each phase costs, and how its progress reads. Voice is corporate × medieval.

// ProjectPhase is one stage of a community build: a display name and the goods
// it consumes before the build advances to the next. Need reuses the recipe
// Ingredient type, so every cost is an inventory.go item ID.
type ProjectPhase struct {
	Name string
	Need []Ingredient
}

// ProjectDef is one community build's static definition. ID is shared with the
// world's live project state and the store row, so the catalog, the world and
// persistence all agree on which build is which.
type ProjectDef struct {
	ID     string
	Name   string
	Blurb  string // one deadpan line for the detail panel
	Phases []ProjectPhase
}

// Projects is the catalog, in display order. Every Need item is an item ID in
// inventory.go's Items, so a contribution spends from the pack like any good.
var Projects = []ProjectDef{
	{ID: "all-hands-hall", Name: "The All-Hands Hall",
		Blurb: "A great hall the whole company convenes in. Attendance, per charter, is mandatory.",
		Phases: []ProjectPhase{
			{Name: "Foundation", Need: []Ingredient{{"stone", 30}, {"wood", 20}}},
			{Name: "Frame", Need: []Ingredient{{"plank", 24}, {"wood", 30}}},
			{Name: "Roof & Fit-out", Need: []Ingredient{{"plank", 18}, {"ingot", 6}, {"lamp", 4}}},
		}},
}

// ProjectByID returns the catalog definition for a build id.
func ProjectByID(id string) (ProjectDef, bool) {
	for _, p := range Projects {
		if p.ID == id {
			return p, true
		}
	}
	return ProjectDef{}, false
}

// PhaseReq is the resource requirement of phase i as the id→count map the
// world's ContributeToProject expects, or nil once i is past the last phase
// (the build is finished).
func (p ProjectDef) PhaseReq(i int) map[string]int {
	if i < 0 || i >= len(p.Phases) {
		return nil
	}
	req := make(map[string]int, len(p.Phases[i].Need))
	for _, ing := range p.Phases[i].Need {
		req[ing.Item] = ing.N
	}
	return req
}

// ProjectStatus formats a one-line progress readout for the build at the given
// phase with the given banked pool (resource id → amount, as World.ProjectState
// reports it). A done or out-of-range phase reads as complete.
func (p ProjectDef) ProjectStatus(phase int, pool map[string]int, done bool) string {
	if done || phase >= len(p.Phases) {
		return p.Name + " — complete. Raised by the company."
	}
	ph := p.Phases[phase]
	parts := make([]string, 0, len(ph.Need))
	for _, ing := range ph.Need {
		parts = append(parts, fmt.Sprintf("%d/%d %s", pool[ing.Item], ing.N, itemLabel(ing.Item)))
	}
	return fmt.Sprintf("%s — %s: %s", p.Name, ph.Name, strings.Join(parts, " · "))
}

// itemLabel is an item's display name, falling back to its id.
func itemLabel(id string) string {
	if it, ok := ItemByID(id); ok {
		return it.Name
	}
	return id
}

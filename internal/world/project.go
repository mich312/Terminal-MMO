package world

// Community projects — the shared, asynchronous builds the whole player base
// raises together (docs/COMMUNITY_PLAN.md). This is the co-op gate pool
// generalized from a one-shot into an ongoing, staged build: instead of a
// single counter that flips a gate open, a project banks several resources
// toward the current phase and advances through a sequence of phases.
//
// As with the gate pool and the creature registry, the world owns *atomicity*
// and stays *schema-agnostic*: it never knows what a "timber" is or how many
// phases a build has — the caller (the game layer, from its project catalog)
// hands it the current phase's requirement and the total phase count, and the
// world serializes contributions so concurrent contributors can never
// double-count, overfill a phase, or trip two advancements off one completion.

// Project is the live state of one community build. Shared world state,
// persisted like the gate pool via a callback; handed out as copies so nobody
// mutates the shared instance outside the mutex.
type Project struct {
	ID    string         // stable project id, e.g. "all-hands-hall"
	Phase int            // 0-based index of the phase currently being built
	Pool  map[string]int // resource id -> amount banked toward the current phase
	Done  bool           // every phase complete; the build is finished
}

// clone returns a deep copy (its own Pool map), safe to hand out or persist
// without sharing the live instance's map.
func (p Project) clone() Project {
	pool := make(map[string]int, len(p.Pool))
	for k, v := range p.Pool {
		pool[k] = v
	}
	p.Pool = pool
	return p
}

// LoadProjects seeds the shared project set from persistence (called once at
// startup). SetProjectPersist wires saving back.
func (w *World) LoadProjects(ps []Project) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, p := range ps {
		c := p.clone()
		if c.Pool == nil {
			c.Pool = map[string]int{}
		}
		w.projects[p.ID] = &c
	}
}

// SetProjectPersist registers a callback to persist a project on change.
func (w *World) SetProjectPersist(fn func(Project)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.projectPersist = fn
}

// ProjectState returns a snapshot of project id, or ok=false if it has never
// been contributed to (no row yet — the caller treats that as phase 0, empty).
func (w *World) ProjectState(id string) (Project, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.projects[id]
	if !ok {
		return Project{ID: id, Pool: map[string]int{}}, false
	}
	return p.clone(), true
}

// ContributeToProject banks up to n of item toward project id's current phase
// and reports how much it actually accepted — the amount the caller should
// spend from the contributor's pack (0 ⇒ spend nothing).
//
// The contract mirrors OfferToGate but for a staged, multi-resource build:
//
//   - phase is an optimistic guard. The caller reads the project, computes the
//     requirement for the phase it saw, and passes that phase back; if a
//     concurrent contributor has since advanced the build, the guard is stale
//     and nothing is banked (accepted 0) — the caller recomputes and retries.
//   - req is the current phase's full requirement (resource id -> amount). The
//     world banks item up to its remaining room in req, never beyond, so a
//     phase can't be overfilled and the contributor never wastes goods.
//   - totalPhases lets the world mark the build Done when the last phase fills.
//
// advanced is true only on the contribution that completes the current phase —
// the one moment a milestone reward should fire and a notice should broadcast.
func (w *World) ContributeToProject(id, by string, phase int, item string, n int, req map[string]int, totalPhases int) (state Project, accepted int, advanced bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	p, ok := w.projects[id]
	if !ok {
		p = &Project{ID: id, Pool: map[string]int{}}
		w.projects[id] = p
	}

	// Nothing to do if the build is finished, the guard is stale, or this item
	// isn't wanted / is already satisfied this phase. Each returns accepted 0,
	// so the caller spends nothing.
	if p.Done || phase != p.Phase || n <= 0 {
		return p.clone(), 0, false
	}
	need := req[item]
	if need <= 0 {
		return p.clone(), 0, false
	}
	room := need - p.Pool[item]
	if room <= 0 {
		return p.clone(), 0, false
	}

	accepted = n
	if accepted > room {
		accepted = room
	}
	p.Pool[item] += accepted

	if phaseComplete(p.Pool, req) {
		p.Phase++
		p.Pool = map[string]int{}
		if p.Phase >= totalPhases {
			p.Done = true
		}
		advanced = true
	}

	if w.projectPersist != nil {
		w.projectPersist(p.clone())
	}

	// Community builds are world-wide news: deliver to every session (including
	// HD pollers, which handle events in their own loop), not just one area.
	w.broadcastAll(Event{Type: EventProjectContributed, Player: by, Detail: id, X: p.Phase})
	if advanced {
		w.broadcastAll(Event{Type: EventProjectAdvanced, Player: by, Detail: id, X: p.Phase})
	}
	return p.clone(), accepted, advanced
}

// phaseComplete reports whether pool satisfies every entry in req.
func phaseComplete(pool, req map[string]int) bool {
	for k, v := range req {
		if pool[k] < v {
			return false
		}
	}
	return true
}

// broadcastAll delivers an event to every subscriber, regardless of area —
// used for world-wide notices like a community build advancing. Callers hold
// the mutex (the same discipline as broadcastToArea).
func (w *World) broadcastAll(ev Event) {
	for _, ch := range w.subs {
		deliver(ch, ev)
	}
}

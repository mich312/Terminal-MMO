package world

// Legends — the shared registry of unique weapons (docs/WEAPON_PLAN.md). Each
// artifact exists once per world: it spawns in a single hidden spot, and the
// first player to reach it claims it for good. After that the world spot is
// empty forever and the weapon changes hands only through trade. Like land
// claims, this is small, shared, persisted-via-callback state; the game layer
// owns where an artifact hides, the world owns who got it first — atomically, so
// two finders racing for the same blade can't both walk away with it.

// LoadArtifacts seeds the claimed-artifact registry from persistence (id →
// discoverer), called once at startup.
func (w *World) LoadArtifacts(claimed map[string]string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, owner := range claimed {
		w.artifacts[id] = owner
	}
}

// SetArtifactPersist registers the callback that saves a freshly claimed
// artifact.
func (w *World) SetArtifactPersist(fn func(id, owner string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.artifactPersist = fn
}

// ArtifactClaimed reports whether a unique weapon has already been discovered
// (and by whom). The renderers use this to stop showing a claimed artifact in
// the world.
func (w *World) ArtifactClaimed(id string) (owner string, claimed bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	owner, claimed = w.artifacts[id]
	return owner, claimed
}

// ClaimArtifact records who first reached an artifact, returning false if it was
// already taken — the atomic first-write-wins that keeps a one-per-world weapon
// truly unique even under a two-player race.
func (w *World) ClaimArtifact(id, owner string) bool {
	w.mu.Lock()
	if _, taken := w.artifacts[id]; taken {
		w.mu.Unlock()
		return false
	}
	w.artifacts[id] = owner
	persist := w.artifactPersist
	w.mu.Unlock()
	if persist != nil {
		persist(id, owner)
	}
	return true
}

// ArtifactHolders returns a snapshot of every claimed artifact (id → discoverer)
// for the /legends rundown.
func (w *World) ArtifactHolders() map[string]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[string]string, len(w.artifacts))
	for id, owner := range w.artifacts {
		out[id] = owner
	}
	return out
}

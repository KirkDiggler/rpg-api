// Package dungeonregistry is the ONE live dungeon-spec registry shared, by
// pointer, between the lobby orchestrator (StartEncounter reads it) and the
// authoring orchestrator (PutDungeon writes it) — see
// ideas/dungeon-builder/plan.md's "Architecture decision: the shared live
// registry" for why this had to be extracted from lobby's own
// contentSpecs field rather than duplicated. A PutDungeon that mutated a
// second, private map would compile and pass its own tests while leaving
// StartEncounter reading stale content forever — this package exists
// specifically so there is only one map to mutate.
package dungeonregistry

import (
	"sort"
	"sync"

	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"
)

// Entry is one dungeon key's live state: either a successfully compiled
// spec (Compiled + its display Name, captured separately via
// dungeonspec.Decode — CompiledDungeon itself carries no Name, see
// plan.md S1's Name-capture note) or a load failure (Err), mirroring
// today's lobby-package contentSpecResult this registry generalizes.
type Entry struct {
	Compiled dungeonspec.CompiledDungeon
	Name     string
	Err      error
}

// DungeonSummary is one dungeon's key + display name — the exact shape
// ListDungeons needs, so Keys() below is a near-direct source for that
// RPC rather than something the lobby orchestrator has to re-derive.
type DungeonSummary struct {
	Key  string
	Name string
}

// Registry is the concurrency-safe live dungeon-spec store. Reads (Get,
// Keys) take an RLock; the one write path (Put) takes a full Lock — plain
// sync.RWMutex over a map, not an atomic-swap-of-immutable-map scheme,
// since PutDungeon's write rate is author-driven (at most one in flight at
// a time in practice) and StartEncounter's read rate never contends hard
// enough to need lock-free reads.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

// New builds a Registry from initial, the startup snapshot
// cmd/server/server.go builds once (today's loadContentSpecs logic,
// moved here). initial may be nil — an empty, immediately-usable registry,
// so callers never need a special-case nil guard.
func New(initial map[string]Entry) *Registry {
	entries := make(map[string]Entry, len(initial))
	for k, v := range initial {
		entries[k] = v
	}
	return &Registry{entries: entries}
}

// Get returns key's live Entry and whether it exists at all. A disabled
// (load-failed) key is still ok=true, its Err set — callers distinguish
// "key unknown" from "key known but broken" the same way lobby's
// contentSpecResult always has.
func (r *Registry) Get(key string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[key]
	return e, ok
}

// Put stores (or replaces) key's Entry — the registry swap half of
// PutDungeon's write-then-swap ordering (plan.md S1: write-through to
// RPG_CONTENT_DIR must succeed FIRST; only then does the caller call
// Put). Visible to the very next Get with no restart, which is this
// whole package's reason to exist.
func (r *Registry) Put(key string, e Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[key] = e
}

// Keys returns a summary (key + name) for every entry that loaded and
// compiled successfully (Err == nil), sorted by key for deterministic
// output — matching content.OverriddenKeys' sort.Strings precedent, so
// both test assertions and any future log line are stable. A disabled
// (load-failed) key is deliberately excluded: it has no reliable display
// name and offering an unplayable dungeon in ListDungeons' dropdown is
// worse than a temporarily-shorter list (plan.md S1's ListDungeons
// checklist item).
func (r *Registry) Keys() []DungeonSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summaries := make([]DungeonSummary, 0, len(r.entries))
	for key, e := range r.entries {
		if e.Err != nil {
			continue
		}
		summaries = append(summaries, DungeonSummary{Key: key, Name: e.Name})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Key < summaries[j].Key })
	return summaries
}

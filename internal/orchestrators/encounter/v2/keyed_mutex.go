package encounter

import "sync"

// keyedMutex serializes mutating operations per encounter ID. Used by
// DriveStalledNPCTurn (drive_npc.go) so concurrent StreamEncounter/GetEncounter
// connect-time kicks for the SAME encounter (e.g. four players reconnecting at
// once, rpg-api#636) serialize instead of each loading the same NPC-active
// snapshot independently and double-dispatching the NPC's turn.
//
// rpg-api is single-process (mirrors the lobby orchestrator's identical
// keyed_mutex.go, whose doc comment records the same assumption for the
// broker wiring), so a per-process, per-encounter-ID sync.Mutex is sufficient
// — no Redis WATCH/MULTI transaction is needed for a guarantee that only has
// to hold within one process. Duplicated here rather than shared with the
// lobby package's unexported type: both are ~20-line, package-private
// concurrency primitives with no other reason to couple the two packages.
//
// Known tradeoff: per-key *sync.Mutex entries are never evicted, so this map
// grows by one entry per distinct encounter ID for the life of the process. An
// encounter ID is minted once per StartEncounter/devseed call, so this is a
// slow, usage-bounded leak (not a hot loop) — acceptable for now.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{locks: make(map[string]*sync.Mutex)}
}

// Lock acquires the per-key lock and returns the unlock func. Callers must
// defer the returned func.
func (k *keyedMutex) Lock(key string) func() {
	k.mu.Lock()
	l, ok := k.locks[key]
	if !ok {
		l = &sync.Mutex{}
		k.locks[key] = l
	}
	k.mu.Unlock()

	l.Lock()
	return l.Unlock
}

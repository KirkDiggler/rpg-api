package lobby

import "sync"

// keyedMutex serializes mutating operations per lobby ID, giving the
// orchestrator its "atomic member-set snapshot" guarantee (lobby-surface.md
// "Start/leave atomicity": StartEncounter's membership snapshot must land
// atomically against a racing LeaveLobby).
//
// rpg-api is single-process (cmd/server/server.go's broker wiring comment
// makes the same assumption for the v2 encounter stack), so a per-process,
// per-lobby-ID sync.Mutex is sufficient here — no Redis WATCH/MULTI
// transaction is needed for a guarantee that only has to hold within one
// process. Every mutating orchestrator method (JoinLobby, SetReady,
// LeaveLobby, StartEncounter, SetConnected) locks on the resolved lobby ID
// before its load-mutate-save sequence.
//
// Known tradeoff: per-key *sync.Mutex entries are never evicted, so this map
// grows by one entry per distinct lobby ID for the life of the process. A
// lobby ID is a UUID minted once per CreateLobby call, so this is a slow,
// usage-bounded leak (not a hot loop) — acceptable for now; a follow-up
// could evict on lobby expiry if it ever matters in practice.
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

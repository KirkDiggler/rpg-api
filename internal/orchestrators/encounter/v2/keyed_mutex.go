package encounter

import "sync"

// keyedMutex serializes mutating operations per encounter ID. It is taken for
// the FULL load -> toolkit-verb -> persist span of every mutating orchestrator
// verb (MoveEntity, TakeAction, EndTurn, Interact, ActivateFeature,
// SetReactionReady, SubmitCheck, SubmitReactionCheck, DriveStalledNPCTurn), so
// two concurrent callers acting on the SAME encounter can never both load the
// same pre-mutation snapshot and race their Saves.
//
// rpg-api#787: MoveEntity (and every other mutating verb) was load -> mutate a
// private in-memory copy -> save the whole snapshot, with no lock and no
// version compare. Two players moving concurrently in free roam both read the
// same snapshot, each wrote back their own full copy, and the second Save
// silently erased the first player's already-applied move (confirmed live:
// the mover's own client believed it had moved; the server's persisted state
// disagreed, permanently, until a resnapshot). Originally this mutex guarded
// only DriveStalledNPCTurn (rpg-api#636, concurrent StreamEncounter/
// GetEncounter connect-time kicks for the same encounter double-dispatching
// an NPC turn); #787 generalized it to every mutating verb.
//
// Read-only paths (GetEncounter's snapshot read, StreamEncounter's equivalent)
// do NOT take this lock: a single repo Get is an atomically consistent
// point-in-time read (the repo's own internal mutex + JSON round-trip
// guarantee that), so there is no read-modify-write to guard. Their one
// side-effecting operation — the DriveStalledNPCTurn connect-time kick — is
// itself a locked verb.
//
// Lock-ordering invariant: no locked verb may itself invoke another locking
// entry point. driveNPCChain (end_turn.go), which MoveEntity/EndTurn/
// DriveStalledNPCTurn call from inside their own already-held lock, is a
// plain helper that never acquires this mutex — see its doc comment.
// DriveStalledNPCTurn is only ever invoked from the outer RPC connect paths
// (StreamEncounter, GetEncounter), never from inside another already-locked
// verb, so it is always the SOLE lock acquisition on its call stack. Reaction
// resume (SubmitCheck's take_reaction branch / SubmitReactionCheck) is always
// a separate, later RPC call — never a callback invoked while the pausing
// verb's lock is still held — so it acquires the lock fresh with no nesting.
// Violating this invariant (a locked verb calling another locked entry point
// on its own call stack) would deadlock a single-per-key sync.Mutex.
//
// rpg-api is single-process (mirrors the lobby orchestrator's identical
// keyed_mutex.go, whose doc comment records the same assumption for the
// broker wiring), so a per-process, per-encounter-ID sync.Mutex is sufficient
// — no Redis WATCH/MULTI transaction is needed for a guarantee that only has
// to hold within one process. Multi-process is a real follow-up (multiple
// rpg-api replicas): the proper fix there is a CAS in Repository.Save — an
// expected-version param using the persisted, monotonic Data.Sequence as the
// token, with a conflict mapping to codes.Aborted for client retry — not
// bigger locks. Duplicated here rather than shared with the lobby package's
// unexported type: both are ~20-line, package-private concurrency primitives
// with no other reason to couple the two packages.
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

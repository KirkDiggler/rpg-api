package lobby_test

import (
	"context"
	"errors"

	"go.uber.org/mock/gomock"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// observedLobbyRepository wraps a REAL lobby repository to observe — and, in
// the refusal and failure cases, inject — behavior at Save, the one write
// StartEncounter makes on the lobby record. It is the smallest wrapper that can
// prove ordering (save before publish) and count writes without a second
// repository implementation.
//
// beforeSave runs BEFORE the underlying write and before any injected error, so
// a test can assert the exact record the launch was about to persist and read
// the world at that moment. It is declared here because Task 3's contract case
// uses it first and the later failure cases need the same type.
type observedLobbyRepository struct {
	lobbyrepo.Repository

	// saveCalls counts Save invocations through this wrapper.
	saveCalls int

	// beforeSave, when set, observes data immediately before the write.
	beforeSave func(*lobbyrepo.Data)

	// saveErr, when set, fails the write with this error instead of persisting.
	saveErr error
}

func (r *observedLobbyRepository) Save(ctx context.Context, data *lobbyrepo.Data) error {
	r.saveCalls++
	if r.beforeSave != nil {
		r.beforeSave(data)
	}
	if r.saveErr != nil {
		return r.saveErr
	}
	return r.Repository.Save(ctx, data)
}

// failingLobbyGet overrides Get alone on a nil embedded repository: the launch
// must stop before it writes anything, so Save (and every other method) is
// never reached and the nil embedding is never touched. It exists to prove an
// ARBITRARY repository read failure stays matchable through the launch's
// contextual wrapping rather than being mistaken for a missing lobby.
type failingLobbyGet struct {
	lobbyrepo.Repository
	err error
}

func (r failingLobbyGet) Get(context.Context, string) (*lobbyrepo.Data, error) {
	return nil, r.err
}

// assertNoEvent fails the case if anything was published to an unsuccessful
// launch's lobby stream. Checked synchronously against the buffered channel:
// Publish is synchronous fan-out, so an empty channel right now is proof, with
// no sleep and no polling.
func (s *StartContractSuite) assertNoEvent(sub *lobbyorch.Subscription) {
	s.T().Helper()
	select {
	case event := <-sub.Events():
		s.Failf("an unsuccessful launch published an event", "%+v", event)
	default:
	}
}

// refusedLaunch seeds the lobby (nil leaves it absent), routes a launch's one
// lobby write through a Save-observing wrapper, subscribes to the lobby's event
// stream, runs StartEncounter and asserts the whole contract every refusal
// shares: an error and no encounter id, ZERO Save attempts, the stored record
// exactly as seeded (or still absent), and no published event. The error is
// returned so the case asserts WHICH refusal fired.
//
// Neither mock gets a permissive default. The registry and SDK expectations are
// armed only where the case itself arms the gate that is supposed to fire, so a
// gate that regressed into a lookup, an SDK call, or a write fails the case
// through the controller-isolated mock.
func (s *StartContractSuite) refusedLaunch(
	lobbyID string, seed *lobbyrepo.Data, in *lobbyorch.StartEncounterInput,
) error {
	s.T().Helper()

	if seed != nil {
		s.seedLobby(seed)
	}
	var before *lobbyrepo.Data
	if seed != nil {
		before = s.snapshotLobby(lobbyID)
	}

	observed := &observedLobbyRepository{Repository: s.lobbyRepo}
	orch := s.newOrchestratorWithLobbyRepo(observed)

	sub, err := s.broker.Subscribe(lobbyID)
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	out, err := orch.StartEncounter(s.ctx, in)
	s.Require().Error(err, "the launch must be refused")
	s.Nil(out, "a refused launch returns no encounter id")

	s.Zero(observed.saveCalls, "a refused launch writes the lobby zero times")
	if seed != nil {
		s.Equal(before, s.snapshotLobby(lobbyID), "and leaves the stored lobby exactly as it was")
	} else {
		_, getErr := s.lobbyRepo.Get(s.ctx, lobbyID)
		s.Require().ErrorIs(getErr, lobbyrepo.ErrNotFound,
			"and a lobby that never existed is still absent")
	}
	s.assertNoEvent(sub)

	return err
}

// failingLaunchFixture is the shared setup for every partial-launch-failure
// case: the ready contract lobby seeded, the repository wrapped so the case can
// observe (and, for the final-write case, inject) the launch's one lobby write,
// the orchestrator built over that wrapper, a snapshot of the seeded record,
// and a live subscription to the lobby's event stream.
func (s *StartContractSuite) failingLaunchFixture() (
	*observedLobbyRepository, *lobbyorch.Orchestrator, *lobbyrepo.Data, *lobbyorch.Subscription,
) {
	s.T().Helper()

	s.seedLobby(readyContractLobby())
	before := s.snapshotLobby(contractLobbyID)

	observed := &observedLobbyRepository{Repository: s.lobbyRepo}
	orch := s.newOrchestratorWithLobbyRepo(observed)

	sub, err := s.broker.Subscribe(contractLobbyID)
	s.Require().NoError(err)

	return observed, orch, before, sub
}

// launchSuccessCalls returns the ordered expectations for the successful prefix
// of a contract launch: the registry lookup for key and the first n SDK verbs,
// in the order the launch promises them — StartSession, Spawn guard-a, Spawn
// guard-b, Join char-b, Join char-a, and (for the reference tomb) PlaceNPC the
// demo vendor. Every armed verb returns a concrete EMPTY success output, so a
// case can append ONE failing expectation and run gomock.InOrder over the whole
// sequence: the failure is ordered after every success the launch must make
// first, and any verb past it is unarmed and fails the controller-isolated mock.
//
// n counts SDK verbs and does not include the registry lookup, so n == 0 arms
// the lookup alone, leaving StartSession as the failing call.
func (s *StartContractSuite) launchSuccessCalls(
	entry *dungeons.Entry, key string, withVendor bool, n int,
) []any {
	s.T().Helper()

	// Built lazily: an EXPECT() that is never armed must not be recorded, or the
	// suite would carry expectations for verbs the launch is supposed to skip.
	verbs := []func() *gomock.Call{
		func() *gomock.Call {
			return s.manager.EXPECT().StartSession(s.ctx, gomock.Any()).
				Return(&sdk.StartSessionOutput{}, nil)
		},
		func() *gomock.Call {
			return s.manager.EXPECT().Spawn(s.ctx, gomock.Any()).
				Return(&sdk.SpawnOutput{}, nil)
		},
		func() *gomock.Call {
			return s.manager.EXPECT().Spawn(s.ctx, gomock.Any()).
				Return(&sdk.SpawnOutput{}, nil)
		},
		func() *gomock.Call {
			return s.manager.EXPECT().Join(s.ctx, gomock.Any()).
				Return(&sdk.JoinOutput{}, nil)
		},
		func() *gomock.Call {
			return s.manager.EXPECT().Join(s.ctx, gomock.Any()).
				Return(&sdk.JoinOutput{}, nil)
		},
	}
	if withVendor {
		verbs = append(verbs, func() *gomock.Call {
			return s.manager.EXPECT().PlaceNPC(s.ctx, gomock.Any()).
				Return(&sdk.PlaceNPCOutput{}, nil)
		})
	}

	calls := []any{s.registry.EXPECT().Get(s.ctx, key).Return(entry, nil)}
	for i := 0; i < n; i++ {
		calls = append(calls, verbs[i]())
	}
	return calls
}

// --- Gates: refused before any SDK verb, write, or event -------------------

// TestStartEncounter_NilInput_Refused pins the very first gate: no input at all
// is refused before the lobby is even locked or read.
func (s *StartContractSuite) TestStartEncounter_NilInput_Refused() {
	err := s.refusedLaunch(contractLobbyID, readyContractLobby(), nil)
	s.Require().ErrorContains(err, "StartEncounterInput is required")
}

// TestStartEncounter_MissingLobby_Refused pins the absent-lobby gate: the launch
// reports ErrLobbyNotFound and the lobby — which never existed — is still gone
// afterwards, rather than compared against a snapshot that cannot be taken.
func (s *StartContractSuite) TestStartEncounter_MissingLobby_Refused() {
	err := s.refusedLaunch(contractLobbyID, nil, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID,
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotFound)
}

// TestStartEncounter_AlreadyStarted_Refused pins the terminal-status gate: a
// STARTED lobby refuses a repeat launch even from its own host, and the stored
// record keeps the encounter it already started.
func (s *StartContractSuite) TestStartEncounter_AlreadyStarted_Refused() {
	started := readyContractLobby()
	started.Status = lobbyrepo.StatusStarted
	started.EncounterID = "enc-existing"

	err := s.refusedLaunch(contractLobbyID, started, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID,
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyAlreadyStarted)
}

// TestStartEncounter_NonHost_Refused pins the host gate: a member who is not the
// host cannot launch, and the ready lobby is untouched.
func (s *StartContractSuite) TestStartEncounter_NonHost_Refused() {
	err := s.refusedLaunch(contractLobbyID, readyContractLobby(), &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: contractLobbyID,
	})
	s.Require().ErrorIs(err, lobbyorch.ErrNotHost)
}

// TestStartEncounter_NotReadyMember_Refused pins the all-ready gate: one member
// who is not ready refuses the launch, before the dungeon is even resolved.
func (s *StartContractSuite) TestStartEncounter_NotReadyMember_Refused() {
	notReady := readyContractLobby()
	notReady.Members["bob"].IsReady = false

	err := s.refusedLaunch(contractLobbyID, notReady, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID,
	})
	s.Require().ErrorIs(err, lobbyorch.ErrNotAllReady)
}

// TestStartEncounter_UnknownDungeonKey_Refused pins design §3c: a key the
// registry does not have is ErrDungeonNotFound, never silently the tomb. The
// lookup is the last thing the launch does.
func (s *StartContractSuite) TestStartEncounter_UnknownDungeonKey_Refused() {
	const key = "nope"
	s.registry.EXPECT().Get(s.ctx, key).Return(nil, dungeons.ErrNotFound)

	err := s.refusedLaunch(contractLobbyID, readyContractLobby(), &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Require().ErrorIs(err, lobbyorch.ErrDungeonNotFound)
}

// TestStartEncounter_RegistryGetFailure_IsWrappedAndStops pins the non-sentinel
// registry control: an arbitrary lookup failure stays matchable through the
// wrapping and is NOT reported as a missing dungeon.
func (s *StartContractSuite) TestStartEncounter_RegistryGetFailure_IsWrappedAndStops() {
	boom := errors.New("registry unavailable")
	const key = "custom-dungeon"
	s.registry.EXPECT().Get(s.ctx, key).Return(nil, boom)

	err := s.refusedLaunch(contractLobbyID, readyContractLobby(), &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Require().ErrorIs(err, boom)
	s.Require().NotErrorIs(err, lobbyorch.ErrDungeonNotFound, "an outage is not a missing dungeon")
}

// TestStartEncounter_LobbyRepoGetFailure_IsWrappedAndStops uses the Get-only
// wrapper the brief prescribes: an arbitrary repository read failure stays
// matchable and is not mistaken for ErrLobbyNotFound. No SDK or final-Save
// expectation is armed; the launch must stop at the read, so the wrapper's nil
// embedded repository is never reached.
func (s *StartContractSuite) TestStartEncounter_LobbyRepoGetFailure_IsWrappedAndStops() {
	boom := errors.New("lobby store unavailable")

	sub, err := s.broker.Subscribe(contractLobbyID)
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	orch := s.newOrchestratorWithLobbyRepo(failingLobbyGet{err: boom})

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID,
	})
	s.Nil(out)
	s.Require().ErrorIs(err, boom)
	s.Require().NotErrorIs(err, lobbyorch.ErrLobbyNotFound)
	s.assertNoEvent(sub)
}

// TestStartEncounter_InsufficientSeats_Refused pins the entrance gate: a party
// larger than the dungeon seats is refused BEFORE anything is written, rather
// than being half-seated.
func (s *StartContractSuite) TestStartEncounter_InsufficientSeats_Refused() {
	const key = "tiny-dungeon"
	entry := contractEntry(key)
	entry.Dungeon.PartySeats = entry.Dungeon.PartySeats[:1] // one seat, two members
	s.registry.EXPECT().Get(s.ctx, key).Return(entry, nil)

	err := s.refusedLaunch(contractLobbyID, readyContractLobby(), &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Require().ErrorContains(err, "seats 1")
	s.Require().NotErrorIs(err, lobbyorch.ErrDungeonNotFound)
}

// TestStartEncounter_DuplicatePartyIDs_Refused pins the one-id-one-member check
// against the party itself: two members claiming one character id are refused
// before anything is written.
func (s *StartContractSuite) TestStartEncounter_DuplicatePartyIDs_Refused() {
	const key = "custom-dungeon"
	s.registry.EXPECT().Get(s.ctx, key).Return(contractEntry(key), nil)

	duplicated := readyContractLobby()
	duplicated.Members["bob"].CharacterID = "char-a"

	err := s.refusedLaunch(contractLobbyID, duplicated, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Require().ErrorContains(err, "claimed twice")
	s.Require().ErrorContains(err, "char-a")
}

// TestStartEncounter_PartyMonsterIDCollision_Refused pins the same check
// crossing the party/monster seam: a character whose id equals an authored
// monster member id is refused, naming both claimants.
func (s *StartContractSuite) TestStartEncounter_PartyMonsterIDCollision_Refused() {
	const key = "custom-dungeon"
	s.registry.EXPECT().Get(s.ctx, key).Return(contractEntry(key), nil)

	colliding := readyContractLobby()
	colliding.Members["alice"].CharacterID = "guard-a"

	err := s.refusedLaunch(contractLobbyID, colliding, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Require().ErrorContains(err, "claimed twice")
	s.Require().ErrorContains(err, "guard-a")
}

// TestStartEncounter_PartyDemoVendorIDCollision_Refused pins the third claimant
// the launch knows about: on the reference tomb the temporary demo vendor's id
// is reserved, so a character claiming it is refused before any write.
func (s *StartContractSuite) TestStartEncounter_PartyDemoVendorIDCollision_Refused() {
	s.registry.EXPECT().Get(s.ctx, dungeons.DefaultKey).
		Return(contractEntry(dungeons.DefaultKey), nil)

	colliding := readyContractLobby()
	colliding.Members["alice"].CharacterID = contractVendorMemberID

	err := s.refusedLaunch(contractLobbyID, colliding, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID,
	})
	s.Require().ErrorContains(err, "claimed twice")
	s.Require().ErrorContains(err, contractVendorMemberID)
}

// --- Partial-launch failures: stop calling, write nothing, announce nothing -

// TestStartEncounter_StartSessionFailure_StopsAndWraps is the first SDK failure:
// only the registry lookup and StartSession are expected. Spawn, Join, PlaceNPC
// and Save are all unarmed, so the failure proves the launch stops rather than
// continuing into a session nobody agreed to.
func (s *StartContractSuite) TestStartEncounter_StartSessionFailure_StopsAndWraps() {
	boom := errors.New("provider unavailable")
	const key = "custom-dungeon"
	entry := contractEntry(key)

	observed, orch, before, sub := s.failingLaunchFixture()
	defer func() { _ = sub.Close() }()

	calls := s.launchSuccessCalls(entry, key, false, 0)
	calls = append(calls, s.manager.EXPECT().StartSession(s.ctx, gomock.Any()).Return(nil, boom))
	gomock.InOrder(calls...)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Nil(out)
	s.Require().ErrorIs(err, boom, "the provider failure stays matchable through the wrapping")
	s.Zero(observed.saveCalls, "a launch stopped at the first verb writes the lobby zero times")
	s.Equal(before, s.snapshotLobby(contractLobbyID), "and leaves the stored lobby exactly as it was")
	s.assertNoEvent(sub)
}

// TestStartEncounter_FirstSpawnFailure_StopsAndWraps: StartSession succeeded, the
// first Spawn failed. The second Spawn is unarmed — the launch must not place
// the rest of the garrison, join anyone, or write.
func (s *StartContractSuite) TestStartEncounter_FirstSpawnFailure_StopsAndWraps() {
	boom := errors.New("spawn refused")
	const key = "custom-dungeon"
	entry := contractEntry(key)

	observed, orch, before, sub := s.failingLaunchFixture()
	defer func() { _ = sub.Close() }()

	calls := s.launchSuccessCalls(entry, key, false, 1)
	calls = append(calls, s.manager.EXPECT().Spawn(s.ctx, gomock.Any()).Return(nil, boom))
	gomock.InOrder(calls...)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Nil(out)
	s.Require().ErrorIs(err, boom)
	s.Zero(observed.saveCalls)
	s.Equal(before, s.snapshotLobby(contractLobbyID))
	s.assertNoEvent(sub)
}

// TestStartEncounter_SecondSpawnFailure_StopsAndWraps: the FIRST monster placed
// successfully; the second Spawn failed. The joins are unarmed — the party never
// arrives, nothing is written, nothing is announced.
func (s *StartContractSuite) TestStartEncounter_SecondSpawnFailure_StopsAndWraps() {
	boom := errors.New("spawn refused")
	const key = "custom-dungeon"
	entry := contractEntry(key)

	observed, orch, before, sub := s.failingLaunchFixture()
	defer func() { _ = sub.Close() }()

	calls := s.launchSuccessCalls(entry, key, false, 2)
	calls = append(calls, s.manager.EXPECT().Spawn(s.ctx, gomock.Any()).Return(nil, boom))
	gomock.InOrder(calls...)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Nil(out)
	s.Require().ErrorIs(err, boom)
	s.Zero(observed.saveCalls)
	s.Equal(before, s.snapshotLobby(contractLobbyID))
	s.assertNoEvent(sub)
}

// TestStartEncounter_FirstJoinFailure_StopsAndWraps: both monsters placed, the
// first party member's Join failed. The second Join is unarmed — the party stops
// arriving, nothing is written, nothing is announced.
func (s *StartContractSuite) TestStartEncounter_FirstJoinFailure_StopsAndWraps() {
	boom := errors.New("join refused")
	const key = "custom-dungeon"
	entry := contractEntry(key)

	observed, orch, before, sub := s.failingLaunchFixture()
	defer func() { _ = sub.Close() }()

	calls := s.launchSuccessCalls(entry, key, false, 3)
	calls = append(calls, s.manager.EXPECT().Join(s.ctx, gomock.Any()).Return(nil, boom))
	gomock.InOrder(calls...)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Nil(out)
	s.Require().ErrorIs(err, boom)
	s.Zero(observed.saveCalls)
	s.Equal(before, s.snapshotLobby(contractLobbyID))
	s.assertNoEvent(sub)
}

// TestStartEncounter_SecondJoinFailure_StopsAndWraps: the first member joined,
// the second Join failed. No PlaceNPC expectation is armed (this key places no
// vendor), so nothing is left but the write the launch must not reach.
func (s *StartContractSuite) TestStartEncounter_SecondJoinFailure_StopsAndWraps() {
	boom := errors.New("join refused")
	const key = "custom-dungeon"
	entry := contractEntry(key)

	observed, orch, before, sub := s.failingLaunchFixture()
	defer func() { _ = sub.Close() }()

	calls := s.launchSuccessCalls(entry, key, false, 4)
	calls = append(calls, s.manager.EXPECT().Join(s.ctx, gomock.Any()).Return(nil, boom))
	gomock.InOrder(calls...)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Nil(out)
	s.Require().ErrorIs(err, boom)
	s.Zero(observed.saveCalls)
	s.Equal(before, s.snapshotLobby(contractLobbyID))
	s.assertNoEvent(sub)
}

// TestStartEncounter_PlaceNPCFailure_StopsAndWraps uses the reference tomb so
// the failing verb is the LAST SDK call before the write: both guards placed,
// both members joined, then PlaceNPC fails. The write is still unarmed.
func (s *StartContractSuite) TestStartEncounter_PlaceNPCFailure_StopsAndWraps() {
	boom := errors.New("vendor placement refused")
	entry := contractEntry(dungeons.DefaultKey)

	observed, orch, before, sub := s.failingLaunchFixture()
	defer func() { _ = sub.Close() }()

	calls := s.launchSuccessCalls(entry, dungeons.DefaultKey, true, 5)
	calls = append(calls, s.manager.EXPECT().PlaceNPC(s.ctx, gomock.Any()).Return(nil, boom))
	gomock.InOrder(calls...)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID,
	})
	s.Nil(out)
	s.Require().ErrorIs(err, boom)
	s.Zero(observed.saveCalls)
	s.Equal(before, s.snapshotLobby(contractLobbyID))
	s.assertNoEvent(sub)
}

// TestStartEncounter_FinalSaveFailure_WritesNothingAndPublishesNothing is the
// last failure the launch can have: every SDK verb succeeded and the lobby write
// failed. The launch reports the failure, the backing store is unchanged, and —
// the load-bearing half — no EncounterStarted event is announced for an
// encounter the lobby record never recorded.
func (s *StartContractSuite) TestStartEncounter_FinalSaveFailure_WritesNothingAndPublishesNothing() {
	boom := errors.New("lobby store unavailable")
	const key = "custom-dungeon"
	entry := contractEntry(key)

	observed, orch, before, sub := s.failingLaunchFixture()
	defer func() { _ = sub.Close() }()
	observed.saveErr = boom

	gomock.InOrder(s.launchSuccessCalls(entry, key, false, 5)...)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Nil(out)
	s.Require().ErrorIs(err, boom)
	s.Equal(1, observed.saveCalls, "the launch attempts the lobby write exactly once")
	s.Equal(before, s.snapshotLobby(contractLobbyID),
		"a failed write leaves the backing store exactly as it was")
	s.assertNoEvent(sub)
}

// TestStartEncounter_UnsupportedArrival_StopsBeforeTheSecondSpawn pins the one
// failure that is NOT an SDK error: the second monster's arrival predicate is a
// form this launch cannot translate, so it refuses by type after placing the
// first monster — before that monster's Spawn, before any Join, before the write.
// Only the lookup, StartSession and the first Spawn are expected; everything
// past them is unarmed.
func (s *StartContractSuite) TestStartEncounter_UnsupportedArrival_StopsBeforeTheSecondSpawn() {
	const key = "bad-arrival"
	entry := contractEntry(key)
	entry.Dungeon.Monsters[1].Arrives = tkencounter.TriggerExternal{}

	observed, orch, before, sub := s.failingLaunchFixture()
	defer func() { _ = sub.Close() }()

	gomock.InOrder(s.launchSuccessCalls(entry, key, false, 2)...)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Nil(out)
	s.Require().ErrorContains(err, "TriggerExternal",
		"the unsupported predicate is named by type, not swallowed")
	s.Zero(observed.saveCalls)
	s.Equal(before, s.snapshotLobby(contractLobbyID))
	s.assertNoEvent(sub)
}

package lobby_test

import (
	"errors"
	"fmt"

	"go.uber.org/mock/gomock"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// seedStartedLobby builds a STARTED lobby whose EncounterID names a session
// that DOES NOT EXIST: the session's state is scripted per test through
// s.manager instead of being made true by starting one, because these tests
// are about what the lobby does with the SDK's answer (the exact End it
// sends, which failures it translates, what it leaves alone), not about
// whether the SDK really closes a session — that is the SDK's own suite.
func (s *LobbySuite) seedStartedLobby(lobbyID, host, hostCharacterID, encID string) {
	s.seedLobby(&lobbyrepo.Data{
		ID: lobbyID, HostPlayerID: host, Status: lobbyrepo.StatusStarted, EncounterID: encID,
		Members:     map[string]*lobbyrepo.Member{host: {PlayerID: host, CharacterID: hostCharacterID, IsHost: true}},
		MemberOrder: []string{host},
	})
}

// snapshotLobby reads the stored lobby record for id. The in-memory
// repository serializes on every write and every read (see
// lobbyrepo.NewInMemory), so the returned value is an independent copy —
// comparing one taken before a call with one taken after is a real deep
// check that the call did not touch the record.
func (s *LobbySuite) snapshotLobby(id string) *lobbyrepo.Data {
	s.T().Helper()
	data, err := s.lobbyRepo.Get(s.ctx, id)
	s.Require().NoError(err)
	return data
}

func (s *LobbySuite) TestAbandonEncounter_Success_EndsEncounter() {
	s.seedStartedLobby("lobby-a1", "alice", "char-alice", "enc-a1")
	before := s.snapshotLobby("lobby-a1")

	s.manager.EXPECT().
		End(s.ctx, &sdk.EndInput{Session: "enc-a1", Ending: sessionworld.EndingWithdrawn}).
		Return(&sdk.EndOutput{}, nil)

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a1",
	})
	s.Require().NoError(err)
	s.Require().Equal(before, s.snapshotLobby("lobby-a1"),
		"abandon ends the session; it deliberately never writes the lobby record")

	// The session now reads closed, so the lobby has nothing left to resume
	// into. GetMyActiveLobby's own liveness check (get_my_active_lobby.go) is
	// the honest cross-check that the abandon took effect at the boundary this
	// package owns: the SDK says closed, and the API reports no active lobby.
	s.manager.EXPECT().
		Status(s.ctx, &sdk.StatusInput{Session: "enc-a1"}).
		Return(&sdk.Status{Open: false}, nil)
	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Empty(out.LobbyID, "the session must persist as ended")
}

func (s *LobbySuite) TestAbandonEncounter_NotHost_PermissionDenied() {
	// No End expectation: the host gate refuses before the session is
	// touched, and the controller-isolated mock fails the test if that gate
	// ever regresses into an SDK call.
	s.seedStartedLobby("lobby-a3", "alice", "char-alice", "enc-a3")
	lobbyData, err := s.lobbyRepo.Get(s.ctx, "lobby-a3")
	s.Require().NoError(err)
	lobbyData.Members["bob"] = &lobbyrepo.Member{PlayerID: "bob", CharacterID: "char-bob"}
	lobbyData.MemberOrder = append(lobbyData.MemberOrder, "bob")
	s.Require().NoError(s.lobbyRepo.Save(s.ctx, lobbyData))

	_, err = s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-a3",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrNotHost)
}

func (s *LobbySuite) TestAbandonEncounter_LobbyNotFound() {
	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "no-such-lobby",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotFound)
}

func (s *LobbySuite) TestAbandonEncounter_WaitingLobby_ErrLobbyNotStarted() {
	// No End expectation: a WAITING lobby has no session to end, so the
	// lifecycle gate must refuse without an SDK call.
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-a4", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a4",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotStarted)
}

// TestAbandonEncounter_AlreadyEnded_ErrEncounterAlreadyEnded is the
// negative control: a second abandon attempt against a session the SDK
// reports as already closed must reject, not silently re-transition or
// double-publish.
func (s *LobbySuite) TestAbandonEncounter_AlreadyEnded_ErrEncounterAlreadyEnded() {
	s.seedStartedLobby("lobby-a5", "alice", "char-alice", "enc-a5")
	before := s.snapshotLobby("lobby-a5")

	s.manager.EXPECT().
		End(s.ctx, &sdk.EndInput{Session: "enc-a5", Ending: sessionworld.EndingWithdrawn}).
		Return(&sdk.EndOutput{}, nil)
	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a5",
	})
	s.Require().NoError(err)

	s.manager.EXPECT().
		End(s.ctx, &sdk.EndInput{Session: "enc-a5", Ending: sessionworld.EndingWithdrawn}).
		Return(nil, fmt.Errorf("provider: %w", sdk.ErrClosed))

	_, err = s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a5",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrEncounterAlreadyEnded)
	s.Require().Equal(before, s.snapshotLobby("lobby-a5"),
		"a refused abandon must leave the lobby record exactly as it was")
}

// TestAbandonEncounter_SessionGone_ErrEncounterAlreadyEnded covers the
// sentinel the SDK uses when the session record itself is gone (expired, or
// never written): the same caller-visible outcome as an already-closed
// session, because either way there is nothing left to abandon.
func (s *LobbySuite) TestAbandonEncounter_SessionGone_ErrEncounterAlreadyEnded() {
	s.seedStartedLobby("lobby-a7", "alice", "char-alice", "enc-a7")
	before := s.snapshotLobby("lobby-a7")

	s.manager.EXPECT().
		End(s.ctx, &sdk.EndInput{Session: "enc-a7", Ending: sessionworld.EndingWithdrawn}).
		Return(nil, fmt.Errorf("provider: %w", sdk.ErrNoSession))

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a7",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrEncounterAlreadyEnded)
	s.Require().Equal(before, s.snapshotLobby("lobby-a7"))
}

// TestAbandonEncounter_EncounterGone_ErrEncounterAlreadyEnded is the
// ErrNoEncounter sibling of the case above: the session exists but the
// encounter record it references does not.
func (s *LobbySuite) TestAbandonEncounter_EncounterGone_ErrEncounterAlreadyEnded() {
	s.seedStartedLobby("lobby-a8", "alice", "char-alice", "enc-a8")
	before := s.snapshotLobby("lobby-a8")

	s.manager.EXPECT().
		End(s.ctx, &sdk.EndInput{Session: "enc-a8", Ending: sessionworld.EndingWithdrawn}).
		Return(nil, fmt.Errorf("provider: %w", sdk.ErrNoEncounter))

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a8",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrEncounterAlreadyEnded)
	s.Require().Equal(before, s.snapshotLobby("lobby-a8"))
}

// TestAbandonEncounter_EndFails_ReturnsWrappedError is the non-sentinel
// control: an SDK failure this package does NOT recognize as "nothing left
// to abandon" must surface as an error to the host rather than being
// reported as a successful abandon — a host believing a stuck session ended
// when it did not is worse than the error.
func (s *LobbySuite) TestAbandonEncounter_EndFails_ReturnsWrappedError() {
	boom := errors.New("provider unavailable")
	s.seedStartedLobby("lobby-a9", "alice", "char-alice", "enc-a9")
	before := s.snapshotLobby("lobby-a9")

	s.manager.EXPECT().
		End(s.ctx, &sdk.EndInput{Session: "enc-a9", Ending: sessionworld.EndingWithdrawn}).
		Return(nil, boom)

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a9",
	})
	s.Require().ErrorIs(err, boom, "an unrecognized SDK failure must remain matchable through the wrapping")
	s.Require().NotErrorIs(err, lobbyorch.ErrEncounterAlreadyEnded,
		"only the three 'nothing left to abandon' sentinels translate; an outage does not")
	s.Require().Equal(before, s.snapshotLobby("lobby-a9"))
}

// TestAbandonEncounter_ThenGetMyActiveLobby_ReturnsEmpty proves the resume
// half: after the host abandons, GetMyActiveLobby reports nothing to resume
// into, exercising the SAME session.Manager.Status this package's own
// get_my_active_lobby.go queries. The expectations are ordered — End first,
// then the liveness read — because that IS the caller-visible sequence.
//
// It does NOT re-prove that the toolkit's End closes a session (the SDK's
// sentinel/closed behavior is its own suite's claim), and it does not
// re-prove first-admission recovery (a fresh Join persisting a dead
// character at full HP with restored resources): that policy lives in the
// Session SDK and is proven through Lobby StartEncounter by
// TestStartEncounter_FirstAdmissionPersistsCompleteLongRestOutcomes.
func (s *LobbySuite) TestAbandonEncounter_ThenGetMyActiveLobby_ReturnsEmpty() {
	s.seedStartedLobby("lobby-a6", "alice", "char-alice", "enc-a6")
	before := s.snapshotLobby("lobby-a6")

	sub, err := s.broker.Subscribe("lobby-a6")
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	gomock.InOrder(
		s.manager.EXPECT().
			End(s.ctx, &sdk.EndInput{Session: "enc-a6", Ending: sessionworld.EndingWithdrawn}).
			Return(&sdk.EndOutput{}, nil),
		s.manager.EXPECT().
			Status(s.ctx, &sdk.StatusInput{Session: "enc-a6"}).
			Return(&sdk.Status{Open: false}, nil),
	)

	_, err = s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a6",
	})
	s.Require().NoError(err)

	// No lobby broadcast from an abandon: the ENDED session announces itself
	// on the session stream, so a second lobby event would double-signal the
	// same transition. Checked synchronously against the buffered channel —
	// Publish is synchronous fan-out, so an empty channel right now is proof,
	// with no sleep and no polling.
	select {
	case evt := <-sub.Events():
		s.Require().Fail("abandonment must not publish a lobby event", "got %v", evt)
	default:
	}

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Empty(out.LobbyID, "abandoned lobby must not resolve as an active lobby")
	s.Require().Empty(out.EncounterID)
	s.Require().Equal(lobbyrepo.StatusUnspecified, out.Status)
	s.Require().Equal(before, s.snapshotLobby("lobby-a6"),
		"abandon never writes the lobby record, before or after the resume read")
}

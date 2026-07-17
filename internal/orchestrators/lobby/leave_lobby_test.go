package lobby_test

import (
	"context"
	"errors"

	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// clearIndexFailingRepo wraps a real Repository and forces every
// ClearPlayerIndex call to fail, so tests can pin LeaveLobby's behavior when
// the index-cleanup step hits an infra error after the membership mutation
// (Save) has already durably succeeded. Not a gomock double — a small
// hand-rolled wrapper is simpler than a full interface mock for one
// overridden method (feedback_test_double_naming: reserve "mock" for
// gomock-generated types).
type clearIndexFailingRepo struct {
	lobbyrepo.Repository
}

func (r *clearIndexFailingRepo) ClearPlayerIndex(context.Context, string) error {
	return errors.New("clearIndexFailingRepo: simulated infra failure")
}

func (s *LobbySuite) TestLeaveLobby_NonHostLeaves_RemovesSeat() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-l1", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", IsHost: true},
			"bob":   {PlayerID: "bob"},
		},
		MemberOrder: []string{"alice", "bob"},
	})

	_, err := s.orch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "bob", LobbyID: "lobby-l1"})
	s.Require().NoError(err)

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-l1")
	s.Require().NoError(err)
	s.Require().NotContains(data.Members, "bob")
	s.Require().Equal([]string{"alice"}, data.MemberOrder)
	s.Require().Equal("alice", data.HostPlayerID, "host must not change when a non-host leaves")
}

func (s *LobbySuite) TestLeaveLobby_HostLeaves_MigratesToOldestRemaining() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-l2", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", IsHost: true},
			"bob":   {PlayerID: "bob"},
			"carol": {PlayerID: "carol"},
		},
		MemberOrder: []string{"alice", "bob", "carol"},
	})

	_, err := s.orch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "alice", LobbyID: "lobby-l2"})
	s.Require().NoError(err)

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-l2")
	s.Require().NoError(err)
	s.Require().NotContains(data.Members, "alice")
	s.Require().Equal("bob", data.HostPlayerID, "oldest remaining member (bob) must become host")
	s.Require().True(data.Members["bob"].IsHost)
	s.Require().False(data.Members["carol"].IsHost)
}

func (s *LobbySuite) TestLeaveLobby_LastMemberLeaves_EmptyLobbyPersists() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-l3", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	_, err := s.orch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "alice", LobbyID: "lobby-l3"})
	s.Require().NoError(err)

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-l3")
	s.Require().NoError(err, "an empty lobby is still saved, not deleted — it expires via TTL")
	s.Require().Empty(data.Members)
	s.Require().Empty(data.MemberOrder)
}

func (s *LobbySuite) TestLeaveLobby_NotAMember_PermissionDenied() {
	s.seedWaitingLobby("lobby-l4", "ref-l4", "alice")

	_, err := s.orch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "stranger", LobbyID: "lobby-l4"})
	s.Require().ErrorIs(err, lobbyorch.ErrPlayerNotInLobby)
}

func (s *LobbySuite) TestLeaveLobby_LobbyStarted_FailedPrecondition() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-l5", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted,
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	_, err := s.orch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "alice", LobbyID: "lobby-l5"})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyAlreadyStarted)
}

func (s *LobbySuite) TestLeaveLobby_ClearsPlayerActiveLobbyIndex() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-l6", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", IsHost: true},
			"bob":   {PlayerID: "bob"},
		},
		MemberOrder: []string{"alice", "bob"},
	})

	_, err := s.orch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "bob", LobbyID: "lobby-l6"})
	s.Require().NoError(err)

	_, err = s.lobbyRepo.GetByPlayerID(s.ctx, "bob")
	s.Require().Error(err, "a departed member's active-lobby index entry must be cleared, not left stale")
	s.Require().ErrorIs(err, lobbyrepo.ErrNotFound)

	stillActive, err := s.lobbyRepo.GetByPlayerID(s.ctx, "alice")
	s.Require().NoError(err, "the remaining member's index entry must be untouched")
	s.Require().Equal("lobby-l6", stillActive.ID)
}

// TestLeaveLobby_ClearPlayerIndexFails_StillSucceedsAndBroadcasts pins the
// Copilot-flagged correctness fix on rpg-api#654: the membership mutation
// (Save) is the durable, authoritative "did the leave happen" — a failure in
// the secondary index cleanup that follows it must NOT turn a real leave
// into a reported error (that would make the RPC non-idempotent: a retry
// would immediately hit ErrPlayerNotInLobby since the member is already
// gone) and must NOT suppress the MemberLeft broadcast other connected
// clients depend on.
func (s *LobbySuite) TestLeaveLobby_ClearPlayerIndexFails_StillSucceedsAndBroadcasts() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-l7", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", IsHost: true},
			"bob":   {PlayerID: "bob"},
		},
		MemberOrder: []string{"alice", "bob"},
	})

	sub, err := s.broker.Subscribe("lobby-l7")
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	failingOrch := s.newOrchestratorWithLobbyRepo(&clearIndexFailingRepo{Repository: s.lobbyRepo})

	_, err = failingOrch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "bob", LobbyID: "lobby-l7"})
	s.Require().NoError(err, "a ClearPlayerIndex failure must not fail the leave itself")

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-l7")
	s.Require().NoError(err)
	s.Require().NotContains(data.Members, "bob", "the membership mutation must still have happened")

	evt := <-sub.Events()
	s.Require().Equal(lobbyorch.EventKindMemberLeft, evt.Kind, "the broadcast must still fire despite the index-cleanup failure")
	s.Require().Equal("bob", evt.MemberLeft.PlayerID)
}

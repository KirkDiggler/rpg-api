package lobby_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

type InMemorySuite struct {
	suite.Suite
	ctx  context.Context
	repo lobbyrepo.Repository
}

func (s *InMemorySuite) SetupTest() {
	s.ctx = context.Background()
	s.repo = lobbyrepo.NewInMemory()
}

func (s *InMemorySuite) TestGet_ReturnsErrNotFound_ForMissingID() {
	_, err := s.repo.Get(s.ctx, "missing")
	s.Require().Error(err)
	s.Require().True(errors.Is(err, lobbyrepo.ErrNotFound))
}

func (s *InMemorySuite) TestGetByJoinRef_ReturnsErrNotFound_ForMissingRef() {
	_, err := s.repo.GetByJoinRef(s.ctx, "missing-ref")
	s.Require().Error(err)
	s.Require().True(errors.Is(err, lobbyrepo.ErrNotFound))
}

func (s *InMemorySuite) TestSaveGet_RoundTrip_PreservesData() {
	original := &lobbyrepo.Data{
		ID: "lobby-1", JoinRef: "ref-1", CampaignID: "campaign-1",
		HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", CharacterID: "char-alice", CharacterName: "Alice", IsHost: true},
		},
		MemberOrder: []string{"alice"},
	}
	s.Require().NoError(s.repo.Save(s.ctx, original))

	loaded, err := s.repo.Get(s.ctx, "lobby-1")
	s.Require().NoError(err)
	s.Require().Equal(original.ID, loaded.ID)
	s.Require().Equal(original.JoinRef, loaded.JoinRef)
	s.Require().Len(loaded.Members, 1)
	s.Require().Equal("char-alice", loaded.Members["alice"].CharacterID)
	s.Require().Equal([]string{"alice"}, loaded.MemberOrder)
}

func (s *InMemorySuite) TestSave_IndexesByJoinRef() {
	s.Require().NoError(s.repo.Save(s.ctx, &lobbyrepo.Data{
		ID: "lobby-2", JoinRef: "ref-2", Status: lobbyrepo.StatusWaiting,
	}))

	loaded, err := s.repo.GetByJoinRef(s.ctx, "ref-2")
	s.Require().NoError(err)
	s.Require().Equal("lobby-2", loaded.ID)
}

func (s *InMemorySuite) TestSave_NilData_ReturnsError() {
	err := s.repo.Save(s.ctx, nil)
	s.Require().Error(err)
}

func (s *InMemorySuite) TestSave_EmptyID_ReturnsError() {
	err := s.repo.Save(s.ctx, &lobbyrepo.Data{})
	s.Require().Error(err)
}

func (s *InMemorySuite) TestGetByPlayerID_ReturnsErrNotFound_ForMissingPlayer() {
	_, err := s.repo.GetByPlayerID(s.ctx, "missing-player")
	s.Require().Error(err)
	s.Require().True(errors.Is(err, lobbyrepo.ErrNotFound))
}

func (s *InMemorySuite) TestSave_IndexesMembersByPlayerID() {
	s.Require().NoError(s.repo.Save(s.ctx, &lobbyrepo.Data{
		ID: "lobby-3", JoinRef: "ref-3", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", IsHost: true},
			"bob":   {PlayerID: "bob"},
		},
		MemberOrder: []string{"alice", "bob"},
	}))

	loadedAlice, err := s.repo.GetByPlayerID(s.ctx, "alice")
	s.Require().NoError(err)
	s.Require().Equal("lobby-3", loadedAlice.ID)

	loadedBob, err := s.repo.GetByPlayerID(s.ctx, "bob")
	s.Require().NoError(err)
	s.Require().Equal("lobby-3", loadedBob.ID)
}

func (s *InMemorySuite) TestSave_PlayerIndex_SecondLobbyOverwrites() {
	// A player who is (or was) a member of two different lobbies has their
	// index entry point at whichever lobby was Saved most recently — the
	// documented one-active-lobby-per-player, last-write-wins semantic.
	s.Require().NoError(s.repo.Save(s.ctx, &lobbyrepo.Data{
		ID: "lobby-a", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice"}},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, &lobbyrepo.Data{
		ID: "lobby-b", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice"}},
	}))

	loaded, err := s.repo.GetByPlayerID(s.ctx, "alice")
	s.Require().NoError(err)
	s.Require().Equal("lobby-b", loaded.ID)
}

func (s *InMemorySuite) TestClearPlayerIndex_RemovesEntry() {
	s.Require().NoError(s.repo.Save(s.ctx, &lobbyrepo.Data{
		ID: "lobby-4", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice"}},
	}))

	s.Require().NoError(s.repo.ClearPlayerIndex(s.ctx, "alice"))

	_, err := s.repo.GetByPlayerID(s.ctx, "alice")
	s.Require().Error(err)
	s.Require().True(errors.Is(err, lobbyrepo.ErrNotFound))
}

func (s *InMemorySuite) TestClearPlayerIndex_NoOpForAbsentPlayer() {
	err := s.repo.ClearPlayerIndex(s.ctx, "nobody")
	s.Require().NoError(err)
}

func TestInMemorySuite(t *testing.T) {
	suite.Run(t, new(InMemorySuite))
}

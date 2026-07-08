package lobby_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

type RedisSuite struct {
	suite.Suite
	ctx       context.Context
	miniredis *miniredis.Miniredis
	client    redisclient.Client
	repo      lobbyrepo.Repository
}

func (s *RedisSuite) SetupTest() {
	s.ctx = context.Background()
	s.miniredis = miniredis.RunT(s.T())
	s.client = goredis.NewClient(&goredis.Options{Addr: s.miniredis.Addr()})
	s.repo = lobbyrepo.NewRedis(s.client, 24*time.Hour)
}

func (s *RedisSuite) TearDownTest() {
	if s.client != nil {
		_ = s.client.Close()
	}
	if s.miniredis != nil {
		s.miniredis.Close()
	}
}

func (s *RedisSuite) TestGet_ReturnsErrNotFound_ForMissingID() {
	_, err := s.repo.Get(s.ctx, "missing")
	s.Require().Error(err)
	s.Require().True(errors.Is(err, lobbyrepo.ErrNotFound))
}

func (s *RedisSuite) TestGetByJoinRef_ReturnsErrNotFound_ForMissingRef() {
	_, err := s.repo.GetByJoinRef(s.ctx, "missing-ref")
	s.Require().Error(err)
	s.Require().True(errors.Is(err, lobbyrepo.ErrNotFound))
}

func (s *RedisSuite) TestSaveGet_RoundTrip_PreservesData() {
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
	s.Require().Len(loaded.Members, 1)
	s.Require().Equal("char-alice", loaded.Members["alice"].CharacterID)
}

func (s *RedisSuite) TestSaveGet_ByJoinRef_ResolvesToSameLobby() {
	s.Require().NoError(s.repo.Save(s.ctx, &lobbyrepo.Data{
		ID: "lobby-2", JoinRef: "ref-2", Status: lobbyrepo.StatusWaiting,
	}))

	loaded, err := s.repo.GetByJoinRef(s.ctx, "ref-2")
	s.Require().NoError(err)
	s.Require().Equal("lobby-2", loaded.ID)
}

func (s *RedisSuite) TestSave_Overwrites_ExistingValue() {
	s.Require().NoError(s.repo.Save(s.ctx, &lobbyrepo.Data{
		ID: "lobby-overwrite", JoinRef: "ref-overwrite", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice"}},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, &lobbyrepo.Data{
		ID: "lobby-overwrite", JoinRef: "ref-overwrite", Status: lobbyrepo.StatusStarted,
		Members: map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice"}, "bob": {PlayerID: "bob"}},
	}))

	loaded, err := s.repo.Get(s.ctx, "lobby-overwrite")
	s.Require().NoError(err)
	s.Require().Equal(lobbyrepo.StatusStarted, loaded.Status)
	s.Require().Len(loaded.Members, 2)
}

func (s *RedisSuite) TestSave_AppliesTTL() {
	s.Require().NoError(s.repo.Save(s.ctx, &lobbyrepo.Data{
		ID: "lobby-ttl", JoinRef: "ref-ttl", Status: lobbyrepo.StatusWaiting,
	}))

	ttl := s.miniredis.TTL("lobby:lobby-ttl")
	s.Require().Equal(24*time.Hour, ttl, "Save must apply the configured TTL to the primary key")
	refTTL := s.miniredis.TTL("lobby:joinref:ref-ttl")
	s.Require().Equal(24*time.Hour, refTTL, "Save must apply the configured TTL to the join_ref index")

	s.miniredis.FastForward(25 * time.Hour)
	_, err := s.repo.Get(s.ctx, "lobby-ttl")
	s.Require().Error(err)
	s.Require().True(errors.Is(err, lobbyrepo.ErrNotFound))
	_, err = s.repo.GetByJoinRef(s.ctx, "ref-ttl")
	s.Require().Error(err)
	s.Require().True(errors.Is(err, lobbyrepo.ErrNotFound))
}

func (s *RedisSuite) TestSave_NilData_ReturnsError() {
	err := s.repo.Save(s.ctx, nil)
	s.Require().Error(err)
}

func (s *RedisSuite) TestSave_EmptyID_ReturnsError() {
	err := s.repo.Save(s.ctx, &lobbyrepo.Data{})
	s.Require().Error(err)
}

func TestRedisSuite(t *testing.T) {
	suite.Run(t, new(RedisSuite))
}

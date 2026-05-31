package encounters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
)

type RedisSuite struct {
	suite.Suite
	ctx       context.Context
	miniredis *miniredis.Miniredis
	client    redisclient.Client
	repo      encountersv2.Repository
}

func (s *RedisSuite) SetupTest() {
	s.ctx = context.Background()
	s.miniredis = miniredis.RunT(s.T())
	s.client = goredis.NewClient(&goredis.Options{Addr: s.miniredis.Addr()})
	s.repo = encountersv2.NewRedis(s.client, 24*time.Hour)
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
	s.Require().True(errors.Is(err, encountersv2.ErrNotFound))
}

func (s *RedisSuite) TestSaveGet_RoundTrip_PreservesData() {
	enc := encounter.New(context.Background(), "enc-1", encounter.NewBroker(encounter.NewInMemoryTransport()))
	s.Require().NoError(enc.AddPlayer(encounter.PlayerInput{
		PlayerID:   "player-A",
		EntityID:   "char-A",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
	}))
	original := enc.ToData()

	s.Require().NoError(s.repo.Save(s.ctx, original))

	loaded, err := s.repo.Get(s.ctx, string(original.ID))
	s.Require().NoError(err)
	s.Require().Equal(original.ID, loaded.ID)
	// Player-map round-trip: player-A's entry must survive Save/Get.
	s.Require().Len(loaded.Players, 1)
	s.Require().Contains(loaded.Players, core.PlayerID("player-A"))
	s.Require().Equal(core.EntityID("char-A"), loaded.Players["player-A"].EntityID)

	// Round-trip via the toolkit serializer to prove ToData/LoadFromData survives storage.
	roundTripped, err := encounter.LoadFromData(context.Background(), loaded, encounter.NewBroker(encounter.NewInMemoryTransport()))
	s.Require().NoError(err)
	s.Require().Equal(original.ID, roundTripped.ID())
}

func (s *RedisSuite) TestSave_Overwrites_ExistingValue() {
	first := encounter.New(context.Background(), "enc-overwrite", encounter.NewBroker(encounter.NewInMemoryTransport())).ToData()
	s.Require().NoError(s.repo.Save(s.ctx, first))

	second := encounter.New(context.Background(), "enc-overwrite", encounter.NewBroker(encounter.NewInMemoryTransport()))
	s.Require().NoError(second.AddPlayer(encounter.PlayerInput{
		PlayerID:   "player-Z",
		EntityID:   "char-Z",
		Position:   core.Hex{Q: 1, R: 0, S: -1},
		SightRange: 5,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, second.ToData()))

	loaded, err := s.repo.Get(s.ctx, "enc-overwrite")
	s.Require().NoError(err)
	s.Require().Len(loaded.Players, 1)
	s.Require().Contains(loaded.Players, core.PlayerID("player-Z"))
}

func (s *RedisSuite) TestSave_AppliesTTL() {
	enc := encounter.New(context.Background(), "enc-ttl", encounter.NewBroker(encounter.NewInMemoryTransport())).ToData()
	s.Require().NoError(s.repo.Save(s.ctx, enc))

	// miniredis exposes the per-key TTL set by SET with EXPIRE; assert it matches.
	ttl := s.miniredis.TTL("enc:v2:enc-ttl")
	s.Require().Equal(24*time.Hour, ttl, "Save must apply the configured TTL")

	// Fast-forward past the TTL and confirm the key is gone (Get returns ErrNotFound).
	s.miniredis.FastForward(25 * time.Hour)
	_, err := s.repo.Get(s.ctx, "enc-ttl")
	s.Require().Error(err)
	s.Require().True(errors.Is(err, encountersv2.ErrNotFound))
}

func (s *RedisSuite) TestSave_NilData_ReturnsError() {
	err := s.repo.Save(s.ctx, nil)
	s.Require().Error(err)
}

func (s *RedisSuite) TestSave_EmptyID_ReturnsError() {
	err := s.repo.Save(s.ctx, &encounter.Data{})
	s.Require().Error(err)
}

func TestRedisSuite(t *testing.T) {
	suite.Run(t, new(RedisSuite))
}

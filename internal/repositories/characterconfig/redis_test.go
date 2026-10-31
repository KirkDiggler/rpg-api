package characterconfig_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	"github.com/KirkDiggler/rpg-api/internal/repositories/characterconfig"
)

const (
	configKey = "character-config:alice"
	legacyKey = "character-exploration:v1:alice"
)

type RedisSuite struct {
	suite.Suite
	ctx       context.Context
	miniredis *miniredis.Miniredis
	client    redisclient.Client
	repo      characterconfig.Repository
}

func TestRedisSuite(t *testing.T) { suite.Run(t, new(RedisSuite)) }

func (s *RedisSuite) SetupTest() {
	s.ctx = context.Background()
	s.miniredis = miniredis.RunT(s.T())
	s.client = goredis.NewClient(&goredis.Options{Addr: s.miniredis.Addr()})
	repo, err := characterconfig.NewRedis(&characterconfig.RedisConfig{Client: s.client})
	s.Require().NoError(err)
	s.repo = repo
}

func (s *RedisSuite) TearDownTest() {
	_ = s.client.Close()
	s.miniredis.Close()
}

func (s *RedisSuite) get() *characterconfig.Data {
	out, err := s.repo.Get(s.ctx, &characterconfig.GetInput{CharacterID: "alice"})
	s.Require().NoError(err)
	s.Require().NotNil(out.Data)
	return out.Data
}

func (s *RedisSuite) put(sharing *bool) {
	_, err := s.repo.Put(s.ctx, &characterconfig.PutInput{
		CharacterID: "alice", Data: &characterconfig.Data{DiscoverySharing: sharing},
	})
	s.Require().NoError(err)
}

func boolPtr(v bool) *bool { return &v }

func (s *RedisSuite) TestPutGet_RoundTrip() {
	for _, sharing := range []bool{true, false} {
		s.put(boolPtr(sharing))
		got := s.get()
		s.Require().NotNil(got.DiscoverySharing)
		s.Equal(sharing, *got.DiscoverySharing)
	}
	stored, err := s.miniredis.Get(configKey)
	s.Require().NoError(err)
	s.JSONEq(`{"discovery_sharing":false}`, stored, "the key and shape are character-config:<id> {discovery_sharing}")

	s.put(nil)
	s.Nil(s.get().DiscoverySharing, "a stored null is not chosen")
	stored, err = s.miniredis.Get(configKey)
	s.Require().NoError(err)
	s.JSONEq(`{"discovery_sharing":null}`, stored)
}

func (s *RedisSuite) TestGet_NothingStoredIsNotChosen() {
	s.Nil(s.get().DiscoverySharing)
}

// The legacy record is the initial value, through its three branches.
func (s *RedisSuite) TestGet_LegacyRecordIsTheInitialValue() {
	for _, tc := range []struct {
		name   string
		legacy string
		want   *bool
	}{
		{name: "an explicit choice wins over private", legacy: `{"character":"alice","private":true,"discovery_sharing":true}`, want: boolPtr(true)},
		{name: "an explicit private choice", legacy: `{"character":"alice","discovery_sharing":false,"private":true}`, want: boolPtr(false)},
		{name: "a record with no choice reads not private: shared", legacy: `{"character":"alice"}`, want: boolPtr(true)},
		{name: "a record with no choice reads not private: private", legacy: `{"character":"alice","private":true,"checks":{"x":{"used":9}}}`, want: boolPtr(false)},
	} {
		s.Run(tc.name, func() {
			s.miniredis.FlushAll()
			s.Require().NoError(s.miniredis.Set(legacyKey, tc.legacy))
			s.Equal(tc.want, s.get().DiscoverySharing)
			after, err := s.miniredis.Get(legacyKey)
			s.Require().NoError(err)
			s.Equal(tc.legacy, after, "the legacy record is never written")
		})
	}
}

func (s *RedisSuite) TestGet_LegacyRecordIsReadOnce() {
	s.Require().NoError(s.miniredis.Set(legacyKey, `{"character":"alice","private":true}`))
	s.Equal(boolPtr(false), s.get().DiscoverySharing)
	seeded, err := s.miniredis.Get(configKey)
	s.Require().NoError(err)
	s.JSONEq(`{"discovery_sharing":false}`, seeded, "the first read seeds the configuration")

	// Anything the legacy key holds later has no authority.
	s.Require().NoError(s.miniredis.Set(legacyKey, `{"character":"alice","discovery_sharing":true}`))
	s.Equal(boolPtr(false), s.get().DiscoverySharing, "the legacy record is not read again")
}

func (s *RedisSuite) TestGet_NoLegacyRecordSeedsNotChosen() {
	s.Nil(s.get().DiscoverySharing)
	s.Require().NoError(s.miniredis.Set(legacyKey, `{"character":"alice","discovery_sharing":true}`))
	s.Nil(s.get().DiscoverySharing, "a legacy record appearing after the first read has no authority")
}

func (s *RedisSuite) TestPut_WinsOverTheLegacyRecord() {
	s.Require().NoError(s.miniredis.Set(legacyKey, `{"character":"alice","private":true}`))
	s.put(boolPtr(true))
	s.Equal(boolPtr(true), s.get().DiscoverySharing)
}

func (s *RedisSuite) TestInputsAreValidated() {
	_, err := s.repo.Get(s.ctx, &characterconfig.GetInput{})
	s.ErrorIs(err, characterconfig.ErrCharacterIDRequired)
	_, err = s.repo.Get(s.ctx, nil)
	s.ErrorIs(err, characterconfig.ErrCharacterIDRequired)
	_, err = s.repo.Put(s.ctx, &characterconfig.PutInput{CharacterID: "alice"})
	s.ErrorIs(err, characterconfig.ErrDataRequired)
	_, err = s.repo.Put(s.ctx, &characterconfig.PutInput{Data: &characterconfig.Data{}})
	s.ErrorIs(err, characterconfig.ErrCharacterIDRequired)
	_, err = characterconfig.NewRedis(nil)
	s.Error(err)
}

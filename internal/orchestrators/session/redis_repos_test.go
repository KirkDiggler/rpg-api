package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

type RedisReposTestSuite struct {
	suite.Suite
	ctx       context.Context
	miniredis *miniredis.Miniredis
	client    redisclient.Client
	sessions  sdk.SessionRepository
	encs      sdk.EncounterRepository
}

func (s *RedisReposTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.miniredis = miniredis.RunT(s.T())
	s.client = goredis.NewClient(&goredis.Options{Addr: s.miniredis.Addr()})
	s.sessions = sessionorch.NewSessionRepository(s.client, 24*time.Hour)
	s.encs = sessionorch.NewEncounterRepository(s.client, 24*time.Hour)
}

func (s *RedisReposTestSuite) TearDownTest() {
	if s.client != nil {
		_ = s.client.Close()
	}
	if s.miniredis != nil {
		s.miniredis.Close()
	}
}

func TestRedisReposSuite(t *testing.T) {
	suite.Run(t, new(RedisReposTestSuite))
}

func (s *RedisReposTestSuite) TestSession_SaveThenGet_RoundTrips() {
	in := &sdk.SessionData{ID: "sess-1", Encounter: "enc-1"}
	s.Require().NoError(s.sessions.SaveSession(s.ctx, in))

	got, err := s.sessions.GetSession(s.ctx, "sess-1")
	s.Require().NoError(err)
	s.Equal(in.ID, got.ID)
	s.Equal(in.Encounter, got.Encounter)
}

func (s *RedisReposTestSuite) TestSession_Get_Missing_IsSDKNotFound() {
	_, err := s.sessions.GetSession(s.ctx, "does-not-exist")
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdk.ErrNotFound)
}

func (s *RedisReposTestSuite) TestSession_Save_NilData_Errors() {
	err := s.sessions.SaveSession(s.ctx, nil)
	s.Require().Error(err)
}

func (s *RedisReposTestSuite) TestSession_Save_EmptyID_Errors() {
	err := s.sessions.SaveSession(s.ctx, &sdk.SessionData{})
	s.Require().Error(err)
}

func (s *RedisReposTestSuite) TestEncounter_SaveThenGet_RoundTrips() {
	in := &tkencounter.EncounterData{}
	s.Require().NoError(s.encs.SaveEncounter(s.ctx, "enc-1", in))

	got, err := s.encs.GetEncounter(s.ctx, "enc-1")
	s.Require().NoError(err)
	s.NotNil(got)
}

func (s *RedisReposTestSuite) TestEncounter_Get_Missing_IsSDKNotFound() {
	_, err := s.encs.GetEncounter(s.ctx, "does-not-exist")
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdk.ErrNotFound)
}

func (s *RedisReposTestSuite) TestEncounter_Save_NilData_Errors() {
	err := s.encs.SaveEncounter(s.ctx, "enc-1", nil)
	s.Require().Error(err)
}

func (s *RedisReposTestSuite) TestEncounter_Save_EmptyID_Errors() {
	err := s.encs.SaveEncounter(s.ctx, "", &tkencounter.EncounterData{})
	s.Require().Error(err)
}

func (s *RedisReposTestSuite) TestKeyPrefixes_AreDistinctFromOldEncounterPath() {
	s.Require().NoError(s.sessions.SaveSession(s.ctx, &sdk.SessionData{ID: "x"}))
	s.Require().NoError(s.encs.SaveEncounter(s.ctx, "x", &tkencounter.EncounterData{}))

	keys := s.miniredis.Keys()
	for _, k := range keys {
		s.NotContains(k, "enc:v2:", "session-stack keys must not collide with the old encounter path's enc:v2: prefix")
	}
}

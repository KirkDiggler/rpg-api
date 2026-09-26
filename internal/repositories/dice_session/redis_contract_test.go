package dicesession_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	dicesession "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
)

type controlledClock struct{ now time.Time }

func (c *controlledClock) Now() time.Time { return c.now }

type DiceRedisContractSuite struct {
	suite.Suite
	ctx    context.Context
	server *miniredis.Miniredis
	client *redis.Client
	clock  *controlledClock
	repo   dicesession.Repository
}

func TestDiceRedisContractSuite(t *testing.T) { suite.Run(t, new(DiceRedisContractSuite)) }
func (s *DiceRedisContractSuite) SetupTest() {
	s.ctx = context.Background()
	s.server = miniredis.RunT(s.T())
	s.client = redis.NewClient(&redis.Options{Addr: s.server.Addr(), MaxRetries: -1})
	client := s.client
	s.T().Cleanup(func() { _ = client.Close() })
	s.clock = &controlledClock{now: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	var err error
	s.repo, err = dicesession.NewRedisRepository(&dicesession.Config{Client: s.client, Clock: s.clock})
	s.Require().NoError(err)
}

func storedRolls() []dicesession.DiceRoll {
	// Deliberately supplied values, not a calculation: the adapter must not
	// parse notation, roll dice, drop dice, or recompute a total.
	return []dicesession.DiceRoll{{RollID: "roll-a", Notation: "opaque notation", Dice: []int{2, 5}, Total: 41, Dropped: []int{2}, Description: "Stored description", DiceTotal: 7, Modifier: 3},
		{RollID: "roll-b", Notation: "another notation", Dice: []int{0}, Total: 0, Dropped: []int{}}}
}
func (s *DiceRedisContractSuite) create(entity, scope string, ttl time.Duration) *dicesession.DiceSession {
	out, err := s.repo.Create(s.ctx, dicesession.CreateInput{EntityID: entity, Context: scope, Rolls: storedRolls(), TTL: ttl})
	s.Require().NoError(err)
	s.Require().NotNil(out)
	return out.Session
}
func (s *DiceRedisContractSuite) get(entity, scope string) *dicesession.DiceSession {
	out, err := s.repo.Get(s.ctx, dicesession.GetInput{EntityID: entity, Context: scope})
	s.Require().NoError(err)
	s.Require().NotNil(out)
	return out.Session
}
func (s *DiceRedisContractSuite) TestPopulatedRoundTrip_PreservesSuppliedRollsAndDetachedReads() {
	created := s.create("entity-a", "scope-a", 0)
	expected := &dicesession.DiceSession{EntityID: "entity-a", Context: "scope-a", Rolls: storedRolls(), CreatedAt: s.clock.now, ExpiresAt: s.clock.now.Add(15 * time.Minute)}
	s.Equal(expected, created)
	got := s.get("entity-a", "scope-a")
	s.Equal(expected, got)
	created.Rolls[0].Total = 999
	got.Rolls[0].Dice[0] = 99
	got.Rolls[0].Dropped[0] = 99
	s.Equal(expected, s.get("entity-a", "scope-a"))
	s.Equal(15*time.Minute, s.server.TTL("dice_session:entity-a:scope-a"))
}
func (s *DiceRedisContractSuite) TestEntityAndContextKeysAreIndependent() {
	first := s.create("entity-a", "scope-a", time.Hour)
	second := s.create("entity-a", "scope-b", time.Hour)
	third := s.create("entity-b", "scope-a", time.Hour)
	first.Rolls = []dicesession.DiceRoll{{RollID: "replacement", Total: 8}}
	s.Require().NoError(s.repo.Update(s.ctx, first))
	s.Equal(first, s.get("entity-a", "scope-a"))
	s.Equal(second, s.get("entity-a", "scope-b"))
	s.Equal(third, s.get("entity-b", "scope-a"))
	out, err := s.repo.Delete(s.ctx, dicesession.DeleteInput{EntityID: "entity-a", Context: "scope-a"})
	s.Require().NoError(err)
	s.Equal(1, out.RollsDeleted)
	s.Equal(second, s.get("entity-a", "scope-b"))
	s.Equal(third, s.get("entity-b", "scope-a"))
}
func (s *DiceRedisContractSuite) TestUpdateUsesRemainingTTL_NotFreshLifetime() {
	in := s.create("entity-a", "scope-a", time.Hour)
	s.clock.now = s.clock.now.Add(20 * time.Minute)
	s.server.FastForward(20 * time.Minute)
	in.Rolls = append(in.Rolls, dicesession.DiceRoll{RollID: "roll-c", Total: -7})
	s.Require().NoError(s.repo.Update(s.ctx, in))
	s.Equal(in, s.get("entity-a", "scope-a"))
	s.Equal(40*time.Minute, s.server.TTL("dice_session:entity-a:scope-a"))
	s.server.FastForward(40 * time.Minute)
	out, err := s.repo.Get(s.ctx, dicesession.GetInput{EntityID: "entity-a", Context: "scope-a"})
	s.True(apierr.IsNotFound(err))
	s.Nil(out)
}
func (s *DiceRedisContractSuite) TestDefaultTTLExpiresInRedis() {
	s.create("entity-a", "scope-a", 0)
	s.server.FastForward(15 * time.Minute)
	out, err := s.repo.Get(s.ctx, dicesession.GetInput{EntityID: "entity-a", Context: "scope-a"})
	s.True(apierr.IsNotFound(err))
	s.Nil(out)
}
func (s *DiceRedisContractSuite) TestExpiredStoredTimestampIsCleanedEvenBeforeRedisExpiry() {
	s.create("entity-a", "scope-a", time.Minute)
	// Advance the application clock only, keeping the Redis key present.
	s.clock.now = s.clock.now.Add(time.Minute + time.Nanosecond)
	s.True(s.server.Exists("dice_session:entity-a:scope-a"))
	out, err := s.repo.Get(s.ctx, dicesession.GetInput{EntityID: "entity-a", Context: "scope-a"})
	s.True(apierr.IsNotFound(err))
	s.Nil(out)
	s.False(s.server.Exists("dice_session:entity-a:scope-a"))
}
func (s *DiceRedisContractSuite) TestPastExpiryUpdateRefusesWithoutWriting() {
	in := s.create("entity-a", "scope-a", time.Minute)
	stored, err := s.server.Get("dice_session:entity-a:scope-a")
	s.Require().NoError(err)
	s.clock.now = s.clock.now.Add(time.Minute + time.Nanosecond)
	in.Rolls = nil
	s.True(apierr.IsInvalidArgument(s.repo.Update(s.ctx, in)))
	after, err := s.server.Get("dice_session:entity-a:scope-a")
	s.Require().NoError(err)
	s.Equal(stored, after)
}
func (s *DiceRedisContractSuite) TestDeleteReportsStoredCount_AndMissingDeleteIsIdempotent() {
	s.create("entity-a", "scope-a", 0)
	out, err := s.repo.Delete(s.ctx, dicesession.DeleteInput{EntityID: "entity-a", Context: "scope-a"})
	s.Require().NoError(err)
	s.Equal(2, out.RollsDeleted)
	missing, err := s.repo.Get(s.ctx, dicesession.GetInput{EntityID: "entity-a", Context: "scope-a"})
	s.True(apierr.IsNotFound(err))
	s.Nil(missing)
	out, err = s.repo.Delete(s.ctx, dicesession.DeleteInput{EntityID: "entity-a", Context: "scope-a"})
	s.Require().NoError(err)
	s.Zero(out.RollsDeleted)
}
func (s *DiceRedisContractSuite) TestMalformedPayloadIsNotNotFound() {
	s.Require().NoError(s.server.Set("dice_session:entity-a:scope-a", "{"))
	out, err := s.repo.Get(s.ctx, dicesession.GetInput{EntityID: "entity-a", Context: "scope-a"})
	var syntax *json.SyntaxError
	s.ErrorAs(err, &syntax)
	s.False(apierr.IsNotFound(err))
	s.Nil(out)
	s.True(s.server.Exists("dice_session:entity-a:scope-a"))
}
func (s *DiceRedisContractSuite) TestValidationRejectsMissingKeysWithoutWrites() {
	for _, key := range []dicesession.GetInput{{}, {EntityID: "entity-a"}, {Context: "scope-a"}} {
		_, err := s.repo.Create(s.ctx, dicesession.CreateInput{EntityID: key.EntityID, Context: key.Context})
		s.True(apierr.IsInvalidArgument(err))
		_, err = s.repo.Get(s.ctx, key)
		s.True(apierr.IsInvalidArgument(err))
		_, err = s.repo.Delete(s.ctx, dicesession.DeleteInput(key))
		s.True(apierr.IsInvalidArgument(err))
		err = s.repo.Update(s.ctx, &dicesession.DiceSession{EntityID: key.EntityID, Context: key.Context})
		s.True(apierr.IsInvalidArgument(err))
	}
	s.True(apierr.IsInvalidArgument(s.repo.Update(s.ctx, nil)))
	s.Empty(s.server.Keys())
}

type diceWriteFailure struct{ cause error }

func (h diceWriteFailure) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h diceWriteFailure) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h diceWriteFailure) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "set" || cmd.Name() == "del" {
			return h.cause
		}
		return next(ctx, cmd)
	}
}
func (s *DiceRedisContractSuite) TestWriteFailuresKeepCauseAndDoNotClaimSuccess() {
	original := s.create("entity-a", "scope-a", time.Hour)
	cause := errors.New("injected write failure")
	s.client.AddHook(diceWriteFailure{cause: cause})
	out, err := s.repo.Create(s.ctx, dicesession.CreateInput{EntityID: "new", Context: "scope-a"})
	s.ErrorIs(err, cause)
	s.Nil(out)
	changed := s.get("entity-a", "scope-a")
	changed.Rolls = nil
	s.ErrorIs(s.repo.Update(s.ctx, changed), cause)
	deleted, err := s.repo.Delete(s.ctx, dicesession.DeleteInput{EntityID: "entity-a", Context: "scope-a"})
	s.ErrorIs(err, cause)
	s.Nil(deleted)
	s.Equal(original, s.get("entity-a", "scope-a"))
	s.False(s.server.Exists("dice_session:new:scope-a"))
}
func (s *DiceRedisContractSuite) TestStorageReadFailureKeepsCause() {
	s.Require().NoError(s.client.Close())
	out, err := s.repo.Get(s.ctx, dicesession.GetInput{EntityID: "entity-a", Context: "scope-a"})
	s.ErrorIs(err, redis.ErrClosed)
	s.False(apierr.IsNotFound(err))
	s.Nil(out)
}

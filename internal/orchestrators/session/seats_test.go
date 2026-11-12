package session

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type SeatRepositorySuite struct {
	suite.Suite
	server *miniredis.Miniredis
	client *goredis.Client
	repo   sdk.SeatRepository
	ctx    context.Context
}

func TestSeatRepositorySuite(t *testing.T) { suite.Run(t, new(SeatRepositorySuite)) }

func (s *SeatRepositorySuite) SetupTest() {
	s.server = miniredis.RunT(s.T())
	s.client = goredis.NewClient(&goredis.Options{Addr: s.server.Addr()})
	s.repo = NewSeatRepository(s.client)
	s.ctx = context.Background()
}

func (s *SeatRepositorySuite) TearDownTest() { _ = s.client.Close() }

func (s *SeatRepositorySuite) liveSession(id string) {
	s.Require().NoError(s.server.Set(sessionKeyPrefix+id, "{}"))
}

func (s *SeatRepositorySuite) TestANeverSeatedCharacterIsNotFound() {
	_, err := s.repo.GetSeat(s.ctx, "alice")
	s.ErrorIs(err, sdk.ErrNotFound)
}

func (s *SeatRepositorySuite) TestASeatRoundTripsByCharacterWithNoExpiry() {
	s.liveSession("run-1")
	s.Require().NoError(s.repo.SaveSeat(s.ctx, &sdk.SeatData{Character: "alice", Session: "run-1"}))
	got, err := s.repo.GetSeat(s.ctx, "alice")
	s.Require().NoError(err)
	s.Equal(&sdk.SeatData{Character: "alice", Session: "run-1"}, got)
	s.Zero(s.server.TTL(seatKeyPrefix + "alice"))
	_, err = s.repo.GetSeat(s.ctx, "bob")
	s.ErrorIs(err, sdk.ErrNotFound, "seats are keyed by character")
}

func (s *SeatRepositorySuite) TestAClearedSeatIsARecordNamingNoSession() {
	s.Require().NoError(s.repo.SaveSeat(s.ctx, &sdk.SeatData{Character: "alice"}))
	got, err := s.repo.GetSeat(s.ctx, "alice")
	s.Require().NoError(err)
	s.Equal(&sdk.SeatData{Character: "alice"}, got)
}

// TestASeatLapsesWithTheRunItNames: the session record expired (its TTL) and
// the seat did not, so the seat reads as never seated rather than holding the
// character in a run nobody can open.
func (s *SeatRepositorySuite) TestASeatLapsesWithTheRunItNames() {
	s.liveSession("run-1")
	s.Require().NoError(s.repo.SaveSeat(s.ctx, &sdk.SeatData{Character: "alice", Session: "run-1"}))
	s.server.Del(sessionKeyPrefix + "run-1")
	_, err := s.repo.GetSeat(s.ctx, "alice")
	s.ErrorIs(err, sdk.ErrNotFound)
}

func (s *SeatRepositorySuite) TestInvalidInputsWriteNothing() {
	s.Error(s.repo.SaveSeat(s.ctx, nil))
	s.Error(s.repo.SaveSeat(s.ctx, &sdk.SeatData{Session: "run-1"}))
	_, err := s.repo.GetSeat(s.ctx, "")
	s.Error(err)
	s.Empty(s.server.Keys())
}

func (s *SeatRepositorySuite) TestAStorageFailureIsNotNotFound() {
	s.Require().NoError(s.client.Close())
	_, err := s.repo.GetSeat(s.ctx, "alice")
	s.Error(err)
	s.NotErrorIs(err, sdk.ErrNotFound)
}

// TestASeatLapsesWithARunNobodyCanLoad: a run marked unloadable
// (rpg-project#942) holds no seat, while its stored record and the seat
// record stay as they were.
func (s *SeatRepositorySuite) TestASeatLapsesWithARunNobodyCanLoad() {
	s.liveSession("run-1")
	s.Require().NoError(s.repo.SaveSeat(s.ctx, &sdk.SeatData{Character: "alice", Session: "run-1"}))
	s.Require().NoError(NewUnloadableRuns(s.client).Mark(s.ctx, "run-1", "invalid encounter data: test"))
	_, err := s.repo.GetSeat(s.ctx, "alice")
	s.ErrorIs(err, sdk.ErrNotFound)
	s.True(s.server.Exists(sessionKeyPrefix+"run-1"), "the dead run's data is left in place")
	s.True(s.server.Exists(seatKeyPrefix+"alice"), "and so is the seat record")
}

// TestAnUnmarkedRunHoldsItsSeat: only the mark releases a seat; a run that is
// merely unread holds it.
func (s *SeatRepositorySuite) TestAnUnmarkedRunHoldsItsSeat() {
	s.liveSession("run-1")
	s.liveSession("run-2")
	s.Require().NoError(s.repo.SaveSeat(s.ctx, &sdk.SeatData{Character: "alice", Session: "run-1"}))
	s.Require().NoError(NewUnloadableRuns(s.client).Mark(s.ctx, "run-2", "invalid encounter data: test"))
	got, err := s.repo.GetSeat(s.ctx, "alice")
	s.Require().NoError(err)
	s.Equal("run-1", got.Session)
}

func (s *SeatRepositorySuite) TestMarkingNeedsASession() {
	s.Error(NewUnloadableRuns(s.client).Mark(s.ctx, "", "why"))
	s.Empty(s.server.Keys())
}

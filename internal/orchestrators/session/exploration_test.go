package session

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type ExplorationHostSuite struct{ suite.Suite }

func TestExplorationHostSuite(t *testing.T) { suite.Run(t, new(ExplorationHostSuite)) }
func (s *ExplorationHostSuite) TestProfileIsDetachedAndDoesNotExpireWithARun() {
	server := miniredis.RunT(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	repo := &redisExplorationRepository{client: client}
	ctx := context.Background()
	_, err := repo.GetExploration(ctx, "alice")
	s.ErrorIs(err, sdk.ErrNotFound)
	data := &sdk.ExplorationData{Character: "alice", PrivateDiscoveries: true}
	s.Require().NoError(repo.SaveExploration(ctx, data))
	server.FastForward(48 * time.Hour)
	got, err := repo.GetExploration(ctx, "alice")
	s.Require().NoError(err)
	s.Equal(data, got)
	got.PrivateDiscoveries = false
	again, err := repo.GetExploration(ctx, "alice")
	s.Require().NoError(err)
	s.True(again.PrivateDiscoveries, "a read hands back a copy, not the stored profile")
	_, err = repo.GetExploration(ctx, "bob")
	s.ErrorIs(err, sdk.ErrNotFound)
}
func (s *ExplorationHostSuite) TestSharedStoreGuardCoversProfilesAcrossSessionIDs() {
	locker := sharedStoreLocker{inner: NewInProcessSessionLocker()}
	first, err := locker.LockSession(context.Background(), &sdk.LockSessionInput{Session: "first-run"})
	s.Require().NoError(err)
	defer first.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = locker.LockSession(ctx, &sdk.LockSessionInput{Session: "second-run"})
	s.ErrorIs(err, context.DeadlineExceeded)
	first.Release()
	second, err := locker.LockSession(context.Background(), &sdk.LockSessionInput{Session: "second-run"})
	s.Require().NoError(err)
	second.Release()
	s.Empty(locker.inner.entries)
}

// TestTheStoreWideGuardLeavesCharacterGuardsPerCharacter is the Launch-path
// deadlock check: holding the one store-wide session key, a verb takes each
// party member's guard. Were characters routed onto that key, this would
// block until the timeout.
func (s *ExplorationHostSuite) TestTheStoreWideGuardLeavesCharacterGuardsPerCharacter() {
	locker := sharedStoreLocker{inner: NewInProcessSessionLocker()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	held, err := locker.LockSession(ctx, &sdk.LockSessionInput{Session: "run-1"})
	s.Require().NoError(err)
	guards := make([]func(), 0, 2)
	for _, id := range []string{"alice", "bob"} {
		guard, err := locker.LockCharacter(ctx, &sdk.LockCharacterInput{Character: id})
		s.Require().NoError(err, "character %s under the held store-wide guard", id)
		guards = append(guards, guard.Release)
	}
	for _, release := range guards {
		release()
	}
	held.Release()
}

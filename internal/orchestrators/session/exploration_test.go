package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type ExplorationHostSuite struct{ suite.Suite }

func TestExplorationHostSuite(t *testing.T) { suite.Run(t, new(ExplorationHostSuite)) }
func (s *ExplorationHostSuite) TestPreferenceIsDetachedAndDoesNotExpireWithARun() {
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
func (s *ExplorationHostSuite) TestLegacyProfileChecksAreIgnoredWithoutMutatingTheStoredRecord() {
	server := miniredis.RunT(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	repo := &redisExplorationRepository{client: client}
	ctx := context.Background()
	legacy := `{"character":"alice","private":true,"checks":{"site/secret":{"used":1,"learned":true}}}`
	s.Require().NoError(client.Set(ctx, explorationPrefix+"alice", legacy, 0).Err())
	got, err := repo.GetExploration(ctx, "alice")
	s.Require().NoError(err)
	s.True(got.PrivateDiscoveries)
	raw, err := json.Marshal(got)
	s.Require().NoError(err)
	s.NotContains(string(raw), "checks", "only preferences cross the host/SDK boundary")
	stored, err := client.Get(ctx, explorationPrefix+"alice").Result()
	s.Require().NoError(err)
	s.Equal(legacy, stored, "a read must not delete the old data")
}

// TestExplorationProfileRoundTripsTheSharingChoice pins that the host stores
// the character's explicit sharing choice (R8) and leaves a pre-choice record
// undecided, so the SDK can tell "never chosen" from "chose shared".
// A missing record returning sdk.ErrNotFound is pinned above.
func (s *ExplorationHostSuite) TestExplorationProfileRoundTripsTheSharingChoice() {
	server := miniredis.RunT(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	repo := &redisExplorationRepository{client: client}
	ctx := context.Background()

	s.Run("an explicit choice survives a save and reload", func() {
		sharing := true
		s.Require().NoError(repo.SaveExploration(ctx, &sdk.ExplorationData{Character: "c", DiscoverySharing: &sharing}))
		got, err := repo.GetExploration(ctx, "c")
		s.Require().NoError(err)
		s.Require().NotNil(got.DiscoverySharing, "a chosen preference must not reload as undecided")
		s.True(*got.DiscoverySharing)
	})

	s.Run("a legacy record reloads undecided and untouched", func() {
		legacy := `{"character":"legacy"}`
		s.Require().NoError(client.Set(ctx, explorationPrefix+"legacy", legacy, 0).Err())
		got, err := repo.GetExploration(ctx, "legacy")
		s.Require().NoError(err)
		s.Nil(got.DiscoverySharing, "a record written before the choice existed has not chosen")
		s.False(got.PrivateDiscoveries)
		stored, err := client.Get(ctx, explorationPrefix+"legacy").Result()
		s.Require().NoError(err)
		s.Equal(legacy, stored, "a read must not write a default down as a choice")
	})
}

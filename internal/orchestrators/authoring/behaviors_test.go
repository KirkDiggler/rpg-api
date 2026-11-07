package authoring_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

type BehaviorsSuite struct {
	suite.Suite
	orch   *authoringorch.Orchestrator
	source []byte
}

func TestBehaviorsSuite(t *testing.T) { suite.Run(t, new(BehaviorsSuite)) }

func (s *BehaviorsSuite) SetupTest() {
	// A real registry with its mandatory default, independent of unrelated shelf
	// fixtures; the policy under test always enters through PutDungeon.
	dir := s.T().TempDir()
	tomb, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(s.T()), "reference-tomb.yaml"))
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(filepath.Join(dir, "reference-tomb.yaml"), tomb, 0o600))
	shipped, err := dungeons.NewFileRegistry(dir, dungeonstest.Projector(s.T()))
	s.Require().NoError(err)
	s.orch, err = authoringorch.New(&authoringorch.Config{Dungeons: dungeonstest.Join(s.T(), shipped, true)})
	s.Require().NoError(err)
	s.source, err = os.ReadFile("testdata/behavior-patrol-consumer.yaml")
	s.Require().NoError(err)
}

func (s *BehaviorsSuite) TestPutDungeonBehaviorRoundTrip() {
	ctx := context.Background()
	out, err := s.orch.PutDungeon(ctx, &authoringorch.PutDungeonInput{Caller: dungeonstest.Caller, Key: "behavior-patrol-consumer", YAML: s.source})
	s.Require().NoError(err)
	s.Require().Empty(out.Errors)
	s.Require().NotNil(out.Atlas)
	got, err := s.orch.GetDungeon(ctx, &authoringorch.GetDungeonInput{Caller: dungeonstest.Caller, Key: "behavior-patrol-consumer"})
	s.Require().NoError(err)
	s.Equal(s.source, got.YAML, "author source, not expanded graphs or private progress, is stored")
	bad := strings.Replace(string(s.source), "use: patrol", "use: missing", 1)
	s.Require().NotEqual(string(s.source), bad)
	refused, err := s.orch.PutDungeon(ctx, &authoringorch.PutDungeonInput{Caller: dungeonstest.Caller, Key: "behavior-patrol-consumer", YAML: []byte(bad)})
	s.Require().NoError(err)
	s.Require().NotEmpty(refused.Errors)
	s.Contains(refused.Errors[0].Path, "room.room.monsterBindings.west.behaviors")
	got, err = s.orch.GetDungeon(ctx, &authoringorch.GetDungeonInput{Caller: dungeonstest.Caller, Key: "behavior-patrol-consumer"})
	s.Require().NoError(err)
	s.Equal(s.source, got.YAML, "a bad binding never replaces the last valid source")
}

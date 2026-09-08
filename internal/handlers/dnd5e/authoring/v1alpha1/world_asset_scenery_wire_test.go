package authoringv1alpha1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

type WorldAssetSceneryWireSuite struct {
	suite.Suite

	ctx     context.Context
	handler *authoringhandler.Handler
}

func TestWorldAssetSceneryWireSuite(t *testing.T) {
	suite.Run(t, new(WorldAssetSceneryWireSuite))
}

func (s *WorldAssetSceneryWireSuite) SetupTest() {
	s.ctx = auth.WithPlayerID(context.Background(), "alice")

	registry, _ := dungeonstest.Scratch(s.T())
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: registry})
	s.Require().NoError(err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)
	s.handler = h
}

func (s *WorldAssetSceneryWireSuite) TestPutDungeonPreservesEveryWorldAssetRef() {
	resp, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{
		Key:          dungeonstest.WorldAssetSceneryKey,
		Yaml:         dungeonstest.WorldAssetSceneryYAML,
		ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Require().Empty(resp.GetErrors(), "the four scenery namespaces must compile")
	s.Require().NotNil(resp.GetAtlas())

	refs := make([]string, 0, len(resp.GetAtlas().GetProps()))
	for _, prop := range resp.GetAtlas().GetProps() {
		refs = append(refs, prop.GetRef())
	}
	s.ElementsMatch(dungeonstest.WorldAssetSceneryRefs, refs)
}

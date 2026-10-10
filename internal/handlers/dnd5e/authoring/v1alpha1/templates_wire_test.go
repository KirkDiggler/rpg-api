package authoringv1alpha1_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// castleKitchenKey is the shipped walk dungeon for authored stat blocks
// (rpg-project#555).
const castleKitchenKey = "castle-kitchen"

// TemplatesWireSuite drives PutDungeon{validate_only} on the castle over a
// real read-only registry: the derived stat blocks reach the wire
// (rpg-project#555 R7).
type TemplatesWireSuite struct {
	suite.Suite

	ctx     context.Context
	handler *authoringhandler.Handler
	yaml    string
}

func TestTemplatesWireSuite(t *testing.T) {
	suite.Run(t, new(TemplatesWireSuite))
}

func (s *TemplatesWireSuite) SetupTest() {
	s.ctx = auth.WithPlayerID(context.Background(), "alice")

	registry, dir := dungeonstest.ScratchReadOnly(s.T())
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: registry})
	s.Require().NoError(err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)
	s.handler = h

	raw, err := os.ReadFile(filepath.Join(dir, castleKitchenKey+".yaml")) //nolint:gosec // dir is t.TempDir()
	s.Require().NoError(err)
	s.yaml = string(raw)
}

// TestPutDungeon_ValidateOnlyEchoesTemplates is rpg-project#555 T5's handler
// test.
//
// GUARDED until rpg-api-protos#387 tags: the response has no `templates`
// field to assert on yet, so this asserts the castle grades clean AND that
// the field is still absent — and FAILS the moment a protos bump brings it,
// naming the conversion to write in put_dungeon.go. Replace the guard with
// the three-entry assertion then.
func (s *TemplatesWireSuite) TestPutDungeon_ValidateOnlyEchoesTemplates() {
	resp, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{
		Key: castleKitchenKey, Yaml: s.yaml, ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Require().Empty(resp.GetErrors())
	s.Require().NotNil(resp.GetAtlas())

	field := resp.ProtoReflect().Descriptor().Fields().ByName("templates")
	s.Nil(field, "rpg-api-protos#387 has landed: convert out.Templates in put_dungeon.go "+
		"and replace this guard with: len(resp.Templates) == 3, errors empty")
}

package authoring_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// castleKitchenKey is the shipped walk dungeon for authored stat blocks
// (rpg-project#555).
const castleKitchenKey = "castle-kitchen"

// TemplatesSuite pins that PutDungeon forwards the derived stat blocks the
// compile produced — over the real registry and the real dungeonspec, so the
// numbers asserted are the rulebook's, not a mock's.
type TemplatesSuite struct {
	suite.Suite

	ctx  context.Context
	orch *authoringorch.Orchestrator
	yaml []byte
}

func TestTemplatesSuite(t *testing.T) {
	suite.Run(t, new(TemplatesSuite))
}

func (s *TemplatesSuite) SetupTest() {
	s.ctx = context.Background()

	registry, dir := dungeonstest.ScratchReadOnly(s.T())
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: registry})
	s.Require().NoError(err)
	s.orch = orch

	raw, err := os.ReadFile(filepath.Join(dir, castleKitchenKey+".yaml")) //nolint:gosec // dir is t.TempDir()
	s.Require().NoError(err)
	s.yaml = raw
}

func (s *TemplatesSuite) put(yaml []byte) *authoringorch.PutDungeonOutput {
	s.T().Helper()

	out, err := s.orch.PutDungeon(s.ctx, &authoringorch.PutDungeonInput{
		Key: castleKitchenKey, YAML: yaml, ValidateOnly: true,
	})
	s.Require().NoError(err)

	return out
}

func (s *TemplatesSuite) TestPutDungeon_ValidateOnlyReturnsTheDerivedBlocks() {
	out := s.put(s.yaml)

	s.Require().Empty(out.Errors)
	s.Require().Len(out.Templates, 3)
	guard := out.Templates[2]
	s.Equal("guard", guard.TemplateID)
	s.Equal(11, guard.HitPoints)
	s.Equal(13, guard.ArmorClass)
}

// TestPutDungeon_NoBlocksBesideErrors: a file that did not compile vouches
// for no block, even the ones that would have derived.
func (s *TemplatesSuite) TestPutDungeon_NoBlocksBesideErrors() {
	broken := strings.Replace(string(s.yaml), "armor: dnd5e:armor:chain-shirt", "armor: dnd5e:armor:tinfoil-hat", 1)
	s.Require().NotEqual(string(s.yaml), broken)

	out := s.put([]byte(broken))

	s.Require().NotEmpty(out.Errors)
	s.Equal("templates.guard.armor", out.Errors[0].Path)
	s.Empty(out.Templates)
}

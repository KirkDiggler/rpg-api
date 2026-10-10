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

// TemplatesSuite pins that PutDungeon carries a template's refusal to the
// author as a field error, over the real registry and the real dungeonspec.
// A template authors like a mind table: the api answers whether it compiles,
// never what it derives (rpg-project#555 R7).
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

// TestPutDungeon_TemplatesWithNoDefectsGradeClean: the castle's templates
// validate with no errors and an atlas.
func (s *TemplatesSuite) TestPutDungeon_TemplatesWithNoDefectsGradeClean() {
	out := s.put(s.yaml)

	s.Empty(out.Errors)
	s.NotNil(out.Atlas)
}

// TestPutDungeon_TemplateRefusalIsAFieldError: a block the rulebook will not
// assemble reaches the author at its template's path.
func (s *TemplatesSuite) TestPutDungeon_TemplateRefusalIsAFieldError() {
	broken := strings.Replace(string(s.yaml), "armor: dnd5e:armor:chain-shirt", "armor: dnd5e:armor:tinfoil-hat", 1)
	s.Require().NotEqual(string(s.yaml), broken)

	out := s.put([]byte(broken))

	s.Require().NotEmpty(out.Errors)
	s.Equal("templates.guard", out.Errors[0].Path)
}

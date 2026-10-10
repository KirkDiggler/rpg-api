package sessionworld

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
)

// castleKitchenPath is the shipped walk dungeon for authored stat blocks
// (rpg-project#555), read from the content tree like the tomb.
var castleKitchenPath = filepath.Join("..", "..", "content", "castle-kitchen.yaml")

type TemplatesSuite struct {
	suite.Suite

	castle []byte
}

func TestTemplatesSuite(t *testing.T) {
	suite.Run(t, new(TemplatesSuite))
}

func (s *TemplatesSuite) SetupTest() {
	raw, err := os.ReadFile(castleKitchenPath)
	s.Require().NoError(err, "the castle must exist at content/castle-kitchen.yaml")
	s.castle = raw
}

// edited is the castle with one exact substring replaced, refusing a test
// whose edit silently matched nothing.
func (s *TemplatesSuite) edited(old, replacement string) []byte {
	s.Require().Contains(string(s.castle), old, "the edit must find its target")
	return []byte(strings.ReplaceAll(string(s.castle), old, replacement))
}

// fieldErrors compiles raw and returns the refusal's defects, failing the
// test when the compile is not a validation refusal.
func (s *TemplatesSuite) fieldErrors(raw []byte) []tkdungeonspec.FieldError {
	_, err := Compile(raw)
	s.Require().Error(err)
	s.Require().ErrorIs(err, tkdungeonspec.ErrBadSpec)
	var verr *tkdungeonspec.ValidationError
	s.Require().True(errors.As(err, &verr), "a template refusal is a ValidationError, got %v", err)
	return verr.Errors
}

func (s *TemplatesSuite) pathsOf(ferrs []tkdungeonspec.FieldError) []string {
	out := make([]string, len(ferrs))
	for i, fe := range ferrs {
		out[i] = fe.Path
	}
	return out
}

// TestCompile_TemplatesDerive: the castle's three blocks come back derived by
// the rulebook, sorted by id, with the guard at the design's numbers — HP 11
// from 2d8 and CON 12, AC 13 from a chain shirt at DEX 10, one spear.
func (s *TemplatesSuite) TestCompile_TemplatesDerive() {
	d, err := Compile(s.castle)
	s.Require().NoError(err)
	s.Require().Len(d.Templates, 3)

	ids := make([]string, len(d.Templates))
	for i, b := range d.Templates {
		ids[i] = b.ID
	}
	s.Equal([]string{"captain", "cook", "guard"}, ids, "sorted by template id")

	guard := d.Templates[2]
	s.Equal("dnd5e:monsters:guard", guard.Ref)
	s.Equal("Castle Guard", guard.Name)
	s.Equal(11, guard.HitPoints)
	s.Equal(13, guard.ArmorClass)
	s.Equal(12, guard.PassivePerception, "10 + WIS 0 + proficiency 2 for trained perception")
	s.Equal(2, guard.ProficiencyBonus, "the base's")
	s.Equal(13, guard.Abilities["str"], "stated")
	s.Equal(10, guard.Abilities["dex"], "the base's")
	s.Len(guard.Abilities, 6, "every score, merged")
	s.Require().Len(guard.Attacks, 1)
	s.Equal("dnd5e:weapons:spear", guard.Attacks[0].WeaponRef)
	s.Equal(3, guard.Attacks[0].AttackBonus, "STR +1 and proficiency 2")
	s.Equal("1d6+1", guard.Attacks[0].Damage)

	captain := d.Templates[0]
	s.Equal(3, captain.ProficiencyBonus, "stated")
	s.Require().Len(captain.Attacks, 2, "authored order: longsword, javelin")
	s.Equal("dnd5e:weapons:longsword", captain.Attacks[0].WeaponRef)
	s.Equal("dnd5e:weapons:javelin", captain.Attacks[1].WeaponRef)
}

// TestCompile_TemplateShadowRefused: a template named for a rulebook monster
// would make one ref name two stat blocks (R2), refused at its block.
func (s *TemplatesSuite) TestCompile_TemplateShadowRefused() {
	raw := s.edited("guard", "goblin")

	ferrs := s.fieldErrors(raw)
	s.Equal([]string{"templates.goblin"}, s.pathsOf(ferrs),
		"one defect, on the block; the placements naming it are not refused again")
	s.Contains(ferrs[0].Message, "shadows rulebook monster")
}

// TestCompile_TemplateShadowsABaseRefused: a rulebook base is rulebook
// content too — a template named `human` would shadow the block every other
// template derives from. THROUGH Compile, dungeonspec refuses it first, on
// shape: every `base: dnd5e:monsters:human` now names a template in the same
// file, and a template does not derive from a template — which is what this
// test pins. The shadowing refusal itself is pinned below by
// TestDeriveTemplates_ATemplateNamedForABaseIsShadowing, which calls
// deriveTemplates with no dungeonspec in front of it.
func (s *TemplatesSuite) TestCompile_TemplateShadowsABaseRefused() {
	raw := s.edited("  cook:\n    base:", "  human:\n    base:")

	ferrs := s.fieldErrors(raw)
	s.Contains(s.pathsOf(ferrs), "templates.human.base")
}

// TestCompile_TemplateUnknownBaseRefused: a base the rulebook does not ship is
// refused by name at the base field (R8), never derived from nothing.
func (s *TemplatesSuite) TestCompile_TemplateUnknownBaseRefused() {
	raw := s.edited("base: dnd5e:monsters:human\n    name: Castle Guard",
		"base: dnd5e:monsters:elf\n    name: Castle Guard")

	ferrs := s.fieldErrors(raw)
	s.Equal([]string{"templates.guard.base"}, s.pathsOf(ferrs))
	s.Contains(ferrs[0].Message, "dnd5e:monsters:elf")
}

// TestCompile_TemplateDerivationRefusalIsOnTheBlock: a refusal from the
// SDK's derivation lands on the template's block carrying the SDK's own
// sentence. The api does not parse it for a field.
func (s *TemplatesSuite) TestCompile_TemplateDerivationRefusalIsOnTheBlock() {
	raw := s.edited("armor: dnd5e:armor:chain-shirt", "armor: dnd5e:armor:tinfoil-hat")

	ferrs := s.fieldErrors(raw)
	s.Equal([]string{"templates.guard"}, s.pathsOf(ferrs))
	s.Contains(ferrs[0].Message, "tinfoil-hat", "the SDK's sentence names what it refused")
}

// TestCompile_TemplateAssemblyRefusalCarriesTheRulebooksReason: a refusal
// from the rulebook's assembly is on the block, carrying the rulebook's
// sentence through the SDK.
func (s *TemplatesSuite) TestCompile_TemplateAssemblyRefusalCarriesTheRulebooksReason() {
	raw := s.edited("abilities: { str: 13, con: 12, wis: 11 }", "abilities: { str: 13, con: 1, wis: 11 }")

	ferrs := s.fieldErrors(raw)
	s.Equal([]string{"templates.guard"}, s.pathsOf(ferrs))
	s.Contains(ferrs[0].Message, "hit points", "the rulebook's reason, carried")
}

// TestCompile_EveryDefectIsReported: two bad blocks are two defects.
func (s *TemplatesSuite) TestCompile_EveryDefectIsReported() {
	raw := []byte(strings.ReplaceAll(
		string(s.edited("armor: dnd5e:armor:chain-shirt", "armor: dnd5e:armor:tinfoil-hat")),
		"actions: [dnd5e:weapons:dagger]", "actions: [dnd5e:weapons:frying-pan]"))

	ferrs := s.fieldErrors(raw)
	s.Equal([]string{"templates.cook", "templates.guard"}, s.pathsOf(ferrs))
}

// TestCompile_UnknownMonsterIsStillRefused: a ref that names neither a
// constructor nor a template is today's unknown-monster refusal.
func (s *TemplatesSuite) TestCompile_UnknownMonsterIsStillRefused() {
	raw := s.edited("ref: 'dnd5e:monsters:cook', startingCell: { location: { q: -1, r: 2 } }",
		"ref: 'dnd5e:monsters:scullion', startingCell: { location: { q: -1, r: 2 } }")

	ferrs := s.fieldErrors(raw)
	s.Require().Len(ferrs, 1)
	s.Contains(ferrs[0].Message, `references unknown monster "dnd5e:monsters:scullion"`)
}

// TestCompile_ReferenceDungeonsDeclareNoTemplates: every other shipped
// dungeon still compiles, with zero derived blocks.
func (s *TemplatesSuite) TestCompile_ReferenceDungeonsDeclareNoTemplates() {
	paths, err := filepath.Glob(filepath.Join("..", "..", "content", "*.yaml"))
	s.Require().NoError(err)
	s.Require().NotEmpty(paths)

	for _, path := range paths {
		if path == castleKitchenPath {
			continue
		}
		raw, err := os.ReadFile(path)
		s.Require().NoError(err)
		d, err := Compile(raw)
		s.Require().NoErrorf(err, "%s must compile", path)
		s.Emptyf(d.Templates, "%s declares no templates", path)
	}
}

// TestDeriveTemplates_ATemplateNamedForABaseIsShadowing is the reviewer's
// probe on rpg-api#1090: a template named `human` takes the ref of the
// rulebook base every other template derives from. Called directly, with no
// dungeonspec in front of it, because through Compile the shape check
// refuses this file first and the shadowing refusal is never reached.
//
// The refusal is session.DeriveTemplate's (ErrShadowedRef: the ref names a
// rulebook monster or base), mapped here onto the template's block — the api
// keeps no second copy of the rule.
func TestDeriveTemplates_ATemplateNamedForABaseIsShadowing(t *testing.T) {
	blocks, ferrs := deriveTemplates(map[string]tkdungeonspec.TemplateSpec{
		"human": {Base: "dnd5e:monsters:human", Actions: []string{"dnd5e:weapons:dagger"}},
	})

	require.Empty(t, blocks, "a shadowing template derives no block")
	require.Len(t, ferrs, 1)
	require.Equal(t, "templates.human", ferrs[0].Path)
	require.Contains(t, ferrs[0].Message, "shadows rulebook monster")
}

// TestDeriveTemplates_ATemplateNamedForAMonsterIsShadowing is the same
// refusal for a rulebook constructor, through the same mapping.
func TestDeriveTemplates_ATemplateNamedForAMonsterIsShadowing(t *testing.T) {
	blocks, ferrs := deriveTemplates(map[string]tkdungeonspec.TemplateSpec{
		"goblin": {Base: "dnd5e:monsters:human", Actions: []string{"dnd5e:weapons:dagger"}},
	})

	require.Empty(t, blocks)
	require.Len(t, ferrs, 1)
	require.Equal(t, "templates.goblin", ferrs[0].Path)
	require.Contains(t, ferrs[0].Message, "shadows rulebook monster")
}

// TestDeriveTemplates_ShadowingIsTheBlocksDefectEvenWithABadBase: a template
// that both shadows a rulebook monster AND names a base the rulebook does not
// ship is one defect, on the block — the ref having two sources is the
// author's first problem, and the ErrShadowedRef mapping is what keeps it
// from being reported as a bad base.
func TestDeriveTemplates_ShadowingIsTheBlocksDefectEvenWithABadBase(t *testing.T) {
	_, ferrs := deriveTemplates(map[string]tkdungeonspec.TemplateSpec{
		"goblin": {Base: "dnd5e:monsters:elf", Actions: []string{"dnd5e:weapons:dagger"}},
	})

	require.Len(t, ferrs, 1)
	require.Equal(t, "templates.goblin", ferrs[0].Path)
	require.Contains(t, ferrs[0].Message, "shadows rulebook monster")
}

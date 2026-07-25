package encounter_test

// character_resolver_test.go — rpg-api#516: unit tests for
// Dnd5eCharacterResolver, the real CharacterResolver implementation that
// replaces StubCharacterResolver in production.
//
// The toolkit's tkenc.CharacterResolver interface hands the resolver only a
// core.PlayerID (no encounterID, no EntityID) — see prompts.go's doc. The
// resolver bridges that to a character by reading the bound encounter's own
// PlayerData.EntityID (data.Players[playerID].EntityID), exactly the field
// rpg-api#516's stated blocker claimed didn't carry a real character
// reference. Wave 2.9's start_encounter.go already sets
// EntityID: core.EntityID(m.CharacterID) — the blocker was stale by the time
// this shipped. Because the toolkit interface only carries a PlayerID (not
// the loaded encounter's Data), the resolver must be constructed per-request
// from that Data (NewDnd5eCharacterResolverForData), mirroring the existing
// Dnd5eCombatResolver / Dnd5eMovementResolver per-request builder pattern —
// a resolver built once at server-startup could never see a specific
// encounter's Players map.

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// Dnd5eCharacterResolverTestSuite exercises AbilityModifier and
// ToolProficiencyBonus against a mocked character store.
type Dnd5eCharacterResolverTestSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
}

func TestDnd5eCharacterResolverTestSuite(t *testing.T) {
	suite.Run(t, new(Dnd5eCharacterResolverTestSuite))
}

func (s *Dnd5eCharacterResolverTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
}

func (s *Dnd5eCharacterResolverTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// aliceData is Alice the Rogue from the live rpg-dnd5e-web#589 evidence:
// dex 16 (+3 modifier), proficiency_bonus 2, proficient with thieves' tools.
func aliceData() *tkcharacter.Data {
	return &tkcharacter.Data{
		ID:               "char-alice",
		PlayerID:         "alice",
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 10,
			abilities.DEX: 16,
			abilities.CON: 12,
			abilities.INT: 10,
			abilities.WIS: 10,
			abilities.CHA: 8,
		},
		ToolProficiencies: []proficiencies.Tool{proficiencies.ToolThieves},
	}
}

// encDataWithAlice builds a minimal tkenc.Data seating "alice" bound to
// "char-alice" — the same PlayerID -> EntityID shape start_encounter.go
// produces via tkenc.PlayerInput{PlayerID, EntityID: core.EntityID(CharacterID)}.
func encDataWithAlice() *tkenc.Data {
	return &tkenc.Data{
		Players: map[encountercore.PlayerID]*tkenc.PlayerData{
			"alice": {ID: "alice", EntityID: "char-alice"},
		},
	}
}

// ---------------------------------------------------------------------------
// AbilityModifier
// ---------------------------------------------------------------------------

// TestAbilityModifier_KnownCharacter_ReturnsRealModifier is the done-bar
// scenario from the issue: Alice's dex 16 must resolve to +3 (the toolkit's
// own shared.AbilityScores.Modifier((16-10)/2)), not the stub's hardcoded 0.
func (s *Dnd5eCharacterResolverTestSuite) TestAbilityModifier_KnownCharacter_ReturnsRealModifier() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: aliceData()}}, nil)

	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		encDataWithAlice(),
	)

	mod, ok := resolver.AbilityModifier("alice", "dex")
	s.True(ok)
	s.Equal(3, mod)
}

// TestAbilityModifier_UppercaseAbility_NormalizesToLowercase proves the
// resolver handles the "DEX" convention (rpg-toolkit encounter/data.go's
// documented 3-letter-code doc comment) as well as the lowercase "dex" the
// crypt boss door actually stores (dungeon_spec.go / start_encounter_test.go
// s.Require().Equal("dex", bossDoor.LockAbility)) — both conventions appear
// in this codebase.
func (s *Dnd5eCharacterResolverTestSuite) TestAbilityModifier_UppercaseAbility_NormalizesToLowercase() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: aliceData()}}, nil)

	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		encDataWithAlice(),
	)

	mod, ok := resolver.AbilityModifier("alice", "DEX")
	s.True(ok)
	s.Equal(3, mod)
}

// TestAbilityModifier_UnknownPlayer_ReturnsFalse: a player not seated on
// this encounter's Data.Players has no bound character. ok=false, which the
// toolkit's SubmitCheck treats as a zero modifier (prompts.go's documented
// "unknowns are zero" contract) rather than erroring.
func (s *Dnd5eCharacterResolverTestSuite) TestAbilityModifier_UnknownPlayer_ReturnsFalse() {
	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		encDataWithAlice(),
	)

	mod, ok := resolver.AbilityModifier("mallory", "dex")
	s.False(ok)
	s.Equal(0, mod)
}

// TestAbilityModifier_CharacterStoreMiss_ReturnsFalse: the player is seated
// (PlayerData.EntityID is set) but the character store has no record for
// that ID (e.g. deleted mid-session). Must degrade to ok=false, not panic
// or fabricate a zero-as-known answer.
func (s *Dnd5eCharacterResolverTestSuite) TestAbilityModifier_CharacterStoreMiss_ReturnsFalse() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(nil, apierr.NotFound("character not found"))

	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		encDataWithAlice(),
	)

	mod, ok := resolver.AbilityModifier("alice", "dex")
	s.False(ok)
	s.Equal(0, mod)
}

// TestAbilityModifier_UnrecognizedAbility_ReturnsFalse: a garbage ability
// string (never emitted by the toolkit today, but the interface takes a
// bare string) must not panic map indexing into shared.AbilityScores.
func (s *Dnd5eCharacterResolverTestSuite) TestAbilityModifier_UnrecognizedAbility_ReturnsFalse() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: aliceData()}}, nil)

	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		encDataWithAlice(),
	)

	mod, ok := resolver.AbilityModifier("alice", "nonsense")
	s.False(ok)
	s.Equal(0, mod)
}

// TestAbilityModifier_NilData_ReturnsFalse: a resolver built with nil data
// (mirrors the lobby orchestrator's StartEncounter call site, which builds
// resolvers before any player exists) must degrade safely rather than
// nil-deref data.Players.
func (s *Dnd5eCharacterResolverTestSuite) TestAbilityModifier_NilData_ReturnsFalse() {
	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		nil,
	)

	mod, ok := resolver.AbilityModifier("alice", "dex")
	s.False(ok)
	s.Equal(0, mod)
}

// TestAbilityModifier_NilCharacterRepo_ReturnsFalse: no character store
// wired (e.g. a handler test fixture) degrades safely instead of panicking
// on a nil CharacterRepo.Get call.
func (s *Dnd5eCharacterResolverTestSuite) TestAbilityModifier_NilCharacterRepo_ReturnsFalse() {
	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{},
		encDataWithAlice(),
	)

	mod, ok := resolver.AbilityModifier("alice", "dex")
	s.False(ok)
	s.Equal(0, mod)
}

// ---------------------------------------------------------------------------
// ToolProficiencyBonus
// ---------------------------------------------------------------------------

// TestToolProficiencyBonus_Proficient_ReturnsProficiencyBonus: Alice is
// proficient with thieves' tools, so the bonus is her stored
// ProficiencyBonus (2) — never computed from level in rpg-api (that's a
// stored field on tkcharacter.Data, not game math this package performs).
func (s *Dnd5eCharacterResolverTestSuite) TestToolProficiencyBonus_Proficient_ReturnsProficiencyBonus() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: aliceData()}}, nil)

	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		encDataWithAlice(),
	)

	bonus, ok := resolver.ToolProficiencyBonus("alice", string(proficiencies.ToolThieves))
	s.True(ok)
	s.Equal(2, bonus)
}

// TestToolProficiencyBonus_Proficient_RefFormTool_ReturnsProficiencyBonus is
// the production shape: SubmitCheck never hands the resolver a bare
// proficiencies.Tool id — it passes door.LockTool verbatim
// (rpg-toolkit/encounter/prompts.go's AttemptUnlock sets
// PendingPrompt.Tool = door.LockTool, and SubmitCheck calls
// resolver.ToolProficiencyBonus(playerID, prompt.Tool) unmodified), and
// LockTool is documented as a full toolkit ref: "LockTool is a toolkit ref
// (e.g. \"dnd5e:item:thieves-tools\")" (rpg-toolkit/encounter/data.go).
// Comparing that ref directly against tkcharacter.Data.ToolProficiencies'
// bare ids (proficiencies.ToolThieves == "thieves-tools") would never
// match, silently reporting a proficient character as not proficient — a
// confident-wrong ok=true,0, not the honest "unknown" the interface
// documents. This is the test the tool-ref/bare-id mismatch bug slipped
// past when only bare-id inputs were exercised.
func (s *Dnd5eCharacterResolverTestSuite) TestToolProficiencyBonus_Proficient_RefFormTool_ReturnsProficiencyBonus() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: aliceData()}}, nil)

	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		encDataWithAlice(),
	)

	bonus, ok := resolver.ToolProficiencyBonus("alice", "dnd5e:item:thieves-tools")
	s.True(ok)
	s.Equal(2, bonus)
}

// TestToolProficiencyBonus_NotProficient_ReturnsKnownZero: a known character
// who simply isn't proficient with the requested tool is a real, known
// answer (bonus is zero) — ok=true, distinct from an unknown player/
// character (ok=false). Matches the crypt boss door today, which sets no
// LockTool at all (start_encounter_test.go: s.Require().Empty(bossDoor.LockTool)),
// so this path — proficiencies present but not matching — is the honest
// "no bonus" case, not a stub artifact.
func (s *Dnd5eCharacterResolverTestSuite) TestToolProficiencyBonus_NotProficient_ReturnsKnownZero() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: aliceData()}}, nil)

	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		encDataWithAlice(),
	)

	bonus, ok := resolver.ToolProficiencyBonus("alice", string(proficiencies.ToolAlchemist))
	s.True(ok)
	s.Equal(0, bonus)
}

// TestToolProficiencyBonus_NilToolProficiencies_ReturnsKnownZero: seeded
// characters carry tool_proficiencies == null (the issue's own note); a nil
// slice must range cleanly to "not proficient", not panic.
func (s *Dnd5eCharacterResolverTestSuite) TestToolProficiencyBonus_NilToolProficiencies_ReturnsKnownZero() {
	data := aliceData()
	data.ToolProficiencies = nil
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: data}}, nil)

	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		encDataWithAlice(),
	)

	bonus, ok := resolver.ToolProficiencyBonus("alice", string(proficiencies.ToolThieves))
	s.True(ok)
	s.Equal(0, bonus)
}

// TestToolProficiencyBonus_EmptyTool_ReturnsKnownZero: SubmitCheck only
// calls ToolProficiencyBonus when prompt.Tool != "" (rpg-toolkit's
// prompts.go), but the resolver itself must not depend on that caller
// discipline — an empty tool string should degrade to "known character,
// not proficient with ”" (ok=true, 0), not panic or falsely match some
// proficiency.
func (s *Dnd5eCharacterResolverTestSuite) TestToolProficiencyBonus_EmptyTool_ReturnsKnownZero() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: aliceData()}}, nil)

	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		encDataWithAlice(),
	)

	bonus, ok := resolver.ToolProficiencyBonus("alice", "")
	s.True(ok)
	s.Equal(0, bonus)
}

// TestToolProficiencyBonus_UnknownPlayer_ReturnsFalse mirrors
// TestAbilityModifier_UnknownPlayer_ReturnsFalse for the tool-bonus method.
func (s *Dnd5eCharacterResolverTestSuite) TestToolProficiencyBonus_UnknownPlayer_ReturnsFalse() {
	resolver := v2encounter.NewDnd5eCharacterResolverForData(
		v2encounter.Dnd5eCharacterResolverConfig{CharacterRepo: s.mockCharRepo},
		encDataWithAlice(),
	)

	bonus, ok := resolver.ToolProficiencyBonus("mallory", string(proficiencies.ToolThieves))
	s.False(ok)
	s.Equal(0, bonus)
}

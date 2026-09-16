package character

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/core"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
)

const (
	levelUpCharacterID = "char-1"
	levelUpPlayerID    = "player-1"
)

type LevelUpHandlerTestSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockService *charactermock.MockService
	handler     *Handler
	ctx         context.Context
}

func TestLevelUpHandlerSuite(t *testing.T) {
	suite.Run(t, new(LevelUpHandlerTestSuite))
}

func (s *LevelUpHandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = charactermock.NewMockService(s.ctrl)
	s.ctx = auth.WithPlayerID(context.Background(), levelUpPlayerID)

	var err error
	s.handler, err = NewHandler(&HandlerConfig{CharacterService: s.mockService})
	s.Require().NoError(err)
}

func (s *LevelUpHandlerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// expectOwned answers the ownership gate with a character this caller owns.
func (s *LevelUpHandlerTestSuite) expectOwned(classID classes.Class) {
	s.mockService.EXPECT().
		GetCharacter(gomock.Any(), &character.GetCharacterInput{CharacterID: levelUpCharacterID}).
		Return(&character.GetCharacterOutput{
			Character: &entities.Character{Data: &toolkitchar.Data{
				ID:       levelUpCharacterID,
				PlayerID: levelUpPlayerID,
				ClassID:  classID,
				Level:    1,
			}},
		}, nil)
}

// TestGetNextLevel_FighterIsAConfirmationThatNamesWhatItBrings pins R4.14: a
// level that asks nothing is still a screen with content. The response carries
// the feature and the hit die, which is what "Level 2: Action Surge" is made
// of; an empty choice list alone would be a bare button.
func (s *LevelUpHandlerTestSuite) TestGetNextLevel_FighterIsAConfirmationThatNamesWhatItBrings() {
	s.expectOwned(classes.Fighter)
	s.mockService.EXPECT().
		GetNextLevel(gomock.Any(), &character.GetNextLevelInput{CharacterID: levelUpCharacterID}).
		Return(&character.GetNextLevelOutput{
			CharacterLevel: 2,
			ClassID:        classes.Fighter,
			ClassLevel:     2,
			Requirements:   &choices.Requirements{},
			FeatureRefs:    []string{refs.Features.ActionSurge().String()},
			HitDice:        10,
		}, nil)

	got, err := s.handler.GetNextLevel(s.ctx, &dnd5ev1alpha1.GetNextLevelRequest{
		CharacterId: levelUpCharacterID,
	})

	s.Require().NoError(err)
	s.Equal(int32(2), got.GetLevel())
	s.Equal(dnd5ev1alpha1.Class_CLASS_FIGHTER, got.GetClass())
	s.Empty(got.GetChoices())
	s.Equal(int32(10), got.GetHitDie())
	s.Require().Len(got.GetFeatures(), 1)
	s.Equal("Action Surge", got.GetFeatures()[0].GetName())
	s.Equal(refs.Features.ActionSurge().String(), got.GetFeatures()[0].GetId())
	s.Equal(int32(2), got.GetFeatures()[0].GetLevel())
	s.Equal("Fighter", got.GetFeatures()[0].GetClassName())
}

// TestGetNextLevel_ProjectsTheRequirementItIsHanded is the wire half of R4.13:
// whatever requirement row arrives becomes the same Choice message creation
// renders, so the screen stays generic.
//
// The row here is WRITTEN OUT rather than fetched from the class table. Which
// options a bard should be offered is the orchestrator's claim, tested there
// against a real sheet; if this built its input from the same table the
// projection reads, it could not fail when the projection dropped or mangled
// an option. One spell in, one spell out, named.
func (s *LevelUpHandlerTestSuite) TestGetNextLevel_ProjectsTheRequirementItIsHanded() {
	s.expectOwned(classes.Bard)
	s.mockService.EXPECT().
		GetNextLevel(gomock.Any(), &character.GetNextLevelInput{CharacterID: levelUpCharacterID}).
		Return(&character.GetNextLevelOutput{
			CharacterLevel: 2,
			ClassID:        classes.Bard,
			ClassLevel:     2,
			Requirements: &choices.Requirements{
				Spellbook: &choices.SpellbookRequirement{
					ID:         "bard-spells-2",
					Count:      1,
					SpellLevel: 1,
					Options:    []spells.Spell{spells.HealingWord},
					Label:      "Choose 1 spell",
				},
			},
			HitDice: 8,
		}, nil)

	got, err := s.handler.GetNextLevel(s.ctx, &dnd5ev1alpha1.GetNextLevelRequest{
		CharacterId: levelUpCharacterID,
	})

	s.Require().NoError(err)
	s.Require().Len(got.GetChoices(), 1, "the spell is the only thing this row asks for")
	choice := got.GetChoices()[0]
	s.Equal("bard-spells-2", choice.GetId())
	s.Equal(int32(1), choice.GetChooseCount())
	s.Equal(dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS, choice.GetChoiceType())
	s.Equal([]string{refs.Spells.HealingWord().String()}, choice.GetSpellOptions().GetAvailableRefs(),
		"canonical refs, the vocabulary the rest of the spell wire speaks, and exactly the one offered")
	s.Empty(got.GetFeatures())
}

// TestGetNextLevel_AForeignCharacterIsNotFound is the v2 handler's own rule:
// NOT_FOUND, never PERMISSION_DENIED, because PERMISSION_DENIED would confirm
// that a character exists at that id.
func (s *LevelUpHandlerTestSuite) TestGetNextLevel_AForeignCharacterIsNotFound() {
	s.mockService.EXPECT().
		GetCharacter(gomock.Any(), &character.GetCharacterInput{CharacterID: levelUpCharacterID}).
		Return(&character.GetCharacterOutput{
			Character: &entities.Character{Data: &toolkitchar.Data{
				ID:       levelUpCharacterID,
				PlayerID: "somebody-else",
			}},
		}, nil)

	_, err := s.handler.GetNextLevel(s.ctx, &dnd5ev1alpha1.GetNextLevelRequest{
		CharacterId: levelUpCharacterID,
	})

	s.Equal(codes.NotFound, status.Code(err))
}

// TestLevelUp_AForeignCharacterIsNotFound: the write refuses the same way the
// read does, and for the same reason.
func (s *LevelUpHandlerTestSuite) TestLevelUp_AForeignCharacterIsNotFound() {
	s.mockService.EXPECT().
		GetCharacter(gomock.Any(), &character.GetCharacterInput{CharacterID: levelUpCharacterID}).
		Return(&character.GetCharacterOutput{
			Character: &entities.Character{Data: &toolkitchar.Data{
				ID:       levelUpCharacterID,
				PlayerID: "somebody-else",
			}},
		}, nil)

	_, err := s.handler.LevelUp(s.ctx, &dnd5ev1alpha1.LevelUpRequest{
		CharacterId:    levelUpCharacterID,
		HitPointMethod: dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_AVERAGE,
	})

	s.Equal(codes.NotFound, status.Code(err))
}

// TestLevelUp_UnspecifiedMethodNeverReachesTheToolkit. The mock has no LevelUp
// expectation, so a call through would fail the test rather than pass
// quietly. UNSPECIFIED is refused rather than defaulted because rolled and
// averaged write different numbers into a record nobody can correct.
func (s *LevelUpHandlerTestSuite) TestLevelUp_UnspecifiedMethodNeverReachesTheToolkit() {
	_, err := s.handler.LevelUp(s.ctx, &dnd5ev1alpha1.LevelUpRequest{
		CharacterId: levelUpCharacterID,
	})

	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(status.Convert(err).Message(), "hit_point_method")
}

// TestLevelUp_ReportsWhatTheLevelBrought is done-when 5 on the wire: the
// chosen spell is applied, the pools that moved are named, and the character
// comes back projected. R4.7 is why the resource changes are here at all --
// "a slot increase is not a question and is applied without asking", so
// nothing on the screen would mention it unless the engine said so.
func (s *LevelUpHandlerTestSuite) TestLevelUp_ReportsWhatTheLevelBrought() {
	s.expectOwned(classes.Bard)
	s.mockService.EXPECT().
		LevelUp(gomock.Any(), &character.LevelUpInput{
			CharacterID:    levelUpCharacterID,
			HitPointMethod: toolkitchar.HitPointMethodAverage,
			Choices: []choices.ChoiceData{{
				Category:       shared.ChoiceSpells,
				Source:         shared.SourceClass,
				ChoiceID:       "bard-spells-2",
				SpellSelection: []spells.Spell{spells.HealingWord},
			}},
		}).
		Return(&character.LevelUpOutput{
			Character: &entities.Character{Data: &toolkitchar.Data{
				ID:         levelUpCharacterID,
				PlayerID:   levelUpPlayerID,
				ClassID:    classes.Bard,
				Level:      2,
				Experience: 300,
			}},
			Gained: toolkitchar.GainedAtLevel{
				CharacterLevel: 2,
				ClassLevel:     2,
				HitPointGain:   6,
				Resources: []toolkitchar.ResourceChange{
					{Key: resources.SpellSlotLevel1, From: 2, To: 3},
					{Key: resources.HitDice, From: 1, To: 2},
				},
			},
		}, nil)

	got, err := s.handler.LevelUp(s.ctx, &dnd5ev1alpha1.LevelUpRequest{
		CharacterId:    levelUpCharacterID,
		HitPointMethod: dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_AVERAGE,
		Choices: []*dnd5ev1alpha1.ChoiceData{{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId: "bard-spells-2",
			Selection: &dnd5ev1alpha1.ChoiceData_Spells{Spells: &dnd5ev1alpha1.SpellSelection{
				SpellRefs: []string{refs.Spells.HealingWord().String()},
			}},
		}},
	})

	s.Require().NoError(err)
	s.Equal(int32(2), got.GetCharacter().GetLevel())
	s.Equal(int32(300), got.GetCharacter().GetExperiencePoints())
	s.Equal(int32(2), got.GetCharacter().GetEntitledLevel(), "the gap closed")
	s.Equal(int32(2), got.GetGained().GetLevel())
	s.Equal(int32(6), got.GetGained().GetHitPointsGained())

	s.Require().Len(got.GetGained().GetResourceChanges(), 2)
	first := got.GetGained().GetResourceChanges()[0]
	s.Equal(string(resources.SpellSlotLevel1), first.GetKey())
	s.Equal("1st-level Spell Slots", first.GetName())
	s.Equal(int32(2), first.GetPreviousMaximum())
	s.Equal(int32(3), first.GetNewMaximum())
	second := got.GetGained().GetResourceChanges()[1]
	s.Equal(string(resources.HitDice), second.GetKey())
	s.Equal("Hit Dice", second.GetName())
	s.Equal(int32(1), second.GetPreviousMaximum())
	s.Equal(int32(2), second.GetNewMaximum())
}

// TestLevelUp_GainedFeaturesAreNamed covers the fighter half of the wire: the
// features Advance reports come back as FeatureInfo the confirmation screen
// can read.
func (s *LevelUpHandlerTestSuite) TestLevelUp_GainedFeaturesAreNamed() {
	s.expectOwned(classes.Fighter)
	s.mockService.EXPECT().
		LevelUp(gomock.Any(), gomock.Any()).
		Return(&character.LevelUpOutput{
			Character: &entities.Character{Data: &toolkitchar.Data{
				ID:       levelUpCharacterID,
				PlayerID: levelUpPlayerID,
				ClassID:  classes.Fighter,
				Level:    2,
			}},
			Gained: toolkitchar.GainedAtLevel{
				CharacterLevel: 2,
				HitPointGain:   7,
				Features:       []core.Ref{*refs.Features.ActionSurge()},
			},
		}, nil)

	got, err := s.handler.LevelUp(s.ctx, &dnd5ev1alpha1.LevelUpRequest{
		CharacterId:    levelUpCharacterID,
		HitPointMethod: dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_ROLLED,
	})

	s.Require().NoError(err)
	s.Require().Len(got.GetGained().GetFeatures(), 1)
	s.Equal("Action Surge", got.GetGained().GetFeatures()[0].GetName())
	s.Equal(refs.Features.ActionSurge().String(), got.GetGained().GetFeatures()[0].GetId())
	s.Empty(got.GetGained().GetResourceChanges(), "a level that moved no pool says so by saying nothing")
}

// TestLevelUp_CarriesTheToolkitRefusalWord for word. The refusal names the
// experience total and the threshold; replacing it would leave the player told
// only that something went wrong.
func (s *LevelUpHandlerTestSuite) TestLevelUp_CarriesTheToolkitRefusalWord() {
	s.expectOwned(classes.Fighter)
	s.mockService.EXPECT().
		LevelUp(gomock.Any(), gomock.Any()).
		Return(nil, apierr.FailedPrecondition(`character "char-1" has 0 experience and level 2 needs 300`))

	_, err := s.handler.LevelUp(s.ctx, &dnd5ev1alpha1.LevelUpRequest{
		CharacterId:    levelUpCharacterID,
		HitPointMethod: dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_AVERAGE,
	})

	s.Equal(codes.FailedPrecondition, status.Code(err))
	s.Contains(status.Convert(err).Message(), "300")
	s.Contains(status.Convert(err).Message(), "0 experience")
}

// TestProtoChoicesToToolkit_ReadsEveryCategoryTheScreenCanSend is the reverse
// converter's own test, written against explicit expected values rather than a
// round trip: a swap that is identical in both directions passes every round
// trip and is still wrong.
func (s *LevelUpHandlerTestSuite) TestProtoChoicesToToolkit_ReadsEveryCategoryTheScreenCanSend() {
	got, err := protoChoicesToToolkit([]*dnd5ev1alpha1.ChoiceData{
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId: "rogue-skills-2",
			Selection: &dnd5ev1alpha1.ChoiceData_Skills{Skills: &dnd5ev1alpha1.SkillSelection{
				Skills: []dnd5ev1alpha1.Skill{dnd5ev1alpha1.Skill_SKILL_STEALTH},
			}},
		},
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId: "bard-expertise-3",
			Selection: &dnd5ev1alpha1.ChoiceData_Expertise{Expertise: &dnd5ev1alpha1.ExpertiseSelection{
				Skills: []dnd5ev1alpha1.Skill{dnd5ev1alpha1.Skill_SKILL_PERSUASION},
			}},
		},
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId: "bard-instruments",
			Selection: &dnd5ev1alpha1.ChoiceData_Tools{Tools: &dnd5ev1alpha1.ToolSelection{
				Tools: []dnd5ev1alpha1.Tool{dnd5ev1alpha1.Tool_TOOL_LUTE},
			}},
		},
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId: "bard-cantrips-4",
			Selection: &dnd5ev1alpha1.ChoiceData_Spells{Spells: &dnd5ev1alpha1.SpellSelection{
				SpellRefs: []string{refs.Spells.ViciousMockery().String()},
			}},
		},
	})

	s.Require().NoError(err)
	s.Require().Len(got, 4)

	s.Equal(shared.ChoiceSkills, got[0].Category)
	s.Equal(shared.SourceClass, got[0].Source)
	s.Equal(choices.ChoiceID("rogue-skills-2"), got[0].ChoiceID)
	s.Equal([]skills.Skill{skills.Stealth}, got[0].SkillSelection)

	s.Equal(shared.ChoiceExpertise, got[1].Category)
	s.Equal([]skills.Skill{skills.Persuasion}, got[1].ExpertiseSelection)

	s.Equal(shared.ChoiceToolProficiency, got[2].Category)
	s.Equal([]proficiencies.Tool{proficiencies.ToolLute}, got[2].ToolSelection)

	s.Equal(shared.ChoiceCantrips, got[3].Category)
	s.Equal([]spells.Spell{spells.ViciousMockery}, got[3].SpellSelection)
}

// TestProtoChoicesToToolkit_RefusesAnUnreadableChoice: a dropped answer
// reaches the engine as a missing one, and the player is told they failed to
// choose something they did choose.
func (s *LevelUpHandlerTestSuite) TestProtoChoicesToToolkit_RefusesAnUnreadableChoice() {
	_, err := protoChoicesToToolkit([]*dnd5ev1alpha1.ChoiceData{{
		Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_UNSPECIFIED,
		ChoiceId: "mystery",
	}})

	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(status.Convert(err).Message(), "mystery")
}

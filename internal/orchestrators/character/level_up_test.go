package character

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/fightingstyles"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/languages"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

// fixedRoller is a dice.Roller that always returns the same face.
//
// A level's hit point gain is written into an append-only record, so a test
// that let the production roller decide it would be asserting on entropy. This
// substitutes at the seam Config.Roller exists for: "the override exists so a
// deterministic roller can be substituted in tests (a fixed source makes a
// reproducible fight)".
type fixedRoller int

func (f fixedRoller) Roll(_ context.Context, _ int) (int, error) { return int(f), nil }

func (f fixedRoller) RollN(_ context.Context, count, _ int) ([]int, error) {
	out := make([]int, 0, count)
	for range count {
		out = append(out, int(f))
	}
	return out, nil
}

// LevelUpTestSuite covers the two advancement verbs against real toolkit
// sheets. The fixtures are built through the draft the way a real character is
// -- Load reconstitutes a sheet from feature BLOBS, never from grants, so a
// hand-authored Data cannot stand in for one that was actually created.
type LevelUpTestSuite struct {
	suite.Suite
	ctrl              *gomock.Controller
	mockCharacterRepo *charactermock.MockRepository
	orchestrator      *Orchestrator
	ctx               context.Context
}

func TestLevelUpSuite(t *testing.T) {
	suite.Run(t, new(LevelUpTestSuite))
}

func (s *LevelUpTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharacterRepo = charactermock.NewMockRepository(s.ctrl)
	s.ctx = context.Background()

	var err error
	s.orchestrator, err = New(&Config{
		DraftRepo:          draftmock.NewMockRepository(s.ctrl),
		CharacterRepo:      s.mockCharacterRepo,
		DiceService:        dicemock.NewMockService(s.ctrl),
		IDGenerator:        idgenmock.NewMockGenerator(s.ctrl),
		DraftIDGenerator:   idgenmock.NewMockGenerator(s.ctrl),
		AppearanceNotifier: NoAppearanceNotifier{},
		// Every face is a 5. A d10 hit die averages 6, so a rolled gain and an
		// averaged one cannot be confused for each other in any assertion
		// below.
		Roller: fixedRoller(5),
	})
	s.Require().NoError(err)
}

func (s *LevelUpTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// bardData finalizes a level-1 bard through the draft and returns its stored
// sheet, holding the experience the caller names.
func (s *LevelUpTestSuite) bardData(id string, experience int) *tkcharacter.Data {
	draft, err := tkcharacter.NewDraft(&tkcharacter.DraftConfig{ID: "bard-draft", PlayerID: "player-1"})
	s.Require().NoError(err)
	s.Require().NoError(draft.SetName(&tkcharacter.SetNameInput{Name: "Scanlan"}))
	s.Require().NoError(draft.SetRace(&tkcharacter.SetRaceInput{
		RaceID:  races.Human,
		Choices: tkcharacter.RaceChoices{Languages: []languages.Language{languages.Dwarvish}},
	}))
	s.Require().NoError(draft.SetClass(&tkcharacter.SetClassInput{
		ClassID: classes.Bard,
		Choices: tkcharacter.ClassChoices{
			Skills:   []skills.Skill{skills.Performance, skills.Persuasion, skills.Deception},
			Tools:    []shared.SelectionID{"lute", "flute", "drum"},
			Cantrips: []shared.SelectionID{spells.TrueStrike, spells.ViciousMockery},
			Spells: []spells.Spell{
				spells.Bane, spells.Thunderwave, spells.DissonantWhispers, spells.Command,
			},
			Equipment: []tkcharacter.EquipmentChoiceSelection{
				{ChoiceID: choices.BardWeaponsPrimary, OptionID: choices.BardWeaponRapier},
				{ChoiceID: choices.BardPack, OptionID: choices.BardPackDiplomat},
				{ChoiceID: choices.BardInstrument, OptionID: choices.BardInstrumentLute},
			},
		},
	}))
	s.Require().NoError(draft.SetBackground(&tkcharacter.SetBackgroundInput{BackgroundID: backgrounds.Hermit}))
	s.Require().NoError(draft.SetAbilityScores(&tkcharacter.SetAbilityScoresInput{
		Scores: shared.AbilityScores{
			abilities.STR: 8, abilities.DEX: 14, abilities.CON: 13,
			abilities.INT: 10, abilities.WIS: 12, abilities.CHA: 16,
		},
		Method: "standard-array",
	}))

	char, err := draft.ToCharacter(s.ctx, id, events.NewEventBus())
	s.Require().NoError(err)

	data := char.ToData()
	data.Experience = experience
	return data
}

// fighterData finalizes a level-1 human fighter through the draft.
func (s *LevelUpTestSuite) fighterData(id string, experience int) *tkcharacter.Data {
	draft, err := tkcharacter.NewDraft(&tkcharacter.DraftConfig{ID: "fighter-draft", PlayerID: "player-1"})
	s.Require().NoError(err)
	s.Require().NoError(draft.SetName(&tkcharacter.SetNameInput{Name: "Arthur"}))
	s.Require().NoError(draft.SetRace(&tkcharacter.SetRaceInput{
		RaceID:  races.Human,
		Choices: tkcharacter.RaceChoices{Languages: []languages.Language{languages.Dwarvish}},
	}))
	s.Require().NoError(draft.SetClass(&tkcharacter.SetClassInput{
		ClassID: classes.Fighter,
		Choices: tkcharacter.ClassChoices{
			Skills:        []skills.Skill{skills.Athletics, skills.Intimidation},
			FightingStyle: fightingstyles.Defense,
			Equipment: []tkcharacter.EquipmentChoiceSelection{
				{ChoiceID: choices.FighterArmor, OptionID: "fighter-armor-a"},
				{
					ChoiceID:           choices.FighterWeaponsPrimary,
					OptionID:           "fighter-weapon-a",
					CategorySelections: []shared.EquipmentID{"longsword"},
				},
				{ChoiceID: choices.FighterWeaponsSecondary, OptionID: "fighter-ranged-a"},
				{ChoiceID: choices.FighterPack, OptionID: "fighter-pack-a"},
			},
		},
	}))
	s.Require().NoError(draft.SetBackground(&tkcharacter.SetBackgroundInput{
		BackgroundID: backgrounds.Soldier,
		Choices: tkcharacter.BackgroundChoices{
			Tools: []shared.SelectionID{"dice-set"},
			Equipment: []tkcharacter.EquipmentChoiceSelection{
				{ChoiceID: choices.SoldierGamingSetItem, OptionID: choices.SoldierGamingSetDice},
			},
		},
	}))
	s.Require().NoError(draft.SetAbilityScores(&tkcharacter.SetAbilityScoresInput{
		Scores: shared.AbilityScores{
			abilities.STR: 16, abilities.DEX: 14, abilities.CON: 15,
			abilities.INT: 10, abilities.WIS: 12, abilities.CHA: 8,
		},
		Method: "standard-array",
	}))

	char, err := draft.ToCharacter(s.ctx, id, events.NewEventBus())
	s.Require().NoError(err)

	data := char.ToData()
	data.Experience = experience
	return data
}

func (s *LevelUpTestSuite) expectGet(data *tkcharacter.Data) {
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: data.ID}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: data},
			Version:   "version-1",
		}, nil)
}

// TestGetNextLevel_FighterConfirmsWithoutAsking pins design R4.14: "A level
// that requires nothing MUST still be a correct screen -- a confirmation, not
// an empty form. A confirmation has content: what the level brings ... so a
// fighter reads 'Level 2: Action Surge' and not a bare button."
//
// The features and the hit die are what make it content. An empty requirement
// list alone would pass a test that asserted only "no choices".
func (s *LevelUpTestSuite) TestGetNextLevel_FighterConfirmsWithoutAsking() {
	data := s.fighterData("char-fighter", 300)
	s.expectGet(data)

	out, err := s.orchestrator.GetNextLevel(s.ctx, &GetNextLevelInput{CharacterID: data.ID})

	s.Require().NoError(err)
	s.Equal(2, out.CharacterLevel)
	s.Equal(2, out.ClassLevel)
	s.Equal(classes.Fighter, out.ClassID)
	s.Empty(out.Requirements.ChoiceIDs(), "fighter level 2 asks for nothing")
	s.Equal([]string{refs.Features.ActionSurge().String()}, out.FeatureRefs,
		"Action Surge is the whole of what fighter level 2 adds")
	s.Equal(10, out.HitDice)
}

// TestGetNextLevel_BardAsksForExactlyOneSpell pins R4.13 and the design's
// proof case: the bard "is offered its spell choice on a screen with no bard
// in it". The choice id is the toolkit's, not one this layer invents.
func (s *LevelUpTestSuite) TestGetNextLevel_BardAsksForExactlyOneSpell() {
	data := s.bardData("char-bard", 300)
	s.expectGet(data)

	out, err := s.orchestrator.GetNextLevel(s.ctx, &GetNextLevelInput{CharacterID: data.ID})

	s.Require().NoError(err)
	s.Equal(2, out.CharacterLevel)
	s.Equal(8, out.HitDice)
	s.Equal([]choices.ChoiceID{"bard-spells-2"}, out.Requirements.ChoiceIDs(),
		"the spell is the only thing level 2 asks a bard for")
	s.Require().NotNil(out.Requirements.Spellbook)
	s.Equal(1, out.Requirements.Spellbook.Count, "five known minus four known")
	s.NotEmpty(out.Requirements.Spellbook.Options, "a choice with no options cannot be made")
	s.Empty(out.FeatureRefs, "bard's level-2 features are not in the repository yet, and that is not a gate")
}

// TestGetNextLevel_AnswersEvenWithoutTheExperience pins the read/write split:
// R4.11 puts the refusal on Advance, so the screen can show a player what they
// are working toward before they have earned it.
func (s *LevelUpTestSuite) TestGetNextLevel_AnswersEvenWithoutTheExperience() {
	data := s.fighterData("char-fighter", 0)
	s.expectGet(data)

	out, err := s.orchestrator.GetNextLevel(s.ctx, &GetNextLevelInput{CharacterID: data.ID})

	s.Require().NoError(err)
	s.Equal(2, out.CharacterLevel, "the read describes the level the write would refuse")
}

// TestLevelUp_BardLearnsTheSpellItChose is done-when 5: "A bard at 300 XP is
// offered its spell choice on a screen with no bard in it, chooses, and comes
// out at level 2 with the spell ON ITS KNOWN LIST, three first-level slots
// reported in the response, and a two-entry record."
//
// The persisted sheet is asserted, not just the response: a response that
// reported a level nobody stored would pass every other check here.
func (s *LevelUpTestSuite) TestLevelUp_BardLearnsTheSpellItChose() {
	data := s.bardData("char-bard", 300)
	s.Require().Len(data.KnownSpells, 4)
	s.expectGet(data)

	var stored *tkcharacter.Data
	s.mockCharacterRepo.EXPECT().
		Update(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			stored = input.Character.Data
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		})

	out, err := s.orchestrator.LevelUp(s.ctx, &LevelUpInput{
		CharacterID:    data.ID,
		HitPointMethod: tkcharacter.HitPointMethodAverage,
		Choices: []choices.ChoiceData{{
			Category:       shared.ChoiceSpells,
			Source:         shared.SourceClass,
			ChoiceID:       "bard-spells-2",
			SpellSelection: []spells.Spell{spells.HealingWord},
		}},
	})

	s.Require().NoError(err)
	s.Equal(2, out.Gained.CharacterLevel)
	s.Equal(2, out.Entry.Level)

	s.Require().NotNil(stored, "the leveled sheet must reach the repository")
	s.Equal(2, stored.Level)
	s.Len(stored.Levels, 2, "an append-only record, two entries deep")
	s.Len(stored.KnownSpells, 5, "four known, one chosen")
	s.Contains(stored.KnownSpells, refs.Spells.HealingWord().String(),
		"the sheet stores canonical refs, and the chosen spell is on the list")

	// R4.7: "a slot increase is not a question and is applied without asking",
	// which is why the response has to say so. Both pools are named because a
	// level that moved only one of them is a different, wrong answer.
	changes := map[string][2]int{}
	for _, change := range out.Gained.Resources {
		changes[string(change.Key)] = [2]int{change.From, change.To}
	}
	s.Equal([2]int{2, 3}, changes[string(resources.SpellSlotLevel1)],
		"first-level slots go two to three at bard 2")
	s.Equal([2]int{1, 2}, changes[string(resources.HitDice)],
		"a level is a hit die")
}

// TestLevelUp_RolledHitPointsComeFromTheSuppliedRoller proves the Config
// roller is the one that is used. The bard's d8 with a face of 5 and a +2
// Constitution modifier is 7; its average gain would be 5+2 = 7 as well, so
// the assertion that discriminates is the stored METHOD alongside the number
// -- and a roller returning any other face would move the number off 7.
func (s *LevelUpTestSuite) TestLevelUp_RolledHitPointsComeFromTheSuppliedRoller() {
	data := s.bardData("char-bard", 300)
	before := data.MaxHitPoints
	s.expectGet(data)

	var stored *tkcharacter.Data
	s.mockCharacterRepo.EXPECT().
		Update(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			stored = input.Character.Data
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		})

	out, err := s.orchestrator.LevelUp(s.ctx, &LevelUpInput{
		CharacterID:    data.ID,
		HitPointMethod: tkcharacter.HitPointMethodRolled,
		Choices: []choices.ChoiceData{{
			Category:       shared.ChoiceSpells,
			Source:         shared.SourceClass,
			ChoiceID:       "bard-spells-2",
			SpellSelection: []spells.Spell{spells.HealingWord},
		}},
	})

	s.Require().NoError(err)
	s.Equal(7, out.Gained.HitPointGain, "a face of 5 on the d8, plus +2 from a human bard's CON 14")
	s.Require().NotNil(stored)
	s.Equal(before+7, stored.MaxHitPoints)
	s.Equal(tkcharacter.HitPointMethodRolled, stored.Levels[1].HitPointMethod,
		"the record stores the method, and the gain it produced")
}

// TestLevelUp_RefusesAnUnearnedLevel is done-when 4: "Advance refuses a level
// the character has not earned, with no bypass and no RPC that could grant
// one." The message is the toolkit's and names the threshold, because that
// sentence is what the player is shown.
func (s *LevelUpTestSuite) TestLevelUp_RefusesAnUnearnedLevel() {
	data := s.fighterData("char-fighter", 0)
	s.expectGet(data)

	out, err := s.orchestrator.LevelUp(s.ctx, &LevelUpInput{
		CharacterID:    data.ID,
		HitPointMethod: tkcharacter.HitPointMethodAverage,
	})

	s.Require().Error(err)
	s.Nil(out)
	s.True(apierr.IsFailedPrecondition(err), "a level the state forbids, not a malformed request")
	s.ErrorContains(err, "300", "the refusal names the threshold the level needs")
	s.ErrorContains(err, "0 experience")
}

// TestLevelUp_RefusesAChoiceTheLevelDidNotAsk pins R4.15 -- validation is the
// engine's -- and the append-only record's reason for it: "a choice nothing
// asked for could never be taken back".
func (s *LevelUpTestSuite) TestLevelUp_RefusesAChoiceTheLevelDidNotAsk() {
	data := s.bardData("char-bard", 300)
	s.expectGet(data)

	out, err := s.orchestrator.LevelUp(s.ctx, &LevelUpInput{
		CharacterID:    data.ID,
		HitPointMethod: tkcharacter.HitPointMethodAverage,
		Choices: []choices.ChoiceData{{
			Category: shared.ChoiceSpells,
			Source:   shared.SourceClass,
			ChoiceID: "bard-spells-2",
			// Two where the level asks for one.
			SpellSelection: []spells.Spell{spells.HealingWord, spells.CureWounds},
		}},
	})

	s.Require().Error(err)
	s.Nil(out)
	s.True(apierr.IsInvalidArgument(err), "a submission that answers the wrong question is the request's fault")
}

// TestLevelUp_NothingIsWrittenWhenTheLevelIsRefused is the atomicity the
// design claims (R4.2): "A failed Advance leaves the character exactly as it
// was, record included." The mock has no Update expectation, so a write here
// fails the test rather than passing quietly.
func (s *LevelUpTestSuite) TestLevelUp_NothingIsWrittenWhenTheLevelIsRefused() {
	data := s.fighterData("char-fighter", 0)
	s.expectGet(data)

	_, err := s.orchestrator.LevelUp(s.ctx, &LevelUpInput{
		CharacterID:    data.ID,
		HitPointMethod: tkcharacter.HitPointMethodAverage,
	})

	s.Require().Error(err)
	s.Equal(1, data.Level, "the repository's own entity is untouched")
	s.Len(data.Levels, 1)
}

// TestLevelUp_RefusesTheLevelOneOnlyHitPointMethod: HIT_POINT_METHOD_MAX is
// not on the wire (design §10), and the toolkit refuses it if it arrives
// another way.
func (s *LevelUpTestSuite) TestLevelUp_RefusesTheLevelOneOnlyHitPointMethod() {
	data := s.fighterData("char-fighter", 300)
	s.expectGet(data)

	_, err := s.orchestrator.LevelUp(s.ctx, &LevelUpInput{
		CharacterID:    data.ID,
		HitPointMethod: tkcharacter.HitPointMethodMax,
	})

	s.Require().Error(err)
	s.True(apierr.IsInvalidArgument(err))
	s.ErrorContains(err, "level 1 only")
}

// TestLevelUp_RequiresAHitPointMethod refuses before the repository is
// touched: an unset method is the proto's UNSPECIFIED, and there is no
// defensible default between rolling and averaging.
func (s *LevelUpTestSuite) TestLevelUp_RequiresAHitPointMethod() {
	_, err := s.orchestrator.LevelUp(s.ctx, &LevelUpInput{CharacterID: "char-fighter"})

	s.Require().Error(err)
	s.True(apierr.IsInvalidArgument(err))
}

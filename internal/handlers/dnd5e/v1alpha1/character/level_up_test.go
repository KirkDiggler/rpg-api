package character

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
)

const (
	levelUpCharacterID = "char-1"
	levelUpPlayerID    = "player-1"
)

// fakeSessions stands in for the SDK's Manager.
//
// A fake rather than a generated mock because the interface is two methods and
// every test here wants to script an OUTCOME, not assert a call shape: the
// handler's whole job is translating what comes back, so what comes back is
// the fixture. It also records what was sent, which is how the submission
// tests read the request without a second mechanism.
type fakeSessions struct {
	nextLevel func(*sdk.NextLevelInput) (*sdk.NextLevelOutput, error)
	levelUp   func(*sdk.LevelUpInput) (*sdk.LevelUpOutput, error)

	nextLevelCalls int
	levelUpCalls   int
	lastLevelUp    *sdk.LevelUpInput
}

func (f *fakeSessions) NextLevel(_ context.Context, in *sdk.NextLevelInput) (*sdk.NextLevelOutput, error) {
	f.nextLevelCalls++
	if f.nextLevel == nil {
		return nil, fmt.Errorf("NextLevel called with no scripted outcome, for %q", in.Character)
	}
	return f.nextLevel(in)
}

func (f *fakeSessions) LevelUp(_ context.Context, in *sdk.LevelUpInput) (*sdk.LevelUpOutput, error) {
	f.levelUpCalls++
	f.lastLevelUp = in
	if f.levelUp == nil {
		return nil, fmt.Errorf("LevelUp called with no scripted outcome, for %q", in.Character)
	}
	return f.levelUp(in)
}

type LevelUpHandlerTestSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockService *charactermock.MockService
	sessions    *fakeSessions
	handler     *Handler
	ctx         context.Context
}

func TestLevelUpHandlerSuite(t *testing.T) {
	suite.Run(t, new(LevelUpHandlerTestSuite))
}

func (s *LevelUpHandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = charactermock.NewMockService(s.ctrl)
	s.sessions = &fakeSessions{}
	s.ctx = auth.WithPlayerID(context.Background(), levelUpPlayerID)

	var err error
	s.handler, err = NewHandler(&HandlerConfig{
		CharacterService: s.mockService,
		Sessions:         s.sessions,
	})
	s.Require().NoError(err)
}

func (s *LevelUpHandlerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *LevelUpHandlerTestSuite) ownedCharacter(classID classes.Class, level int) *character.GetCharacterOutput {
	return &character.GetCharacterOutput{
		Character: &entities.Character{Data: &toolkitchar.Data{
			ID:         levelUpCharacterID,
			PlayerID:   levelUpPlayerID,
			ClassID:    classID,
			Level:      level,
			Experience: 300,
		}},
	}
}

// expectOwned answers the ownership gate once.
func (s *LevelUpHandlerTestSuite) expectOwned(classID classes.Class) {
	s.mockService.EXPECT().
		GetCharacter(gomock.Any(), &character.GetCharacterInput{CharacterID: levelUpCharacterID}).
		Return(s.ownedCharacter(classID, 1), nil)
}

// TestGetNextLevel_ProjectsWhatTheSDKHandsIt is the whole of this handler's
// job now: one SDK call, one projection, no rule.
//
// One spell ref in, one Choice out carrying that ref. Which options a
// character should be offered is the SDK's claim and is tested there; what is
// tested HERE is that the projection neither drops nor mangles what it was
// given, which is why the assertion names the ref rather than checking the
// list is non-empty.
func (s *LevelUpHandlerTestSuite) TestGetNextLevel_ProjectsWhatTheSDKHandsIt() {
	s.expectOwned(classes.Bard)
	s.sessions.nextLevel = func(in *sdk.NextLevelInput) (*sdk.NextLevelOutput, error) {
		s.Equal(levelUpCharacterID, in.Character)
		return &sdk.NextLevelOutput{
			// The sheet HOLDS 1; the level being described is 2. The two are
			// separate fields on the SDK for a reason, and a fixture that set
			// only one of them is how the projection read the wrong one in the
			// first place.
			Level:          1,
			ClassLevel:     2,
			CharacterLevel: 2,
			Class:          refs.Classes.Bard().String(),
			ClassName:      "Bard",
			HitDie:         8,
			Choices: []sdk.LevelChoice{{
				ID:         "bard-spells-2",
				Label:      "Choose 1 spell",
				Kind:       sdk.LevelChoiceSpell,
				Count:      1,
				SpellLevel: 1,
				Options:    []string{refs.Spells.HealingWord().String()},
			}},
		}, nil
	}

	got, err := s.handler.GetNextLevel(s.ctx, &dnd5ev1alpha1.GetNextLevelRequest{
		CharacterId: levelUpCharacterID,
	})

	s.Require().NoError(err)
	s.Equal(int32(2), got.GetLevel(),
		"the level being TAKEN, not the one the sheet holds")
	s.Equal(dnd5ev1alpha1.Class_CLASS_BARD, got.GetClass(), "the class ref becomes the wire's enum")
	s.Equal(int32(8), got.GetHitDie())

	s.Require().Len(got.GetChoices(), 1)
	choice := got.GetChoices()[0]
	s.Equal("bard-spells-2", choice.GetId())
	s.Equal("Choose 1 spell", choice.GetDescription())
	s.Equal(int32(1), choice.GetChooseCount())
	s.Equal(dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS, choice.GetChoiceType())
	s.Equal(int32(1), choice.GetSpellOptions().GetSpellLevel())
	s.Equal([]string{refs.Spells.HealingWord().String()}, choice.GetSpellOptions().GetAvailableRefs(),
		"one ref in, the same one ref out")
}

// TestGetNextLevel_AConfirmationNamesWhatItBrings is R4.14 on the wire: a
// level that asks nothing is still a screen with content. The feature and the
// hit die are what make it "Level 2: Action Surge" rather than a bare button.
func (s *LevelUpHandlerTestSuite) TestGetNextLevel_AConfirmationNamesWhatItBrings() {
	s.expectOwned(classes.Fighter)
	s.sessions.nextLevel = func(*sdk.NextLevelInput) (*sdk.NextLevelOutput, error) {
		return &sdk.NextLevelOutput{
			Level:          1,
			ClassLevel:     2,
			CharacterLevel: 2,
			Class:          refs.Classes.Fighter().String(),
			ClassName:      "Fighter",
			HitDie:         10,
			Features:       []string{refs.Features.ActionSurge().String()},
		}, nil
	}

	got, err := s.handler.GetNextLevel(s.ctx, &dnd5ev1alpha1.GetNextLevelRequest{
		CharacterId: levelUpCharacterID,
	})

	s.Require().NoError(err)
	s.Empty(got.GetChoices())
	s.Equal(int32(10), got.GetHitDie())
	s.Require().Len(got.GetFeatures(), 1)
	s.Equal(refs.Features.ActionSurge().String(), got.GetFeatures()[0].GetId())
	s.Equal("Action Surge", got.GetFeatures()[0].GetName())
	s.Equal(int32(2), got.GetFeatures()[0].GetLevel())
	s.Equal("Fighter", got.GetFeatures()[0].GetClassName())
}

// TestGetNextLevel_AForeignCharacterIsNotFoundAndNeverReachesTheSDK pins the
// gate. Ownership of the calling player is transport's, not the engine's: the SDK is
// handed a character id and has no notion of who holds the connection.
func (s *LevelUpHandlerTestSuite) TestGetNextLevel_AForeignCharacterIsNotFoundAndNeverReachesTheSDK() {
	s.mockService.EXPECT().
		GetCharacter(gomock.Any(), &character.GetCharacterInput{CharacterID: levelUpCharacterID}).
		Return(&character.GetCharacterOutput{
			Character: &entities.Character{Data: &toolkitchar.Data{
				ID: levelUpCharacterID, PlayerID: "somebody-else",
			}},
		}, nil)

	_, err := s.handler.GetNextLevel(s.ctx, &dnd5ev1alpha1.GetNextLevelRequest{
		CharacterId: levelUpCharacterID,
	})

	s.Equal(codes.NotFound, status.Code(err))
	s.Zero(s.sessions.nextLevelCalls, "a character the caller does not own never reaches the engine")
}

// TestLevelUp_AForeignCharacterIsNotFoundAndNeverReachesTheSDK: the write
// refuses the same way the read does, and for the same reason.
func (s *LevelUpHandlerTestSuite) TestLevelUp_AForeignCharacterIsNotFoundAndNeverReachesTheSDK() {
	s.mockService.EXPECT().
		GetCharacter(gomock.Any(), &character.GetCharacterInput{CharacterID: levelUpCharacterID}).
		Return(&character.GetCharacterOutput{
			Character: &entities.Character{Data: &toolkitchar.Data{
				ID: levelUpCharacterID, PlayerID: "somebody-else",
			}},
		}, nil)

	_, err := s.handler.LevelUp(s.ctx, &dnd5ev1alpha1.LevelUpRequest{
		CharacterId:    levelUpCharacterID,
		HitPointMethod: dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_AVERAGE,
	})

	s.Equal(codes.NotFound, status.Code(err))
	s.Zero(s.sessions.levelUpCalls, "nothing is leveled for a caller who does not own the sheet")
}

// TestLevelUp_UnspecifiedMethodNeverReachesTheSDK. Rolled and averaged write
// different numbers into a record that is never corrected, so there is no
// defensible default between them and the refusal comes before anything runs.
func (s *LevelUpHandlerTestSuite) TestLevelUp_UnspecifiedMethodNeverReachesTheSDK() {
	_, err := s.handler.LevelUp(s.ctx, &dnd5ev1alpha1.LevelUpRequest{
		CharacterId: levelUpCharacterID,
	})

	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(status.Convert(err).Message(), "hit_point_method")
	s.Zero(s.sessions.levelUpCalls)
}

// TestLevelUp_SendsTheChoiceIdAndRefsAndNoCategory pins the submission shape.
//
// The SDK derives the kind of each answer from the question it asked under
// that id, so a category on the way back would be a second name for one fact,
// free to disagree with the first. What crosses is the id and the refs.
func (s *LevelUpHandlerTestSuite) TestLevelUp_SendsTheChoiceIdAndRefsAndNoCategory() {
	s.mockService.EXPECT().
		GetCharacter(gomock.Any(), &character.GetCharacterInput{CharacterID: levelUpCharacterID}).
		Return(s.ownedCharacter(classes.Bard, 1), nil)
	s.mockService.EXPECT().
		GetCharacter(gomock.Any(), &character.GetCharacterInput{CharacterID: levelUpCharacterID}).
		Return(s.ownedCharacter(classes.Bard, 2), nil)
	s.sessions.levelUp = func(*sdk.LevelUpInput) (*sdk.LevelUpOutput, error) {
		return &sdk.LevelUpOutput{Gained: sdk.LevelGained{CharacterLevel: 2}}, nil
	}

	_, err := s.handler.LevelUp(s.ctx, &dnd5ev1alpha1.LevelUpRequest{
		CharacterId:    levelUpCharacterID,
		HitPointMethod: dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_AVERAGE,
		Choices: []*dnd5ev1alpha1.ChoiceData{{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS,
			ChoiceId: "bard-spells-2",
			Selection: &dnd5ev1alpha1.ChoiceData_Spells{Spells: &dnd5ev1alpha1.SpellSelection{
				SpellRefs: []string{refs.Spells.HealingWord().String()},
			}},
		}},
	})

	s.Require().NoError(err)
	s.Require().NotNil(s.sessions.lastLevelUp)
	s.Equal(levelUpCharacterID, s.sessions.lastLevelUp.Character)
	s.Equal(sdk.HitPointsAverage, s.sessions.lastLevelUp.HitPointMethod)
	s.Require().Len(s.sessions.lastLevelUp.Choices, 1)
	s.Equal("bard-spells-2", s.sessions.lastLevelUp.Choices[0].ChoiceID)
	s.Equal([]string{refs.Spells.HealingWord().String()}, s.sessions.lastLevelUp.Choices[0].Selections)
}

// TestLevelUp_ReportsWhatTheLevelBroughtAndRereadsTheSheet pins both halves of
// the response.
//
// The SDK returns no character by its own boundary law -- it reports that it
// saved -- so the projected sheet comes from a READ, which is also what makes
// the response show what is actually stored rather than a value handed back.
//
// R4.7 is why the resource changes are on the wire at all: "a slot increase is
// not a question and is applied without asking", so nothing on the screen
// would mention it unless the engine said so.
func (s *LevelUpHandlerTestSuite) TestLevelUp_ReportsWhatTheLevelBroughtAndRereadsTheSheet() {
	s.mockService.EXPECT().
		GetCharacter(gomock.Any(), &character.GetCharacterInput{CharacterID: levelUpCharacterID}).
		Return(s.ownedCharacter(classes.Bard, 1), nil)
	s.mockService.EXPECT().
		GetCharacter(gomock.Any(), &character.GetCharacterInput{CharacterID: levelUpCharacterID}).
		Return(s.ownedCharacter(classes.Bard, 2), nil)
	s.sessions.levelUp = func(*sdk.LevelUpInput) (*sdk.LevelUpOutput, error) {
		return &sdk.LevelUpOutput{Gained: sdk.LevelGained{
			CharacterLevel: 2,
			ClassLevel:     2,
			HitPointGain:   6,
			Resources: []sdk.ResourceMaximumChange{
				{Key: "spell_slot_level_1", Name: "1st-level Spell Slots", From: 2, To: 3},
				{Key: "hit_dice", Name: "Hit Dice", From: 1, To: 2},
			},
		}}, nil
	}

	got, err := s.handler.LevelUp(s.ctx, &dnd5ev1alpha1.LevelUpRequest{
		CharacterId:    levelUpCharacterID,
		HitPointMethod: dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_AVERAGE,
	})

	s.Require().NoError(err)
	s.Equal(int32(2), got.GetCharacter().GetLevel(), "the re-read sheet, not the request")
	s.Equal(int32(300), got.GetCharacter().GetExperiencePoints())
	s.Equal(int32(2), got.GetGained().GetLevel())
	s.Equal(int32(6), got.GetGained().GetHitPointsGained())

	s.Require().Len(got.GetGained().GetResourceChanges(), 2)
	first := got.GetGained().GetResourceChanges()[0]
	s.Equal("spell_slot_level_1", first.GetKey())
	s.Equal("1st-level Spell Slots", first.GetName(), "the SDK names the pool; nothing here looks it up")
	s.Equal(int32(2), first.GetPreviousMaximum())
	s.Equal(int32(3), first.GetNewMaximum())
	second := got.GetGained().GetResourceChanges()[1]
	s.Equal("hit_dice", second.GetKey())
	s.Equal(int32(1), second.GetPreviousMaximum())
	s.Equal(int32(2), second.GetNewMaximum())
}

// TestLevelUp_CarriesTheSDKRefusalWithItsOwnCodeAndSentence pins what a refused
// level tells the player.
//
// The engine names what it refused and why; the client shows that sentence.
// The code comes from the one shared translation table, not from a second one
// in this package. And the sheet is never re-read after a refusal: a client
// that got a character back alongside an error would have no way to know
// whether the level happened.
func (s *LevelUpHandlerTestSuite) TestLevelUp_CarriesTheSDKRefusalWithItsOwnCodeAndSentence() {
	s.expectOwned(classes.Fighter)
	s.sessions.levelUp = func(*sdk.LevelUpInput) (*sdk.LevelUpOutput, error) {
		// ErrCannotAdvance is the SDK's unearned-level refusal. What this test
		// pins is the CODE and the SENTENCE, not the sentinel: the engine
		// names the total and the threshold, and that is what the player
		// reads.
		return nil, fmt.Errorf(
			"character %q has 0 experience and level 2 needs 300: %w",
			levelUpCharacterID, sdk.ErrCannotAdvance)
	}

	_, err := s.handler.LevelUp(s.ctx, &dnd5ev1alpha1.LevelUpRequest{
		CharacterId:    levelUpCharacterID,
		HitPointMethod: dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_AVERAGE,
	})

	s.Equal(codes.FailedPrecondition, status.Code(err))
	s.Contains(status.Convert(err).Message(), "300", "the engine's own sentence reaches the client")
	s.Contains(status.Convert(err).Message(), "0 experience")
}

// TestLevelUpSubmissions_RefuseACategoryThisPathCannotAnswer. A dropped answer
// reaches the engine as a missing one, and the player is told they failed to
// choose something they did choose -- so an unreadable choice is an error that
// names it.
func (s *LevelUpHandlerTestSuite) TestLevelUpSubmissions_RefuseACategoryThisPathCannotAnswer() {
	_, err := levelChoiceSubmissions([]*dnd5ev1alpha1.ChoiceData{{
		Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
		ChoiceId: "rogue-skills-2",
	}})

	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(status.Convert(err).Message(), "rogue-skills-2")
}

// TestLevelUpSubmissions_AcceptBothSpellShapes: cantrips and leveled spells
// are separate categories on the wire and both are level-up answers.
func (s *LevelUpHandlerTestSuite) TestLevelUpSubmissions_AcceptBothSpellShapes() {
	got, err := levelChoiceSubmissions([]*dnd5ev1alpha1.ChoiceData{
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS,
			ChoiceId: "bard-cantrips-4",
			Selection: &dnd5ev1alpha1.ChoiceData_Spells{Spells: &dnd5ev1alpha1.SpellSelection{
				SpellRefs: []string{refs.Spells.ViciousMockery().String()},
			}},
		},
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS,
			ChoiceId: "bard-spells-2",
			Selection: &dnd5ev1alpha1.ChoiceData_Spells{Spells: &dnd5ev1alpha1.SpellSelection{
				SpellRefs: []string{refs.Spells.HealingWord().String()},
			}},
		},
	})

	s.Require().NoError(err)
	s.Require().Len(got, 2)
	s.Equal("bard-cantrips-4", got[0].ChoiceID)
	s.Equal([]string{refs.Spells.ViciousMockery().String()}, got[0].Selections)
	s.Equal("bard-spells-2", got[1].ChoiceID)
	s.Equal([]string{refs.Spells.HealingWord().String()}, got[1].Selections)
}

// TestNewHandler_RefusesWithoutTheSDK. A handler that serves every other RPC
// and panics on the two advancement ones is a worse failure than one that
// refuses to build.
func (s *LevelUpHandlerTestSuite) TestNewHandler_RefusesWithoutTheSDK() {
	_, err := NewHandler(&HandlerConfig{CharacterService: s.mockService})

	s.Require().Error(err)
	s.Contains(err.Error(), "session SDK is required")
}

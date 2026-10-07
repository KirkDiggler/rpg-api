package character

import (
	"encoding/json"
	"fmt"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resolution"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// The armour class every read response carries is the resolution door's fold
// of the stored sheet, asked at that read (rpg-project#538 slice 5). These
// tests ride EquipItemTestSuite's fixtures: a monk whose fold is 15 only when
// WIS is read through the cast, and a fighter.

// unarmouredBarbarian carries Unarmored Defense (barbarian) with DEX 14 and
// CON 16, so its folded AC is 15 = 10 + DEX 2 + CON 3.
func (s *EquipItemTestSuite) unarmouredBarbarian(id string) *entities.Character {
	entity := s.fighterWithLongswordAndShield()
	entity.Data.ID = id
	entity.Data.Name = "Test Barbarian"
	entity.Data.ClassID = "barbarian"
	entity.Data.AbilityScores = shared.AbilityScores{
		abilities.STR: 16,
		abilities.DEX: 14,
		abilities.CON: 16,
		abilities.INT: 8,
		abilities.WIS: 10,
		abilities.CHA: 10,
	}
	unarmoredDefense, err := (&conditions.UnarmoredDefenseCondition{
		MemberID: id,
		Type:     conditions.UnarmoredDefenseBarbarian,
	}).ToJSON()
	s.Require().NoError(err)
	entity.Data.Conditions = []json.RawMessage{unarmoredDefense}
	return entity
}

// unprojectable is a sheet the door refuses: an Inspired die with no granting
// bard parses but will not attach (resolution.ProjectCharacter is strict).
func (s *EquipItemTestSuite) unprojectable(id string) *entities.Character {
	entity := s.fighterWithLongswordAndShield()
	entity.Data.ID = id
	inspired, err := (&conditions.InspiredCondition{MemberID: id}).ToJSON()
	s.Require().NoError(err)
	entity.Data.Conditions = []json.RawMessage{inspired}
	return entity
}

func (s *EquipItemTestSuite) TestGetCharacter_AMonkReturnsTheWisdomInclusiveAC() {
	entity := s.unarmouredMonk()
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: entity, Version: testCharacterRepositoryVersion}, nil)

	out, err := s.orchestrator.GetCharacter(s.ctx, &GetCharacterInput{CharacterID: s.testCharacterID})
	s.Require().NoError(err)
	s.Require().NotNil(out.ArmorClass)
	s.Equal(15, out.ArmorClass.Total, "10 base + 3 DEX + 2 WIS")
}

func (s *EquipItemTestSuite) TestGetCharacter_ASheetTheDoorRefusesFailsTheRead() {
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: "char-broken"}).
		Return(&characterrepo.GetOutput{Character: s.unprojectable("char-broken"), Version: testCharacterRepositoryVersion}, nil)

	out, err := s.orchestrator.GetCharacter(s.ctx, &GetCharacterInput{CharacterID: "char-broken"})
	s.Require().Error(err)
	s.Nil(out)
	s.True(apierr.IsInternal(err), "%v", err)
	s.Contains(err.Error(), refs.Conditions.Inspired().String())
}

func (s *EquipItemTestSuite) TestListCharacters_ReturnsEachCharactersFoldedAC() {
	monk := s.unarmouredMonk()
	barbarian := s.unarmouredBarbarian("char-barbarian")
	fighter := s.fighterWithLongswordAndShield()
	fighter.Data.ID = "char-fighter-shield"
	fighter.Data.EquipmentSlots = character.EquipmentSlots{character.SlotOffHand: "shield"}
	s.mockCharacterRepo.EXPECT().
		ListByPlayerID(s.ctx, characterrepo.ListByPlayerIDInput{PlayerID: "player-1"}).
		Return(&characterrepo.ListByPlayerIDOutput{Characters: []*entities.Character{monk, barbarian, fighter}}, nil)

	out, err := s.orchestrator.ListCharacters(s.ctx, &ListCharactersInput{PlayerID: "player-1"})
	s.Require().NoError(err)
	s.Require().Contains(out.ArmorClasses, s.testCharacterID)
	s.Equal(15, out.ArmorClasses[s.testCharacterID].Total, "monk: 10 + DEX 3 + WIS 2")
	s.Require().Contains(out.ArmorClasses, "char-barbarian")
	s.Equal(15, out.ArmorClasses["char-barbarian"].Total, "barbarian: 10 + DEX 2 + CON 3")
	s.Require().Contains(out.ArmorClasses, "char-fighter-shield")
	s.Equal(13, out.ArmorClasses["char-fighter-shield"].Total, "fighter: 10 + DEX 1 + shield 2")
}

// TestListCharacters_OneUnprojectableSheetFailsTheWholeList pins R11: the
// list does not drop the row, default it, or return the others.
func (s *EquipItemTestSuite) TestListCharacters_OneUnprojectableSheetFailsTheWholeList() {
	s.mockCharacterRepo.EXPECT().
		ListBySessionID(s.ctx, characterrepo.ListBySessionIDInput{SessionID: "session-1"}).
		Return(&characterrepo.ListBySessionIDOutput{Characters: []*entities.Character{
			s.unarmouredMonk(),
			s.unprojectable("char-broken"),
		}}, nil)

	out, err := s.orchestrator.ListCharacters(s.ctx, &ListCharactersInput{SessionID: "session-1"})
	s.Require().Error(err)
	s.Nil(out)
	var coded *apierr.Error
	s.Require().ErrorAs(err, &coded)
	s.Equal(apierr.CodeInternal, coded.Code)
	s.Contains(coded.Message, `"char-broken"`, "the refusal names the character")
}

// TestProjectArmorClass_IsTheDoorsFold: the helper every read response goes
// through answers exactly what the resolution door answers, for both
// Unarmored Defense shapes.
func (s *EquipItemTestSuite) TestProjectArmorClass_IsTheDoorsFold() {
	for _, entity := range []*entities.Character{s.unarmouredMonk(), s.unarmouredBarbarian("char-barbarian")} {
		got, err := projectArmorClass(s.ctx, &projectArmorClassInput{Data: entity.Data})
		s.Require().NoError(err)
		want, err := resolution.ProjectCharacter(s.ctx, &resolution.ProjectCharacterInput{Character: entity.Data})
		s.Require().NoError(err)
		s.Equal(want.ArmorClass.Total, got.ArmorClass.Total, fmt.Sprintf("%s: one door, one answer", entity.Data.ClassID))
	}
}

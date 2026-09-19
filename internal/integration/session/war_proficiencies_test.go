package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/events"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

type WarProficienciesSuite struct{ suite.Suite }

func TestWarProficienciesSuite(t *testing.T) { suite.Run(t, new(WarProficienciesSuite)) }
func (s *WarProficienciesSuite) TestNativeCreationPersistsAndProjectsDomainProficiencies() {
	h := newAcceptanceHarness(s.T())
	id := createNativeCleric(s.T(), h, spells.DivineFavor)
	ctx := auth.WithPlayerID(context.Background(), "cleric-player")
	stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
	s.Require().NoError(err)
	s.ElementsMatch([]proficiencies.Weapon{proficiencies.WeaponSimple, proficiencies.WeaponMartial}, stored.Character.Data.WeaponProficiencies)
	s.ElementsMatch([]proficiencies.Armor{proficiencies.ArmorLight, proficiencies.ArmorMedium, proficiencies.ArmorHeavy, proficiencies.ArmorShields}, stored.Character.Data.ArmorProficiencies)
	loaded, err := tkcharacter.LoadFromData(ctx, stored.Character.Data, events.NewEventBus())
	s.Require().NoError(err)
	sword, err := weapons.GetByID(weapons.Longsword)
	s.Require().NoError(err)
	s.True(loaded.IsProficientWith(&sword))
	bow, err := weapons.GetByID(weapons.Longbow)
	s.Require().NoError(err)
	s.True(loaded.IsProficientWith(&bow))
	// A fresh handler reads the repository and maps the actual character grants,
	// not the class catalog's advertised equipment choices.
	handler := newCharacterCreationHandler(s.T(), h)
	view, err := handler.GetCharacter(ctx, &pb.GetCharacterRequest{CharacterId: id})
	s.Require().NoError(err)
	s.Contains(view.GetCharacter().GetProficiencies().GetWeaponCategories(), pb.WeaponProficiencyCategory_WEAPON_PROFICIENCY_CATEGORY_MARTIAL)
	s.Contains(view.GetCharacter().GetProficiencies().GetArmorCategories(), pb.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_HEAVY)
	lifeHarness := newAcceptanceHarness(s.T())
	lifeID := createNativeCleric(s.T(), lifeHarness)
	handler = newCharacterCreationHandler(s.T(), lifeHarness)
	life, err := handler.GetCharacter(ctx, &pb.GetCharacterRequest{CharacterId: lifeID})
	s.Require().NoError(err)
	s.NotContains(life.GetCharacter().GetProficiencies().GetWeaponCategories(), pb.WeaponProficiencyCategory_WEAPON_PROFICIENCY_CATEGORY_MARTIAL)
}

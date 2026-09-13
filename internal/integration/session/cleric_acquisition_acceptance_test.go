package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
)

func TestAcceptance_ClericAcquisitionPersistsOpenSpellRefs(t *testing.T) {
	h := newAcceptanceHarness(t)
	handler := newCharacterCreationHandler(t, h)
	ctx := auth.WithPlayerID(context.Background(), "cleric-player")
	created, err := handler.CreateDraft(ctx, &pb.CreateDraftRequest{})
	require.NoError(t, err)
	id := created.GetDraft().GetId()
	_, err = handler.UpdateName(ctx, &pb.UpdateNameRequest{DraftId: id, Name: "Mercy"})
	require.NoError(t, err)
	_, err = handler.UpdateRace(ctx, &pb.UpdateRaceRequest{DraftId: id, Race: pb.Race_RACE_HUMAN,
		RaceChoices: []*pb.ChoiceData{{Category: pb.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES, Source: pb.ChoiceSource_CHOICE_SOURCE_RACE,
			Selection: &pb.ChoiceData_Languages{Languages: &pb.LanguageSelection{Languages: []pb.Language{pb.Language_LANGUAGE_DWARVISH}}}}}})
	require.NoError(t, err)
	spellRefs := []string{"dnd5e:spells:bane", "dnd5e:spells:bless", "dnd5e:spells:command", "dnd5e:spells:cure-wounds", "dnd5e:spells:healing-word"}
	classChoices := make([]*pb.ChoiceData, 0, 8)
	classChoices = append(classChoices, []*pb.ChoiceData{
		{Category: pb.ChoiceCategory_CHOICE_CATEGORY_SKILLS, Source: pb.ChoiceSource_CHOICE_SOURCE_CLASS, ChoiceId: "cleric-skills",
			Selection: &pb.ChoiceData_Skills{Skills: &pb.SkillSelection{Skills: []pb.Skill{pb.Skill_SKILL_MEDICINE, pb.Skill_SKILL_RELIGION}}}},
		{Category: pb.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS, Source: pb.ChoiceSource_CHOICE_SOURCE_CLASS, ChoiceId: string(choices.ClericCantrips1),
			Selection: &pb.ChoiceData_Spells{Spells: &pb.SpellSelection{SpellRefs: []string{"dnd5e:spells:sacred-flame", "dnd5e:spells:guidance", "dnd5e:spells:light"}}}},
		{Category: pb.ChoiceCategory_CHOICE_CATEGORY_SPELLS, Source: pb.ChoiceSource_CHOICE_SOURCE_CLASS, ChoiceId: string(choices.ClericSpells1),
			Selection: &pb.ChoiceData_Spells{Spells: &pb.SpellSelection{SpellRefs: spellRefs}}},
	}...)
	for _, equipment := range []struct{ choice, option string }{
		{string(choices.ClericWeapons), choices.ClericWeaponMace},
		{string(choices.ClericArmor), choices.ClericArmorChainMail},
		{string(choices.ClericSecondaryWeapon), choices.ClericSecondaryShortbow},
		{string(choices.ClericPack), choices.ClericPackExplorer},
		{string(choices.ClericHolySymbol), choices.ClericHolyAmulet},
	} {
		classChoices = append(classChoices, &pb.ChoiceData{Category: pb.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
			Source: pb.ChoiceSource_CHOICE_SOURCE_CLASS, ChoiceId: equipment.choice, OptionId: equipment.option,
			Selection: &pb.ChoiceData_Equipment{Equipment: &pb.EquipmentSelection{}}})
	}
	_, err = handler.UpdateClass(ctx, &pb.UpdateClassRequest{DraftId: id, Class: pb.Class_CLASS_CLERIC,
		Subclass: pb.Subclass_SUBCLASS_LIFE_DOMAIN, ClassChoices: classChoices})
	require.NoError(t, err)
	_, err = handler.UpdateBackground(ctx, &pb.UpdateBackgroundRequest{DraftId: id, Background: pb.Background_BACKGROUND_HERMIT})
	require.NoError(t, err)
	_, err = handler.UpdateAbilityScores(ctx, &pb.UpdateAbilityScoresRequest{DraftId: id,
		ScoresInput: &pb.UpdateAbilityScoresRequest_AbilityScores{AbilityScores: &pb.AbilityScores{
			Strength: 14, Dexterity: 10, Constitution: 13, Intelligence: 8, Wisdom: 15, Charisma: 12}}})
	require.NoError(t, err)
	draft, err := handler.GetDraft(ctx, &pb.GetDraftRequest{DraftId: id})
	require.NoError(t, err)
	require.Equal(t, pb.Subclass_SUBCLASS_LIFE_DOMAIN, draft.GetDraft().GetSubclass())
	finalized, err := handler.FinalizeDraft(ctx, &pb.FinalizeDraftRequest{DraftId: id})
	require.NoError(t, err)
	require.ElementsMatch(t, spellRefs, finalized.GetCharacter().GetKnownSpells())
	stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: finalized.GetCharacter().GetId()})
	require.NoError(t, err)
	require.ElementsMatch(t, spellRefs, stored.Character.Data.KnownSpells)
	require.Equal(t, 2, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
}

package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"

	pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/character"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

func TestAcceptance_ClericAcquisitionPersistsOpenSpellRefs(t *testing.T) {
	h := newAcceptanceHarness(t)
	createNativeCleric(t, h)
}

func createNativeCleric(t *testing.T, h *acceptanceHarness) string {
	t.Helper()
	handler := newCharacterCreationHandler(t, h)
	ctx := auth.WithPlayerID(context.Background(), "cleric-player")
	catalog, err := handler.ListClasses(ctx, &pb.ListClassesRequest{})
	require.NoError(t, err)
	details, err := handler.GetClassDetails(ctx, &pb.GetClassDetailsRequest{ClassId: "cleric"})
	require.NoError(t, err)
	var listed *pb.ClassInfo
	for _, class := range catalog.GetClasses() {
		if class.GetClassId() == pb.Class_CLASS_CLERIC {
			listed = class
		}
	}
	require.NotNil(t, listed)
	require.True(t, proto.Equal(listed, details.GetClass()))
	var selectedDomain pb.Subclass
	for _, domain := range listed.GetSubclasses() {
		if domain.GetSubclassId() == pb.Subclass_SUBCLASS_LIFE_DOMAIN {
			selectedDomain = domain.GetSubclassId()
		}
	}
	require.NotEqual(t, pb.Subclass_SUBCLASS_UNSPECIFIED, selectedDomain, "select a returned domain, never a hidden seeded option")
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
		Subclass: selectedDomain, ClassChoices: classChoices})
	require.NoError(t, err)
	_, err = handler.UpdateBackground(ctx, &pb.UpdateBackgroundRequest{DraftId: id, Background: pb.Background_BACKGROUND_HERMIT})
	require.NoError(t, err)
	_, err = handler.UpdateAbilityScores(ctx, &pb.UpdateAbilityScoresRequest{DraftId: id,
		ScoresInput: &pb.UpdateAbilityScoresRequest_AbilityScores{AbilityScores: &pb.AbilityScores{
			Strength: 14, Dexterity: 10, Constitution: 13, Intelligence: 8, Wisdom: 15, Charisma: 12}}})
	require.NoError(t, err)
	// A fresh service reads the saved draft and accepts its returned choices.
	handler = newCharacterCreationHandler(t, h)
	draft, err := handler.GetDraft(ctx, &pb.GetDraftRequest{DraftId: id})
	require.NoError(t, err)
	require.Equal(t, pb.Subclass_SUBCLASS_LIFE_DOMAIN, draft.GetDraft().GetSubclass())
	require.NotNil(t, draft.GetDraft().GetClassInfo().GetSpellcasting())
	var resumedChoices []*pb.ChoiceData
	for _, choice := range draft.GetDraft().GetChoices() {
		if choice.GetSource() != pb.ChoiceSource_CHOICE_SOURCE_CLASS {
			continue
		}
		if choice.GetChoiceId() == string(choices.ClericCantrips1) {
			require.Equal(t, pb.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS, choice.GetCategory())
		}
		if choice.GetChoiceId() == string(choices.ClericSpells1) {
			require.Equal(t, pb.ChoiceCategory_CHOICE_CATEGORY_SPELLS, choice.GetCategory())
			require.ElementsMatch(t, spellRefs, choice.GetSpells().GetSpellRefs())
		}
		resumedChoices = append(resumedChoices, choice)
	}
	_, err = handler.UpdateClass(ctx, &pb.UpdateClassRequest{DraftId: id, Class: draft.GetDraft().GetClass(),
		Subclass: draft.GetDraft().GetSubclass(), ClassChoices: resumedChoices})
	require.NoError(t, err)
	finalized, err := handler.FinalizeDraft(ctx, &pb.FinalizeDraftRequest{DraftId: id})
	require.NoError(t, err)
	require.ElementsMatch(t, spellRefs, finalized.GetCharacter().GetKnownSpells())
	stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: finalized.GetCharacter().GetId()})
	require.NoError(t, err)
	require.ElementsMatch(t, spellRefs, stored.Character.Data.KnownSpells)
	require.Equal(t, 2, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
	return finalized.GetCharacter().GetId()
}

func TestAcceptance_NativeClericOwnerPrivateData(t *testing.T) {
	h := newAcceptanceHarness(t)
	id := createNativeCleric(t, h)
	handler, err := characterhandler.New(&characterhandler.HandlerConfig{CharacterService: newAcceptanceCharacterService(t, h)})
	require.NoError(t, err)
	view, err := handler.GetCharacterData(auth.WithPlayerID(context.Background(), "cleric-player"), &characterpb.GetCharacterDataRequest{CharacterId: id})
	require.NoError(t, err, "a natively finalized Cleric must have an owner-private view")
	require.NotEmpty(t, view.GetCharacter().GetResources())
	require.NotEmpty(t, view.GetCharacter().GetInventory())
}

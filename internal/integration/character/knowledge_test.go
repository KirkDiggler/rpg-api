package characterintegration

import (
	v1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
)

func (s *CharacterCreationSuite) TestKnowledgeCreationRoundTrip() {
	ctx := s.authCtx("knowledge-owner")
	created, err := s.server.CharacterClient.CreateDraft(ctx, &v1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	id := created.Draft.Id
	_, err = s.server.CharacterClient.UpdateName(ctx, &v1alpha1.UpdateNameRequest{DraftId: id, Name: "Knowledge Check"})
	s.Require().NoError(err)
	_, err = s.server.CharacterClient.UpdateRace(ctx, &v1alpha1.UpdateRaceRequest{DraftId: id, Race: v1alpha1.Race_RACE_HUMAN, RaceChoices: []*v1alpha1.ChoiceData{
		{Category: v1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES, Source: v1alpha1.ChoiceSource_CHOICE_SOURCE_RACE, Selection: &v1alpha1.ChoiceData_Languages{Languages: &v1alpha1.LanguageSelection{Languages: []v1alpha1.Language{v1alpha1.Language_LANGUAGE_DWARVISH}}}},
	}})
	s.Require().NoError(err)
	skillChoice := func(id string, values ...v1alpha1.Skill) *v1alpha1.ChoiceData {
		return &v1alpha1.ChoiceData{ChoiceId: id, Category: v1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS, Source: v1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, Selection: &v1alpha1.ChoiceData_Skills{Skills: &v1alpha1.SkillSelection{Skills: values}}}
	}
	answers := make([]*v1alpha1.ChoiceData, 0, 10)
	answers = append(answers, []*v1alpha1.ChoiceData{
		skillChoice("cleric-skills", v1alpha1.Skill_SKILL_MEDICINE, v1alpha1.Skill_SKILL_RELIGION),
		skillChoice("cleric-knowledge-skills", v1alpha1.Skill_SKILL_ARCANA, v1alpha1.Skill_SKILL_HISTORY),
		{ChoiceId: "cleric-knowledge-languages", Category: v1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES, Source: v1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, Selection: &v1alpha1.ChoiceData_Languages{Languages: &v1alpha1.LanguageSelection{Languages: []v1alpha1.Language{v1alpha1.Language_LANGUAGE_ELVISH, v1alpha1.Language_LANGUAGE_GNOMISH}}}},
		{ChoiceId: "cleric-cantrips-1", Category: v1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS, Source: v1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, Selection: &v1alpha1.ChoiceData_Spells{Spells: &v1alpha1.SpellSelection{SpellRefs: []string{"dnd5e:spells:sacred-flame", "dnd5e:spells:guidance", "dnd5e:spells:light"}}}},
		{ChoiceId: "cleric-spells-1", Category: v1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS, Source: v1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, Selection: &v1alpha1.ChoiceData_Spells{Spells: &v1alpha1.SpellSelection{SpellRefs: []string{"dnd5e:spells:bane", "dnd5e:spells:healing-word", "dnd5e:spells:sanctuary", "dnd5e:spells:guiding-bolt"}}}},
	}...)
	for _, equipment := range []struct {
		id     choices.ChoiceID
		option choices.OptionID
	}{
		{choices.ClericWeapons, choices.ClericWeaponMace}, {choices.ClericArmor, choices.ClericArmorScale}, {choices.ClericSecondaryWeapon, choices.ClericSecondaryShortbow}, {choices.ClericPack, choices.ClericPackExplorer}, {choices.ClericHolySymbol, choices.ClericHolyAmulet},
	} {
		answers = append(answers, &v1alpha1.ChoiceData{ChoiceId: string(equipment.id), OptionId: equipment.option, Category: v1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT, Source: v1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, Selection: &v1alpha1.ChoiceData_Equipment{Equipment: &v1alpha1.EquipmentSelection{}}})
	}
	update := &v1alpha1.UpdateClassRequest{DraftId: id, Class: v1alpha1.Class_CLASS_CLERIC, Subclass: v1alpha1.Subclass_SUBCLASS_KNOWLEDGE_DOMAIN, ClassChoices: answers}
	response, err := s.server.CharacterClient.UpdateClass(ctx, update)
	s.Require().NoError(err)
	s.True(response.Draft.Progress.HasClass)
	loaded, err := s.server.CharacterClient.GetDraft(ctx, &v1alpha1.GetDraftRequest{DraftId: id})
	s.Require().NoError(err)
	persisted := map[string]*v1alpha1.ChoiceData{}
	for _, choice := range loaded.Draft.Choices {
		persisted[choice.ChoiceId] = choice
	}
	for _, key := range []string{"cleric-knowledge-skills", "cleric-knowledge-languages"} {
		s.Require().NotNil(persisted[key])
		s.Equal(v1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, persisted[key].Source)
	}
	s.Len(persisted["cleric-knowledge-skills"].GetSkills().Skills, 2)
	s.Len(persisted["cleric-knowledge-languages"].GetLanguages().Languages, 2)
	// Re-save exactly what a refreshed client receives.
	update.ClassChoices = nil
	for _, choice := range loaded.Draft.Choices {
		if choice.Source == v1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS {
			update.ClassChoices = append(update.ClassChoices, choice)
		}
	}
	_, err = s.server.CharacterClient.UpdateClass(ctx, update)
	s.Require().NoError(err)
	_, err = s.server.CharacterClient.UpdateBackground(ctx, &v1alpha1.UpdateBackgroundRequest{DraftId: id, Background: v1alpha1.Background_BACKGROUND_HERMIT})
	s.Require().NoError(err)
	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &v1alpha1.UpdateAbilityScoresRequest{DraftId: id, ScoresInput: &v1alpha1.UpdateAbilityScoresRequest_AbilityScores{AbilityScores: &v1alpha1.AbilityScores{Strength: 14, Dexterity: 10, Constitution: 13, Intelligence: 8, Wisdom: 15, Charisma: 12}}})
	s.Require().NoError(err)
	finalized, err := s.server.CharacterClient.FinalizeDraft(ctx, &v1alpha1.FinalizeDraftRequest{DraftId: id})
	s.Require().NoError(err)
	read, err := s.server.CharacterClient.GetCharacter(ctx, &v1alpha1.GetCharacterRequest{CharacterId: finalized.Character.Id})
	s.Require().NoError(err)
	s.Contains(read.Character.KnownSpells, "dnd5e:spells:command")
	s.Contains(read.Character.KnownSpells, "dnd5e:spells:identify")
	s.Len(read.Character.KnownSpells, 6)
	s.ElementsMatch([]v1alpha1.Language{v1alpha1.Language_LANGUAGE_COMMON, v1alpha1.Language_LANGUAGE_DWARVISH, v1alpha1.Language_LANGUAGE_ELVISH, v1alpha1.Language_LANGUAGE_GNOMISH}, read.Character.Languages)
	s.ElementsMatch([]v1alpha1.Skill{v1alpha1.Skill_SKILL_ARCANA, v1alpha1.Skill_SKILL_HISTORY}, read.Character.Proficiencies.ExpertiseSkills)
	s.Contains(read.Character.Proficiencies.Skills, v1alpha1.Skill_SKILL_RELIGION)
	stored, err := s.server.CharacterRepo.Get(s.ctx, characterrepo.GetInput{ID: read.Character.Id})
	s.Require().NoError(err)
	character, err := tk.Load(s.ctx, stored.Character.Data)
	s.Require().NoError(err)
	for _, skill := range []skills.Skill{skills.Arcana, skills.History} {
		s.Equal(shared.Expert, stored.Character.Data.Skills[skill])
		s.Equal(character.GetAbilityModifier(abilities.INT)+2*character.ProficiencyBonus(), character.GetSkillModifier(skill))
	}
	s.Equal(shared.Proficient, stored.Character.Data.Skills[skills.Religion])
}

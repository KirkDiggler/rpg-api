package sandboxseed

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

func skillChoice(id string, count int32, available ...dnd5ev1alpha1.Skill) *dnd5ev1alpha1.Choice {
	return &dnd5ev1alpha1.Choice{
		Id: id, ChooseCount: count,
		ChoiceType: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
		Options: &dnd5ev1alpha1.Choice_SkillOptions{
			SkillOptions: &dnd5ev1alpha1.SkillOptions{Available: available},
		},
	}
}

func expertiseChoice(id string, count int32, available ...dnd5ev1alpha1.Skill) *dnd5ev1alpha1.Choice {
	return &dnd5ev1alpha1.Choice{
		Id: id, ChooseCount: count,
		ChoiceType: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE,
		Options: &dnd5ev1alpha1.Choice_ExpertiseOptions{
			ExpertiseOptions: &dnd5ev1alpha1.ExpertiseOptions{AvailableSkills: available},
		},
	}
}

// TestSatisfyChoice_TakesTheFirstOptionsTheEngineListed states the selection
// rule once, against the categories a class can ask for. First, not best: this
// fixture proves a class can be created at all, and any preference table here
// would be twelve hand-written functions wearing a different coat.
func TestSatisfyChoice_TakesTheFirstOptionsTheEngineListed(t *testing.T) {
	got, err := satisfyChoice(skillChoice("c-skills", 2,
		dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
		dnd5ev1alpha1.Skill_SKILL_STEALTH,
		dnd5ev1alpha1.Skill_SKILL_ARCANA,
	))

	require.NoError(t, err)
	require.Equal(t, "c-skills", got.GetChoiceId())
	require.Equal(t, dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, got.GetSource())
	require.Equal(t, []dnd5ev1alpha1.Skill{
		dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
		dnd5ev1alpha1.Skill_SKILL_STEALTH,
	}, got.GetSkills().GetSkills(), "the first two, and Arcana left alone")
}

// TestSatisfyChoice_RefusesAChoiceItCannotFill. A choice offering fewer options
// than it demands is a content defect, and the fixture's whole value is naming
// those rather than sending a short answer the engine will refuse later with a
// less specific message.
func TestSatisfyChoice_RefusesAChoiceItCannotFill(t *testing.T) {
	_, err := satisfyChoice(skillChoice("c-skills", 3, dnd5ev1alpha1.Skill_SKILL_ATHLETICS))

	require.Error(t, err)
	require.Contains(t, err.Error(), "c-skills")
	require.Contains(t, err.Error(), "offers only 1")
}

// TestSatisfyChoice_RefusesACategoryItCannotRead: an unknown category is an
// error rather than an empty answer, because an empty answer reaches the engine
// as a choice nobody made.
func TestSatisfyChoice_RefusesACategoryItCannotRead(t *testing.T) {
	_, err := satisfyChoice(&dnd5ev1alpha1.Choice{
		Id: "mystery", ChooseCount: 1,
		ChoiceType: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_UNSPECIFIED,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "mystery")
}

// TestSatisfyChoice_EquipmentTakesTheFirstBundleAndFillsItsCategoryPicks. The
// bundle id becomes the option id, and a "choose two martial weapons" rider is
// answered from the concrete, eligible options the bundle carries.
func TestSatisfyChoice_EquipmentTakesTheFirstBundleAndFillsItsCategoryPicks(t *testing.T) {
	got, err := satisfyChoice(&dnd5ev1alpha1.Choice{
		Id: "c-weapons", ChooseCount: 1,
		ChoiceType: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
		Options: &dnd5ev1alpha1.Choice_EquipmentOptions{
			EquipmentOptions: &dnd5ev1alpha1.EquipmentOptions{Bundles: []*dnd5ev1alpha1.EquipmentBundle{
				{
					Id: "c-weapons-a",
					CategoryChoices: []*dnd5ev1alpha1.EquipmentCategoryChoice{{
						Choose: 2,
						Options: []*dnd5ev1alpha1.EquipmentItem{
							{SelectionId: "longsword"},
							{SelectionId: "warhammer"},
							{SelectionId: "greataxe"},
						},
					}},
				},
				{Id: "c-weapons-b"},
			}},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "c-weapons-a", got.GetOptionId(), "the first bundle")
	items := got.GetEquipment().GetItems()
	require.Len(t, items, 2)
	require.Equal(t, "longsword", items[0].GetOtherEquipmentId())
	require.Equal(t, "warhammer", items[1].GetOtherEquipmentId())
}

// TestSatisfyChoice_EquipmentRefusesACategoryWithNoConcreteOptions. A bundle
// that says "choose two martial weapons" and names none it would accept cannot
// be answered, and guessing is how a fixture starts asserting content it
// invented.
func TestSatisfyChoice_EquipmentRefusesACategoryWithNoConcreteOptions(t *testing.T) {
	_, err := satisfyChoice(&dnd5ev1alpha1.Choice{
		Id: "c-weapons", ChooseCount: 1,
		ChoiceType: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
		Options: &dnd5ev1alpha1.Choice_EquipmentOptions{
			EquipmentOptions: &dnd5ev1alpha1.EquipmentOptions{Bundles: []*dnd5ev1alpha1.EquipmentBundle{{
				Id: "c-weapons-a",
				CategoryChoices: []*dnd5ev1alpha1.EquipmentCategoryChoice{{
					Choose: 2, Label: "Choose two martial weapons",
				}},
			}}},
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "carries no concrete options")
	require.Contains(t, err.Error(), "Choose two martial weapons")
}

// TestSatisfyClassChoices_ExpertisePicksOnlyFromSkillsThisSubmissionChose is
// the rogue case, and the reason the answers are built in two passes.
//
// The offered list is every skill in the game, because the toolkit's
// ExpertiseRequirement names no options and rpg-api's catalog converter fills
// the gap with skills.List(). The engine refuses expertise in a skill the
// character is not proficient in, so the only correct answer is the
// intersection with what this same submission just picked.
func TestSatisfyClassChoices_ExpertisePicksOnlyFromSkillsThisSubmissionChose(t *testing.T) {
	class := &dnd5ev1alpha1.ClassInfo{Choices: []*dnd5ev1alpha1.Choice{
		// Deliberately listed AFTER the expertise choice, to prove the answer
		// does not depend on the catalog's ordering.
		expertiseChoice("rogue-expertise-1", 2,
			dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING,
			dnd5ev1alpha1.Skill_SKILL_ARCANA,
			dnd5ev1alpha1.Skill_SKILL_STEALTH,
			dnd5ev1alpha1.Skill_SKILL_ACROBATICS,
		),
		skillChoice("rogue-skills", 2,
			dnd5ev1alpha1.Skill_SKILL_STEALTH,
			dnd5ev1alpha1.Skill_SKILL_ACROBATICS,
			dnd5ev1alpha1.Skill_SKILL_ARCANA,
		),
	}}

	answers, ids, err := satisfyClassChoices(class, nil)

	require.NoError(t, err)
	require.Equal(t, []string{"rogue-skills", "rogue-expertise-1"}, ids,
		"expertise is answered last, whatever order the catalog listed it in")

	var expertise []dnd5ev1alpha1.Skill
	for _, answer := range answers {
		if answer.GetChoiceId() == "rogue-expertise-1" {
			expertise = answer.GetExpertise().GetSkills()
		}
	}
	require.Equal(t, []dnd5ev1alpha1.Skill{
		dnd5ev1alpha1.Skill_SKILL_STEALTH,
		dnd5ev1alpha1.Skill_SKILL_ACROBATICS,
	}, expertise, "Animal Handling is offered, was not chosen, and must not be taken")
}

// TestSatisfyClassChoices_ExpertiseRefusesWhenNothingChosenIsEligible keeps the
// intersection honest: an empty overlap is a content defect, not an excuse to
// fall back to the offered list the engine would refuse.
func TestSatisfyClassChoices_ExpertiseRefusesWhenNothingChosenIsEligible(t *testing.T) {
	class := &dnd5ev1alpha1.ClassInfo{Choices: []*dnd5ev1alpha1.Choice{
		skillChoice("c-skills", 1, dnd5ev1alpha1.Skill_SKILL_STEALTH),
		expertiseChoice("c-expertise", 1, dnd5ev1alpha1.Skill_SKILL_ARCANA),
	}}

	_, _, err := satisfyClassChoices(class, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "c-expertise")
	require.Contains(t, err.Error(), "proficient in")
}

// TestResolveChoices_ASubclassReplacesByIDAndAppendsTheRest is how a domain
// cleric is answered: the catalog says a subclass's choices "replace base
// choices with matching IDs", so answering the base list alone would send the
// generic cantrip list its domain overlays.
func TestResolveChoices_ASubclassReplacesByIDAndAppendsTheRest(t *testing.T) {
	base := []*dnd5ev1alpha1.Choice{
		skillChoice("cleric-skills", 1, dnd5ev1alpha1.Skill_SKILL_MEDICINE),
		skillChoice("cleric-cantrips-1", 1, dnd5ev1alpha1.Skill_SKILL_RELIGION),
	}
	overlay := []*dnd5ev1alpha1.Choice{
		skillChoice("cleric-cantrips-1", 1, dnd5ev1alpha1.Skill_SKILL_NATURE),
		skillChoice("life-domain-extra", 1, dnd5ev1alpha1.Skill_SKILL_INSIGHT),
	}

	resolved := resolveChoices(base, overlay)

	require.Len(t, resolved, 3)
	require.Equal(t, "cleric-skills", resolved[0].GetId())
	require.Equal(t, "cleric-cantrips-1", resolved[1].GetId())
	require.Equal(t, []dnd5ev1alpha1.Skill{dnd5ev1alpha1.Skill_SKILL_NATURE},
		resolved[1].GetSkillOptions().GetAvailable(), "the subclass's version wins")
	require.Equal(t, "life-domain-extra", resolved[2].GetId(), "and its own choices are appended")
}

// TestStandardArrayFor_PutsThePrimaryAbilityHighest, so a wizard is not seeded
// with an 8 in Intelligence.
func TestStandardArrayFor_PutsThePrimaryAbilityHighest(t *testing.T) {
	wizard := standardArrayFor(dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE)
	require.Equal(t, int32(15), wizard.GetIntelligence())
	require.Equal(t, int32(14), wizard.GetConstitution(), "then the fill order takes over")
	require.Equal(t, int32(13), wizard.GetDexterity())

	barbarian := standardArrayFor(dnd5ev1alpha1.Ability_ABILITY_STRENGTH)
	require.Equal(t, int32(15), barbarian.GetStrength())
	require.Equal(t, int32(14), barbarian.GetConstitution())

	// Every value in the array is dealt exactly once, whichever ability is
	// primary: a fixture that quietly dropped one would seed a character with
	// a zero the engine reads as "unset".
	for _, primary := range []dnd5ev1alpha1.Ability{
		dnd5ev1alpha1.Ability_ABILITY_STRENGTH,
		dnd5ev1alpha1.Ability_ABILITY_CHARISMA,
		dnd5ev1alpha1.Ability_ABILITY_UNSPECIFIED,
	} {
		scores := standardArrayFor(primary)
		got := []int32{
			scores.GetStrength(), scores.GetDexterity(), scores.GetConstitution(),
			scores.GetIntelligence(), scores.GetWisdom(), scores.GetCharisma(),
		}
		require.ElementsMatch(t, standardArray, got, "primary=%s", primary)
	}
}

// TestClassSlug_ComesFromTheEnumNotATable, so a class added to the contract
// gets a walk identity without anyone remembering to add one.
func TestClassSlug_ComesFromTheEnumNotATable(t *testing.T) {
	require.Equal(t, "fighter", classSlug(dnd5ev1alpha1.Class_CLASS_FIGHTER))
	require.Equal(t, "ranger", classSlug(dnd5ev1alpha1.Class_CLASS_RANGER))
	require.Empty(t, classSlug(dnd5ev1alpha1.Class_CLASS_UNSPECIFIED),
		"unspecified is not a class and must not become an identity")
}

// TestSeedLevelUpClasses_RefusesWithoutItsDependencies. The store is as
// required as the client: experience has no service call that writes it, so a
// store-less run would produce twelve characters that cannot level.
func TestSeedLevelUpClasses_RefusesWithoutItsDependencies(t *testing.T) {
	_, err := SeedLevelUpClasses(context.Background(), &SeedLevelUpClassesInput{
		Client: newGalleryFakeClient(),
	})
	require.ErrorContains(t, err, "character store is required")

	_, err = SeedLevelUpClasses(context.Background(), &SeedLevelUpClassesInput{
		Store: newSeedFakeStore(),
	})
	require.ErrorContains(t, err, "character RPC client is required")
}

// TestSeedLevelUpClasses_RefusesAnEmptyCatalog. A catalog that offers nothing
// is a broken build, not a run that seeded zero classes successfully.
func TestSeedLevelUpClasses_RefusesAnEmptyCatalog(t *testing.T) {
	_, err := SeedLevelUpClasses(context.Background(), &SeedLevelUpClassesInput{
		Client: newGalleryFakeClient(),
		Store:  newSeedFakeStore(),
	})
	require.ErrorContains(t, err, "the catalog is empty")
}

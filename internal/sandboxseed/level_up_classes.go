// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package sandboxseed

import (
	"context"
	"fmt"
	"sort"
	"strings"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

// expectedClassCount is how many classes the 2014 Player's Handbook has, and
// therefore how many characters this fixture set must produce.
//
// Written out rather than read from the catalog. The catalog's length is the
// thing under test: a set that seeded "however many classes there are" could
// not notice the day one stopped being offered.
const expectedClassCount = 12

// levelUpClassesSkippedChoiceIDs names class choices this fixture answers
// somewhere other than the generic loop.
//
// Empty today, and kept as the seam rather than an inline condition: the
// subclass is the one class-level question that is NOT a Choice (it rides on
// UpdateClassRequest.subclass), and it is absent from ClassInfo.choices
// entirely, so nothing needs skipping yet.
var levelUpClassesSkippedChoiceIDs = map[string]bool{}

// standardArray is the 2014 standard ability array (PHB p.13), highest first.
var standardArray = []int32{15, 14, 13, 12, 10, 8}

// abilityFillOrder is the order the non-primary abilities take what is left of
// the standard array. Constitution first because every class wants hit points,
// then Dexterity for armor class; the rest is arbitrary and only has to be
// deterministic, because a fixture whose scores moved between runs would make
// every per-class report incomparable with the last one.
var abilityFillOrder = []dnd5ev1alpha1.Ability{
	dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION,
	dnd5ev1alpha1.Ability_ABILITY_DEXTERITY,
	dnd5ev1alpha1.Ability_ABILITY_WISDOM,
	dnd5ev1alpha1.Ability_ABILITY_STRENGTH,
	dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE,
	dnd5ev1alpha1.Ability_ABILITY_CHARISMA,
}

// fallbackLanguages answers a language choice whose option list is empty.
//
// An empty list means "any language" (LanguageOptions' own doc), not "no
// languages", so it is satisfiable and must not be treated as a content gap.
var fallbackLanguages = []dnd5ev1alpha1.Language{
	dnd5ev1alpha1.Language_LANGUAGE_DWARVISH,
	dnd5ev1alpha1.Language_LANGUAGE_ELVISH,
	dnd5ev1alpha1.Language_LANGUAGE_GIANT,
	dnd5ev1alpha1.Language_LANGUAGE_GNOMISH,
	dnd5ev1alpha1.Language_LANGUAGE_GOBLIN,
	dnd5ev1alpha1.Language_LANGUAGE_ORC,
}

// SeedLevelUpClassesInput contains the dependencies for the per-class fixture set.
type SeedLevelUpClassesInput struct {
	Client CharacterRPC
	Store  CharacterStore
}

// SeedLevelUpClassesOutput reports what was created, in class order.
type SeedLevelUpClassesOutput struct {
	Classes []LevelUpClassResult
}

// LevelUpClassResult is one seeded class.
type LevelUpClassResult struct {
	Identity    string
	CharacterID string
	Class       string
	ChoiceIDs   []string
}

// SeedLevelUpClasses creates one level-1 character per class, each holding the
// experience for level 2, so every class can be walked through the real
// level-up screen.
//
// NOT IN THE DEFAULT SET. Twelve characters on every `up` of every environment
// is a cost no other walk should pay; this one is selected with
// `-fixture level-up-classes`.
//
// THE CREATION IS DRIVEN BY THE CATALOG, not by twelve hand-written functions.
// It reads ClassInfo.choices and answers each Choice from the options that
// Choice itself carries, which is the same thing the level-up screen does with
// the requirements it is handed (design R4.13: the screen "MUST contain no
// class-specific branch"). A fixture with a branch per class would prove the
// twelve functions work, not that the generic path does.
//
// EVERY FAILURE IS NAMED, AND NONE IS SKIPPED. A class the generic path cannot
// finalize is a finding about that class's content, so it is collected with the
// validation error that refused it and reported at the end. The run continues
// through the remaining classes rather than stopping at the first, because
// stopping would hide eleven answers to get one; the classes that did succeed
// are seeded and walkable either way.
func SeedLevelUpClasses(
	ctx context.Context,
	input *SeedLevelUpClassesInput,
) (*SeedLevelUpClassesOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("%s: input is required", levelUpClassesFixtureName)
	}
	if input.Client == nil {
		return nil, fmt.Errorf("%s: character RPC client is required", levelUpClassesFixtureName)
	}
	if input.Store == nil {
		return nil, fmt.Errorf("%s: character store is required to seed experience", levelUpClassesFixtureName)
	}

	// The catalog is static content, but every RPC on this service is
	// authenticated, so the read is made under the fixture set's own name
	// rather than under one of the twelve characters' -- there is no character
	// yet when this runs, and borrowing one would imply the catalog it returns
	// depends on who asked.
	catalogCtx := authenticatedContext(ctx, levelUpClassesFixtureName)
	catalog, err := input.Client.ListClasses(catalogCtx, &dnd5ev1alpha1.ListClassesRequest{PageSize: listPageSize})
	if err != nil {
		return nil, rpcError(levelUpClassesFixtureName, "ListClasses", err)
	}
	classes := catalog.GetClasses()
	if len(classes) == 0 {
		return nil, fmt.Errorf("%s ListClasses: the catalog is empty", levelUpClassesFixtureName)
	}

	out := &SeedLevelUpClassesOutput{}
	var failures []string
	for _, class := range classes {
		result, seedErr := seedOneLevelUpClass(ctx, input, class)
		if seedErr != nil {
			failures = append(failures, seedErr.Error())
			continue
		}
		out.Classes = append(out.Classes, *result)
		fmt.Printf("sandboxseed: identity=%s character_id=%s class=%s choices=[%s]\n",
			result.Identity, result.CharacterID, result.Class, strings.Join(result.ChoiceIDs, " "))
	}

	if len(failures) > 0 {
		return out, fmt.Errorf("%s: %d of %d classes could not be created generically:\n  %s",
			levelUpClassesFixtureName, len(failures), len(classes), strings.Join(failures, "\n  "))
	}
	if len(classes) != expectedClassCount {
		return out, fmt.Errorf("%s ListClasses: the catalog offers %d classes, want %d",
			levelUpClassesFixtureName, len(classes), expectedClassCount)
	}
	return out, nil
}

// seedOneLevelUpClass creates, finalizes and seeds a single class.
func seedOneLevelUpClass(
	ctx context.Context,
	input *SeedLevelUpClassesInput,
	class *dnd5ev1alpha1.ClassInfo,
) (*LevelUpClassResult, error) {
	slug := classSlug(class.GetClassId())
	if slug == "" {
		return nil, fmt.Errorf("class %v has no enum name to build an identity from", class.GetClassId())
	}
	identity := levelUpIdentityPrefix + slug
	name := displayNameForClass(class)

	// The subclass a class picks AT LEVEL 1, if it has one. ClassInfo only
	// carries Subclasses when the pick is reachable at creation (rpg-api#990),
	// so a non-empty list here means the class is asking now.
	var subclass *dnd5ev1alpha1.SubclassInfo
	if len(class.GetSubclasses()) > 0 {
		subclass = class.GetSubclasses()[0]
	}

	choiceData, choiceIDs, err := satisfyClassChoices(class, subclass)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", identity, err)
	}

	identityCtx := authenticatedContext(ctx, identity)
	if delErr := deleteListedCharacters(identityCtx, input.Client, identity); delErr != nil {
		return nil, delErr
	}

	createErr := createGenericClassCharacter(identityCtx, &genericClassCharacterInput{
		Client:   input.Client,
		Identity: identity,
		Name:     name,
		Class:    class,
		Subclass: subclass,
		Choices:  choiceData,
	})
	if createErr != nil {
		return nil, createErr
	}

	characterID, err := listExactlyOne(identityCtx, input.Client, identity, name)
	if err != nil {
		return nil, err
	}
	if seedErr := seedLevelUpExperience(identityCtx, &SeedInput{
		Client: input.Client, Store: input.Store,
	}, identity, characterID); seedErr != nil {
		return nil, seedErr
	}

	return &LevelUpClassResult{
		Identity:    identity,
		CharacterID: characterID,
		Class:       slug,
		ChoiceIDs:   choiceIDs,
	}, nil
}

// satisfyClassChoices answers every choice the class asks at level 1.
//
// When a subclass is taken, its AdditionalChoices REPLACE base choices with
// matching IDs and add the rest — the same resolution the catalog converter
// describes at the site that builds them. Answering the base list alone would
// send a domain cleric the generic cantrip list its domain overlays.
func satisfyClassChoices(
	class *dnd5ev1alpha1.ClassInfo,
	subclass *dnd5ev1alpha1.SubclassInfo,
) ([]*dnd5ev1alpha1.ChoiceData, []string, error) {
	effective := resolveChoices(class.GetChoices(), subclass.GetAdditionalChoices())

	// TWO PASSES, because one choice depends on another. Expertise must name
	// skills the character is PROFICIENT in, and at creation that set is
	// whatever the skill choice in this same submission just picked. The wire
	// cannot know it -- ExpertiseOptions carries every skill in the game -- so
	// the only client that can answer correctly is one holding the whole
	// submission. Doing it in two passes rather than relying on the catalog
	// listing skills before expertise keeps that independent of ordering.
	answers := make([]*dnd5ev1alpha1.ChoiceData, 0, len(effective))
	ids := make([]string, 0, len(effective))
	var deferred []*dnd5ev1alpha1.Choice
	chosenSkills := map[dnd5ev1alpha1.Skill]bool{}

	for _, choice := range effective {
		if levelUpClassesSkippedChoiceIDs[choice.GetId()] {
			continue
		}
		if choice.GetChoiceType() == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE {
			deferred = append(deferred, choice)
			continue
		}
		answer, err := satisfyChoice(choice)
		if err != nil {
			return nil, nil, err
		}
		for _, skill := range answer.GetSkills().GetSkills() {
			chosenSkills[skill] = true
		}
		answers = append(answers, answer)
		ids = append(ids, choice.GetId())
	}

	for _, choice := range deferred {
		answer, err := satisfyExpertiseChoice(choice, chosenSkills)
		if err != nil {
			return nil, nil, err
		}
		answers = append(answers, answer)
		ids = append(ids, choice.GetId())
	}

	return answers, ids, nil
}

// satisfyExpertiseChoice picks expertise from the skills this submission has
// already chosen.
//
// THE OFFERED LIST IS NOT THE LEGAL LIST. ExpertiseOptions.available_skills
// carries all eighteen skills, because the toolkit's ExpertiseRequirement is
// {ID, Count, Label} and names no options at all, so rpg-api's catalog
// converter fills the field with skills.List(). The engine then refuses any
// expertise skill the character is not proficient in.
//
// THIS IS THE CLIENT COMPUTING A RULE THE TOOLKIT DOES NOT YET EXPOSE, and it
// is temporary by intent. The seam that ends it is a character-aware view on
// the toolkit side -- the same shape Character.NextLevelRequirements already
// took for spells, which removes what the character knows from what it is
// offered (design R4.4e). When expertise gets that view, this intersection
// deletes itself and the offered list becomes the legal list. Until then it is
// the only answer a client can compute, here or on the level-up screen.
//
// TRACKED AS rpg-toolkit#1794, which names this function as what closing it
// deletes -- along with the two-pass ordering above, which exists only so the
// skill choice is answered before this one. An undated workaround with a good
// comment is how a stopgap becomes permanent; the issue is what makes it
// visibly removable instead.
func satisfyExpertiseChoice(
	choice *dnd5ev1alpha1.Choice,
	chosenSkills map[dnd5ev1alpha1.Skill]bool,
) (*dnd5ev1alpha1.ChoiceData, error) {
	eligible := make([]dnd5ev1alpha1.Skill, 0, len(chosenSkills))
	for _, skill := range choice.GetExpertiseOptions().GetAvailableSkills() {
		if chosenSkills[skill] {
			eligible = append(eligible, skill)
		}
	}

	count := int(choice.GetChooseCount())
	if len(eligible) < count {
		return nil, fmt.Errorf(
			"expertise choice %q asks for %d skills the character is proficient in, "+
				"but only %d of this class's chosen skills appear in its option list",
			choice.GetId(), count, len(eligible))
	}

	return &dnd5ev1alpha1.ChoiceData{
		Category: choice.GetChoiceType(),
		Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
		ChoiceId: choice.GetId(),
		Selection: &dnd5ev1alpha1.ChoiceData_Expertise{
			Expertise: &dnd5ev1alpha1.ExpertiseSelection{Skills: eligible[:count]},
		},
	}, nil
}

// resolveChoices overlays a subclass's choices onto the class's own, by ID,
// preserving the base order and appending anything new.
func resolveChoices(base, overlay []*dnd5ev1alpha1.Choice) []*dnd5ev1alpha1.Choice {
	if len(overlay) == 0 {
		return base
	}

	byID := make(map[string]*dnd5ev1alpha1.Choice, len(overlay))
	for _, choice := range overlay {
		byID[choice.GetId()] = choice
	}

	resolved := make([]*dnd5ev1alpha1.Choice, 0, len(base)+len(overlay))
	seen := make(map[string]bool, len(base))
	for _, choice := range base {
		seen[choice.GetId()] = true
		if replacement, ok := byID[choice.GetId()]; ok {
			resolved = append(resolved, replacement)
			continue
		}
		resolved = append(resolved, choice)
	}
	for _, choice := range overlay {
		if !seen[choice.GetId()] {
			resolved = append(resolved, choice)
		}
	}
	return resolved
}

// satisfyChoice takes the first choose_count options a Choice offers.
//
// FIRST, NOT BEST. This fixture exists to prove that a class can be created and
// leveled at all, and "the first options the engine listed" is the only
// selection rule that stays honest for a class nobody has written yet. A
// preference table here would be twelve hand-written functions wearing a
// different coat.
//
//nolint:gocyclo // One arm per wire category; splitting it would hide the mapping it exists to state.
func satisfyChoice(choice *dnd5ev1alpha1.Choice) (*dnd5ev1alpha1.ChoiceData, error) {
	answer := &dnd5ev1alpha1.ChoiceData{
		Category: choice.GetChoiceType(),
		Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
		ChoiceId: choice.GetId(),
	}
	count := int(choice.GetChooseCount())

	switch choice.GetChoiceType() {
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS:
		picked, err := takeFirst(choice.GetSkillOptions().GetAvailable(), count, choice, "skills")
		if err != nil {
			return nil, err
		}
		answer.Selection = &dnd5ev1alpha1.ChoiceData_Skills{
			Skills: &dnd5ev1alpha1.SkillSelection{Skills: picked},
		}

	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS:
		picked, err := takeFirst(choice.GetToolOptions().GetAvailable(), count, choice, "tools")
		if err != nil {
			return nil, err
		}
		answer.Selection = &dnd5ev1alpha1.ChoiceData_Tools{
			Tools: &dnd5ev1alpha1.ToolSelection{Tools: picked},
		}

	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES:
		available := choice.GetLanguageOptions().GetAvailable()
		if len(available) == 0 {
			// "nil in toolkit means any language" (LanguageOptions' doc), so
			// an empty list is satisfiable rather than a content gap.
			available = fallbackLanguages
		}
		picked, err := takeFirst(available, count, choice, "languages")
		if err != nil {
			return nil, err
		}
		answer.Selection = &dnd5ev1alpha1.ChoiceData_Languages{
			Languages: &dnd5ev1alpha1.LanguageSelection{Languages: picked},
		}

	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE:
		picked, err := takeFirst(choice.GetFightingStyleOptions().GetAvailable(), count, choice, "fighting styles")
		if err != nil {
			return nil, err
		}
		answer.Selection = &dnd5ev1alpha1.ChoiceData_FightingStyle{
			FightingStyle: &dnd5ev1alpha1.FightingStyleSelection{Style: picked[0]},
		}

	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS,
		dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS:
		// Canonical refs, never the deprecated closed enum: "the closed enum
		// cannot name a spell the catalog learned after it was generated"
		// (SpellOptions' own doc).
		picked, err := takeFirst(choice.GetSpellOptions().GetAvailableRefs(), count, choice, "spells")
		if err != nil {
			return nil, err
		}
		answer.Selection = &dnd5ev1alpha1.ChoiceData_Spells{
			Spells: &dnd5ev1alpha1.SpellSelection{SpellRefs: picked},
		}

	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT:
		selection, optionID, err := satisfyEquipmentChoice(choice)
		if err != nil {
			return nil, err
		}
		answer.OptionId = optionID
		answer.Selection = &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: selection}

	default:
		return nil, fmt.Errorf(
			"choice %q asks for category %s, which this fixture cannot answer generically",
			choice.GetId(), choice.GetChoiceType())
	}

	return answer, nil
}

// satisfyEquipmentChoice takes the first bundle and fills in each of its
// category picks from the concrete options the bundle carries.
func satisfyEquipmentChoice(
	choice *dnd5ev1alpha1.Choice,
) (*dnd5ev1alpha1.EquipmentSelection, string, error) {
	bundles := choice.GetEquipmentOptions().GetBundles()
	if len(bundles) == 0 {
		return nil, "", fmt.Errorf("equipment choice %q offers no bundles", choice.GetId())
	}
	bundle := bundles[0]

	selection := &dnd5ev1alpha1.EquipmentSelection{}
	for _, category := range bundle.GetCategoryChoices() {
		options := category.GetOptions()
		if len(options) == 0 {
			// The bundle says "choose two martial weapons" and names none it
			// would accept. A fixture cannot guess, and guessing is how a
			// fixture starts asserting content it invented.
			return nil, "", fmt.Errorf(
				"equipment choice %q bundle %q asks for %d from %q and carries no concrete options",
				choice.GetId(), bundle.GetId(), category.GetChoose(), category.GetLabel())
		}
		need := int(category.GetChoose())
		if len(options) < need {
			return nil, "", fmt.Errorf(
				"equipment choice %q bundle %q asks for %d from %q but offers only %d",
				choice.GetId(), bundle.GetId(), need, category.GetLabel(), len(options))
		}
		for _, item := range options[:need] {
			// OtherEquipmentId carries the toolkit's own SelectionID straight
			// through, which is what the equipment reader falls back to and
			// what the eligible-options list is written in. Mapping it onto a
			// Weapon/Armor/Tool enum first would only add a table that can
			// fail to name something the catalog knows.
			selection.Items = append(selection.Items, &dnd5ev1alpha1.EquipmentSelectionItem{
				Equipment: &dnd5ev1alpha1.EquipmentSelectionItem_OtherEquipmentId{
					OtherEquipmentId: item.GetSelectionId(),
				},
				Quantity: 1,
			})
		}
	}

	return selection, bundle.GetId(), nil
}

// takeFirst is the selection rule, stated once: the first n the engine listed.
func takeFirst[T any](available []T, n int, choice *dnd5ev1alpha1.Choice, what string) ([]T, error) {
	if n <= 0 {
		return nil, fmt.Errorf("choice %q asks for %d %s", choice.GetId(), n, what)
	}
	if len(available) < n {
		return nil, fmt.Errorf("choice %q asks for %d %s but offers only %d",
			choice.GetId(), n, what, len(available))
	}
	return available[:n], nil
}

type genericClassCharacterInput struct {
	Client   CharacterRPC
	Identity string
	Name     string
	Class    *dnd5ev1alpha1.ClassInfo
	Subclass *dnd5ev1alpha1.SubclassInfo
	Choices  []*dnd5ev1alpha1.ChoiceData
}

// createGenericClassCharacter walks the production creation RPC chain, the same
// one every other fixture uses, so the stored sheet carries real feature blobs.
//
// Race and background are FIXED and the class is the only variable: human, and
// the Soldier background the sandbox fighter already uses, both with the same
// answers those fixtures give. The per-class question is what the class asks,
// and holding everything else still is what makes twelve reports comparable.
func createGenericClassCharacter(ctx context.Context, input *genericClassCharacterInput) error {
	createResponse, createErr := input.Client.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	if createErr != nil {
		return rpcError(input.Identity, "CreateDraft", createErr)
	}
	draftID := createResponse.GetDraft().GetId()
	if draftID == "" {
		return fmt.Errorf("%s CreateDraft: response draft ID is empty", input.Identity)
	}

	if _, err := input.Client.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    input.Name,
	}); err != nil {
		return rpcError(input.Identity, "UpdateName", err)
	}
	if _, err := input.Client.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
			Selection: &dnd5ev1alpha1.ChoiceData_Languages{
				Languages: &dnd5ev1alpha1.LanguageSelection{
					Languages: []dnd5ev1alpha1.Language{dnd5ev1alpha1.Language_LANGUAGE_DWARVISH},
				},
			},
		}},
	}); err != nil {
		return rpcError(input.Identity, "UpdateRace", err)
	}

	classRequest := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId:      draftID,
		Class:        input.Class.GetClassId(),
		ClassChoices: input.Choices,
	}
	if input.Subclass != nil {
		classRequest.Subclass = input.Subclass.GetSubclassId()
	}
	if _, err := input.Client.UpdateClass(ctx, classRequest); err != nil {
		return rpcError(input.Identity, "UpdateClass", err)
	}

	if _, err := input.Client.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:           draftID,
		Background:        dnd5ev1alpha1.Background_BACKGROUND_SOLDIER,
		BackgroundChoices: soldierBackgroundChoices(),
	}); err != nil {
		return rpcError(input.Identity, "UpdateBackground", err)
	}
	if _, err := input.Client.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: standardArrayFor(input.Class.GetPrimaryAbility()),
		},
	}); err != nil {
		return rpcError(input.Identity, "UpdateAbilityScores", err)
	}
	if _, err := input.Client.GetDraft(ctx, &dnd5ev1alpha1.GetDraftRequest{DraftId: draftID}); err != nil {
		return rpcError(input.Identity, "GetDraft", err)
	}
	if _, err := input.Client.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	}); err != nil {
		return rpcError(input.Identity, "FinalizeDraft", err)
	}
	return nil
}

// soldierBackgroundChoices answers the Soldier background's two independent
// picks, the same two the sandbox fighter answers (rpg-toolkit#1554): an
// equipment pick and an unrelated gaming-set proficiency.
func soldierBackgroundChoices() []*dnd5ev1alpha1.ChoiceData {
	return []*dnd5ev1alpha1.ChoiceData{
		{
			Category:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
			Source:    dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND,
			ChoiceId:  "soldier-gaming-set-item",
			OptionId:  "soldier-gaming-set-a",
			Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{}},
		},
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND,
			ChoiceId: "soldier-gaming-set-proficiency",
			Selection: &dnd5ev1alpha1.ChoiceData_Tools{Tools: &dnd5ev1alpha1.ToolSelection{
				Tools: []dnd5ev1alpha1.Tool{dnd5ev1alpha1.Tool_TOOL_DICE_SET},
			}},
		},
	}
}

// standardArrayFor deals the standard array out with the class's own primary
// ability highest, so a wizard is not seeded with an 8 in Intelligence.
func standardArrayFor(primary dnd5ev1alpha1.Ability) *dnd5ev1alpha1.AbilityScores {
	scores := map[dnd5ev1alpha1.Ability]int32{}
	remaining := standardArray

	if primary != dnd5ev1alpha1.Ability_ABILITY_UNSPECIFIED {
		scores[primary] = remaining[0]
		remaining = remaining[1:]
	}
	for _, ability := range abilityFillOrder {
		if _, taken := scores[ability]; taken {
			continue
		}
		if len(remaining) == 0 {
			break
		}
		scores[ability] = remaining[0]
		remaining = remaining[1:]
	}

	return &dnd5ev1alpha1.AbilityScores{
		Strength:     scores[dnd5ev1alpha1.Ability_ABILITY_STRENGTH],
		Dexterity:    scores[dnd5ev1alpha1.Ability_ABILITY_DEXTERITY],
		Constitution: scores[dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION],
		Intelligence: scores[dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE],
		Wisdom:       scores[dnd5ev1alpha1.Ability_ABILITY_WISDOM],
		Charisma:     scores[dnd5ev1alpha1.Ability_ABILITY_CHARISMA],
	}
}

// classSlug turns CLASS_FIGHTER into "fighter".
//
// Derived from the enum's own name rather than from a table here or from the
// display name: the enum IS the contract, so a class added to it gets an
// identity without anyone remembering to add one.
func classSlug(id dnd5ev1alpha1.Class) string {
	name, ok := dnd5ev1alpha1.Class_name[int32(id)]
	if !ok || name == "" || id == dnd5ev1alpha1.Class_CLASS_UNSPECIFIED {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(name, "CLASS_"))
}

// displayNameForClass is what the character is called on the walk. The
// catalog's own name when it has one, so the sheet reads "Fighter" rather than
// an identity slug.
func displayNameForClass(class *dnd5ev1alpha1.ClassInfo) string {
	if name := class.GetName(); name != "" {
		return name
	}
	return classSlug(class.GetClassId())
}

// SortedClassSlugs is the identities this fixture set produces, for a caller
// that wants to name them without running it.
func SortedClassSlugs(classes []*dnd5ev1alpha1.ClassInfo) []string {
	slugs := make([]string, 0, len(classes))
	for _, class := range classes {
		if slug := classSlug(class.GetClassId()); slug != "" {
			slugs = append(slugs, slug)
		}
	}
	sort.Strings(slugs)
	return slugs
}

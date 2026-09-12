// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package sandboxseed creates dev-only sandbox fixtures through the
// production CharacterService RPC surface, including the fixed toolkit
// contributors and the repeatable weapon gallery character.
package sandboxseed

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

const (
	fighterIdentity   = "toolkit-sandbox-fighter"
	barbarianIdentity = "toolkit-sandbox-barbarian"
	bardIdentity      = "toolkit-sandbox-bard"

	fighterName   = "Toolkit Sandbox Fighter"
	barbarianName = "Toolkit Sandbox Barbarian"
	bardName      = "Toolkit Sandbox Bard"

	listPageSize = 100
	shieldItemID = "shield"

	// The bard fixture's known spells, as the canonical refs the live
	// SpellSelection.spell_refs field takes.
	bladeWardRef      = "dnd5e:spells:blade-ward"
	viciousMockeryRef = "dnd5e:spells:vicious-mockery"
	// ALL FOUR, because the bard's level-1 pick takes the whole catalogue
	// while the catalogue is no larger than the class progression
	// (rpg-toolkit#1661). The requirement validates len(chosen) == Count
	// exactly, so this list is not a preference -- it is the whole of what
	// the pick allows, and a fixture carrying fewer would be refused at
	// finalize. Command is the arrival that took the count to four, which is
	// where the progression stops: from here the catalogue outgrows the pick
	// and this list becomes a choice again.
	//
	// The four are also the four worth having. Bane is the cast aimed at
	// CREATURES the caller names; Thunderwave is the one aimed at a CELL,
	// the caster-edge cube that the cell on CastRequest exists to point;
	// Dissonant Whispers is the one creature that saves for HALF and, on a
	// failure, pays its reaction and runs (rpg-project#437); Command is the
	// one that asks the CASTER a question before it goes -- the word rides
	// on CastRequest.option the way the cell rides on CastRequest.cell --
	// and then spends its victim's whole next turn (rpg-project#442). A
	// sandbox bard who knows all four can walk every selector shape, both
	// save outcomes, and the only cast-time menu on the board.
	baneRef              = "dnd5e:spells:bane"
	thunderwaveRef       = "dnd5e:spells:thunderwave"
	dissonantWhispersRef = "dnd5e:spells:dissonant-whispers"
	commandRef           = "dnd5e:spells:command"
)

// CharacterRPC is the narrow CharacterService client surface used by Seed.
type CharacterRPC interface {
	ListCharacters(context.Context, *dnd5ev1alpha1.ListCharactersRequest, ...grpc.CallOption) (*dnd5ev1alpha1.ListCharactersResponse, error)
	DeleteCharacter(context.Context, *dnd5ev1alpha1.DeleteCharacterRequest, ...grpc.CallOption) (*dnd5ev1alpha1.DeleteCharacterResponse, error)
	CreateDraft(context.Context, *dnd5ev1alpha1.CreateDraftRequest, ...grpc.CallOption) (*dnd5ev1alpha1.CreateDraftResponse, error)
	UpdateName(context.Context, *dnd5ev1alpha1.UpdateNameRequest, ...grpc.CallOption) (*dnd5ev1alpha1.UpdateNameResponse, error)
	UpdateRace(context.Context, *dnd5ev1alpha1.UpdateRaceRequest, ...grpc.CallOption) (*dnd5ev1alpha1.UpdateRaceResponse, error)
	UpdateClass(context.Context, *dnd5ev1alpha1.UpdateClassRequest, ...grpc.CallOption) (*dnd5ev1alpha1.UpdateClassResponse, error)
	UpdateBackground(context.Context, *dnd5ev1alpha1.UpdateBackgroundRequest, ...grpc.CallOption) (*dnd5ev1alpha1.UpdateBackgroundResponse, error)
	UpdateAbilityScores(context.Context, *dnd5ev1alpha1.UpdateAbilityScoresRequest, ...grpc.CallOption) (*dnd5ev1alpha1.UpdateAbilityScoresResponse, error)
	GetDraft(context.Context, *dnd5ev1alpha1.GetDraftRequest, ...grpc.CallOption) (*dnd5ev1alpha1.GetDraftResponse, error)
	FinalizeDraft(context.Context, *dnd5ev1alpha1.FinalizeDraftRequest, ...grpc.CallOption) (*dnd5ev1alpha1.FinalizeDraftResponse, error)
	GetCharacter(context.Context, *dnd5ev1alpha1.GetCharacterRequest, ...grpc.CallOption) (*dnd5ev1alpha1.GetCharacterResponse, error)
	EquipItem(context.Context, *dnd5ev1alpha1.EquipItemRequest, ...grpc.CallOption) (*dnd5ev1alpha1.EquipItemResponse, error)
}

// Seed resets the two fixed sandbox identities and recreates their fixed
// characters through implemented CharacterService RPCs only.
func Seed(ctx context.Context, client CharacterRPC) error {
	if client == nil {
		return errors.New("sandbox seed: character RPC client is required")
	}

	if err := seedFighter(ctx, client); err != nil {
		return err
	}
	if err := seedBarbarian(ctx, client); err != nil {
		return err
	}
	return seedBard(ctx, client)
}

// seedBard creates the fixed caster fixture: a level-one bard who already knows
// two castable cantrips and the one supported leveled spell.
//
// The sandbox had a fighter and a barbarian and no caster at all, so every walk
// of spell work started by building a bard through the creation flow by hand.
// This is that character, made once through the same production RPCs the other
// two use.
//
// The cantrips are Blade Ward and Vicious Mockery deliberately: one self-target
// and one creature-target, so the two cast shapes are both reachable the moment
// the fixture loads. Charisma is 16 rather than the array's default so the spell
// save DC is a number worth reading rather than the minimum.
func seedBard(ctx context.Context, client CharacterRPC) error {
	identityCtx := authenticatedContext(ctx, bardIdentity)
	if err := deleteListedCharacters(identityCtx, client, bardIdentity); err != nil {
		return err
	}

	createResponse, createErr := client.CreateDraft(identityCtx, &dnd5ev1alpha1.CreateDraftRequest{})
	if createErr != nil {
		return rpcError(bardIdentity, "CreateDraft", createErr)
	}
	draftID := createResponse.GetDraft().GetId()
	if draftID == "" {
		return fmt.Errorf("%s CreateDraft: response draft ID is empty", bardIdentity)
	}

	if _, err := client.UpdateName(identityCtx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    bardName,
	}); err != nil {
		return rpcError(bardIdentity, "UpdateName", err)
	}
	if _, err := client.UpdateRace(identityCtx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
			Selection: &dnd5ev1alpha1.ChoiceData_Languages{
				Languages: &dnd5ev1alpha1.LanguageSelection{
					Languages: []dnd5ev1alpha1.Language{dnd5ev1alpha1.Language_LANGUAGE_ELVISH},
				},
			},
		}},
	}); err != nil {
		return rpcError(bardIdentity, "UpdateRace", err)
	}
	if _, err := client.UpdateClass(identityCtx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_BARD,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_PERSUASION,
							dnd5ev1alpha1.Skill_SKILL_PERFORMANCE,
							dnd5ev1alpha1.Skill_SKILL_DECEPTION,
						},
					},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-instruments",
				Selection: &dnd5ev1alpha1.ChoiceData_Tools{Tools: &dnd5ev1alpha1.ToolSelection{
					Tools: []dnd5ev1alpha1.Tool{
						dnd5ev1alpha1.Tool_TOOL_LUTE,
						dnd5ev1alpha1.Tool_TOOL_FLUTE,
						dnd5ev1alpha1.Tool_TOOL_DRUM,
					},
				}},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-weapons-primary",
				OptionId: "bard-weapon-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-pack",
				OptionId: "bard-pack-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-instrument",
				OptionId: "bard-instrument-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-cantrips-1",
				Selection: &dnd5ev1alpha1.ChoiceData_Spells{Spells: &dnd5ev1alpha1.SpellSelection{
					SpellRefs: []string{bladeWardRef, viciousMockeryRef},
				}},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-spells-1",
				Selection: &dnd5ev1alpha1.ChoiceData_Spells{Spells: &dnd5ev1alpha1.SpellSelection{
					SpellRefs: []string{baneRef, thunderwaveRef, dissonantWhispersRef, commandRef},
				}},
			},
		},
	}); err != nil {
		return rpcError(bardIdentity, "UpdateClass", err)
	}
	if _, err := client.UpdateBackground(identityCtx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER,
		BackgroundChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND,
				ChoiceId: "outlander-instrument",
				Selection: &dnd5ev1alpha1.ChoiceData_Tools{Tools: &dnd5ev1alpha1.ToolSelection{
					Tools: []dnd5ev1alpha1.Tool{dnd5ev1alpha1.Tool_TOOL_LYRE},
				}},
			},
		},
	}); err != nil {
		return rpcError(bardIdentity, "UpdateBackground", err)
	}
	if _, err := client.UpdateAbilityScores(identityCtx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     8,
				Dexterity:    14,
				Constitution: 13,
				Intelligence: 10,
				Wisdom:       12,
				Charisma:     16,
			},
		},
	}); err != nil {
		return rpcError(bardIdentity, "UpdateAbilityScores", err)
	}
	if _, err := client.GetDraft(identityCtx, &dnd5ev1alpha1.GetDraftRequest{DraftId: draftID}); err != nil {
		return rpcError(bardIdentity, "GetDraft", err)
	}
	if _, err := client.FinalizeDraft(identityCtx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	}); err != nil {
		return rpcError(bardIdentity, "FinalizeDraft", err)
	}

	characterID, err := listExactlyOne(identityCtx, client, bardIdentity, bardName)
	if err != nil {
		return err
	}
	characterResponse, err := client.GetCharacter(identityCtx, &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: characterID,
	})
	if err != nil {
		return rpcError(bardIdentity, "GetCharacter", err)
	}

	// The known lists are printed rather than assumed. A bard that finalized
	// but learned nothing is the failure worth catching here: it looks like a
	// working fixture right up until the action dock has no cast row on it.
	character := characterResponse.GetCharacter()
	if len(character.GetKnownCantrips()) == 0 {
		return fmt.Errorf("%s GetCharacter: finalized with no known cantrips", bardIdentity)
	}
	if len(character.GetKnownSpells()) == 0 {
		return fmt.Errorf("%s GetCharacter: finalized with no known spells", bardIdentity)
	}

	fmt.Printf("sandboxseed: identity=%s character_id=%s charisma=%d cantrips=%v spells=%v\n",
		bardIdentity,
		characterID,
		character.GetAbilityScores().GetCharisma(),
		character.GetKnownCantrips(),
		character.GetKnownSpells(),
	)
	return nil
}

func seedFighter(ctx context.Context, client CharacterRPC) error {
	identityCtx := authenticatedContext(ctx, fighterIdentity)
	if err := deleteListedCharacters(identityCtx, client, fighterIdentity); err != nil {
		return err
	}

	if err := createHumanFighter(identityCtx, &createHumanFighterInput{
		Client:   client,
		Identity: fighterIdentity,
		Name:     fighterName,
	}); err != nil {
		return err
	}

	characterID, err := listExactlyOne(identityCtx, client, fighterIdentity, fighterName)
	if err != nil {
		return err
	}
	characterResponse, err := client.GetCharacter(identityCtx, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: characterID})
	if err != nil {
		return rpcError(fighterIdentity, "GetCharacter", err)
	}
	shieldID, err := inventoryItemID(characterResponse.GetCharacter(), shieldItemID)
	if err != nil {
		return fmt.Errorf("%s GetCharacter: %w", fighterIdentity, err)
	}
	equipResponse, err := client.EquipItem(identityCtx, &dnd5ev1alpha1.EquipItemRequest{
		CharacterId: characterID,
		ItemId:      shieldID,
		Slot:        dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_OFF_HAND,
	})
	if err != nil {
		return rpcError(fighterIdentity, "EquipItem", err)
	}
	if equipResponse.GetCharacter().GetEquipmentSlots().GetOffHand().GetItemId() != shieldItemID {
		return fmt.Errorf("%s EquipItem: off hand item is %q, want %q",
			fighterIdentity,
			equipResponse.GetCharacter().GetEquipmentSlots().GetOffHand().GetItemId(),
			shieldItemID,
		)
	}
	if _, finalListErr := listExactlyOne(identityCtx, client, fighterIdentity, fighterName); finalListErr != nil {
		return finalListErr
	}

	fmt.Printf("sandboxseed: identity=%s character_id=%s strength=%d off_hand=%s\n",
		fighterIdentity,
		characterID,
		equipResponse.GetCharacter().GetAbilityScores().GetStrength(),
		equipResponse.GetCharacter().GetEquipmentSlots().GetOffHand().GetItemId(),
	)
	return nil
}

func seedBarbarian(ctx context.Context, client CharacterRPC) error {
	identityCtx := authenticatedContext(ctx, barbarianIdentity)
	if err := deleteListedCharacters(identityCtx, client, barbarianIdentity); err != nil {
		return err
	}

	createResponse, createErr := client.CreateDraft(identityCtx, &dnd5ev1alpha1.CreateDraftRequest{})
	if createErr != nil {
		return rpcError(barbarianIdentity, "CreateDraft", createErr)
	}
	draftID := createResponse.GetDraft().GetId()
	if draftID == "" {
		return fmt.Errorf("%s CreateDraft: response draft ID is empty", barbarianIdentity)
	}

	if _, err := client.UpdateName(identityCtx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    barbarianName,
	}); err != nil {
		return rpcError(barbarianIdentity, "UpdateName", err)
	}
	if _, err := client.UpdateRace(identityCtx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
			Selection: &dnd5ev1alpha1.ChoiceData_Languages{
				Languages: &dnd5ev1alpha1.LanguageSelection{
					Languages: []dnd5ev1alpha1.Language{dnd5ev1alpha1.Language_LANGUAGE_ORC},
				},
			},
		}},
	}); err != nil {
		return rpcError(barbarianIdentity, "UpdateRace", err)
	}
	if _, err := client.UpdateClass(identityCtx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_BARBARIAN,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
							dnd5ev1alpha1.Skill_SKILL_INTIMIDATION,
						},
					},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-weapons-primary",
				OptionId: "barbarian-weapon-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-weapons-secondary",
				OptionId: "barbarian-secondary-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-pack",
				OptionId: "barbarian-pack-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
		},
	}); err != nil {
		return rpcError(barbarianIdentity, "UpdateClass", err)
	}
	if _, err := client.UpdateBackground(identityCtx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER,
		// Outlander's own real choice (rpg-toolkit#1554): one musical
		// instrument proficiency, no physical item.
		BackgroundChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND,
				ChoiceId: "outlander-instrument",
				Selection: &dnd5ev1alpha1.ChoiceData_Tools{Tools: &dnd5ev1alpha1.ToolSelection{
					Tools: []dnd5ev1alpha1.Tool{dnd5ev1alpha1.Tool_TOOL_LUTE},
				}},
			},
		},
	}); err != nil {
		return rpcError(barbarianIdentity, "UpdateBackground", err)
	}
	if _, err := client.UpdateAbilityScores(identityCtx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     15,
				Dexterity:    13,
				Constitution: 14,
				Intelligence: 8,
				Wisdom:       12,
				Charisma:     10,
			},
		},
	}); err != nil {
		return rpcError(barbarianIdentity, "UpdateAbilityScores", err)
	}
	if _, err := client.GetDraft(identityCtx, &dnd5ev1alpha1.GetDraftRequest{DraftId: draftID}); err != nil {
		return rpcError(barbarianIdentity, "GetDraft", err)
	}
	if _, err := client.FinalizeDraft(identityCtx, &dnd5ev1alpha1.FinalizeDraftRequest{DraftId: draftID}); err != nil {
		return rpcError(barbarianIdentity, "FinalizeDraft", err)
	}

	characterID, err := listExactlyOne(identityCtx, client, barbarianIdentity, barbarianName)
	if err != nil {
		return err
	}
	characterResponse, err := client.GetCharacter(identityCtx, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: characterID})
	if err != nil {
		return rpcError(barbarianIdentity, "GetCharacter", err)
	}
	if characterResponse.GetCharacter() == nil {
		return fmt.Errorf("%s GetCharacter: response character is empty", barbarianIdentity)
	}
	if _, finalListErr := listExactlyOne(identityCtx, client, barbarianIdentity, barbarianName); finalListErr != nil {
		return finalListErr
	}

	fmt.Printf("sandboxseed: identity=%s character_id=%s strength=%d off_hand=%s\n",
		barbarianIdentity,
		characterID,
		characterResponse.GetCharacter().GetAbilityScores().GetStrength(),
		characterResponse.GetCharacter().GetEquipmentSlots().GetOffHand().GetItemId(),
	)
	return nil
}

func authenticatedContext(ctx context.Context, identity string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Dev "+identity)
}

func deleteListedCharacters(ctx context.Context, client CharacterRPC, identity string) error {
	response, err := client.ListCharacters(ctx, &dnd5ev1alpha1.ListCharactersRequest{PageSize: listPageSize})
	if err != nil {
		return rpcError(identity, "ListCharacters", err)
	}
	if response.GetNextPageToken() != "" {
		return fmt.Errorf("%s ListCharacters: unexpected next page token", identity)
	}
	for _, character := range response.GetCharacters() {
		if _, deleteErr := client.DeleteCharacter(ctx, &dnd5ev1alpha1.DeleteCharacterRequest{CharacterId: character.GetId()}); deleteErr != nil {
			return rpcError(identity, "DeleteCharacter", deleteErr)
		}
	}
	return nil
}

func listExactlyOne(ctx context.Context, client CharacterRPC, identity, expectedName string) (string, error) {
	response, err := client.ListCharacters(ctx, &dnd5ev1alpha1.ListCharactersRequest{PageSize: listPageSize})
	if err != nil {
		return "", rpcError(identity, "ListCharacters", err)
	}
	if response.GetNextPageToken() != "" {
		return "", fmt.Errorf("%s ListCharacters: unexpected next page token", identity)
	}
	characters := response.GetCharacters()
	if len(characters) != 1 {
		return "", fmt.Errorf("%s ListCharacters: got %d characters, want exactly one", identity, len(characters))
	}
	if characters[0].GetName() != expectedName {
		return "", fmt.Errorf("%s ListCharacters: character name is %q, want %q", identity, characters[0].GetName(), expectedName)
	}
	if characters[0].GetId() == "" {
		return "", fmt.Errorf("%s ListCharacters: character ID is empty", identity)
	}
	return characters[0].GetId(), nil
}

func inventoryItemID(character *dnd5ev1alpha1.Character, expectedItemID string) (string, error) {
	if character == nil {
		return "", errors.New("response character is empty")
	}
	for _, item := range character.GetInventory() {
		if item.GetItemId() == expectedItemID {
			return item.GetItemId(), nil
		}
	}
	return "", fmt.Errorf("inventory item %q not found", expectedItemID)
}

func rpcError(identity, method string, err error) error {
	return fmt.Errorf("%s %s: %w", identity, method, err)
}

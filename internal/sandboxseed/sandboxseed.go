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

	fighterName   = "Toolkit Sandbox Fighter"
	barbarianName = "Toolkit Sandbox Barbarian"

	listPageSize = 100
	shieldItemID = "shield"
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
	return seedBarbarian(ctx, client)
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

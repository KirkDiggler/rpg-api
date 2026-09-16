// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package sandboxseed creates dev-only sandbox fixtures through the
// production CharacterService RPC surface, including the fixed toolkit
// contributors, the two level-up fixtures, and the repeatable weapon gallery
// character.
//
// Creation always goes through the RPCs, because character.Load reconstitutes
// a sheet from its stored feature BLOBS and never from the class tables: a
// hand-authored Data would produce a character that looks right in Redis and
// has no real features on it.
//
// EXPERIENCE IS THE ONE THING THIS TOOL WRITES DIRECTLY, and it writes it
// through the character repository rather than through a service call --
// because no service call can. Design R4.12: "There is no bypass and no RPC.
// Experience is read-only over the wire ... Seeding a character for a walk
// means writing experience on the persisted sheet through the fixture tool,
// the way every fixture is written -- not through the served API, which has no
// code path that writes it." That is why Seed needs a CharacterStore as well
// as a client, and why adding an AwardExperience RPC to make this easier would
// be the exact shortcut the design refuses (§8: "Not built, deliberately").
package sandboxseed

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

const (
	fighterIdentity   = "toolkit-sandbox-fighter"
	barbarianIdentity = "toolkit-sandbox-barbarian"
	bardIdentity      = "toolkit-sandbox-bard"

	fighterName   = "Toolkit Sandbox Fighter"
	barbarianName = "Toolkit Sandbox Barbarian"
	bardName      = "Toolkit Sandbox Bard"

	// The two level-up fixtures. The identity doubles as the walk's
	// ?playerId= override, so opening the environment at
	// ?playerId=level-up-fighter lands on the character the walk is about.
	//
	// They are separate identities rather than experience written onto the
	// three above, because a fixture that is ALWAYS one click from levelling
	// is the wrong default for every other walk: done-when 7 is "a freshly
	// created character shows 0 of 300 and no prompt -- the true state of a
	// game that awards no experience yet", and the sandbox fighter is that
	// character.
	levelUpFighterIdentity = "level-up-fighter"
	levelUpBardIdentity    = "level-up-bard"

	levelUpFighterName = "Arthur"
	levelUpBardName    = "Scanlan"

	listPageSize = 100
	shieldItemID = "shield"

	// The bard fixture's known spells, as the canonical refs the live
	// SpellSelection.spell_refs field takes.
	bladeWardRef      = "dnd5e:spells:blade-ward"
	viciousMockeryRef = "dnd5e:spells:vicious-mockery"
	// ALL FOUR, because the bard's level-1 pick takes the whole catalog
	// while the catalog is no larger than the class progression
	// (rpg-toolkit#1661). The requirement validates len(chosen) == Count
	// exactly, so this list is not a preference -- it is the whole of what
	// the pick allows, and a fixture carrying fewer would be refused at
	// finalize. Command is the arrival that took the count to four, which is
	// where the progression stops: from here the catalog outgrows the pick
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

// SeedInput carries the two capabilities the default fixture set needs.
type SeedInput struct {
	// Client creates every character through the production RPCs.
	Client CharacterRPC

	// Store writes experience onto an already-created sheet. REQUIRED: the
	// two level-up fixtures exist to be one level-up away, and the served API
	// has no code path that grants experience (R4.12). A nil store here would
	// mean two fixtures that look correct until someone clicks "level up" and
	// is told they have not earned it.
	Store CharacterStore
}

// Seed resets the fixed sandbox identities and recreates their characters
// through implemented CharacterService RPCs, then writes the level-up
// fixtures' experience through the repository.
func Seed(ctx context.Context, input *SeedInput) error {
	if input == nil {
		return errors.New("sandbox seed: input is required")
	}
	if input.Client == nil {
		return errors.New("sandbox seed: character RPC client is required")
	}
	if input.Store == nil {
		return errors.New("sandbox seed: character store is required to seed level-up experience")
	}

	if err := seedFighter(ctx, input.Client); err != nil {
		return err
	}
	if err := seedBarbarian(ctx, input.Client); err != nil {
		return err
	}
	if err := seedBard(ctx, input.Client); err != nil {
		return err
	}
	if err := seedLevelUpFighter(ctx, input); err != nil {
		return err
	}
	return seedLevelUpBard(ctx, input)
}

// levelUpExperience is the 2014 Character Advancement table's threshold for
// level 2 (PHB p.15), and the two numbers a correctly projected sheet reports
// alongside it.
//
// WRITTEN OUT, not read from the toolkit. These are the fixture's assertion
// that the projection works; deriving them from the same table the projection
// derives from would make the check unable to fail.
const (
	levelUpExperience         = 300
	levelUpEntitledLevel      = 2
	levelUpNextLevelThreshold = 900
)

// seedLevelUpFighter is done-when 6's character: "A fighter at 300 XP is
// offered a confirmation that names Action Surge, takes it, and comes out with
// Action Surge."
func seedLevelUpFighter(ctx context.Context, input *SeedInput) error {
	identityCtx := authenticatedContext(ctx, levelUpFighterIdentity)
	if err := deleteListedCharacters(identityCtx, input.Client, levelUpFighterIdentity); err != nil {
		return err
	}
	if err := createHumanFighter(identityCtx, &createHumanFighterInput{
		Client:   input.Client,
		Identity: levelUpFighterIdentity,
		Name:     levelUpFighterName,
	}); err != nil {
		return err
	}

	characterID, err := listExactlyOne(identityCtx, input.Client, levelUpFighterIdentity, levelUpFighterName)
	if err != nil {
		return err
	}
	return seedLevelUpExperience(identityCtx, input, levelUpFighterIdentity, characterID)
}

// seedLevelUpBard is done-when 5's character: the one class of the five whose
// level 2 asks a question, so it is the proof that the level-up screen renders
// a choice it has never heard of.
//
// It is created by the same function the sandbox bard uses, so the two cantrips
// and four spells are one description in one place: a second copy would be free
// to drift, and the level-2 spell choice is "five known minus four known" --
// an assertion about how many spells this character already has.
func seedLevelUpBard(ctx context.Context, input *SeedInput) error {
	identityCtx := authenticatedContext(ctx, levelUpBardIdentity)
	if err := deleteListedCharacters(identityCtx, input.Client, levelUpBardIdentity); err != nil {
		return err
	}
	if err := createBard(identityCtx, &createBardInput{
		Client:   input.Client,
		Identity: levelUpBardIdentity,
		Name:     levelUpBardName,
	}); err != nil {
		return err
	}

	characterID, err := listExactlyOne(identityCtx, input.Client, levelUpBardIdentity, levelUpBardName)
	if err != nil {
		return err
	}
	return seedLevelUpExperience(identityCtx, input, levelUpBardIdentity, characterID)
}

// seedLevelUpExperience writes the level-2 threshold onto a persisted sheet and
// then reads it back through GetCharacter.
//
// The read-back is the point. Writing a field into Redis proves nothing about
// whether the wire says so, and the three numbers the projection derives
// (R4.10) are exactly what the level-up prompt is built from -- a fixture that
// stored 300 and reported entitled_level 1 would look seeded and offer no
// level.
func seedLevelUpExperience(
	ctx context.Context,
	input *SeedInput,
	identity string,
	characterID string,
) error {
	stored, err := input.Store.Get(ctx, characterrepo.GetInput{ID: characterID})
	if err != nil {
		return fmt.Errorf("%s repository Get: %w", identity, err)
	}
	if stored == nil || stored.Character == nil || stored.Character.Data == nil {
		return fmt.Errorf("%s repository Get: no stored character data", identity)
	}

	stored.Character.Data.Experience = levelUpExperience
	if _, updateErr := input.Store.Update(ctx, characterrepo.UpdateInput{
		Character: stored.Character,
	}); updateErr != nil {
		return fmt.Errorf("%s repository Update: %w", identity, updateErr)
	}

	response, err := input.Client.GetCharacter(ctx, &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: characterID,
	})
	if err != nil {
		return rpcError(identity, "GetCharacter", err)
	}
	character := response.GetCharacter()
	if got := character.GetExperiencePoints(); got != levelUpExperience {
		return fmt.Errorf("%s GetCharacter: experience_points is %d, want %d",
			identity, got, levelUpExperience)
	}
	if got := character.GetEntitledLevel(); got != levelUpEntitledLevel {
		return fmt.Errorf("%s GetCharacter: entitled_level is %d, want %d",
			identity, got, levelUpEntitledLevel)
	}
	if got := character.GetNextLevelThreshold(); got != levelUpNextLevelThreshold {
		return fmt.Errorf("%s GetCharacter: next_level_threshold is %d, want %d",
			identity, got, levelUpNextLevelThreshold)
	}

	fmt.Printf("sandboxseed: identity=%s character_id=%s level=%d experience=%d entitled_level=%d next_level_threshold=%d\n",
		identity,
		characterID,
		character.GetLevel(),
		character.GetExperiencePoints(),
		character.GetEntitledLevel(),
		character.GetNextLevelThreshold(),
	)
	return nil
}

// seedBard resets the sandbox caster identity and recreates its character.
//
// The sandbox had a fighter and a barbarian and no caster at all, so every walk
// of spell work started by building a bard through the creation flow by hand.
// This is that character, made once through the same production RPCs the other
// two use; what it is made OF is [createBard].
//
// It holds NO experience. Done-when 7 is that "a freshly created character
// shows 0 of 300 and no prompt -- the true state of a game that awards no
// experience yet", and this is the fixture that shows it.
func seedBard(ctx context.Context, client CharacterRPC) error {
	identityCtx := authenticatedContext(ctx, bardIdentity)
	if err := deleteListedCharacters(identityCtx, client, bardIdentity); err != nil {
		return err
	}

	if err := createBard(identityCtx, &createBardInput{
		Client:   client,
		Identity: bardIdentity,
		Name:     bardName,
	}); err != nil {
		return err
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

// createBardInput names the identity and display name of a bard fixture.
type createBardInput struct {
	Client   CharacterRPC
	Identity string
	Name     string
}

// createBard builds the fixed caster fixture through the production creation
// RPCs: a level-one bard who already knows two castable cantrips and all four
// supported leveled spells.
//
// The cantrips are Blade Ward and Vicious Mockery deliberately: one self-target
// and one creature-target, so the two cast shapes are both reachable the moment
// the fixture loads. Charisma is 16 rather than the array's default so the spell
// save DC is a number worth reading rather than the minimum.
//
// Shared by the sandbox bard and the level-up bard. The four known spells are
// what makes bard level 2 a one-spell question -- "five known minus four
// known" -- so a second copy of this list free to drift would quietly change
// what the level-up screen asks.
func createBard(ctx context.Context, input *createBardInput) error {
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
					Languages: []dnd5ev1alpha1.Language{dnd5ev1alpha1.Language_LANGUAGE_ELVISH},
				},
			},
		}},
	}); err != nil {
		return rpcError(input.Identity, "UpdateRace", err)
	}
	if _, err := input.Client.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
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
		return rpcError(input.Identity, "UpdateClass", err)
	}
	if _, err := input.Client.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
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
		return rpcError(input.Identity, "UpdateBackground", err)
	}
	if _, err := input.Client.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
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

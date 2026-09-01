package sandboxseed

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

const (
	galleryIdentity      = "weapon-gallery"
	galleryCharacterName = "Weapon Gallery"
)

type galleryWeaponSpecification struct {
	ID       string
	Quantity int
}

var galleryWeaponSpecifications = [...]galleryWeaponSpecification{
	{ID: "shortbow", Quantity: 1},
	{ID: "longsword", Quantity: 1},
	{ID: "shortsword", Quantity: 1},
	{ID: "dagger", Quantity: 1},
	{ID: "greataxe", Quantity: 1},
	{ID: "quarterstaff", Quantity: 1},
	{ID: "greatsword", Quantity: 1},
	{ID: "battleaxe", Quantity: 1},
	{ID: "handaxe", Quantity: 1},
	{ID: "club", Quantity: 1},
	{ID: "greatclub", Quantity: 1},
	{ID: "warhammer", Quantity: 1},
	{ID: "light-crossbow", Quantity: 1},
	{ID: "longbow", Quantity: 1},
	{ID: "javelin", Quantity: 1},
	{ID: "rapier", Quantity: 1},
	{ID: "light-hammer", Quantity: 1},
	{ID: "mace", Quantity: 1},
	{ID: "sickle", Quantity: 1},
	{ID: "spear", Quantity: 1},
	{ID: "sling", Quantity: 1},
	{ID: "dart", Quantity: 1},
	{ID: "halberd", Quantity: 1},
	{ID: "maul", Quantity: 1},
	{ID: "morningstar", Quantity: 1},
	{ID: "pike", Quantity: 1},
	{ID: "war-pick", Quantity: 1},
	{ID: "glaive", Quantity: 1},
	{ID: "scimitar", Quantity: 2},
	{ID: "trident", Quantity: 1},
}

// CharacterStore is the narrow character repository surface used by the gallery fixture.
type CharacterStore interface {
	Get(context.Context, characterrepo.GetInput) (*characterrepo.GetOutput, error)
	Update(context.Context, characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error)
}

// SeedWeaponGalleryInput contains the dependencies for the weapon gallery fixture.
type SeedWeaponGalleryInput struct {
	Client CharacterRPC
	Store  CharacterStore
}

// SeedWeaponGalleryOutput reports the stable gallery character and weapon count.
type SeedWeaponGalleryOutput struct {
	CharacterID string
	WeaponCount int
}

// SeedWeaponGallery creates or reuses the dedicated gallery character and normalizes its weapons.
func SeedWeaponGallery(ctx context.Context, input *SeedWeaponGalleryInput) (*SeedWeaponGalleryOutput, error) {
	if input == nil {
		return nil, errors.New("weapon-gallery: input is required")
	}
	if input.Client == nil {
		return nil, errors.New("weapon-gallery: character RPC client is required")
	}
	if input.Store == nil {
		return nil, errors.New("weapon-gallery: character store is required")
	}

	identityCtx := authenticatedContext(ctx, galleryIdentity)
	characterID, err := galleryListedCharacterID(identityCtx, input.Client)
	if err != nil {
		return nil, err
	}
	if characterID == "" {
		if createErr := createHumanFighter(identityCtx, &createHumanFighterInput{
			Client:   input.Client,
			Identity: galleryIdentity,
			Name:     galleryCharacterName,
		}); createErr != nil {
			return nil, createErr
		}
		characterID, err = listExactlyOne(identityCtx, input.Client, galleryIdentity, galleryCharacterName)
		if err != nil {
			return nil, err
		}
	}

	getOutput, err := input.Store.Get(ctx, characterrepo.GetInput{ID: characterID})
	if err != nil {
		return nil, fmt.Errorf("%s Get: %w", galleryIdentity, err)
	}
	if getOutput == nil || getOutput.Character == nil || getOutput.Character.Data == nil {
		return nil, fmt.Errorf("%s Get: response character data is empty", galleryIdentity)
	}

	updated := cloneGalleryCharacter(getOutput.Character)
	updated.Data.Inventory = normalizeGalleryInventory(updated.Data.Inventory)
	if reflect.DeepEqual(getOutput.Character.Data.Inventory, updated.Data.Inventory) {
		return &SeedWeaponGalleryOutput{CharacterID: characterID, WeaponCount: len(galleryWeaponSpecifications)}, nil
	}
	if _, err := input.Store.Update(ctx, characterrepo.UpdateInput{Character: updated}); err != nil {
		return nil, fmt.Errorf("%s Update: %w", galleryIdentity, err)
	}
	return &SeedWeaponGalleryOutput{CharacterID: characterID, WeaponCount: len(galleryWeaponSpecifications)}, nil
}

func galleryListedCharacterID(ctx context.Context, client CharacterRPC) (string, error) {
	response, err := client.ListCharacters(ctx, &dnd5ev1alpha1.ListCharactersRequest{PageSize: listPageSize})
	if err != nil {
		return "", rpcError(galleryIdentity, "ListCharacters", err)
	}
	if response.GetNextPageToken() != "" {
		return "", fmt.Errorf("%s ListCharacters: unexpected next page token", galleryIdentity)
	}
	characters := response.GetCharacters()
	if len(characters) == 0 {
		return "", nil
	}
	if len(characters) != 1 {
		return "", fmt.Errorf("%s ListCharacters: got %d characters, want zero or exactly one", galleryIdentity, len(characters))
	}
	if characters[0].GetName() != galleryCharacterName {
		return "", fmt.Errorf("%s ListCharacters: character name is %q, want %q", galleryIdentity, characters[0].GetName(), galleryCharacterName)
	}
	if characters[0].GetId() == "" {
		return "", fmt.Errorf("%s ListCharacters: character ID is empty", galleryIdentity)
	}
	return characters[0].GetId(), nil
}

func normalizeGalleryInventory(inventory []tkcharacter.InventoryItemData) []tkcharacter.InventoryItemData {
	out := make([]tkcharacter.InventoryItemData, 0, len(inventory)+len(galleryWeaponSpecifications))
	insertedWeapons := false
	for _, item := range inventory {
		if item.Type == shared.EquipmentTypeWeapon {
			if !insertedWeapons {
				out = append(out, galleryWeaponInventory()...)
				insertedWeapons = true
			}
			continue
		}
		out = append(out, item)
	}
	if !insertedWeapons {
		out = append(out, galleryWeaponInventory()...)
	}
	return out
}

func galleryWeaponInventory() []tkcharacter.InventoryItemData {
	items := make([]tkcharacter.InventoryItemData, 0, len(galleryWeaponSpecifications))
	for _, specification := range &galleryWeaponSpecifications {
		items = append(items, tkcharacter.InventoryItemData{
			Type:     shared.EquipmentTypeWeapon,
			ID:       specification.ID,
			Quantity: specification.Quantity,
		})
	}
	return items
}

func cloneGalleryCharacter(in *entities.Character) *entities.Character {
	out := *in
	data := *in.Data
	data.Inventory = append([]tkcharacter.InventoryItemData(nil), in.Data.Inventory...)
	out.Data = &data
	out.Appearance = cloneAppearance(in.Appearance)
	return &out
}

func cloneAppearance(in *entities.Appearance) *entities.Appearance {
	if in == nil {
		return nil
	}
	out := *in
	if in.Hair == nil {
		return &out
	}

	hair := *in.Hair
	if in.Hair.Scalp != nil {
		scalp := *in.Hair.Scalp
		hair.Scalp = &scalp
	}
	if in.Hair.FacialHair != nil {
		facialHair := *in.Hair.FacialHair
		hair.FacialHair = &facialHair
	}
	if in.Hair.ColorSRGB != nil {
		color := *in.Hair.ColorSRGB
		hair.ColorSRGB = &color
	}
	if in.Hair.Roughness != nil {
		roughness := *in.Hair.Roughness
		hair.Roughness = &roughness
	}
	out.Hair = &hair
	return &out
}

type createHumanFighterInput struct {
	Client   CharacterRPC
	Identity string
	Name     string
}

func createHumanFighter(ctx context.Context, input *createHumanFighterInput) error {
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
	if _, err := input.Client.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_FIGHTER,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
							dnd5ev1alpha1.Skill_SKILL_PERCEPTION,
						},
					},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
					FightingStyle: &dnd5ev1alpha1.FightingStyleSelection{
						Style: dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_PROTECTION,
					},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-armor",
				OptionId: "fighter-armor-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-weapons-primary",
				OptionId: "fighter-weapon-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{
						Items: []*dnd5ev1alpha1.EquipmentSelectionItem{{
							Equipment: &dnd5ev1alpha1.EquipmentSelectionItem_Weapon{
								Weapon: dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD,
							},
						}},
					},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-weapons-secondary",
				OptionId: "fighter-ranged-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-pack",
				OptionId: "fighter-pack-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
		},
	}); err != nil {
		return rpcError(input.Identity, "UpdateClass", err)
	}
	if _, err := input.Client.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_SOLDIER,
	}); err != nil {
		return rpcError(input.Identity, "UpdateBackground", err)
	}
	if _, err := input.Client.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     15,
				Dexterity:    13,
				Constitution: 14,
				Intelligence: 10,
				Wisdom:       12,
				Charisma:     8,
			},
		},
	}); err != nil {
		return rpcError(input.Identity, "UpdateAbilityScores", err)
	}
	if _, err := input.Client.GetDraft(ctx, &dnd5ev1alpha1.GetDraftRequest{DraftId: draftID}); err != nil {
		return rpcError(input.Identity, "GetDraft", err)
	}
	if _, err := input.Client.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{DraftId: draftID}); err != nil {
		return rpcError(input.Identity, "FinalizeDraft", err)
	}
	return nil
}

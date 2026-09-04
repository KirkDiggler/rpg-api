package sandboxseed

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/languages"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/saves"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
)

func TestSeedWeaponGallery_CreatesMissingGalleryCharacterThenNormalizesRepositoryInventory(t *testing.T) {
	client := newGalleryFakeClient()
	client.listResponses = []*dnd5ev1alpha1.ListCharactersResponse{
		{},
		{Characters: []*dnd5ev1alpha1.Character{{Id: "gallery-id", Name: galleryCharacterName}}},
	}
	store := &galleryFakeStore{character: galleryCharacter("gallery-id", []tkcharacter.InventoryItemData{
		{Type: shared.EquipmentTypeItem, ID: "rope", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "club", Quantity: 2},
		{Type: shared.EquipmentTypeWeapon, ID: "obsolete-homebrew", Quantity: 1},
		{Type: shared.EquipmentTypePack, ID: "explorer-pack", Quantity: 1},
	})}

	out, err := SeedWeaponGallery(context.Background(), &SeedWeaponGalleryInput{Client: client, Store: store})

	require.NoError(t, err)
	require.Equal(t, &SeedWeaponGalleryOutput{CharacterID: "gallery-id", WeaponCount: 30}, out)
	require.Equal(t, []string{"ListCharacters", "CreateDraft", "UpdateName", "UpdateRace", "UpdateClass", "UpdateBackground", "UpdateAbilityScores", "GetDraft", "FinalizeDraft", "ListCharacters"}, client.calls)
	require.Empty(t, client.deletedIDs)
	require.Len(t, store.updates, 1)
	require.Equal(t, "gallery-id", store.getInputs[0].ID)
	assertGalleryInventory(t, store.updates[0].Character.Data.Inventory)
	require.Equal(t, []string{"rope", "explorer-pack"}, nonWeaponIDs(store.updates[0].Character.Data.Inventory))
	assertNormalHumanFighterCreationRequests(t, client, galleryCharacterName)
	assertGalleryRPCIdentity(t, client)
}

func TestGalleryWeaponInventory_MatchesCanonicalOrderAndQuantities(t *testing.T) {
	require.Equal(t, expectedGalleryWeaponInventory(), galleryWeaponInventory())
}

func TestSeedWeaponGallery_ReportsCanonicalThirtyWeaponCount(t *testing.T) {
	client := newGalleryFakeClient()
	client.listResponses = []*dnd5ev1alpha1.ListCharactersResponse{{
		Characters: []*dnd5ev1alpha1.Character{{Id: "gallery-id", Name: galleryCharacterName}},
	}}
	store := &galleryFakeStore{character: galleryCharacter("gallery-id", []tkcharacter.InventoryItemData{{Type: shared.EquipmentTypeItem, ID: "rope", Quantity: 1}})}

	out, err := SeedWeaponGallery(context.Background(), &SeedWeaponGalleryInput{Client: client, Store: store})

	require.NoError(t, err)
	require.Equal(t, 30, out.WeaponCount)
	require.Len(t, store.updates, 1)
	assertGalleryInventory(t, store.updates[0].Character.Data.Inventory)
}

func TestSeedWeaponGallery_MigratesExistingTwentySevenWeaponInventoryToThirty(t *testing.T) {
	client := newGalleryFakeClient()
	client.listResponses = []*dnd5ev1alpha1.ListCharactersResponse{{
		Characters: []*dnd5ev1alpha1.Character{{Id: "stable-id", Name: galleryCharacterName}},
	}}
	original := galleryCharacter("stable-id", legacyTwentySevenWeaponInventoryWithNonWeapons())
	store := &galleryFakeStore{character: cloneEntity(original)}
	expected := cloneEntity(original)
	expected.Data.Inventory = exactGalleryInventoryWithNonWeapons()

	out, err := SeedWeaponGallery(context.Background(), &SeedWeaponGalleryInput{Client: client, Store: store})

	require.NoError(t, err)
	require.Equal(t, &SeedWeaponGalleryOutput{CharacterID: "stable-id", WeaponCount: 30}, out)
	require.Equal(t, []string{"ListCharacters"}, client.calls)
	require.Empty(t, client.deletedIDs)
	require.Len(t, store.updates, 1)
	require.Equal(t, "stable-id", store.getInputs[0].ID)
	require.Equal(t, expected, store.updates[0].Character)
	assertGalleryRPCIdentity(t, client)
}

func TestSeedWeaponGallery_RepeatedRunPreservesStableCharacterAndSkipsExactInventoryUpdate(t *testing.T) {
	client := newGalleryFakeClient()
	client.listResponses = []*dnd5ev1alpha1.ListCharactersResponse{{
		Characters: []*dnd5ev1alpha1.Character{{Id: "stable-id", Name: galleryCharacterName}},
	}}
	store := &galleryFakeStore{character: galleryCharacter("stable-id", exactGalleryInventoryWithNonWeapons())}

	out, err := SeedWeaponGallery(context.Background(), &SeedWeaponGalleryInput{Client: client, Store: store})

	require.NoError(t, err)
	require.Equal(t, &SeedWeaponGalleryOutput{CharacterID: "stable-id", WeaponCount: 30}, out)
	require.Equal(t, []string{"ListCharacters"}, client.calls)
	require.Empty(t, client.deletedIDs)
	require.Empty(t, store.updates)
	assertGalleryRPCIdentity(t, client)
}

func TestSeedWeaponGallery_NormalizesMissingDuplicateAndExtraWeaponsWhilePreservingOnlyInventoryChanges(t *testing.T) {
	client := newGalleryFakeClient()
	client.listResponses = []*dnd5ev1alpha1.ListCharactersResponse{{
		Characters: []*dnd5ev1alpha1.Character{{Id: "gallery-id", Name: galleryCharacterName}},
	}}
	original := galleryCharacter("gallery-id", []tkcharacter.InventoryItemData{
		{Type: shared.EquipmentTypeItem, ID: "torch", Quantity: 5},
		{Type: shared.EquipmentTypeWeapon, ID: "dagger", Quantity: 7},
		{Type: shared.EquipmentTypeWeapon, ID: "dagger", Quantity: 3},
		{Type: shared.EquipmentTypeWeapon, ID: "glaive", Quantity: 9},
		{Type: shared.EquipmentTypeWeapon, ID: "scimitar", Quantity: 7},
		{Type: shared.EquipmentTypeWeapon, ID: "scimitar", Quantity: 3},
		{Type: shared.EquipmentTypeWeapon, ID: "trident", Quantity: 4},
		{Type: shared.EquipmentTypeAmmunition, ID: "arrows", Quantity: 20},
		{Type: shared.EquipmentTypeWeapon, ID: "homebrew-sword", Quantity: 1},
	})
	store := &galleryFakeStore{character: cloneEntity(original)}
	expected := cloneEntity(original)
	expected.Data.Inventory = []tkcharacter.InventoryItemData{
		{Type: shared.EquipmentTypeItem, ID: "torch", Quantity: 5},
		{Type: shared.EquipmentTypeWeapon, ID: "shortbow", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "longsword", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "shortsword", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "dagger", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "greataxe", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "quarterstaff", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "greatsword", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "battleaxe", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "handaxe", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "club", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "greatclub", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "warhammer", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "light-crossbow", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "longbow", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "javelin", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "rapier", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "light-hammer", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "mace", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "sickle", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "spear", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "sling", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "dart", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "halberd", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "maul", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "morningstar", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "pike", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "war-pick", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "glaive", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "scimitar", Quantity: 2},
		{Type: shared.EquipmentTypeWeapon, ID: "trident", Quantity: 1},
		{Type: shared.EquipmentTypeAmmunition, ID: "arrows", Quantity: 20},
	}

	out, err := SeedWeaponGallery(context.Background(), &SeedWeaponGalleryInput{Client: client, Store: store})

	require.NoError(t, err)
	require.Equal(t, "gallery-id", out.CharacterID)
	require.Len(t, store.updates, 1)
	updated := store.updates[0].Character
	assertGalleryInventory(t, updated.Data.Inventory)
	require.Equal(t, []string{"torch", "arrows"}, nonWeaponIDs(updated.Data.Inventory))
	require.Equal(t, expected, updated)
}

func TestSeedWeaponGallery_RejectsAmbiguousListedCharactersWithoutRepositoryWrites(t *testing.T) {
	tests := []struct {
		name       string
		characters []*dnd5ev1alpha1.Character
		wantErr    string
	}{
		{name: "wrong name", characters: []*dnd5ev1alpha1.Character{{Id: "id", Name: "Wrong"}}, wantErr: "character name"},
		{name: "empty id", characters: []*dnd5ev1alpha1.Character{{Name: galleryCharacterName}}, wantErr: "character ID is empty"},
		{name: "multiple", characters: []*dnd5ev1alpha1.Character{{Id: "one", Name: galleryCharacterName}, {Id: "two", Name: galleryCharacterName}}, wantErr: "got 2 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newGalleryFakeClient()
			client.listResponses = []*dnd5ev1alpha1.ListCharactersResponse{{Characters: tt.characters}}
			store := &galleryFakeStore{character: galleryCharacter("id", nil)}

			out, err := SeedWeaponGallery(context.Background(), &SeedWeaponGalleryInput{Client: client, Store: store})

			require.Nil(t, out)
			require.ErrorContains(t, err, tt.wantErr)
			require.Empty(t, store.getInputs)
			require.Empty(t, store.updates)
			require.Empty(t, client.deletedIDs)
		})
	}
}

func TestSeedWeaponGallery_WrapsRepositoryErrorsWithIdentityAndMethod(t *testing.T) {
	tests := []struct {
		name    string
		store   *galleryFakeStore
		wantErr string
	}{
		{name: "get", store: &galleryFakeStore{getErr: errors.New("redis get down")}, wantErr: "weapon-gallery Get: redis get down"},
		{name: "update", store: &galleryFakeStore{character: galleryCharacter("gallery-id", nil), updateErr: errors.New("redis update down")}, wantErr: "weapon-gallery Update: redis update down"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newGalleryFakeClient()
			client.listResponses = []*dnd5ev1alpha1.ListCharactersResponse{{
				Characters: []*dnd5ev1alpha1.Character{{Id: "gallery-id", Name: galleryCharacterName}},
			}}

			out, err := SeedWeaponGallery(context.Background(), &SeedWeaponGalleryInput{Client: client, Store: tt.store})

			require.Nil(t, out)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSeed_DefaultStillDeletesAndRecreatesToolkitFixtures(t *testing.T) {
	client := newGalleryFakeClient()
	client.listResponses = []*dnd5ev1alpha1.ListCharactersResponse{
		{Characters: []*dnd5ev1alpha1.Character{{Id: "old-fighter", Name: fighterName}}},
		{Characters: []*dnd5ev1alpha1.Character{{Id: "new-fighter", Name: fighterName}}},
		{Characters: []*dnd5ev1alpha1.Character{{Id: "new-fighter", Name: fighterName}}},
		{Characters: []*dnd5ev1alpha1.Character{{Id: "old-barbarian", Name: barbarianName}}},
		{Characters: []*dnd5ev1alpha1.Character{{Id: "new-barbarian", Name: barbarianName}}},
		{Characters: []*dnd5ev1alpha1.Character{{Id: "new-barbarian", Name: barbarianName}}},
	}

	err := Seed(context.Background(), client)

	require.NoError(t, err)
	require.Equal(t, []string{"old-fighter", "old-barbarian"}, client.deletedIDs)
	require.Equal(t, 2, client.createDrafts)
	require.Contains(t, client.authHeaders, "Dev "+fighterIdentity)
	require.Contains(t, client.authHeaders, "Dev "+barbarianIdentity)
}

func TestCloneGalleryCharacterPreservesToolkitAppearance(t *testing.T) {
	original := galleryCharacter("id", exactGalleryInventoryWithNonWeapons())
	clone := cloneGalleryCharacter(original)

	require.True(t, reflect.DeepEqual(original, clone))
	require.NotSame(t, original.Data.Appearance, clone.Data.Appearance)
	require.NotSame(t, original.Data.Appearance.Hair, clone.Data.Appearance.Hair)
	require.NotSame(t, original.Data.Appearance.Hair.Scalp, clone.Data.Appearance.Hair.Scalp)
	require.NotSame(t, original.Data.Appearance.Hair.FacialHair, clone.Data.Appearance.Hair.FacialHair)
	require.NotSame(t, original.Data.Appearance.Hair.ColorSRGB, clone.Data.Appearance.Hair.ColorSRGB)
	require.NotSame(t, original.Data.Appearance.Hair.Roughness, clone.Data.Appearance.Hair.Roughness)

	clone.Data.Appearance.Hair.Scalp.StyleRef = "changed:hair"
	clone.Data.Appearance.Hair.FacialHair.Kind = customization.StyleSelectionStyle
	clone.Data.Appearance.Hair.FacialHair.StyleRef = "changed:beard"
	*clone.Data.Appearance.Hair.ColorSRGB = 0xffffff
	*clone.Data.Appearance.Hair.Roughness = 1

	require.Equal(t, "modular-fantasy-hero:hair:38", original.Data.Appearance.Hair.Scalp.StyleRef)
	require.Equal(t, customization.StyleSelectionNone, original.Data.Appearance.Hair.FacialHair.Kind)
	require.Empty(t, original.Data.Appearance.Hair.FacialHair.StyleRef)
	require.Equal(t, uint32(0x123456), *original.Data.Appearance.Hair.ColorSRGB)
	require.InDelta(t, 0.33, *original.Data.Appearance.Hair.Roughness, 0.000001)
}

func TestCloneEntityTestHelperDeepCopiesRepresentativeState(t *testing.T) {
	original := galleryCharacter("id", exactGalleryInventoryWithNonWeapons())
	clone := cloneEntity(original)

	require.True(t, reflect.DeepEqual(original, clone))
	require.NotSame(t, original, clone)
	require.NotSame(t, original.Data, clone.Data)
	require.NotSame(t, original.Data.Appearance, clone.Data.Appearance)
	require.NotSame(t, original.Data.DeathSaveState, clone.Data.DeathSaveState)
	require.NotSame(t, original.Data.ActionEconomy, clone.Data.ActionEconomy)

	clone.Data.Inventory[0].ID = "rations"
	clone.Data.AbilityScores[abilities.STR] = 8
	clone.Data.Skills[skills.Athletics] = shared.Expertise
	clone.Data.SavingThrows[abilities.STR] = shared.NotProficient
	clone.Data.Languages[0] = languages.Orc
	clone.Data.ArmorProficiencies[0] = proficiencies.ArmorHeavy
	clone.Data.WeaponProficiencies[0] = proficiencies.WeaponSimple
	clone.Data.ToolProficiencies[0] = proficiencies.ToolThieves
	clone.Data.EquipmentSlots[tkcharacter.SlotOffHand] = "torch"
	clone.Data.SpellSlots[1] = tkcharacter.SpellSlotData{Max: 1, Used: 1}
	clone.Data.ClassResources[shared.ClassResourceSecondWind] = tkcharacter.ResourceData{Name: "Second Wind", Current: 0, Max: 1, Resets: shared.ResetTypeLongRest}
	clone.Data.Resources[coreResources.ResourceKey("hit-dice")] = tkcharacter.RecoverableResourceData{Current: 0, Maximum: 2, ResetType: coreResources.ResetShortRest}
	clone.Data.Features[0] = json.RawMessage(`{"id":"action-surge"}`)
	clone.Data.Conditions[0] = json.RawMessage(`{"id":"dodging"}`)
	clone.Data.ActionEconomy.TurnNumber = 9
	clone.Data.ActionEconomy.Granted[tkcharacter.GrantedAttacks] = 0
	clone.Data.DeathSaveState.Failures = 3
	clone.Data.Appearance.Hair.Scalp.StyleRef = "changed:hair"
	clone.Data.Appearance.Hair.FacialHair.Kind = customization.StyleSelectionStyle
	clone.Data.Appearance.Hair.FacialHair.StyleRef = "changed:beard"
	*clone.Data.Appearance.Hair.ColorSRGB = 0xffffff
	*clone.Data.Appearance.Hair.Roughness = 1

	require.Equal(t, "rope", original.Data.Inventory[0].ID)
	require.Equal(t, 15, original.Data.AbilityScores[abilities.STR])
	require.Equal(t, shared.Proficient, original.Data.Skills[skills.Athletics])
	require.Equal(t, shared.Proficient, original.Data.SavingThrows[abilities.STR])
	require.Equal(t, languages.Common, original.Data.Languages[0])
	require.Equal(t, proficiencies.ArmorLight, original.Data.ArmorProficiencies[0])
	require.Equal(t, proficiencies.WeaponSimple, original.Data.WeaponProficiencies[0])
	require.Equal(t, proficiencies.ToolSmith, original.Data.ToolProficiencies[0])
	require.Equal(t, "shield", original.Data.EquipmentSlots[tkcharacter.SlotOffHand])
	require.Equal(t, tkcharacter.SpellSlotData{Max: 2, Used: 1}, original.Data.SpellSlots[1])
	require.Equal(t, tkcharacter.ResourceData{Name: "Second Wind", Current: 1, Max: 1, Resets: shared.ResetTypeShortRest}, original.Data.ClassResources[shared.ClassResourceSecondWind])
	require.Equal(t, tkcharacter.RecoverableResourceData{Current: 1, Maximum: 2, ResetType: coreResources.ResetLongRest}, original.Data.Resources[coreResources.ResourceKey("hit-dice")])
	require.Equal(t, json.RawMessage(`{"id":"second-wind"}`), original.Data.Features[0])
	require.Equal(t, json.RawMessage(`{"id":"fighting-style-protection"}`), original.Data.Conditions[0])
	require.Equal(t, 3, original.Data.ActionEconomy.TurnNumber)
	require.Equal(t, 1, original.Data.ActionEconomy.Granted[tkcharacter.GrantedAttacks])
	require.Equal(t, 2, original.Data.DeathSaveState.Failures)
	require.Equal(t, "modular-fantasy-hero:hair:38", original.Data.Appearance.Hair.Scalp.StyleRef)
	require.Equal(t, customization.StyleSelectionNone, original.Data.Appearance.Hair.FacialHair.Kind)
	require.Empty(t, original.Data.Appearance.Hair.FacialHair.StyleRef)
	require.Equal(t, uint32(0x123456), *original.Data.Appearance.Hair.ColorSRGB)
	require.InDelta(t, 0.33, *original.Data.Appearance.Hair.Roughness, 0.000001)
}

func assertGalleryInventory(t *testing.T, inventory []tkcharacter.InventoryItemData) {
	t.Helper()
	weapons := make([]tkcharacter.InventoryItemData, 0, 30)
	for _, item := range inventory {
		if item.Type == shared.EquipmentTypeWeapon {
			weapons = append(weapons, item)
		}
	}
	require.Equal(t, expectedGalleryWeaponInventory(), weapons)
}

func expectedGalleryWeaponInventory() []tkcharacter.InventoryItemData {
	return []tkcharacter.InventoryItemData{
		{Type: shared.EquipmentTypeWeapon, ID: "shortbow", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "longsword", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "shortsword", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "dagger", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "greataxe", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "quarterstaff", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "greatsword", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "battleaxe", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "handaxe", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "club", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "greatclub", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "warhammer", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "light-crossbow", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "longbow", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "javelin", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "rapier", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "light-hammer", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "mace", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "sickle", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "spear", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "sling", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "dart", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "halberd", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "maul", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "morningstar", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "pike", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "war-pick", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "glaive", Quantity: 1},
		{Type: shared.EquipmentTypeWeapon, ID: "scimitar", Quantity: 2},
		{Type: shared.EquipmentTypeWeapon, ID: "trident", Quantity: 1},
	}
}

func assertNormalHumanFighterCreationRequests(t *testing.T, client *galleryFakeClient, name string) {
	t.Helper()
	require.Len(t, client.createDraftRequests, 1)
	requireProtoEqual(t, &dnd5ev1alpha1.CreateDraftRequest{}, client.createDraftRequests[0])
	require.Len(t, client.updateNameRequests, 1)
	requireProtoEqual(t, &dnd5ev1alpha1.UpdateNameRequest{DraftId: "draft-id", Name: name}, client.updateNameRequests[0])
	require.Len(t, client.updateRaceRequests, 1)
	requireProtoEqual(t, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: "draft-id",
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
	}, client.updateRaceRequests[0])
	require.Len(t, client.updateClassRequests, 1)
	requireProtoEqual(t, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: "draft-id",
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
					FightingStyle: &dnd5ev1alpha1.FightingStyleSelection{Style: dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_PROTECTION},
				},
			},
			{
				Category:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:    dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId:  "fighter-armor",
				OptionId:  "fighter-armor-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{}},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-weapons-primary",
				OptionId: "fighter-weapon-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{Items: []*dnd5ev1alpha1.EquipmentSelectionItem{{
						Equipment: &dnd5ev1alpha1.EquipmentSelectionItem_Weapon{Weapon: dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD},
					}}},
				},
			},
			{
				Category:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:    dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId:  "fighter-weapons-secondary",
				OptionId:  "fighter-ranged-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{}},
			},
			{
				Category:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:    dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId:  "fighter-pack",
				OptionId:  "fighter-pack-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{}},
			},
		},
	}, client.updateClassRequests[0])
	require.Len(t, client.updateBackgroundRequests, 1)
	requireProtoEqual(t, &dnd5ev1alpha1.UpdateBackgroundRequest{DraftId: "draft-id", Background: dnd5ev1alpha1.Background_BACKGROUND_SOLDIER}, client.updateBackgroundRequests[0])
	require.Len(t, client.updateAbilityScoreRequests, 1)
	requireProtoEqual(t, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: "draft-id",
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{AbilityScores: &dnd5ev1alpha1.AbilityScores{
			Strength:     15,
			Dexterity:    13,
			Constitution: 14,
			Intelligence: 10,
			Wisdom:       12,
			Charisma:     8,
		}},
	}, client.updateAbilityScoreRequests[0])
	require.Len(t, client.getDraftRequests, 1)
	requireProtoEqual(t, &dnd5ev1alpha1.GetDraftRequest{DraftId: "draft-id"}, client.getDraftRequests[0])
	require.Len(t, client.finalizeDraftRequests, 1)
	requireProtoEqual(t, &dnd5ev1alpha1.FinalizeDraftRequest{DraftId: "draft-id"}, client.finalizeDraftRequests[0])
}

func requireProtoEqual(t *testing.T, want proto.Message, got proto.Message) {
	t.Helper()
	require.True(t, proto.Equal(want, got), "want %v, got %v", want, got)
}

func exactGalleryInventoryWithNonWeapons() []tkcharacter.InventoryItemData {
	inventory := make([]tkcharacter.InventoryItemData, 0, 32)
	inventory = append(inventory, tkcharacter.InventoryItemData{Type: shared.EquipmentTypeItem, ID: "rope", Quantity: 1})
	inventory = append(inventory, expectedGalleryWeaponInventory()...)
	inventory = append(inventory, tkcharacter.InventoryItemData{Type: shared.EquipmentTypePack, ID: "explorer-pack", Quantity: 1})
	return inventory
}

func legacyTwentySevenWeaponInventoryWithNonWeapons() []tkcharacter.InventoryItemData {
	inventory := []tkcharacter.InventoryItemData{{Type: shared.EquipmentTypeItem, ID: "rope", Quantity: 1}}
	for _, id := range []string{
		"shortbow",
		"longsword",
		"shortsword",
		"dagger",
		"greataxe",
		"quarterstaff",
		"greatsword",
		"battleaxe",
		"handaxe",
		"club",
		"greatclub",
		"warhammer",
		"light-crossbow",
		"longbow",
		"javelin",
		"rapier",
		"light-hammer",
		"mace",
		"sickle",
		"spear",
		"sling",
		"dart",
		"halberd",
		"maul",
		"morningstar",
		"pike",
		"war-pick",
	} {
		inventory = append(inventory, tkcharacter.InventoryItemData{Type: shared.EquipmentTypeWeapon, ID: id, Quantity: 1})
	}
	return append(inventory, tkcharacter.InventoryItemData{Type: shared.EquipmentTypePack, ID: "explorer-pack", Quantity: 1})
}

func nonWeaponIDs(inventory []tkcharacter.InventoryItemData) []string {
	ids := make([]string, 0)
	for _, item := range inventory {
		if item.Type != shared.EquipmentTypeWeapon {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func galleryCharacter(id string, inventory []tkcharacter.InventoryItemData) *entities.Character {
	return &entities.Character{
		Data: &tkcharacter.Data{
			ID:               id,
			PlayerID:         galleryIdentity,
			Name:             galleryCharacterName,
			Level:            1,
			ProficiencyBonus: 2,
			RaceID:           races.Human,
			ClassID:          classes.Fighter,
			BackgroundID:     backgrounds.Soldier,
			AbilityScores: shared.AbilityScores{
				abilities.STR: 15,
				abilities.DEX: 13,
				abilities.CON: 14,
				abilities.INT: 10,
				abilities.WIS: 12,
				abilities.CHA: 8,
			},
			HitPoints:      9,
			MaxHitPoints:   11,
			ArmorClass:     18,
			DeathSaveState: &saves.DeathSaveState{Successes: 1, Failures: 2},
			Skills: map[skills.Skill]shared.ProficiencyLevel{
				skills.Athletics:  shared.Proficient,
				skills.Perception: shared.Proficient,
			},
			SavingThrows: map[abilities.Ability]shared.ProficiencyLevel{
				abilities.STR: shared.Proficient,
				abilities.CON: shared.Proficient,
			},
			Languages:           []languages.Language{languages.Common, languages.Dwarvish},
			ArmorProficiencies:  []proficiencies.Armor{proficiencies.ArmorLight, proficiencies.ArmorMedium, proficiencies.ArmorShields},
			WeaponProficiencies: []proficiencies.Weapon{proficiencies.WeaponSimple, proficiencies.WeaponMartial},
			ToolProficiencies:   []proficiencies.Tool{proficiencies.ToolSmith, proficiencies.ToolDiceSet},
			Inventory:           append([]tkcharacter.InventoryItemData(nil), inventory...),
			EquipmentSlots: tkcharacter.EquipmentSlots{
				tkcharacter.SlotMainHand: "longsword",
				tkcharacter.SlotOffHand:  "shield",
				tkcharacter.SlotArmor:    "chain-mail",
			},
			SpellSlots: map[int]tkcharacter.SpellSlotData{1: {Max: 2, Used: 1}},
			ClassResources: map[shared.ClassResourceType]tkcharacter.ResourceData{
				shared.ClassResourceSecondWind: {Name: "Second Wind", Current: 1, Max: 1, Resets: shared.ResetTypeShortRest},
			},
			Resources: map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData{
				coreResources.ResourceKey("hit-dice"): {Current: 1, Maximum: 2, ResetType: coreResources.ResetLongRest},
			},
			Features: []json.RawMessage{json.RawMessage(`{"id":"second-wind"}`)},
			Conditions: []json.RawMessage{
				json.RawMessage(`{"id":"fighting-style-protection"}`),
			},
			ActionEconomy: &tkcharacter.ActionEconomyData{
				TurnNumber:            3,
				ActionsRemaining:      1,
				BonusActionsRemaining: 0,
				ReactionsRemaining:    1,
				MovementRemaining:     15,
				Granted: map[tkcharacter.GrantedActionKey]int{
					tkcharacter.GrantedAttacks:        1,
					tkcharacter.GrantedOffHandStrikes: 1,
				},
			},
			CreatedAt:  time.Unix(1, 0),
			UpdatedAt:  time.Unix(2, 0),
			Appearance: galleryAppearance(),
		},
	}
}

func galleryAppearance() *customization.Appearance {
	color := uint32(0x123456)
	roughness := float32(0.33)
	return &customization.Appearance{Hair: &customization.HairCustomization{
		Scalp:      &customization.StyleSelection{Kind: customization.StyleSelectionStyle, StyleRef: "modular-fantasy-hero:hair:38"},
		FacialHair: &customization.StyleSelection{Kind: customization.StyleSelectionNone},
		ColorSRGB:  &color,
		Roughness:  &roughness,
	}}
}

func cloneEntity(in *entities.Character) *entities.Character {
	if in == nil {
		return nil
	}
	out := &entities.Character{}
	if in.Data != nil {
		data := *in.Data
		if in.Data.AbilityScores != nil {
			data.AbilityScores = make(shared.AbilityScores, len(in.Data.AbilityScores))
			for k, v := range in.Data.AbilityScores {
				data.AbilityScores[k] = v
			}
		}
		if in.Data.DeathSaveState != nil {
			deathSaves := *in.Data.DeathSaveState
			data.DeathSaveState = &deathSaves
		}
		if in.Data.Skills != nil {
			data.Skills = make(map[skills.Skill]shared.ProficiencyLevel, len(in.Data.Skills))
			for k, v := range in.Data.Skills {
				data.Skills[k] = v
			}
		}
		if in.Data.SavingThrows != nil {
			data.SavingThrows = make(map[abilities.Ability]shared.ProficiencyLevel, len(in.Data.SavingThrows))
			for k, v := range in.Data.SavingThrows {
				data.SavingThrows[k] = v
			}
		}
		data.Languages = append([]languages.Language(nil), in.Data.Languages...)
		data.ArmorProficiencies = append([]proficiencies.Armor(nil), in.Data.ArmorProficiencies...)
		data.WeaponProficiencies = append([]proficiencies.Weapon(nil), in.Data.WeaponProficiencies...)
		data.ToolProficiencies = append([]proficiencies.Tool(nil), in.Data.ToolProficiencies...)
		data.Inventory = append([]tkcharacter.InventoryItemData(nil), in.Data.Inventory...)
		if in.Data.EquipmentSlots != nil {
			data.EquipmentSlots = make(tkcharacter.EquipmentSlots, len(in.Data.EquipmentSlots))
			for k, v := range in.Data.EquipmentSlots {
				data.EquipmentSlots[k] = v
			}
		}
		if in.Data.SpellSlots != nil {
			data.SpellSlots = make(map[int]tkcharacter.SpellSlotData, len(in.Data.SpellSlots))
			for k, v := range in.Data.SpellSlots {
				data.SpellSlots[k] = v
			}
		}
		if in.Data.ClassResources != nil {
			data.ClassResources = make(map[shared.ClassResourceType]tkcharacter.ResourceData, len(in.Data.ClassResources))
			for k, v := range in.Data.ClassResources {
				data.ClassResources[k] = v
			}
		}
		if in.Data.Resources != nil {
			data.Resources = make(map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData, len(in.Data.Resources))
			for k, v := range in.Data.Resources {
				data.Resources[k] = v
			}
		}
		if in.Data.Features != nil {
			data.Features = make([]json.RawMessage, len(in.Data.Features))
			for i, feature := range in.Data.Features {
				data.Features[i] = append(json.RawMessage(nil), feature...)
			}
		}
		if in.Data.Conditions != nil {
			data.Conditions = make([]json.RawMessage, len(in.Data.Conditions))
			for i, condition := range in.Data.Conditions {
				data.Conditions[i] = append(json.RawMessage(nil), condition...)
			}
		}
		if in.Data.ActionEconomy != nil {
			actionEconomy := *in.Data.ActionEconomy
			if in.Data.ActionEconomy.Granted != nil {
				actionEconomy.Granted = make(map[tkcharacter.GrantedActionKey]int, len(in.Data.ActionEconomy.Granted))
				for k, v := range in.Data.ActionEconomy.Granted {
					actionEconomy.Granted[k] = v
				}
			}
			data.ActionEconomy = &actionEconomy
		}
		data.Appearance = customization.CloneAppearance(in.Data.Appearance)
		out.Data = &data
	}
	return out
}

func assertGalleryRPCIdentity(t *testing.T, client *galleryFakeClient) {
	t.Helper()
	for _, auth := range client.authHeaders {
		require.Equal(t, "Dev "+galleryIdentity, auth)
	}
}

type galleryFakeStore struct {
	character *entities.Character
	getErr    error
	updateErr error
	getInputs []characterrepo.GetInput
	updates   []characterrepo.UpdateInput
}

func (s *galleryFakeStore) Get(_ context.Context, input characterrepo.GetInput) (*characterrepo.GetOutput, error) {
	s.getInputs = append(s.getInputs, input)
	if s.getErr != nil {
		return nil, s.getErr
	}
	return &characterrepo.GetOutput{Character: cloneEntity(s.character), Version: "version"}, nil
}

func (s *galleryFakeStore) Update(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
	s.updates = append(s.updates, characterrepo.UpdateInput{Character: cloneEntity(input.Character)})
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	s.character = cloneEntity(input.Character)
	return &characterrepo.UpdateOutput{Character: cloneEntity(input.Character)}, nil
}

type galleryFakeClient struct {
	calls                      []string
	authHeaders                []string
	listResponses              []*dnd5ev1alpha1.ListCharactersResponse
	deletedIDs                 []string
	createDrafts               int
	createDraftRequests        []*dnd5ev1alpha1.CreateDraftRequest
	updateNameRequests         []*dnd5ev1alpha1.UpdateNameRequest
	updateRaceRequests         []*dnd5ev1alpha1.UpdateRaceRequest
	updateClassRequests        []*dnd5ev1alpha1.UpdateClassRequest
	updateBackgroundRequests   []*dnd5ev1alpha1.UpdateBackgroundRequest
	updateAbilityScoreRequests []*dnd5ev1alpha1.UpdateAbilityScoresRequest
	getDraftRequests           []*dnd5ev1alpha1.GetDraftRequest
	finalizeDraftRequests      []*dnd5ev1alpha1.FinalizeDraftRequest
}

func newGalleryFakeClient() *galleryFakeClient { return &galleryFakeClient{} }

func (c *galleryFakeClient) record(ctx context.Context, method string) {
	c.calls = append(c.calls, method)
	md, _ := metadata.FromOutgoingContext(ctx)
	c.authHeaders = append(c.authHeaders, first(md.Get("authorization")))
}

func (c *galleryFakeClient) ListCharacters(ctx context.Context, _ *dnd5ev1alpha1.ListCharactersRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.ListCharactersResponse, error) {
	c.record(ctx, "ListCharacters")
	if len(c.listResponses) == 0 {
		return &dnd5ev1alpha1.ListCharactersResponse{}, nil
	}
	response := c.listResponses[0]
	c.listResponses = c.listResponses[1:]
	return response, nil
}

func (c *galleryFakeClient) DeleteCharacter(ctx context.Context, request *dnd5ev1alpha1.DeleteCharacterRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.DeleteCharacterResponse, error) {
	c.record(ctx, "DeleteCharacter")
	c.deletedIDs = append(c.deletedIDs, request.GetCharacterId())
	return &dnd5ev1alpha1.DeleteCharacterResponse{}, nil
}

func (c *galleryFakeClient) CreateDraft(ctx context.Context, request *dnd5ev1alpha1.CreateDraftRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.CreateDraftResponse, error) {
	c.record(ctx, "CreateDraft")
	c.createDrafts++
	c.createDraftRequests = append(c.createDraftRequests, proto.Clone(request).(*dnd5ev1alpha1.CreateDraftRequest))
	return &dnd5ev1alpha1.CreateDraftResponse{Draft: &dnd5ev1alpha1.CharacterDraft{Id: "draft-id"}}, nil
}

func (c *galleryFakeClient) UpdateName(ctx context.Context, request *dnd5ev1alpha1.UpdateNameRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.UpdateNameResponse, error) {
	c.record(ctx, "UpdateName")
	c.updateNameRequests = append(c.updateNameRequests, proto.Clone(request).(*dnd5ev1alpha1.UpdateNameRequest))
	return &dnd5ev1alpha1.UpdateNameResponse{}, nil
}

func (c *galleryFakeClient) UpdateRace(ctx context.Context, request *dnd5ev1alpha1.UpdateRaceRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.UpdateRaceResponse, error) {
	c.record(ctx, "UpdateRace")
	c.updateRaceRequests = append(c.updateRaceRequests, proto.Clone(request).(*dnd5ev1alpha1.UpdateRaceRequest))
	return &dnd5ev1alpha1.UpdateRaceResponse{}, nil
}

func (c *galleryFakeClient) UpdateClass(ctx context.Context, request *dnd5ev1alpha1.UpdateClassRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.UpdateClassResponse, error) {
	c.record(ctx, "UpdateClass")
	c.updateClassRequests = append(c.updateClassRequests, proto.Clone(request).(*dnd5ev1alpha1.UpdateClassRequest))
	return &dnd5ev1alpha1.UpdateClassResponse{}, nil
}

func (c *galleryFakeClient) UpdateBackground(ctx context.Context, request *dnd5ev1alpha1.UpdateBackgroundRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.UpdateBackgroundResponse, error) {
	c.record(ctx, "UpdateBackground")
	c.updateBackgroundRequests = append(c.updateBackgroundRequests, proto.Clone(request).(*dnd5ev1alpha1.UpdateBackgroundRequest))
	return &dnd5ev1alpha1.UpdateBackgroundResponse{}, nil
}

func (c *galleryFakeClient) UpdateAbilityScores(ctx context.Context, request *dnd5ev1alpha1.UpdateAbilityScoresRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.UpdateAbilityScoresResponse, error) {
	c.record(ctx, "UpdateAbilityScores")
	c.updateAbilityScoreRequests = append(c.updateAbilityScoreRequests, proto.Clone(request).(*dnd5ev1alpha1.UpdateAbilityScoresRequest))
	return &dnd5ev1alpha1.UpdateAbilityScoresResponse{}, nil
}

func (c *galleryFakeClient) GetDraft(ctx context.Context, request *dnd5ev1alpha1.GetDraftRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.GetDraftResponse, error) {
	c.record(ctx, "GetDraft")
	c.getDraftRequests = append(c.getDraftRequests, proto.Clone(request).(*dnd5ev1alpha1.GetDraftRequest))
	return &dnd5ev1alpha1.GetDraftResponse{}, nil
}

func (c *galleryFakeClient) FinalizeDraft(ctx context.Context, request *dnd5ev1alpha1.FinalizeDraftRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.FinalizeDraftResponse, error) {
	c.record(ctx, "FinalizeDraft")
	c.finalizeDraftRequests = append(c.finalizeDraftRequests, proto.Clone(request).(*dnd5ev1alpha1.FinalizeDraftRequest))
	return &dnd5ev1alpha1.FinalizeDraftResponse{}, nil
}

func (c *galleryFakeClient) GetCharacter(ctx context.Context, request *dnd5ev1alpha1.GetCharacterRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.GetCharacterResponse, error) {
	c.record(ctx, "GetCharacter")
	character := &dnd5ev1alpha1.Character{Id: request.GetCharacterId(), AbilityScores: &dnd5ev1alpha1.AbilityScores{Strength: 15}, EquipmentSlots: &dnd5ev1alpha1.EquipmentSlots{}, Inventory: []*dnd5ev1alpha1.InventoryItem{{ItemId: shieldItemID}}}
	return &dnd5ev1alpha1.GetCharacterResponse{Character: character}, nil
}

func (c *galleryFakeClient) EquipItem(ctx context.Context, request *dnd5ev1alpha1.EquipItemRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.EquipItemResponse, error) {
	c.record(ctx, "EquipItem")
	character := &dnd5ev1alpha1.Character{Id: request.GetCharacterId(), AbilityScores: &dnd5ev1alpha1.AbilityScores{Strength: 15}, EquipmentSlots: &dnd5ev1alpha1.EquipmentSlots{OffHand: &dnd5ev1alpha1.InventoryItem{ItemId: shieldItemID}}}
	return &dnd5ev1alpha1.EquipItemResponse{Character: character}, nil
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

var _ CharacterRPC = (*galleryFakeClient)(nil)

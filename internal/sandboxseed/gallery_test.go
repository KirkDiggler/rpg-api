package sandboxseed

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
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
	require.Equal(t, &SeedWeaponGalleryOutput{CharacterID: "gallery-id", WeaponCount: 22}, out)
	require.Equal(t, []string{"ListCharacters", "CreateDraft", "UpdateName", "UpdateRace", "UpdateClass", "UpdateBackground", "UpdateAbilityScores", "GetDraft", "FinalizeDraft", "ListCharacters"}, client.calls)
	require.Empty(t, client.deletedIDs)
	require.Len(t, store.updates, 1)
	require.Equal(t, "gallery-id", store.getInputs[0].ID)
	assertGalleryInventory(t, store.updates[0].Character.Data.Inventory)
	require.Equal(t, []string{"rope", "explorer-pack"}, nonWeaponIDs(store.updates[0].Character.Data.Inventory))
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
	require.Equal(t, &SeedWeaponGalleryOutput{CharacterID: "stable-id", WeaponCount: 22}, out)
	require.Equal(t, []string{"ListCharacters"}, client.calls)
	require.Empty(t, client.deletedIDs)
	require.Empty(t, store.updates)
	assertGalleryRPCIdentity(t, client)
}

func TestSeedWeaponGallery_NormalizesMissingDuplicateAndExtraWeaponsWhilePreservingOtherState(t *testing.T) {
	client := newGalleryFakeClient()
	client.listResponses = []*dnd5ev1alpha1.ListCharactersResponse{{
		Characters: []*dnd5ev1alpha1.Character{{Id: "gallery-id", Name: galleryCharacterName}},
	}}
	original := galleryCharacter("gallery-id", []tkcharacter.InventoryItemData{
		{Type: shared.EquipmentTypeItem, ID: "torch", Quantity: 5},
		{Type: shared.EquipmentTypeWeapon, ID: "dagger", Quantity: 7},
		{Type: shared.EquipmentTypeWeapon, ID: "dagger", Quantity: 3},
		{Type: shared.EquipmentTypeAmmunition, ID: "arrows", Quantity: 20},
		{Type: shared.EquipmentTypeWeapon, ID: "homebrew-sword", Quantity: 1},
	})
	original.Data.EquipmentSlots = tkcharacter.EquipmentSlots{"off_hand": "shield"}
	original.Data.AbilityScores = shared.AbilityScores{abilities.STR: 17, abilities.DEX: 9}
	original.Data.HitPoints = 3
	original.Data.MaxHitPoints = 11
	original.Appearance = &entities.Appearance{SkinTone: "#111111", PrimaryColor: "#222222", SecondaryColor: "#333333", EyeColor: "#444444"}
	store := &galleryFakeStore{character: cloneEntity(original)}

	out, err := SeedWeaponGallery(context.Background(), &SeedWeaponGalleryInput{Client: client, Store: store})

	require.NoError(t, err)
	require.Equal(t, "gallery-id", out.CharacterID)
	require.Len(t, store.updates, 1)
	updated := store.updates[0].Character
	assertGalleryInventory(t, updated.Data.Inventory)
	require.Equal(t, []string{"torch", "arrows"}, nonWeaponIDs(updated.Data.Inventory))
	require.Equal(t, original.Data.ID, updated.Data.ID)
	require.Equal(t, original.Data.PlayerID, updated.Data.PlayerID)
	require.Equal(t, original.Data.Name, updated.Data.Name)
	require.Equal(t, original.Data.EquipmentSlots, updated.Data.EquipmentSlots)
	require.Equal(t, original.Data.AbilityScores, updated.Data.AbilityScores)
	require.Equal(t, original.Data.HitPoints, updated.Data.HitPoints)
	require.Equal(t, original.Data.MaxHitPoints, updated.Data.MaxHitPoints)
	require.Equal(t, original.Appearance, updated.Appearance)
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

func assertGalleryInventory(t *testing.T, inventory []tkcharacter.InventoryItemData) {
	t.Helper()
	weapons := make([]string, 0, 22)
	seen := map[string]bool{}
	for _, item := range inventory {
		if item.Type != shared.EquipmentTypeWeapon {
			continue
		}
		weapons = append(weapons, item.ID)
		require.Equal(t, 1, item.Quantity, item.ID)
		require.False(t, seen[item.ID], "duplicate weapon %s", item.ID)
		seen[item.ID] = true
	}
	require.Equal(t, []string{
		"shortbow", "longsword", "shortsword", "dagger", "greataxe", "quarterstaff", "greatsword", "battleaxe", "handaxe", "club", "greatclub", "warhammer", "light-crossbow", "longbow", "javelin", "rapier", "light-hammer", "mace", "sickle", "spear", "sling", "dart",
	}, weapons)
}

func exactGalleryInventoryWithNonWeapons() []tkcharacter.InventoryItemData {
	inventory := make([]tkcharacter.InventoryItemData, 0, 24)
	inventory = append(inventory, tkcharacter.InventoryItemData{Type: shared.EquipmentTypeItem, ID: "rope", Quantity: 1})
	for _, id := range []string{"shortbow", "longsword", "shortsword", "dagger", "greataxe", "quarterstaff", "greatsword", "battleaxe", "handaxe", "club", "greatclub", "warhammer", "light-crossbow", "longbow", "javelin", "rapier", "light-hammer", "mace", "sickle", "spear", "sling", "dart"} {
		inventory = append(inventory, tkcharacter.InventoryItemData{Type: shared.EquipmentTypeWeapon, ID: id, Quantity: 1})
	}
	inventory = append(inventory, tkcharacter.InventoryItemData{Type: shared.EquipmentTypePack, ID: "explorer-pack", Quantity: 1})
	return inventory
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
			ID:        id,
			PlayerID:  galleryIdentity,
			Name:      galleryCharacterName,
			Level:     1,
			RaceID:    races.Human,
			ClassID:   classes.Fighter,
			Inventory: append([]tkcharacter.InventoryItemData(nil), inventory...),
			CreatedAt: time.Unix(1, 0),
			UpdatedAt: time.Unix(2, 0),
		},
	}
}

func cloneEntity(in *entities.Character) *entities.Character {
	out := *in
	data := *in.Data
	data.Inventory = append([]tkcharacter.InventoryItemData(nil), in.Data.Inventory...)
	if in.Data.EquipmentSlots != nil {
		data.EquipmentSlots = make(tkcharacter.EquipmentSlots, len(in.Data.EquipmentSlots))
		for k, v := range in.Data.EquipmentSlots {
			data.EquipmentSlots[k] = v
		}
	}
	out.Data = &data
	if in.Appearance != nil {
		appearance := *in.Appearance
		out.Appearance = &appearance
	}
	return &out
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
	calls         []string
	authHeaders   []string
	listResponses []*dnd5ev1alpha1.ListCharactersResponse
	deletedIDs    []string
	createDrafts  int
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

func (c *galleryFakeClient) CreateDraft(ctx context.Context, _ *dnd5ev1alpha1.CreateDraftRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.CreateDraftResponse, error) {
	c.record(ctx, "CreateDraft")
	c.createDrafts++
	return &dnd5ev1alpha1.CreateDraftResponse{Draft: &dnd5ev1alpha1.CharacterDraft{Id: "draft-id"}}, nil
}

func (c *galleryFakeClient) UpdateName(ctx context.Context, _ *dnd5ev1alpha1.UpdateNameRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.UpdateNameResponse, error) {
	c.record(ctx, "UpdateName")
	return &dnd5ev1alpha1.UpdateNameResponse{}, nil
}

func (c *galleryFakeClient) UpdateRace(ctx context.Context, _ *dnd5ev1alpha1.UpdateRaceRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.UpdateRaceResponse, error) {
	c.record(ctx, "UpdateRace")
	return &dnd5ev1alpha1.UpdateRaceResponse{}, nil
}

func (c *galleryFakeClient) UpdateClass(ctx context.Context, _ *dnd5ev1alpha1.UpdateClassRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.UpdateClassResponse, error) {
	c.record(ctx, "UpdateClass")
	return &dnd5ev1alpha1.UpdateClassResponse{}, nil
}

func (c *galleryFakeClient) UpdateBackground(ctx context.Context, _ *dnd5ev1alpha1.UpdateBackgroundRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.UpdateBackgroundResponse, error) {
	c.record(ctx, "UpdateBackground")
	return &dnd5ev1alpha1.UpdateBackgroundResponse{}, nil
}

func (c *galleryFakeClient) UpdateAbilityScores(ctx context.Context, _ *dnd5ev1alpha1.UpdateAbilityScoresRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.UpdateAbilityScoresResponse, error) {
	c.record(ctx, "UpdateAbilityScores")
	return &dnd5ev1alpha1.UpdateAbilityScoresResponse{}, nil
}

func (c *galleryFakeClient) GetDraft(ctx context.Context, _ *dnd5ev1alpha1.GetDraftRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.GetDraftResponse, error) {
	c.record(ctx, "GetDraft")
	return &dnd5ev1alpha1.GetDraftResponse{}, nil
}

func (c *galleryFakeClient) FinalizeDraft(ctx context.Context, _ *dnd5ev1alpha1.FinalizeDraftRequest, _ ...grpc.CallOption) (*dnd5ev1alpha1.FinalizeDraftResponse, error) {
	c.record(ctx, "FinalizeDraft")
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

func TestCloneEntityTestHelperPreservesShape(t *testing.T) {
	original := galleryCharacter("id", exactGalleryInventoryWithNonWeapons())
	clone := cloneEntity(original)
	require.True(t, reflect.DeepEqual(original, clone))
	require.NotSame(t, original, clone)
	require.NotSame(t, original.Data, clone.Data)
}

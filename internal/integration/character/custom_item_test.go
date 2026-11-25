package characterintegration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	characterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/testsupport/levelfixture"
	"github.com/KirkDiggler/rpg-toolkit/core"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/magicitems"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// TestEquippingACustomItemThroughTheV2Handler pins rpg-project#850: the item's
// id goes straight to the session's equip verb, with no catalogue check on
// the way. Worn in a slot it names it is answered, and the sheet carries it at
// its own ref; named for a slot it does not fit the verb's own refusal comes
// back, not a catalogue error.
func TestEquippingACustomItemThroughTheV2Handler(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	release := sharedRedis.Lease()
	defer release()

	server, err := harness.NewWithRedis(ctx, nil, sharedRedis.Addr)
	require.NoError(t, err)
	defer server.Close()
	require.NoError(t, server.FlushRedis(ctx))

	const (
		owner  = "cloak-owner"
		charID = "char-cloak-fighter"
		itemID = "lieutenants-cloak"
	)
	cloak, err := magicitems.New(itemID, magicitems.Description{
		Ref:   core.Ref{Module: "site", Type: "items", ID: itemID},
		Name:  "Lieutenant's cloak",
		Slots: []string{"cloak"},
		Pools: map[string]magicitems.Pool{
			"charges": {Maximum: 1, RestoreOn: []coreResources.ResetType{coreResources.ResetShortRest, coreResources.ResetLongRest}},
		},
		Grants: map[string]magicitems.Grant{
			"second-wind": {Ability: *refs.Features.SecondWind(), Parameters: json.RawMessage(`{"level":1}`), Pool: "charges", Uses: 1},
		},
	})
	require.NoError(t, err)
	data := &tkcharacter.Data{
		ID: charID, PlayerID: owner, Name: "Cloaked", Level: 1, Levels: levelfixture.Synthetic(classes.Fighter, 1),
		ClassID: classes.Fighter, RaceID: races.Human, ProficiencyBonus: 2, HitPoints: 12, MaxHitPoints: 12,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, abilities.DEX: 12, abilities.CON: 14, abilities.INT: 10, abilities.WIS: 10, abilities.CHA: 10,
		},
		Inventory:      []tkcharacter.InventoryItemData{{Type: shared.EquipmentTypeWeapon, ID: "longsword", Quantity: 1}},
		EquipmentSlots: tkcharacter.EquipmentSlots{},
	}
	require.NoError(t, tkcharacter.AddItem(data, cloak))
	_, err = server.CharacterRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: data}})
	require.NoError(t, err)

	callerCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Dev "+owner)
	equip := func(slot string) (*characterv2pb.EquipItemResponse, error) {
		return server.CharacterClientV2.EquipItem(callerCtx, &characterv2pb.EquipItemRequest{
			CharacterId: charID,
			Item:        &encounterv2pb.Ref{Module: "site", Type: "items", Id: itemID},
			SlotKey:     slot,
		})
	}

	_, err = equip("ring_left")
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err), "%v", err)
	require.NotContains(t, status.Convert(err).Message(), "not found", "the equip verb's refusal, not a catalogue error")

	resp, err := equip("cloak")
	require.NoError(t, err)
	cd := resp.GetCharacter()
	require.Contains(t, cd.GetEquipped(), "cloak")
	require.Equal(t, &encounterv2pb.Ref{Module: "site", Type: "items", Id: itemID}, cd.GetEquipped()["cloak"])
	var row *encounterv2pb.Item
	for _, item := range cd.GetInventory() {
		if item.GetRef().GetId() == itemID {
			row = item
		}
	}
	require.NotNil(t, row, "the sheet carries the cloak's row")
	require.Equal(t, cd.GetEquipped()["cloak"].GetModule(), row.GetRef().GetModule())
}

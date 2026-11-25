package character

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	"github.com/KirkDiggler/rpg-api/internal/testsupport/levelfixture"
	"github.com/KirkDiggler/rpg-toolkit/core"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/magicitems"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// TestEquipItem_ACustomItemRidesTheRealVerb pins rpg-project#850 with the REAL
// session equip verb (no scripted answer, no server: an in-process miniredis
// under the real repository): a custom item's id goes straight to the verb
// with no catalog check on the way. Worn in a slot it names it succeeds and
// the returned view has it in that slot; named for a slot it does not fit the
// verb's own refusal comes back, not a catalog "not found".
func TestEquipItem_ACustomItemRidesTheRealVerb(t *testing.T) {
	const (
		charID = "char-cloak-fighter"
		itemID = "lieutenants-cloak"
	)
	ctx := context.Background()
	mini := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	repo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	require.NoError(t, err)
	sess, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: repo, TTL: 24 * time.Hour,
		PresentationIDs: idgen.NewSequential("presentation"),
	})
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	orch, err := New(&Config{
		DraftRepo:        draftmock.NewMockRepository(ctrl),
		CharacterRepo:    repo,
		DiceService:      dicemock.NewMockService(ctrl),
		IDGenerator:      idgenmock.NewMockGenerator(ctrl),
		DraftIDGenerator: idgenmock.NewMockGenerator(ctrl),
		Equipment:        sess.Manager,
	})
	require.NoError(t, err)

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
	data := &character.Data{
		ID: charID, PlayerID: "cloak-owner", Name: "Cloaked", Level: 1, Levels: levelfixture.Synthetic(classes.Fighter, 1),
		ClassID: classes.Fighter, RaceID: races.Human, ProficiencyBonus: 2, HitPoints: 12, MaxHitPoints: 12,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, abilities.DEX: 12, abilities.CON: 14, abilities.INT: 10, abilities.WIS: 10, abilities.CHA: 10,
		},
		Inventory:      []character.InventoryItemData{{Type: shared.EquipmentTypeWeapon, ID: "longsword", Quantity: 1}},
		EquipmentSlots: character.EquipmentSlots{},
	}
	require.NoError(t, character.AddItem(data, cloak))
	_, err = repo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: data}})
	require.NoError(t, err)

	out, err := orch.EquipItem(ctx, &EquipItemInput{CharacterID: charID, ItemID: "lieutenants-cloak", Slot: "ring_left"})
	require.Error(t, err)
	require.Nil(t, out)
	t.Log(err)
	require.ErrorIs(t, err, sdk.ErrBadEquip, "the equip verb's own refusal")
	require.NotContains(t, err.Error(), "not found", "not a catalog error")

	out, err = orch.EquipItem(ctx, &EquipItemInput{CharacterID: charID, ItemID: itemID, Slot: "cloak"})
	require.NoError(t, err)
	require.Equal(t, itemID, out.View.Equipment.Equipped[character.InventorySlot("cloak")], "the view has the cloak in that slot")
	require.Equal(t, itemID, out.Character.Data.EquipmentSlots[character.InventorySlot("cloak")])
}

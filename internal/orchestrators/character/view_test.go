package character

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/currency"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/features"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

func TestProjectView_StrictLevel3Fighter(t *testing.T) {
	data := level3FighterData(t, "fighter-3")

	out, err := ProjectView(context.Background(), &ProjectViewInput{Data: data})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, out.View)
	require.NotNil(t, out.View.Equipment)
	require.NotNil(t, out.View.Status)

	view := out.View
	require.Equal(t, "player-1", view.Identity.PlayerID)
	require.Equal(t, classes.Fighter, view.Identity.ClassID)
	require.Equal(t, races.Human, view.Identity.RaceID)
	require.Equal(t, 3, view.Status.Level)
	require.Equal(t, 24, view.Status.HitPoints.Current)
	require.Equal(t, 30, view.Status.HitPoints.Maximum)
	require.Equal(t, 30, view.Status.BaseSpeedFeet)
	require.Len(t, view.Status.Features, 2)
	require.Equal(t, refs.Features.ActionSurge().String(), view.Status.Features[0].Ref.String())
	require.Equal(t, refs.Features.SecondWind().String(), view.Status.Features[1].Ref.String())
	require.Len(t, view.Status.Conditions, 1)
	require.Equal(t, refs.Conditions.FightingStyleDefense().String(), view.Status.Conditions[0].Ref.String())
	require.Equal(t,
		[]coreResources.ResourceKey{resources.ActionSurge, resources.HitDice, resources.SecondWind},
		statusResourceKeys(view.Status.Resources),
	)
	require.Len(t, view.Equipment.Items, 3)
	require.Equal(t, "longsword", view.Equipment.Items[0].ItemID)
	require.Equal(t, currency.FromGold(15), view.Wallet)

	// Both projections are detached values. Mutating persistence after the
	// projection cannot rewrite a response already handed to a caller.
	data.Level = 99
	data.Inventory[0].ID = "greatsword"
	require.Equal(t, 3, view.Status.Level)
	require.Equal(t, "longsword", view.Equipment.Items[0].ItemID)
}

func TestCharacterDataUnavailableRetainsDetailedCause(t *testing.T) {
	const secret = "PRIVATE_CHARACTER_JSON_MARKER"
	wrapped := characterDataUnavailable(errors.New("malformed feature: " + secret))

	require.Equal(t, CharacterDataUnavailableMessage, wrapped.Message)
	require.Error(t, wrapped.Cause)
	require.Contains(t, wrapped.Cause.Error(), secret)
}

func TestProjectView_RejectsMissingOwnerIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*tkcharacter.Data)
	}{
		{name: "player id", mutate: func(data *tkcharacter.Data) { data.PlayerID = "" }},
		{name: "class id", mutate: func(data *tkcharacter.Data) { data.ClassID = "" }},
		{name: "race id", mutate: func(data *tkcharacter.Data) { data.RaceID = "" }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := level3FighterData(t, "fighter-missing-identity")
			tc.mutate(data)

			out, err := ProjectView(context.Background(), &ProjectViewInput{Data: data})
			require.Error(t, err)
			require.Nil(t, out)
		})
	}
}

func TestProjectView_RejectsEveryUnprojectablePersistedShape(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *tkcharacter.Data)
	}{
		{
			name: "malformed condition",
			mutate: func(_ *testing.T, data *tkcharacter.Data) {
				data.Conditions = []json.RawMessage{json.RawMessage(`{"ref":{"module":"dnd5e","type":"conditions","id":"unknown"}}`)}
			},
		},
		{
			name: "malformed feature",
			mutate: func(_ *testing.T, data *tkcharacter.Data) {
				data.Features = []json.RawMessage{json.RawMessage(`{"ref":`)}
			},
		},
		{
			name: "unknown inventory item",
			mutate: func(_ *testing.T, data *tkcharacter.Data) {
				data.Inventory = append(data.Inventory, tkcharacter.InventoryItemData{Type: "item", ID: "vorpal-spork", Quantity: 1})
			},
		},
		{
			name: "malformed resource",
			mutate: func(_ *testing.T, data *tkcharacter.Data) {
				data.Resources[resources.HitDice] = tkcharacter.RecoverableResourceData{Current: 4, Maximum: 3}
			},
		},
		{
			name: "unknown status descriptor",
			mutate: func(t *testing.T, data *tkcharacter.Data) {
				data.Conditions = []json.RawMessage{mustJSON(t, conditions.ShieldSpellConditionData{
					Ref: refs.Spells.Shield(), MemberID: data.ID,
				})}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := level3FighterData(t, "fighter-bad")
			tc.mutate(t, data)

			out, err := ProjectView(context.Background(), &ProjectViewInput{Data: data})
			require.Error(t, err)
			require.Nil(t, out)
		})
	}
}

func level3FighterData(t *testing.T, id string) *tkcharacter.Data {
	t.Helper()

	return &tkcharacter.Data{
		ID:               id,
		PlayerID:         "player-1",
		Name:             "Arthur",
		Level:            3,
		ProficiencyBonus: 2,
		RaceID:           races.Human,
		ClassID:          classes.Fighter,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 12,
			abilities.CON: 14,
			abilities.INT: 10,
			abilities.WIS: 10,
			abilities.CHA: 10,
		},
		HitPoints:    24,
		MaxHitPoints: 30,
		ArmorClass:   10,
		Wallet:       currency.FromGold(15),
		Inventory: []tkcharacter.InventoryItemData{
			{Type: "weapon", ID: "longsword", Quantity: 1},
			{Type: "armor", ID: "shield", Quantity: 1},
			{Type: "weapon", ID: "greatsword", Quantity: 1},
		},
		EquipmentSlots: tkcharacter.EquipmentSlots{
			tkcharacter.SlotMainHand: "longsword",
			tkcharacter.SlotOffHand:  "shield",
		},
		Features: []json.RawMessage{
			mustJSON(t, features.SecondWindData{
				Ref: refs.Features.SecondWind(), ID: "second-wind", Name: "Second Wind",
				Level: 3, CharacterID: id, Uses: 1, MaxUses: 1,
			}),
			mustJSON(t, features.ActionSurgeData{
				Ref: refs.Features.ActionSurge(), ID: "action-surge", Name: "Action Surge",
				CharacterID: id, Uses: 1, MaxUses: 1,
			}),
		},
		Conditions: []json.RawMessage{
			mustJSON(t, conditions.FightingStyleDefenseData{
				Ref: refs.Conditions.FightingStyleDefense(), MemberID: id,
			}),
		},
		Resources: map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData{
			resources.HitDice: {
				Current: 2, Maximum: 3, ResetType: coreResources.ResetLongRest,
			},
		},
		SpellSlots: map[int]tkcharacter.SpellSlotData{1: {Max: 2}},
		ClassResources: map[shared.ClassResourceType]tkcharacter.ResourceData{
			shared.ClassResourceType(99): {Name: "legacy-resource", Current: 1, Max: 1},
		},
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

func statusResourceKeys(views []tkcharacter.ResourceView) []coreResources.ResourceKey {
	keys := make([]coreResources.ResourceKey, 0, len(views))
	for _, view := range views {
		keys = append(keys, view.Key)
	}
	return keys
}

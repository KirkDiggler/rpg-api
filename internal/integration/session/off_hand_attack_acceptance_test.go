// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package session_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// offHandAcceptanceDice makes ordinary attacks hit without critting and rolls
// one on damage dice, so the ability-modifier rule is visible in the total.
type offHandAcceptanceDice struct{}

func (offHandAcceptanceDice) Roll(_ context.Context, size int) (int, error) {
	if size == 20 {
		return 10, nil
	}
	return 1, nil
}

func dualWieldingFighter(
	t *testing.T, id, playerID string, fightingStyle bool,
) *tkcharacter.Data {
	t.Helper()
	sheet := &tkcharacter.Data{
		ID: id, PlayerID: playerID, Name: id, Level: 3,
		ClassID: classes.Fighter, RaceID: races.Human,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, abilities.DEX: 14, abilities.CON: 14,
			abilities.INT: 10, abilities.WIS: 12, abilities.CHA: 8,
		},
		HitPoints: 24, MaxHitPoints: 28, ArmorClass: 16, ProficiencyBonus: 2,
		WeaponProficiencies: []proficiencies.Weapon{proficiencies.WeaponMartial},
		Inventory: []tkcharacter.InventoryItemData{
			{Type: shared.EquipmentTypeWeapon, ID: weapons.Shortsword, Quantity: 1},
			{Type: shared.EquipmentTypeWeapon, ID: weapons.Scimitar, Quantity: 1},
		},
		EquipmentSlots: tkcharacter.EquipmentSlots{
			tkcharacter.SlotMainHand: weapons.Shortsword,
			tkcharacter.SlotOffHand:  weapons.Scimitar,
		},
	}
	if fightingStyle {
		condition := conditions.NewFightingStyleTwoWeaponFightingCondition(id)
		blob, err := condition.ToJSON()
		require.NoError(t, err)
		sheet.Conditions = []json.RawMessage{blob}
	}
	return sheet
}

func adjacentOffHandFight(
	t *testing.T, sheet *tkcharacter.Data,
) (*acceptanceHarness, context.Context) {
	t.Helper()
	h := newAcceptanceHarnessWithDice(t, offHandAcceptanceDice{})
	ctx := auth.WithPlayerID(context.Background(), sheet.PlayerID)

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: sheet},
	})
	require.NoError(t, err)
	world := buildThreeRoomTomb(t)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "off-hand-run", Encounter: "off-hand-encounter", World: world,
	})
	require.NoError(t, err)
	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "off-hand-run", Member: sheet.ID, Position: pbAt(18, 3),
	})
	require.NoError(t, err)
	spawned, err := h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: "off-hand-run", ID: "skel-1", Ref: refs.Monsters.Skeleton().String(),
		Position: at(19, 3),
	})
	require.NoError(t, err)
	require.NotNil(t, spawned.Formed, "adjacent visible skeleton must form a fight")

	return h, ctx
}

func attackDeclarationForProtoSlot(
	ctx context.Context,
	t *testing.T,
	h *acceptanceHarness,
	slot sessionpb.Slot,
) *sessionpb.Declaration {
	t.Helper()
	out, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{
		Session: "off-hand-run", Member: "alice",
	})
	require.NoError(t, err)
	var found *sessionpb.Declaration
	for _, declaration := range out.GetDeclarations() {
		if declaration.GetVerb() != sessionpb.Verb_VERB_ATTACK || declaration.GetSlot() != slot {
			continue
		}
		require.Nil(t, found, "only one Attack declaration may use slot %s", slot)
		found = declaration
	}
	require.NotNil(t, found, "Afford must return an Attack declaration for slot %s", slot)
	return found
}

func offHandStruckEvent(
	ctx context.Context, t *testing.T, h *acceptanceHarness,
) *sessionpb.Struck {
	t.Helper()
	story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{
		Session: "off-hand-run", Member: "alice", FromSeq: 1,
	})
	require.NoError(t, err)
	for i := len(story.GetEntries()) - 1; i >= 0; i-- {
		struck := story.GetEntries()[i].GetStruck()
		if struck != nil && struck.GetAttack().GetRef() == "dnd5e:weapons:scimitar" {
			return struck
		}
	}
	t.Fatal("story contains no off-hand scimitar strike")
	return nil
}

func TestAcceptance_OffHandAttackIsASecondServerDeclaration(t *testing.T) {
	tests := []struct {
		name              string
		fightingStyle     bool
		wantOffHandDamage int32
		wantComponents    int
	}{
		{name: "base rule omits positive modifier", wantOffHandDamage: 1, wantComponents: 1},
		{name: "fighting style restores modifier", fightingStyle: true, wantOffHandDamage: 4, wantComponents: 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, ctx := adjacentOffHandFight(t, dualWieldingFighter(t, "alice", "player-alice", tc.fightingStyle))

			before, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: "off-hand-run", Member: "alice"})
			require.NoError(t, err)
			for _, declaration := range before.GetDeclarations() {
				require.False(t,
					declaration.GetVerb() == sessionpb.Verb_VERB_ATTACK && declaration.GetSlot() == sessionpb.Slot_SLOT_BONUS,
					"off-hand Attack must not be offered before the qualifying Attack action")
			}

			main := attackDeclarationForProtoSlot(ctx, t, h, sessionpb.Slot_SLOT_ACTION)
			require.True(t, main.GetAvailable())
			require.Equal(t, "dnd5e:weapons:shortsword", main.GetAttack().GetRef())
			mainResult, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
				Session: "off-hand-run", Attacker: "alice", Target: "skel-1", DeclarationId: main.GetId(),
			})
			require.NoError(t, err)
			require.True(t, mainResult.GetHit())
			require.Equal(t, int32(4), mainResult.GetDamage())

			bonus := attackDeclarationForProtoSlot(ctx, t, h, sessionpb.Slot_SLOT_BONUS)
			require.True(t, bonus.GetAvailable())
			require.Equal(t, "dnd5e:weapons:scimitar", bonus.GetAttack().GetRef())
			require.NotEqual(t, main.GetId(), bonus.GetId())
			offHandResult, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
				Session: "off-hand-run", Attacker: "alice", Target: "skel-1", DeclarationId: bonus.GetId(),
			})
			require.NoError(t, err)
			require.True(t, offHandResult.GetHit())
			require.Equal(t, tc.wantOffHandDamage, offHandResult.GetDamage())
			require.Equal(t, "dnd5e:weapons:scimitar", offHandResult.GetAttack().GetRef())

			struck := offHandStruckEvent(ctx, t, h)
			require.Len(t, struck.GetDamageComponents(), tc.wantComponents)
			if tc.fightingStyle {
				feature := struck.GetDamageComponents()[1]
				require.Equal(t, "feature", feature.GetSource())
				require.NotNil(t, feature.GetRoll())
				require.Equal(t, refs.Conditions.FightingStyleTwoWeaponFighting().String(), feature.GetRoll().GetSource().GetRef())
				require.NotNil(t, feature.GetRoll().Modifier)
				require.Equal(t, int32(3), feature.GetRoll().GetModifier())
				require.Empty(t, feature.GetSourceRef())
				require.Zero(t, feature.GetFlatBonus())
			}

			stored := storedSheetOf(t, h.charRepo, "alice")
			require.NotNil(t, stored.ActionEconomy)
			require.Zero(t, stored.ActionEconomy.BonusActionsRemaining)
			require.Zero(t, stored.ActionEconomy.Granted[tkcharacter.GrantedOffHandStrikes])

			after, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: "off-hand-run", Member: "alice"})
			require.NoError(t, err)
			for _, declaration := range after.GetDeclarations() {
				require.False(t,
					declaration.GetVerb() == sessionpb.Verb_VERB_ATTACK && declaration.GetSlot() == sessionpb.Slot_SLOT_BONUS,
					"spent off-hand capacity must remove the bonus Attack declaration")
			}
		})
	}
}

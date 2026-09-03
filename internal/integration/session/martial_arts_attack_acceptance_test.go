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
)

func quarterstaffMonkAcceptance(t *testing.T, id, playerID string) *tkcharacter.Data {
	t.Helper()
	martialArts, err := conditions.NewMartialArtsCondition(conditions.MartialArtsInput{
		MemberID: id, MonkLevel: 1,
	}).ToJSON()
	require.NoError(t, err)

	return &tkcharacter.Data{
		ID: id, PlayerID: playerID, Name: id, Level: 1,
		ClassID: classes.Monk, RaceID: races.Human,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 12, abilities.DEX: 18, abilities.CON: 14,
			abilities.INT: 10, abilities.WIS: 15, abilities.CHA: 8,
		},
		HitPoints: 10, MaxHitPoints: 10, ArmorClass: 15, ProficiencyBonus: 2,
		WeaponProficiencies: []proficiencies.Weapon{proficiencies.WeaponSimple},
		Inventory: []tkcharacter.InventoryItemData{{
			Type: shared.EquipmentTypeWeapon, ID: weapons.Quarterstaff, Quantity: 1,
		}},
		EquipmentSlots: tkcharacter.EquipmentSlots{
			tkcharacter.SlotMainHand: weapons.Quarterstaff,
		},
		Conditions: []json.RawMessage{martialArts},
	}
}

func struckEventForAttack(
	ctx context.Context, t *testing.T, h *acceptanceHarness, attackRef string,
) *sessionpb.Struck {
	t.Helper()
	story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{
		Session: "off-hand-run", Member: "alice", FromSeq: 1,
	})
	require.NoError(t, err)
	for i := len(story.GetEntries()) - 1; i >= 0; i-- {
		struck := story.GetEntries()[i].GetStruck()
		if struck != nil && struck.GetAttack().GetRef() == attackRef {
			return struck
		}
	}
	t.Fatalf("story contains no strike with attack %q", attackRef)
	return nil
}

func TestAcceptance_QuarterstaffAttackGrantsBonusUnarmedStrike(t *testing.T) {
	h, ctx := adjacentOffHandFight(
		t, quarterstaffMonkAcceptance(t, "alice", "player-alice"),
	)
	_, err := h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: "off-hand-run", ID: "skel-captain",
		Ref: refs.Monsters.SkeletonCaptain().String(), Position: at(18, 4),
	})
	require.NoError(t, err)

	before, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{
		Session: "off-hand-run", Member: "alice",
	})
	require.NoError(t, err)
	for _, declaration := range before.GetDeclarations() {
		require.False(t,
			declaration.GetVerb() == sessionpb.Verb_VERB_ATTACK &&
				declaration.GetSlot() == sessionpb.Slot_SLOT_BONUS,
			"Martial Arts is granted by the qualifying Attack, not before it")
	}

	main := attackDeclarationForProtoSlot(ctx, t, h, sessionpb.Slot_SLOT_ACTION)
	require.True(t, main.GetAvailable())
	require.Equal(t, "dnd5e:weapons:quarterstaff", main.GetAttack().GetRef())
	mainResult, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
		Session: "off-hand-run", Attacker: "alice", Target: "skel-captain", DeclarationId: main.GetId(),
	})
	require.NoError(t, err)
	require.True(t, mainResult.GetHit())
	require.Equal(t, "dnd5e:weapons:quarterstaff", mainResult.GetAttack().GetRef())

	storedAfterMain := storedSheetOf(t, h.charRepo, "alice")
	require.NotNil(t, storedAfterMain.ActionEconomy)
	require.Zero(t, storedAfterMain.ActionEconomy.ActionsRemaining)
	require.Equal(t, 1, storedAfterMain.ActionEconomy.BonusActionsRemaining)
	require.Equal(t, 1, storedAfterMain.ActionEconomy.Granted[tkcharacter.GrantedMartialArtsBonus])
	require.Zero(t, storedAfterMain.ActionEconomy.Granted[tkcharacter.GrantedOffHandStrikes])

	bonus := attackDeclarationForProtoSlot(ctx, t, h, sessionpb.Slot_SLOT_BONUS)
	require.True(t, bonus.GetAvailable())
	require.Equal(t, "dnd5e:weapons:unarmed-strike", bonus.GetAttack().GetRef())
	require.Equal(t, "Unarmed Strike", bonus.GetAttack().GetName())
	require.NotEqual(t, main.GetId(), bonus.GetId())
	require.Equal(t, main.GetCandidates(), bonus.GetCandidates())

	bonusResult, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
		Session: "off-hand-run", Attacker: "alice", Target: "skel-captain", DeclarationId: bonus.GetId(),
	})
	require.NoError(t, err)
	require.True(t, bonusResult.GetHit())
	require.Equal(t, "dnd5e:weapons:unarmed-strike", bonusResult.GetAttack().GetRef())

	struck := struckEventForAttack(ctx, t, h, "dnd5e:weapons:unarmed-strike")
	var weaponComponent, abilityComponent *sessionpb.DamageComponent
	for _, component := range struck.GetDamageComponents() {
		switch component.GetSource() {
		case "weapon":
			weaponComponent = component
		case "ability":
			abilityComponent = component
		}
	}
	require.NotNil(t, weaponComponent)
	require.NotNil(t, weaponComponent.GetRoll())
	require.Equal(t, "dnd5e:weapons:unarmed-strike", weaponComponent.GetRoll().GetSource().GetRef())
	require.Equal(t, "d4", weaponComponent.GetRoll().GetDice().GetNotation(),
		"the normalized trace records the physical level-1 Martial Arts die")
	require.Empty(t, weaponComponent.GetSourceRef())
	require.Empty(t, weaponComponent.GetDice())
	require.NotNil(t, abilityComponent)
	require.NotNil(t, abilityComponent.GetRoll())
	require.Equal(t, refs.Abilities.Dexterity().String(), abilityComponent.GetRoll().GetSource().GetRef())
	require.NotNil(t, abilityComponent.GetRoll().Modifier)
	require.Equal(t, int32(4), abilityComponent.GetRoll().GetModifier())
	require.Empty(t, abilityComponent.GetSourceRef())
	require.Zero(t, abilityComponent.GetFlatBonus())

	storedAfterBonus := storedSheetOf(t, h.charRepo, "alice")
	require.NotNil(t, storedAfterBonus.ActionEconomy)
	require.Zero(t, storedAfterBonus.ActionEconomy.BonusActionsRemaining)
	require.Zero(t, storedAfterBonus.ActionEconomy.Granted[tkcharacter.GrantedMartialArtsBonus])

	after, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{
		Session: "off-hand-run", Member: "alice",
	})
	require.NoError(t, err)
	for _, declaration := range after.GetDeclarations() {
		require.False(t,
			declaration.GetVerb() == sessionpb.Verb_VERB_ATTACK &&
				declaration.GetSlot() == sessionpb.Slot_SLOT_BONUS,
			"spent Martial Arts capacity must remove the bonus Attack declaration")
	}
}

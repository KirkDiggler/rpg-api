// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package session_test

import (
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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
)

func quarterstaffMonkAcceptance(t *testing.T, id, playerID string) *tkcharacter.Data {
	t.Helper()
	martialArts, err := conditions.NewMartialArtsCondition(conditions.MartialArtsInput{
		MemberID: id,
	}).ToJSON()
	require.NoError(t, err)

	return &tkcharacter.Data{
		ID: id, PlayerID: playerID, Name: id, Level: 1,
		ClassID: classes.Monk, RaceID: races.Human,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 12, abilities.DEX: 18, abilities.CON: 14,
			abilities.INT: 10, abilities.WIS: 15, abilities.CHA: 8,
		},
		HitPoints: 10, MaxHitPoints: 10, ProficiencyBonus: 2,
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

func TestAcceptance_QuarterstaffAttackGrantsBonusUnarmedStrike(t *testing.T) {
	h, ctx := adjacentOffHandFight(
		t, quarterstaffMonkAcceptance(t, "alice", "player-alice"),
		monsterAt("skel-captain", refs.Monsters.SkeletonCaptain().String(), 18, 4),
	)

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

	struck := offHandAttackAct(ctx, t, h, "dnd5e:weapons:unarmed-strike")
	require.Equal(t, storypb.AttackOutcome_ATTACK_OUTCOME_HIT, struck.GetOutcome())
	require.Equal(t, "skel-captain", struck.GetTo().GetId())
	var weaponComponent, abilityComponent *storypb.DamageComponent
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
	require.Equal(t, "1d4", weaponComponent.GetRoll().GetExpression(),
		"the roll records the physical level-1 Martial Arts die")
	require.Len(t, weaponComponent.GetRoll().GetDice(), 1)
	require.Equal(t, "dnd5e:weapons:unarmed-strike", weaponComponent.GetSourceRef(),
		"the component names its provider (rpg-project#643 C)")
	require.NotNil(t, abilityComponent)
	require.NotNil(t, abilityComponent.GetRoll())
	require.Len(t, abilityComponent.GetRoll().GetModifiers(), 1)
	require.Equal(t, "Dexterity", abilityComponent.GetRoll().GetModifiers()[0].GetSource())
	require.Equal(t, int32(4), abilityComponent.GetRoll().GetModifiers()[0].GetValue())
	require.Equal(t, refs.Abilities.Dexterity().String(), abilityComponent.GetRoll().GetModifiers()[0].GetSourceRef(),
		"the modifier names its provider (rpg-project#643 D)")
	require.Equal(t, refs.Abilities.Dexterity().String(), abilityComponent.GetSourceRef(),
		"the component names its provider (rpg-project#643 C)")

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

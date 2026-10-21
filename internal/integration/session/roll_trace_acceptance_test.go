// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

type acceptanceDie struct {
	size  int
	value int
}

// acceptanceSequenceDice is one Session-scoped script. It refuses an
// unexpected die size or an extra call, so a provider cannot silently fall
// through to process-global randomness.
type acceptanceSequenceDice struct {
	rolls []acceptanceDie
	next  int
}

func newAcceptanceSequenceDice(rolls ...acceptanceDie) *acceptanceSequenceDice {
	return &acceptanceSequenceDice{rolls: rolls}
}

func (d *acceptanceSequenceDice) Roll(_ context.Context, size int) (int, error) {
	if d.next >= len(d.rolls) {
		return 0, fmt.Errorf("acceptance dice: unexpected d%d call after %d scripted rolls", size, d.next)
	}
	roll := d.rolls[d.next]
	if roll.size != size {
		return 0, fmt.Errorf("acceptance dice: call %d requested d%d, want d%d", d.next+1, size, roll.size)
	}
	if roll.value < 1 || roll.value > size {
		return 0, fmt.Errorf("acceptance dice: scripted face %d is outside d%d", roll.value, size)
	}
	d.next++
	return roll.value, nil
}

func (d *acceptanceSequenceDice) requireExhausted(t *testing.T) {
	t.Helper()
	require.Equal(t, len(d.rolls), d.next, "every scripted Session roll must be consumed")
}

func greatWeaponFighter(t *testing.T, id, playerID string) *tkcharacter.Data {
	t.Helper()
	gwf := &conditions.FightingStyleGreatWeaponFightingCondition{MemberID: id}
	condition, err := gwf.ToJSON()
	require.NoError(t, err)

	return &tkcharacter.Data{
		ID: id, PlayerID: playerID, Name: id, Level: 1,
		ClassID: classes.Fighter, RaceID: races.Human,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, abilities.DEX: 14, abilities.CON: 14,
			abilities.INT: 10, abilities.WIS: 12, abilities.CHA: 8,
		},
		HitPoints: 12, MaxHitPoints: 12, ProficiencyBonus: 2,
		WeaponProficiencies: []proficiencies.Weapon{proficiencies.WeaponMartial},
		Inventory: []tkcharacter.InventoryItemData{{
			Type: shared.EquipmentTypeWeapon, ID: weapons.Greatsword, Quantity: 1,
		}},
		EquipmentSlots: tkcharacter.EquipmentSlots{tkcharacter.SlotMainHand: weapons.Greatsword},
		Conditions:     []json.RawMessage{condition},
	}
}

// requireGreatWeaponFightingAct asserts every fact of the GWF swing the act
// stream states. The v1 Struck beat's damage trace also carried the original
// faces, the reroll (die 0, 1 -> 4, sourced to Great Weapon Fighting), and each
// component's provider ref; the attack act's damage roll carries only the
// faces that counted and the modifier's source name (rpg-project#631 c1
// report), so those facts are not asserted here.
func requireGreatWeaponFightingAct(t *testing.T, act *storypb.Act) {
	t.Helper()
	require.Equal(t, storypb.Family_FAMILY_ACT, act.GetFamily())
	attack := act.GetAttack()
	require.NotNil(t, attack)
	require.Equal(t, "alice", attack.GetBy().GetId())
	require.Equal(t, "skel-1", attack.GetTo().GetId())
	require.Equal(t, storypb.AttackOutcome_ATTACK_OUTCOME_HIT, attack.GetOutcome())
	require.Equal(t, []int32{15}, attack.GetRoll().GetDice())
	require.Equal(t, int32(20), attack.GetRoll().GetTotal())
	require.Equal(t, int32(12), attack.GetDamage())
	require.False(t, attack.GetCritical())
	require.Equal(t, "dnd5e:weapons:greatsword", attack.GetAttack().GetRef())
	require.Len(t, attack.GetDamageComponents(), 2)

	weapon := attack.GetDamageComponents()[0]
	require.Equal(t, "weapon", weapon.GetSource())
	require.Equal(t, storypb.DamageType_DAMAGE_TYPE_SLASHING, weapon.GetDamageType())
	require.NotNil(t, weapon.GetRoll())
	require.Equal(t, "2d6", weapon.GetRoll().GetExpression())
	require.Equal(t, []int32{4, 5}, weapon.GetRoll().GetDice(),
		"the faces that counted: Great Weapon Fighting's 4 replaced die 0's 1")
	require.Equal(t, int32(9), weapon.GetRoll().GetTotal())
	require.Empty(t, weapon.GetRoll().GetModifiers(), "the weapon die carries no flat part")

	ability := attack.GetDamageComponents()[1]
	require.Equal(t, "ability", ability.GetSource())
	require.Equal(t, storypb.DamageType_DAMAGE_TYPE_SLASHING, ability.GetDamageType())
	require.NotNil(t, ability.GetRoll())
	require.Empty(t, ability.GetRoll().GetDice())
	require.Len(t, ability.GetRoll().GetModifiers(), 1)
	require.Equal(t, "Strength", ability.GetRoll().GetModifiers()[0].GetSource())
	require.Equal(t, int32(3), ability.GetRoll().GetModifiers()[0].GetValue())
	require.Equal(t, int32(3), ability.GetRoll().GetTotal())
}

// TestAcceptance_GreatWeaponFightingRollTraceCrossesLiveAndStory drives one
// persisted GWF condition through the real Handler, Session Manager, Broker,
// Redis repositories, and the StoryService act stream. One Session-scoped roller
// supplies formation, attack, original damage, and the condition reroll.
func TestAcceptance_GreatWeaponFightingRollTraceCrossesLiveAndStory(t *testing.T) {
	dice := newAcceptanceSequenceDice(
		acceptanceDie{size: 20, value: 10}, // alice initiative
		acceptanceDie{size: 20, value: 10}, // skeleton initiative; tie breaks to alice
		acceptanceDie{size: 20, value: 15}, // attack
		acceptanceDie{size: 6, value: 1},   // original greatsword die 0
		acceptanceDie{size: 6, value: 5},   // original greatsword die 1
		acceptanceDie{size: 6, value: 4},   // GWF replacement for die 0
	)
	h := newAcceptanceHarnessWithDice(t, dice)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	const sessionID = "gwf-roll-trace-run"

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: greatWeaponFighter(t, "alice", "player-alice")},
	})
	require.NoError(t, err)
	launched := h.launch(t, sessionID,
		withMonsters(buildThreeRoomTomb(t), monsterAt("skel-1", refs.Monsters.Skeleton().String(), 19, 3)),
		seatAt("alice", 18, 3))
	require.Len(t, launched.Formed, 1)
	require.Equal(t, []string{"alice", "skel-1"}, launched.Formed[0].Order)

	var attack *sessionpb.AttackResponse
	live := watchActs(ctx, t, h, sessionID, "alice", func() {
		declarationID := currentDeclarationID(ctx, t, h.handler, sessionID, "alice", sessionpb.Verb_VERB_ATTACK)
		attack, err = h.handler.Attack(ctx, &sessionpb.AttackRequest{
			Session: sessionID, Attacker: "alice", Target: "skel-1", DeclarationId: declarationID,
		})
		require.NoError(t, err)
	})
	require.True(t, attack.GetHit())
	require.Equal(t, int32(12), attack.GetDamage(), "the provider's authoritative final damage crosses the verb response")
	dice.requireExhausted(t)

	require.Len(t, live, 1, "one non-lethal attack authors one attack act, with no duplicate")
	requireGreatWeaponFightingAct(t, live[0])
	requireLiveIsStory(ctx, t, h, sessionID, "alice", live)

	stored := storedSheetOf(t, h.charRepo, "alice")
	require.NotNil(t, stored.ActionEconomy)
	require.Zero(t, stored.ActionEconomy.ActionsRemaining, "the attack's action spend is persisted")
	var gwf int
	for _, c := range stored.Conditions {
		if strings.Contains(string(c), "fighting_style_great_weapon_fighting") {
			gwf++
		}
	}
	require.Equal(t, 1, gwf, "the persisted GWF condition survives resolution, once")
}

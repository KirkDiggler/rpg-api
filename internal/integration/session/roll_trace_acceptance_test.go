// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

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
		HitPoints: 12, MaxHitPoints: 12, ArmorClass: 16, ProficiencyBonus: 2,
		WeaponProficiencies: []proficiencies.Weapon{proficiencies.WeaponMartial},
		Inventory: []tkcharacter.InventoryItemData{{
			Type: shared.EquipmentTypeWeapon, ID: weapons.Greatsword, Quantity: 1,
		}},
		EquipmentSlots: tkcharacter.EquipmentSlots{tkcharacter.SlotMainHand: weapons.Greatsword},
		Conditions:     []json.RawMessage{condition},
	}
}

func requireGreatWeaponFightingTrace(t *testing.T, struck *sessionpb.Struck) {
	t.Helper()
	require.NotNil(t, struck)
	require.Equal(t, "alice", struck.GetAttacker())
	require.Equal(t, "skel-1", struck.GetTarget())
	require.Equal(t, int32(15), struck.GetRoll())
	require.Equal(t, int32(20), struck.GetTotal())
	require.Equal(t, int32(12), struck.GetDamage())
	require.False(t, struck.GetCritical())
	require.Equal(t, "dnd5e:weapons:greatsword", struck.GetAttack().GetRef())
	require.Len(t, struck.GetDamageComponents(), 2)

	weapon := struck.GetDamageComponents()[0]
	require.Equal(t, "weapon", weapon.GetSource())
	require.Equal(t, sessionpb.DamageType_DAMAGE_TYPE_SLASHING, weapon.GetDamageType())
	require.NotNil(t, weapon.GetRoll())
	require.Equal(t, "dnd5e:weapons:greatsword", weapon.GetRoll().GetSource().GetRef())
	require.Equal(t, "Greatsword", weapon.GetRoll().GetSource().GetName())
	require.Nil(t, weapon.GetRoll().Modifier)

	trace := weapon.GetRoll().GetDice()
	require.NotNil(t, trace)
	require.Equal(t, "2d6", trace.GetNotation())
	require.Equal(t, int32(6), trace.GetDieSize())
	require.Equal(t, []int32{1, 5}, trace.GetOriginalRolls())
	require.Len(t, trace.GetRerolls(), 1)
	reroll := trace.GetRerolls()[0]
	require.Equal(t, int32(0), reroll.GetDieIndex())
	require.Equal(t, int32(1), reroll.GetBefore())
	require.Equal(t, int32(4), reroll.GetAfter())
	require.Equal(t, refs.Conditions.FightingStyleGreatWeaponFighting().String(), reroll.GetSource().GetRef())
	require.Equal(t, "Great Weapon Fighting", reroll.GetSource().GetName())
	require.Empty(t, reroll.GetSource().GetLabel(), "the provider authored no narrower role label")
	require.Equal(t, []int32{4, 5}, trace.GetFinalRolls())
	require.Empty(t, trace.GetKeptIndices())
	require.Equal(t, int32(9), trace.GetSubtotal())

	ability := struck.GetDamageComponents()[1]
	require.Equal(t, "ability", ability.GetSource())
	require.Equal(t, sessionpb.DamageType_DAMAGE_TYPE_SLASHING, ability.GetDamageType())
	require.NotNil(t, ability.GetRoll())
	require.Equal(t, refs.Abilities.Strength().String(), ability.GetRoll().GetSource().GetRef())
	require.Equal(t, "Strength", ability.GetRoll().GetSource().GetName())
	require.Nil(t, ability.GetRoll().GetDice())
	require.NotNil(t, ability.GetRoll().Modifier)
	require.Equal(t, int32(3), ability.GetRoll().GetModifier())

	for _, component := range struck.GetDamageComponents() {
		require.Empty(t, component.GetSourceRef(), "new components do not duplicate identity in deprecated scalars")
		require.Empty(t, component.GetDice(), "new components do not duplicate notation in deprecated scalars")
		require.Nil(t, component.GetFinalRolls(), "new components do not duplicate faces in deprecated scalars")
		require.Zero(t, component.GetFlatBonus(), "new components do not duplicate modifiers in deprecated scalars")
	}
}

// TestAcceptance_GreatWeaponFightingRollTraceCrossesLiveAndStory drives one
// persisted GWF condition through the real Handler, Session Manager, Broker,
// Redis repositories, and shared event converter. One Session-scoped roller
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
	world := buildThreeRoomTomb(t)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: sessionID, Encounter: "gwf-encounter", World: world,
	})
	require.NoError(t, err)
	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: sessionID, Member: "alice", Position: pbAt(18, 3),
	})
	require.NoError(t, err)
	spawned, err := h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: sessionID, ID: "skel-1", Ref: refs.Monsters.Skeleton().String(), Position: at(19, 3),
	})
	require.NoError(t, err)
	require.NotNil(t, spawned.Formed)
	require.Equal(t, []string{"alice", "skel-1"}, spawned.Formed.Order)

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newRecordingStream(streamCtx)
	done := make(chan error, 1)
	go func() {
		done <- h.handler.StreamEvents(&sessionpb.StreamEventsRequest{Session: sessionID, Member: "alice"}, stream)
	}()
	waitForLive(t, h.manager.Broker, sessionID, "alice", stream)
	baseline := len(stream.snapshot())

	declarationID := currentDeclarationID(ctx, t, h.handler, sessionID, "alice", sessionpb.Verb_VERB_ATTACK)
	attack, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
		Session: sessionID, Attacker: "alice", Target: "skel-1", DeclarationId: declarationID,
	})
	require.NoError(t, err)
	require.True(t, attack.GetHit())
	require.Equal(t, int32(12), attack.GetDamage(), "the provider's authoritative final damage crosses the verb response")
	dice.requireExhausted(t)

	live := waitForQuiescence(t, stream, 2*time.Second)[baseline:]
	require.Len(t, live, 1, "one non-lethal attack authors one Struck beat, with no duplicate")
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_STRUCK, live[0].GetKind())
	requireGreatWeaponFightingTrace(t, live[0].GetStruck())

	story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{
		Session: sessionID, Member: "alice", FromSeq: live[0].GetSeq(),
	})
	require.NoError(t, err)
	require.Len(t, story.GetEntries(), 1, "catch-up returns the same one sequence without a duplicate")
	requireGreatWeaponFightingTrace(t, story.GetEntries()[0].GetStruck())
	require.True(t, proto.Equal(live[0], story.GetEntries()[0]), "live and GetStory project the same sequence byte-for-byte")

	cancel()
	require.NoError(t, <-done)

	stored := storedSheetOf(t, h.charRepo, "alice")
	require.NotNil(t, stored.ActionEconomy)
	require.Zero(t, stored.ActionEconomy.ActionsRemaining, "the attack's action spend is persisted")
	require.Len(t, stored.Conditions, 1, "the persisted GWF condition survives resolution")
	require.Contains(t, string(stored.Conditions[0]), "fighting_style_great_weapon_fighting")
}

package encounter_test

import (
	"errors"
	"time"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/events"
)

// Combat event translator tests — covers the Wave 2.8 additions:
// DamageDealtEvent, ConditionAppliedEvent, ModeChangedEvent, TurnStartedEvent,
// TurnEndedEvent — plus the TakeAction wave (#597) additions: the un-suppressed
// AttackResolvedEvent (fires on hit AND miss — the #594 fix), the first-class
// ActionResolvedEvent, and the pushed TurnStateChangedEvent.

func (s *TranslateSuite) TestTranslateEvent_AttackResolvedEvent_Hit_Translated() {
	// TakeAction wave (#597): AttackResolved is no longer suppressed — protos
	// v0.1.100 added the variant, so the cause/roll-detail event is translated
	// and sent. Timestamp + correlation_id come from the event (Inv 5, 8).
	evt := events.NewAttackResolvedEvent(
		"enc-1", uint64(10),
		"char-A", "goblin-1",
		true, false, 18, 4, 15,
		map[core.PlayerID]events.AttackResolvedSlice{
			"player-A": {Visible: true},
		},
	)
	gameTime := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	evt.Stamp(gameTime, "corr-attack-1")

	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(out)

	atk := out.GetAttackResolved()
	s.Require().NotNil(atk, "expected AttackResolved envelope")
	s.Require().Equal("char-A", atk.GetAttackerEntityId())
	s.Require().Equal("goblin-1", atk.GetTargetEntityId())
	s.Require().True(atk.GetHit())
	s.Require().False(atk.GetCritical())
	s.Require().Equal(int32(18), atk.GetAttackRoll())
	s.Require().Equal(int32(4), atk.GetAttackBonus())
	s.Require().Equal(int32(15), atk.GetTargetAc())
	s.Require().Equal(int64(10), out.Sequence)
	// Invariant 5: envelope timestamp is the event's game-time, not s.now.
	s.Require().Equal(gameTime.Unix(), out.GetTimestamp().GetSeconds())
	// Invariant 8: correlation_id copied through verbatim.
	s.Require().Equal("corr-attack-1", out.GetCorrelationId())
}

func (s *TranslateSuite) TestTranslateEvent_AttackResolvedEvent_Miss_Translated() {
	// The #594 fix: a miss must be visible on the wire. AttackResolved fires on
	// miss (no parallel DamageDealt), so this is the only event carrying the
	// swing — it must translate, not be dropped.
	evt := events.NewAttackResolvedEvent(
		"enc-1", uint64(11),
		"char-A", "goblin-1",
		false, false, 3, 4, 15,
		map[core.PlayerID]events.AttackResolvedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(out)

	atk := out.GetAttackResolved()
	s.Require().NotNil(atk, "a miss must still produce an AttackResolved envelope")
	s.Require().False(atk.GetHit(), "hit=false is the whole point of the #594 fix")
	s.Require().Equal(int32(3), atk.GetAttackRoll())
}

func (s *TranslateSuite) TestTranslateEvent_AttackResolvedEvent_NotVisible_SawNothing() {
	evt := events.NewAttackResolvedEvent(
		"enc-1", uint64(10),
		"char-A", "goblin-1",
		true, false, 18, 4, 15,
		map[core.PlayerID]events.AttackResolvedSlice{
			"player-A": {Visible: false},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
	s.Require().Nil(out)
}

func (s *TranslateSuite) TestTranslateEvent_ActionResolvedEvent_HappyPath() {
	evt := events.NewActionResolvedEvent(
		"enc-1", uint64(20),
		"char-A",
		"dnd5e:combat_abilities:attack",
		"goblin-1",
		events.EconomyConsumed{
			Actions:         1,
			GrantedConsumed: map[string]int{"attacks": 1},
		},
		map[core.PlayerID]events.ActionResolvedSlice{
			"player-A": {Visible: true},
		},
	)
	gameTime := time.Date(2026, 6, 7, 12, 0, 1, 0, time.UTC)
	evt.Stamp(gameTime, "corr-action-1")

	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(out)

	act := out.GetActionResolved()
	s.Require().NotNil(act, "expected ActionResolved envelope")
	s.Require().Equal("char-A", act.GetActorEntityId())
	s.Require().Equal("goblin-1", act.GetTargetEntityId())
	// action_ref split verbatim into the proto triple — both toolkit namespaces
	// preserved (combat_abilities here), no flattening (D13).
	s.Require().NotNil(act.GetActionRef())
	s.Require().Equal("dnd5e", act.GetActionRef().GetModule())
	s.Require().Equal("combat_abilities", act.GetActionRef().GetType())
	s.Require().Equal("attack", act.GetActionRef().GetId())
	// economy_consumed projected field-for-field.
	s.Require().NotNil(act.GetEconomyConsumed())
	s.Require().Equal(int32(1), act.GetEconomyConsumed().GetActions())
	s.Require().Equal(int32(1), act.GetEconomyConsumed().GetGrantedConsumed()["attacks"])
	s.Require().Equal(gameTime.Unix(), out.GetTimestamp().GetSeconds())
	s.Require().Equal("corr-action-1", out.GetCorrelationId())
}

func (s *TranslateSuite) TestTranslateEvent_ActionResolvedEvent_ActionsNamespace_NotFlattened() {
	// The "actions:*" (doing) namespace must survive verbatim too, distinct from
	// "combat_abilities:*" — rpg-api copies the triple through, never remaps.
	evt := events.NewActionResolvedEvent(
		"enc-1", uint64(21),
		"char-A",
		"dnd5e:actions:strike",
		"goblin-1",
		events.EconomyConsumed{GrantedConsumed: map[string]int{"attacks": 1}},
		map[core.PlayerID]events.ActionResolvedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().Equal("actions", out.GetActionResolved().GetActionRef().GetType())
	s.Require().Equal("strike", out.GetActionResolved().GetActionRef().GetId())
}

func (s *TranslateSuite) TestTranslateEvent_ActionResolvedEvent_DegenerateRef_NoPanic() {
	// protos disc-014: a degenerate ref ("::attack") must not index-panic; it
	// falls back to {dnd5e, action, "::attack"} rather than crashing. No real
	// caller produces this — purely defensive.
	evt := events.NewActionResolvedEvent(
		"enc-1", uint64(22),
		"char-A",
		"::attack",
		"",
		events.EconomyConsumed{Actions: 1},
		map[core.PlayerID]events.ActionResolvedSlice{
			"player-A": {Visible: true},
		},
	)
	s.Require().NotPanics(func() {
		out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
		s.Require().NoError(err)
		s.Require().NotNil(out.GetActionResolved().GetActionRef())
		// Degenerate input has empty parts → falls back to id=raw, not a panic.
		s.Require().Equal("dnd5e", out.GetActionResolved().GetActionRef().GetModule())
		s.Require().Equal("action", out.GetActionResolved().GetActionRef().GetType())
		s.Require().Equal("::attack", out.GetActionResolved().GetActionRef().GetId())
	})
}

func (s *TranslateSuite) TestTranslateEvent_ActionResolvedEvent_NotVisible_SawNothing() {
	evt := events.NewActionResolvedEvent(
		"enc-1", uint64(20),
		"char-A", "dnd5e:combat_abilities:attack", "goblin-1",
		events.EconomyConsumed{Actions: 1},
		map[core.PlayerID]events.ActionResolvedSlice{
			"player-A": {Visible: false},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
	s.Require().Nil(out)
}

func (s *TranslateSuite) TestTranslateEvent_TurnStateChangedEvent_HappyPath() {
	snap := events.TurnStateSnapshot{
		InCombat:              true,
		TurnNumber:            2,
		ActionsRemaining:      0,
		BonusActionsRemaining: 1,
		ReactionsRemaining:    1,
		MovementRemaining:     30,
		Menu: []events.MenuEntry{
			{
				Ref:         "dnd5e:combat_abilities:attack",
				Name:        "Attack",
				EconomySlot: "action",
				TargetKind:  "single_entity",
				Available:   false,
				Reason:      "no actions remaining",
			},
			{
				Ref:         "dnd5e:actions:move",
				Name:        "Move",
				EconomySlot: "movement",
				TargetKind:  "position",
				Available:   false,
				Reason:      "movement lands in Beat 2",
			},
			{
				Ref:         "dnd5e:combat_abilities:dodge",
				Name:        "Dodge",
				EconomySlot: "action",
				TargetKind:  "self",
				Available:   false,
				Reason:      "no actions remaining",
			},
		},
	}
	evt := events.NewTurnStateChangedEvent(
		"enc-1", uint64(30),
		"char-A",
		snap,
		map[core.PlayerID]events.TurnStateChangedSlice{
			"player-A": {Visible: true},
		},
	)
	evt.Stamp(s.now, "corr-action-1")

	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(out)

	tsc := out.GetTurnStateChanged()
	s.Require().NotNil(tsc, "expected TurnStateChanged envelope")
	ts := tsc.GetTurnState()
	s.Require().NotNil(ts)
	s.Require().Equal("char-A", ts.GetActiveEntityId())

	// economy projected field-for-field.
	s.Require().NotNil(ts.GetEconomy())
	s.Require().Equal(int32(0), ts.GetEconomy().GetActionsRemaining())
	s.Require().Equal(int32(1), ts.GetEconomy().GetBonusActionsRemaining())
	s.Require().Equal(int32(1), ts.GetEconomy().GetReactionsRemaining())
	s.Require().Equal(int32(30), ts.GetEconomy().GetMovementRemaining())

	// menu projected verbatim: ref, name, availability+reason, slot enum, target enum.
	s.Require().Len(ts.GetAvailableActions(), 3)
	attack := ts.GetAvailableActions()[0]
	s.Require().Equal("Attack", attack.GetDisplayName())
	s.Require().Equal("combat_abilities", attack.GetRef().GetType())
	s.Require().False(attack.GetAvailable())
	s.Require().Equal("no actions remaining", attack.GetUnavailableReason())
	s.Require().Equal(encounterv2pb.EconomySlot_ECONOMY_SLOT_ACTION, attack.GetEconomySlot())
	s.Require().Equal(encounterv2pb.TargetKind_TARGET_KIND_SINGLE_ENTITY, attack.GetTargetKind())

	move := ts.GetAvailableActions()[1]
	s.Require().Equal("movement lands in Beat 2", move.GetUnavailableReason())
	s.Require().Equal(encounterv2pb.EconomySlot_ECONOMY_SLOT_MOVEMENT, move.GetEconomySlot())
	s.Require().Equal(encounterv2pb.TargetKind_TARGET_KIND_POSITION, move.GetTargetKind())

	// D15: SELF must survive as its own kind (not collapsed to NONE).
	dodge := ts.GetAvailableActions()[2]
	s.Require().Equal(encounterv2pb.TargetKind_TARGET_KIND_SELF, dodge.GetTargetKind())

	s.Require().Equal("corr-action-1", out.GetCorrelationId())
}

func (s *TranslateSuite) TestTranslateEvent_TurnStateChangedEvent_NotInCombat_NoEconomy() {
	// A snapshot for an actor not in combat carries no economy and an empty menu.
	evt := events.NewTurnStateChangedEvent(
		"enc-1", uint64(31),
		"char-A",
		events.TurnStateSnapshot{InCombat: false},
		map[core.PlayerID]events.TurnStateChangedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	ts := out.GetTurnStateChanged().GetTurnState()
	s.Require().Nil(ts.GetEconomy(), "no economy when not in combat")
	s.Require().Empty(ts.GetAvailableActions())
}

func (s *TranslateSuite) TestTranslateEvent_TurnStateChangedEvent_NotVisible_SawNothing() {
	evt := events.NewTurnStateChangedEvent(
		"enc-1", uint64(30),
		"char-A",
		events.TurnStateSnapshot{InCombat: true},
		map[core.PlayerID]events.TurnStateChangedSlice{
			"player-A": {Visible: false},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
	s.Require().Nil(out)
}

func (s *TranslateSuite) TestTranslateEvent_DamageDealtEvent_HappyPath() {
	evt := events.NewDamageDealtEvent(
		"enc-1", uint64(11),
		"goblin-1", "char-A",
		5, "slashing",
		2, 7,
		map[core.PlayerID]events.DamageDealtSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(out)

	dmg := out.GetEntityDamaged()
	s.Require().NotNil(dmg, "expected EntityDamaged envelope")
	s.Require().Equal("goblin-1", dmg.GetEntityId())
	s.Require().Equal(int32(5), dmg.GetAmount())
	s.Require().Equal("char-A", dmg.GetSourceEntityId())
	s.Require().NotNil(dmg.GetHpAfter())
	s.Require().Equal(int32(2), dmg.GetHpAfter().GetCurrent())
	s.Require().Equal(int32(7), dmg.GetHpAfter().GetMax())
	s.Require().NotNil(dmg.GetDamageType())
	s.Require().Equal("dnd5e", dmg.GetDamageType().GetModule())
	s.Require().Equal("damage", dmg.GetDamageType().GetType())
	s.Require().Equal("slashing", dmg.GetDamageType().GetId())
	s.Require().Equal(int64(11), out.Sequence)
}

func (s *TranslateSuite) TestTranslateEvent_DamageDealtEvent_WithBreakdown_PopulatesProto() {
	// Verify that DamageDealtEvent.Components are forwarded into the proto's
	// damage_breakdown field. Two components: weapon + sneak attack.
	evt := events.NewDamageDealtEvent(
		"enc-1", uint64(15),
		"goblin-1", "char-A",
		14, "slashing",
		6, 20,
		map[core.PlayerID]events.DamageDealtSlice{
			"player-A": {Visible: true},
		},
	)
	evt.Components = []core.DamageComponent{
		{Source: "weapon", Amount: 9, DamageType: "slashing", IsCritical: false},
		{Source: "dnd5e:conditions:sneak_attack", Amount: 5, DamageType: "slashing", IsCritical: false},
	}

	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(out)

	dmg := out.GetEntityDamaged()
	s.Require().NotNil(dmg, "expected EntityDamaged envelope")
	s.Require().Len(dmg.GetDamageBreakdown(), 2, "expected 2 damage components")

	weapon := dmg.GetDamageBreakdown()[0]
	s.Require().Equal("weapon", weapon.GetSource())
	s.Require().Equal(int32(9), weapon.GetAmount())
	s.Require().Equal("slashing", weapon.GetDamageType().GetId())
	s.Require().False(weapon.GetIsCritical())

	sneak := dmg.GetDamageBreakdown()[1]
	s.Require().Equal("dnd5e:conditions:sneak_attack", sneak.GetSource())
	s.Require().Equal(int32(5), sneak.GetAmount())
	s.Require().Equal("slashing", sneak.GetDamageType().GetId())
	s.Require().False(sneak.GetIsCritical())
}

func (s *TranslateSuite) TestTranslateEvent_DamageDealtEvent_NoComponents_EmptyBreakdown() {
	// When Components is nil (stand-in resolver), damage_breakdown must be empty.
	evt := events.NewDamageDealtEvent(
		"enc-1", uint64(16),
		"goblin-1", "char-A",
		5, "slashing",
		2, 7,
		map[core.PlayerID]events.DamageDealtSlice{
			"player-A": {Visible: true},
		},
	)
	// Components is nil by default.

	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	dmg := out.GetEntityDamaged()
	s.Require().NotNil(dmg)
	s.Require().Empty(dmg.GetDamageBreakdown(), "no components → empty breakdown")
}

func (s *TranslateSuite) TestTranslateEvent_DamageDealtEvent_CritBreakdown_SetsCritFlag() {
	// IsCritical on a component must pass through to the proto.
	evt := events.NewDamageDealtEvent(
		"enc-1", uint64(17),
		"goblin-1", "char-A",
		18, "slashing",
		2, 20,
		map[core.PlayerID]events.DamageDealtSlice{
			"player-A": {Visible: true},
		},
	)
	evt.Components = []core.DamageComponent{
		{Source: "weapon", Amount: 18, DamageType: "slashing", IsCritical: true},
	}

	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	dmg := out.GetEntityDamaged()
	s.Require().Len(dmg.GetDamageBreakdown(), 1)
	s.Require().True(dmg.GetDamageBreakdown()[0].GetIsCritical(), "critical component flag must propagate")
}

func (s *TranslateSuite) TestTranslateEvent_DamageDealtEvent_EmptyDamageTypeSkipsRef() {
	evt := events.NewDamageDealtEvent(
		"enc-1", uint64(12),
		"char-A", "goblin-1",
		3, "",
		9, 12,
		map[core.PlayerID]events.DamageDealtSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	dmg := out.GetEntityDamaged()
	s.Require().NotNil(dmg)
	s.Require().Nil(dmg.GetDamageType(), "empty toolkit damage type → nil proto Ref")
}

func (s *TranslateSuite) TestTranslateEvent_DamageDealtEvent_NotVisible_ReturnsErrViewerSawNothing() {
	evt := events.NewDamageDealtEvent(
		"enc-1", uint64(13),
		"goblin-1", "char-A",
		5, "slashing",
		2, 7,
		map[core.PlayerID]events.DamageDealtSlice{
			"player-A": {Visible: false},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_DamageDealtEvent_ViewerNotInPerPlayer_ReturnsErrViewerSawNothing() {
	evt := events.NewDamageDealtEvent(
		"enc-1", uint64(14),
		"goblin-1", "char-A",
		5, "slashing",
		2, 7,
		map[core.PlayerID]events.DamageDealtSlice{
			"player-X": {Visible: true},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_ConditionAppliedEvent_HappyPath_FullyQualifiedRef() {
	evt := events.NewConditionAppliedEvent(
		"enc-1", uint64(20),
		"char-A", "goblin-1",
		"dnd5e:conditions:poisoned", 3,
		map[core.PlayerID]events.ConditionAppliedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)

	app := out.GetStatusApplied()
	s.Require().NotNil(app)
	s.Require().Equal("char-A", app.GetEntityId())
	s.Require().Equal("goblin-1", app.GetSourceEntityId())
	s.Require().NotNil(app.GetStatus())
	s.Require().NotNil(app.GetStatus().GetSource())
	s.Require().Equal("dnd5e", app.GetStatus().GetSource().GetModule())
	s.Require().Equal("conditions", app.GetStatus().GetSource().GetType())
	s.Require().Equal("poisoned", app.GetStatus().GetSource().GetId())
	s.Require().Equal(int32(3), app.GetStatus().GetDurationRounds())
}

func (s *TranslateSuite) TestTranslateEvent_ConditionAppliedEvent_BareIDFallsBackToDefaults() {
	evt := events.NewConditionAppliedEvent(
		"enc-1", uint64(21),
		"char-A", "goblin-1",
		"prone", 0,
		map[core.PlayerID]events.ConditionAppliedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	app := out.GetStatusApplied()
	src := app.GetStatus().GetSource()
	s.Require().Equal("dnd5e", src.GetModule())
	s.Require().Equal("condition", src.GetType())
	s.Require().Equal("prone", src.GetId())
}

func (s *TranslateSuite) TestTranslateEvent_ConditionAppliedEvent_NotVisible_ReturnsErrViewerSawNothing() {
	evt := events.NewConditionAppliedEvent(
		"enc-1", uint64(22),
		"char-A", "goblin-1",
		"dnd5e:conditions:poisoned", 3,
		map[core.PlayerID]events.ConditionAppliedSlice{
			"player-A": {Visible: false},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_ModeChangedEvent_FreeRoamToTurnBased() {
	evt := events.NewModeChangedEvent(
		"enc-1", uint64(30),
		core.ModeFreeRoam, core.ModeTurnBased,
		"ambush",
		map[core.PlayerID]events.ModeChangedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)

	mc := out.GetModeChanged()
	s.Require().NotNil(mc)
	s.Require().Equal(encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM, mc.GetFrom())
	s.Require().Equal(encounterv2pb.EncounterMode_ENCOUNTER_MODE_TURN_BASED, mc.GetTo())
	s.Require().Equal("ambush", mc.GetReason())
}

func (s *TranslateSuite) TestTranslateEvent_ModeChangedEvent_ViewerNotInPerPlayer_ReturnsErrViewerSawNothing() {
	evt := events.NewModeChangedEvent(
		"enc-1", uint64(31),
		core.ModeFreeRoam, core.ModeTurnBased,
		"",
		map[core.PlayerID]events.ModeChangedSlice{
			"player-X": {Visible: true},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_TurnStartedEvent_HappyPath() {
	evt := events.NewTurnStartedEvent(
		"enc-1", uint64(40),
		"char-A", 2,
		map[core.PlayerID]events.TurnStartedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)

	ts := out.GetTurnStarted()
	s.Require().NotNil(ts)
	s.Require().Equal("char-A", ts.GetEntityId())
	s.Require().Equal(int32(2), ts.GetRound())
}

func (s *TranslateSuite) TestTranslateEvent_TurnStartedEvent_ViewerNotInPerPlayer_ReturnsErrViewerSawNothing() {
	evt := events.NewTurnStartedEvent(
		"enc-1", uint64(41),
		"char-A", 1,
		map[core.PlayerID]events.TurnStartedSlice{
			"player-X": {Visible: true},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_TurnEndedEvent_HappyPath() {
	evt := events.NewTurnEndedEvent(
		"enc-1", uint64(50),
		"char-A",
		map[core.PlayerID]events.TurnEndedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)

	te := out.GetTurnEnded()
	s.Require().NotNil(te)
	s.Require().Equal("char-A", te.GetEntityId())
}

func (s *TranslateSuite) TestTranslateEvent_TurnEndedEvent_ViewerNotInPerPlayer_ReturnsErrViewerSawNothing() {
	evt := events.NewTurnEndedEvent(
		"enc-1", uint64(51),
		"char-A",
		map[core.PlayerID]events.TurnEndedSlice{
			"player-X": {Visible: true},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

// EntityDied translator tests --------------------------------------------

func (s *TranslateSuite) TestTranslateEvent_EntityDiedEvent_HappyPath_WithKiller() {
	evt := events.NewEntityDiedEvent(
		"enc-1", uint64(60),
		"goblin-1", "char-A",
		map[core.PlayerID]events.EntityDiedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)

	died := out.GetEntityDied()
	s.Require().NotNil(died, "expected EntityDied envelope")
	s.Require().Equal("goblin-1", died.GetEntityId())
	s.Require().Equal("char-A", died.GetKillerEntityId())
	s.Require().Equal(int64(60), out.Sequence)
}

func (s *TranslateSuite) TestTranslateEvent_EntityDiedEvent_EmptyKiller_OmitsKillerField() {
	// KillerID="" represents environmental damage / indirect kill — the
	// proto optional field should remain unset so clients can distinguish
	// "killed by X" from "killed by something untracked".
	evt := events.NewEntityDiedEvent(
		"enc-1", uint64(61),
		"goblin-1", "",
		map[core.PlayerID]events.EntityDiedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)

	died := out.GetEntityDied()
	s.Require().NotNil(died)
	s.Require().Equal("goblin-1", died.GetEntityId())
	s.Require().Nil(died.KillerEntityId, "empty killer must leave proto field unset (oneof)")
}

func (s *TranslateSuite) TestTranslateEvent_EntityDiedEvent_NotVisible_ReturnsErrViewerSawNothing() {
	// Per-viewer projection: viewer has no LoS to dying entity OR killer →
	// Visible:false → drop on the floor (stream-loop continues silently).
	evt := events.NewEntityDiedEvent(
		"enc-1", uint64(62),
		"goblin-1", "char-A",
		map[core.PlayerID]events.EntityDiedSlice{
			"player-A": {Visible: false},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_EntityDiedEvent_ViewerNotInPerPlayer_ReturnsErrViewerSawNothing() {
	evt := events.NewEntityDiedEvent(
		"enc-1", uint64(63),
		"goblin-1", "char-A",
		map[core.PlayerID]events.EntityDiedSlice{
			"player-X": {Visible: true},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

// EntityRemoved translator tests -----------------------------------------

func (s *TranslateSuite) TestTranslateEvent_EntityRemovedEvent_HappyPath_BroadcastAudience() {
	// Audience is broadcast: every viewer is in PerPlayer with Visible:true.
	evt := events.NewEntityRemovedEvent(
		"enc-1", uint64(70),
		"goblin-1", "destroyed",
		map[core.PlayerID]events.EntityRemovedSlice{
			"player-A": {Visible: true},
			"player-B": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)

	rem := out.GetEntityRemoved()
	s.Require().NotNil(rem, "expected EntityRemoved envelope")
	s.Require().Equal("goblin-1", rem.GetEntityId())
	s.Require().Equal("destroyed", rem.GetReason())
	s.Require().Equal(int64(70), out.Sequence)
}

func (s *TranslateSuite) TestTranslateEvent_EntityRemovedEvent_BothViewersGetEvent() {
	// Same event, two viewers — both must receive a populated envelope.
	evt := events.NewEntityRemovedEvent(
		"enc-1", uint64(71),
		"goblin-1", "destroyed",
		map[core.PlayerID]events.EntityRemovedSlice{
			"player-A": {Visible: true},
			"player-B": {Visible: true},
		},
	)
	outA, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(outA.GetEntityRemoved())

	outB, err := v2encounter.TranslateEvent(evt, "player-B", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(outB.GetEntityRemoved())
}

func (s *TranslateSuite) TestTranslateEvent_EntityRemovedEvent_ViewerNotInPerPlayer_ReturnsErrViewerSawNothing() {
	// Defensive case: toolkit contract is broadcast, so a missing entry
	// would be a toolkit bug. Translator returns ErrViewerSawNothing so the
	// stream loop drops it silently rather than emitting a malformed event.
	evt := events.NewEntityRemovedEvent(
		"enc-1", uint64(72),
		"goblin-1", "destroyed",
		map[core.PlayerID]events.EntityRemovedSlice{
			"player-X": {Visible: true},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

// EncounterEnded translator tests ---------------------------------------

func (s *TranslateSuite) TestTranslateEvent_EncounterEndedEvent_HappyPath() {
	evt := events.NewEncounterEndedEvent(
		"enc-1", uint64(80),
		"all_hostiles_defeated",
		map[core.PlayerID]events.EncounterEndedSlice{
			"player-A": {Visible: true},
			"player-B": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)

	end := out.GetEncounterEnded()
	s.Require().NotNil(end, "expected EncounterEnded envelope")
	s.Require().Equal("all_hostiles_defeated", end.GetReason())
	s.Require().Equal(int64(80), out.Sequence)
}

func (s *TranslateSuite) TestTranslateEvent_EncounterEndedEvent_BothViewersGetEvent() {
	// Broadcast audience — same event reaches every viewer.
	evt := events.NewEncounterEndedEvent(
		"enc-1", uint64(81),
		"all_hostiles_defeated",
		map[core.PlayerID]events.EncounterEndedSlice{
			"player-A": {Visible: true},
			"player-B": {Visible: true},
		},
	)
	outA, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(outA.GetEncounterEnded())
	outB, err := v2encounter.TranslateEvent(evt, "player-B", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(outB.GetEncounterEnded())
}

func (s *TranslateSuite) TestTranslateEvent_EncounterEndedEvent_EmptyReason_PassesThrough() {
	// Reason is documented as a free-form string — empty is valid, the
	// proto field stays empty.
	evt := events.NewEncounterEndedEvent(
		"enc-1", uint64(82),
		"",
		map[core.PlayerID]events.EncounterEndedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(out.GetEncounterEnded())
	s.Require().Equal("", out.GetEncounterEnded().GetReason())
}

func (s *TranslateSuite) TestTranslateEvent_EncounterEndedEvent_ViewerNotInPerPlayer_ReturnsErrViewerSawNothing() {
	evt := events.NewEncounterEndedEvent(
		"enc-1", uint64(83),
		"all_hostiles_defeated",
		map[core.PlayerID]events.EncounterEndedSlice{
			"player-X": {Visible: true},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

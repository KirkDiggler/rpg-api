package encounter_test

import (
	"errors"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/events"
)

// Combat event translator tests — covers the Wave 2.8 additions:
// AttackResolvedEvent (suppressed), DamageDealtEvent, ConditionAppliedEvent,
// ModeChangedEvent, TurnStartedEvent, TurnEndedEvent.

func (s *TranslateSuite) TestTranslateEvent_AttackResolvedEvent_Suppressed() {
	// AttackResolvedEvent is the cause-stage event; the proto wire shape uses
	// EntityDamaged (effect) as the canonical attack-result event. The
	// translator deliberately drops AttackResolved to avoid double-emitting.
	evt := events.NewAttackResolvedEvent(
		"enc-1", uint64(10),
		"char-A", "goblin-1",
		true, false, 18, 4, 15,
		map[core.PlayerID]events.AttackResolvedSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrEventSuppressed),
		"AttackResolvedEvent must return ErrEventSuppressed, got: %v", err)
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

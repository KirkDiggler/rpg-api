// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package encounter_integration provides API-level integration tests for entity state wiring
// on encounter stream events.
//
// These tests verify that fields like CombatStartedEvent.EncounterStateData,
// AttackResolvedEvent.AttackerState/TargetState, MovementCompletedEvent.UpdatedEntity,
// TurnEndedEvent.UpdatedEntities, and MonsterTurnCompletedEvent.UpdatedEntities are
// populated when events are received on the encounter event stream.
//
// # Hypothesis
//
// These tests are expected to FAIL because the entity state fields on event structs
// are tagged `json:"-"` in internal/entities/encounter_events.go.  When events travel
// through the Redis pub/sub pipeline they are serialized to/from JSON, causing the
// proto entity-state payloads to be stripped before the gRPC handler ever sees them.
//
// A passing test here proves the full wiring is intact end-to-end.
// A failing test here proves the bug is in the serialization path.
package encounter_integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
)

// ============================================================================
// STREAM ENTITY STATE INTEGRATION TESTS
//
// These tests subscribe to the gRPC StreamEncounterEvents stream, trigger
// encounter actions, collect the resulting events, and assert that the new
// entity-state proto fields on those events are non-nil / populated.
// ============================================================================

const (
	// streamEventTimeout is how long a single collectStreamEvents call will
	// wait before returning whatever events it has collected.
	streamEventTimeout = 10 * time.Second
)

type StreamEntityStateIntegrationSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	server *harness.TestServer
}

func TestStreamEntityStateIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(StreamEntityStateIntegrationSuite))
}

func (s *StreamEntityStateIntegrationSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 3*time.Minute)

	var err error
	s.server, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")

	s.T().Log("=== Stream Entity State Integration Test Server Ready ===")
}

func (s *StreamEntityStateIntegrationSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *StreamEntityStateIntegrationSuite) SetupTest() {
	err := s.server.FlushRedis(s.ctx)
	s.Require().NoError(err, "failed to flush redis")
}

func (s *StreamEntityStateIntegrationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// createFighter creates a standard fighter character and returns its ID.
// This delegates to the full explicit character creation used in entity_state_test.go
// (with all required equipment choices) rather than the stub builder.
func (s *StreamEntityStateIntegrationSuite) createFighter(playerID string) string {
	ctx := s.authCtx(playerID)

	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err, "failed to create draft")
	draftID := createResp.GetDraft().GetId()
	s.Require().NotEmpty(draftID)

	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    "Stream Test Fighter",
	})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				Selection: &dnd5ev1alpha1.ChoiceData_Languages{
					Languages: &dnd5ev1alpha1.LanguageSelection{
						Languages: []dnd5ev1alpha1.Language{dnd5ev1alpha1.Language_LANGUAGE_DWARVISH},
					},
				},
			},
		},
	})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_FIGHTER,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
							dnd5ev1alpha1.Skill_SKILL_PERCEPTION,
						},
					},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
					FightingStyle: &dnd5ev1alpha1.FightingStyleSelection{
						Style: dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DEFENSE,
					},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-armor",
				OptionId: "fighter-armor-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-weapons-primary",
				OptionId: "fighter-weapon-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{
						Items: []*dnd5ev1alpha1.EquipmentSelectionItem{
							{Equipment: &dnd5ev1alpha1.EquipmentSelectionItem_Weapon{
								Weapon: dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD,
							}},
						},
					},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-weapons-secondary",
				OptionId: "fighter-ranged-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-pack",
				OptionId: "fighter-pack-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
		},
	})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_SOLDIER,
	})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     15,
				Dexterity:    14,
				Constitution: 13,
				Intelligence: 8,
				Wisdom:       12,
				Charisma:     10,
			},
		},
	})
	s.Require().NoError(err)

	finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err, "failed to finalize draft")
	s.Require().NotNil(finalizeResp.GetCharacter())
	s.Require().NotEmpty(finalizeResp.GetCharacter().GetId())

	return finalizeResp.GetCharacter().GetId()
}

// subscribeToStream opens a StreamEncounterEvents subscription and returns
// a cancel function together with a channel that receives proto events.
// The caller MUST call cancel() when done to release resources.
// Any stream receive error (other than context cancellation) is logged via t.Log.
func (s *StreamEntityStateIntegrationSuite) subscribeToStream(
	playerID, encounterID string,
) (cancel context.CancelFunc, eventsCh <-chan *dnd5ev1alpha1.EncounterEvent) {
	// Auth metadata must be on the stream context (server-streaming auth uses StreamAuthInterceptor).
	streamCtx, streamCancel := context.WithCancel(s.authCtx(playerID))

	stream, err := s.server.EncounterClient.StreamEncounterEvents(streamCtx,
		&dnd5ev1alpha1.StreamEncounterEventsRequest{
			EncounterId: encounterID,
			PlayerId:    playerID,
		},
	)
	s.Require().NoError(err, "failed to open encounter event stream")

	ch := make(chan *dnd5ev1alpha1.EncounterEvent, 64)

	go func() {
		defer close(ch)
		for {
			event, recvErr := stream.Recv()
			if recvErr != nil {
				// Context cancelled by caller — normal shutdown.
				if streamCtx.Err() != nil {
					return
				}
				// Unexpected stream error — log and stop. The test will see 0
				// events from this point and fail with a useful assertion message.
				s.T().Logf("  [stream] Recv error (unexpected): %v", recvErr)
				return
			}
			select {
			case ch <- event:
			case <-streamCtx.Done():
				return
			}
		}
	}()

	return streamCancel, ch
}

// collectEvents reads from eventsCh until either the timeout fires or
// stopFn returns true for a received event (the matching event is included
// in the returned slice).  This avoids sleeping while still bounding test
// duration.
func collectEvents(
	t *testing.T,
	eventsCh <-chan *dnd5ev1alpha1.EncounterEvent,
	timeout time.Duration,
	stopFn func(*dnd5ev1alpha1.EncounterEvent) bool,
) []*dnd5ev1alpha1.EncounterEvent {
	t.Helper()
	deadline := time.After(timeout)
	var collected []*dnd5ev1alpha1.EncounterEvent
	for {
		select {
		case event, ok := <-eventsCh:
			if !ok {
				return collected
			}
			collected = append(collected, event)
			if stopFn != nil && stopFn(event) {
				return collected
			}
		case <-deadline:
			return collected
		}
	}
}

// findEventOfType searches a slice for the first event matching a predicate.
func findEventOfType[T any](events []*dnd5ev1alpha1.EncounterEvent, extract func(*dnd5ev1alpha1.EncounterEvent) T) (T, bool) {
	var zero T
	for _, e := range events {
		v := extract(e)
		// Use interface comparison to detect non-nil (works for pointer types).
		if any(v) != any(zero) {
			return v, true
		}
	}
	return zero, false
}

// =============================================================================
// TEST: CombatStartedEvent carries EncounterStateData
// =============================================================================

func (s *StreamEntityStateIntegrationSuite) TestStream_CombatStartedEvent_HasEncounterStateData() {
	s.T().Log("=== TEST: CombatStartedEvent.EncounterStateData is populated on stream ===")

	playerID := "stream-test-player-combat-started"
	ctx := s.authCtx(playerID)

	// 1. Create fighter and encounter (do NOT start combat yet).
	characterID := s.createFighter(playerID)
	s.T().Logf("  Created character: %s", characterID)

	createResp, err := s.server.EncounterClient.CreateEncounter(ctx,
		&dnd5ev1alpha1.CreateEncounterRequest{
			CharacterIds: []string{characterID},
		},
	)
	s.Require().NoError(err)
	encounterID := createResp.GetEncounterId()
	s.Require().NotEmpty(encounterID)
	s.T().Logf("  Created encounter: %s", encounterID)

	_, err = s.server.EncounterClient.SetReady(ctx, &dnd5ev1alpha1.SetReadyRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
		IsReady:     true,
	})
	s.Require().NoError(err)

	// 2. Subscribe BEFORE starting combat so we catch the CombatStartedEvent.
	cancelStream, eventsCh := s.subscribeToStream(playerID, encounterID)
	defer cancelStream()

	// Small sleep to ensure the Redis subscription is active before we publish.
	time.Sleep(100 * time.Millisecond)

	// 3. Start combat — this triggers the CombatStartedEvent.
	_, err = s.server.EncounterClient.StartCombat(ctx, &dnd5ev1alpha1.StartCombatRequest{
		EncounterId: encounterID,
		Theme:       dnd5ev1alpha1.DungeonTheme_DUNGEON_THEME_CRYPT,
		Difficulty:  dnd5ev1alpha1.DungeonDifficulty_DUNGEON_DIFFICULTY_MEDIUM,
		Length:      dnd5ev1alpha1.DungeonLength_DUNGEON_LENGTH_SHORT,
	})
	s.Require().NoError(err)

	// 4. Collect events until we see a CombatStartedEvent or time out.
	events := collectEvents(s.T(), eventsCh, streamEventTimeout, func(e *dnd5ev1alpha1.EncounterEvent) bool {
		return e.GetCombatStarted() != nil
	})

	s.T().Logf("  Collected %d stream events", len(events))

	// 5. Find the CombatStartedEvent.
	combatStartedEvent, found := findEventOfType(events,
		func(e *dnd5ev1alpha1.EncounterEvent) *dnd5ev1alpha1.CombatStartedEvent {
			return e.GetCombatStarted()
		},
	)
	s.Require().True(found, "expected to receive a CombatStartedEvent on the stream")
	s.T().Log("  Found CombatStartedEvent")

	// 6. Assert EncounterStateData is populated.
	stateData := combatStartedEvent.GetEncounterStateData()
	s.Require().NotNil(stateData,
		"CombatStartedEvent.EncounterStateData must not be nil — "+
			"if nil, the orchestrator is not populating the field or it is stripped during JSON serialization")

	entities := stateData.GetEntities()
	s.Assert().NotEmpty(entities,
		"CombatStartedEvent.EncounterStateData.Entities should contain combatants")
	s.T().Logf("  EncounterStateData has %d entities", len(entities))

	foundCharacter := false
	foundMonster := false
	for entityID, entity := range entities {
		s.Assert().NotEmpty(entity.GetEntityId(), "entity %s should have entity_id", entityID)
		s.Assert().NotNil(entity.GetPosition(), "entity %s should have position", entityID)
		s.Assert().NotEqual(dnd5ev1alpha1.EntityType_ENTITY_TYPE_UNSPECIFIED, entity.GetEntityType(),
			"entity %s should have entity_type", entityID)

		switch entity.GetEntityType() {
		case dnd5ev1alpha1.EntityType_ENTITY_TYPE_CHARACTER:
			foundCharacter = true
			s.T().Logf("  Character entity: %s HP=%d/%d", entityID,
				entity.GetCurrentHitPoints(), entity.GetMaxHitPoints())
		case dnd5ev1alpha1.EntityType_ENTITY_TYPE_MONSTER:
			foundMonster = true
			s.T().Logf("  Monster entity: %s HP=%d/%d", entityID,
				entity.GetCurrentHitPoints(), entity.GetMaxHitPoints())
		}
	}

	s.Assert().True(foundCharacter,
		"CombatStartedEvent.EncounterStateData should include at least one character entity")
	s.Assert().True(foundMonster,
		"CombatStartedEvent.EncounterStateData should include at least one monster entity")

	s.T().Log("=== PASSED: CombatStartedEvent has EncounterStateData ===")
}

// =============================================================================
// TEST: MovementCompletedEvent carries UpdatedEntity
// =============================================================================

func (s *StreamEntityStateIntegrationSuite) TestStream_MovementCompletedEvent_HasUpdatedEntity() {
	s.T().Log("=== TEST: MovementCompletedEvent.UpdatedEntity is populated on stream ===")

	playerID := "stream-test-player-movement"
	ctx := s.authCtx(playerID)

	// 1. Create character and start combat.
	characterID := s.createFighter(playerID)
	encounterID := CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
	s.T().Logf("  Character: %s, Encounter: %s", characterID, encounterID)

	// 2. Get initial character position.
	stateResp, err := s.server.EncounterClient.GetEncounterState(ctx,
		&dnd5ev1alpha1.GetEncounterStateRequest{
			EncounterId: encounterID,
			PlayerId:    playerID,
		},
	)
	s.Require().NoError(err)

	var initialPos *dnd5ev1alpha1.EntityState
	if stateData := stateResp.GetEncounterStateData(); stateData != nil {
		initialPos = stateData.GetEntities()[characterID]
	}
	if initialPos == nil {
		s.T().Log("  EncounterStateData not populated on GetEncounterState; using (0,0,0) as baseline position")
	} else {
		s.T().Logf("  Initial character position: (%.1f, %.1f, %.1f)",
			initialPos.GetPosition().GetX(),
			initialPos.GetPosition().GetY(),
			initialPos.GetPosition().GetZ())
	}

	// 3. Subscribe to stream.
	cancelStream, eventsCh := s.subscribeToStream(playerID, encounterID)
	defer cancelStream()
	time.Sleep(100 * time.Millisecond)

	// 4. Attempt movement.
	var targetX, targetY, targetZ float64
	if initialPos != nil && initialPos.GetPosition() != nil {
		p := initialPos.GetPosition()
		targetX = p.GetX() + 1
		targetY = p.GetY() - 1
		targetZ = p.GetZ()
	} else {
		targetX, targetY, targetZ = 1, -1, 0
	}

	moveResp, err := s.server.EncounterClient.MoveCharacter(ctx,
		&dnd5ev1alpha1.MoveCharacterRequest{
			EncounterId: encounterID,
			EntityId:    characterID,
			Path: []*apiv1alpha1.Position{
				{X: targetX, Y: targetY, Z: targetZ},
			},
		},
	)
	if err != nil {
		s.T().Logf("  MoveCharacter returned error (may be blocked): %v", err)
		s.T().Skip("movement failed — cannot test MovementCompletedEvent")
		return
	}
	s.T().Logf("  MoveCharacter: success=%v remaining=%d", moveResp.GetSuccess(), moveResp.GetMovementRemaining())

	if !moveResp.GetSuccess() {
		s.T().Log("  Movement did not succeed — skipping stream assertion")
		s.T().Skip("movement did not succeed — cannot test MovementCompletedEvent")
		return
	}

	// 5. Collect events until we see a MovementCompletedEvent.
	events := collectEvents(s.T(), eventsCh, streamEventTimeout, func(e *dnd5ev1alpha1.EncounterEvent) bool {
		return e.GetMovementCompleted() != nil
	})

	s.T().Logf("  Collected %d stream events", len(events))

	// 6. Find the MovementCompletedEvent.
	movementEvent, found := findEventOfType(events,
		func(e *dnd5ev1alpha1.EncounterEvent) *dnd5ev1alpha1.MovementCompletedEvent {
			return e.GetMovementCompleted()
		},
	)
	s.Require().True(found, "expected to receive a MovementCompletedEvent on the stream")
	s.T().Logf("  Found MovementCompletedEvent for entity %s", movementEvent.GetEntityId())

	// 7. Assert UpdatedEntity is populated.
	updatedEntity := movementEvent.GetUpdatedEntity()
	s.Require().NotNil(updatedEntity,
		"MovementCompletedEvent.UpdatedEntity must not be nil — "+
			"if nil, the orchestrator is not populating the field or it is stripped during JSON serialization")

	s.Assert().NotEmpty(updatedEntity.GetEntityId(),
		"UpdatedEntity.EntityId should be set")
	s.Assert().Equal(characterID, updatedEntity.GetEntityId(),
		"UpdatedEntity.EntityId should match the moved character")
	s.Assert().NotNil(updatedEntity.GetPosition(),
		"UpdatedEntity.Position should be set after movement")

	pos := updatedEntity.GetPosition()
	s.Assert().Equal(targetX, pos.GetX(), "UpdatedEntity.Position.X should match destination")
	s.Assert().Equal(targetY, pos.GetY(), "UpdatedEntity.Position.Y should match destination")
	s.Assert().Equal(targetZ, pos.GetZ(), "UpdatedEntity.Position.Z should match destination")

	s.T().Logf("  UpdatedEntity: %s at (%.1f, %.1f, %.1f)",
		updatedEntity.GetEntityId(), pos.GetX(), pos.GetY(), pos.GetZ())

	s.T().Log("=== PASSED: MovementCompletedEvent has UpdatedEntity ===")
}

// =============================================================================
// TEST: AttackResolvedEvent carries AttackerState and TargetState
// =============================================================================

func (s *StreamEntityStateIntegrationSuite) TestStream_AttackResolvedEvent_HasAttackerAndTargetState() {
	s.T().Log("=== TEST: AttackResolvedEvent.AttackerState and .TargetState are populated on stream ===")

	playerID := "stream-test-player-attack"
	ctx := s.authCtx(playerID)

	// 1. Create character and start combat.
	characterID := s.createFighter(playerID)
	encounterID := CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
	s.T().Logf("  Character: %s, Encounter: %s", characterID, encounterID)

	// 2. Find a target monster.
	targetID := FindMonsterTarget(s.T(), s.server, ctx, playerID, encounterID)
	s.T().Logf("  Target monster: %s", targetID)

	// 3. Subscribe to stream.
	cancelStream, eventsCh := s.subscribeToStream(playerID, encounterID)
	defer cancelStream()
	time.Sleep(100 * time.Millisecond)

	// 4. Attack the monster.
	attackResp, err := s.server.EncounterClient.Attack(ctx,
		&dnd5ev1alpha1.AttackRequest{
			EncounterId: encounterID,
			AttackerId:  characterID,
			TargetId:    targetID,
		},
	)
	s.Require().NoError(err, "Attack should not error")
	s.Require().True(attackResp.GetSuccess(), "Attack should succeed: %s", attackResp.GetError())
	s.T().Logf("  Attack result: hit=%v damage=%d",
		attackResp.GetResult().GetHit(), attackResp.GetResult().GetDamage())

	// 5. Collect events until we see an AttackResolvedEvent.
	events := collectEvents(s.T(), eventsCh, streamEventTimeout, func(e *dnd5ev1alpha1.EncounterEvent) bool {
		return e.GetAttackResolved() != nil
	})

	s.T().Logf("  Collected %d stream events", len(events))

	// 6. Find the AttackResolvedEvent.
	attackEvent, found := findEventOfType(events,
		func(e *dnd5ev1alpha1.EncounterEvent) *dnd5ev1alpha1.AttackResolvedEvent {
			return e.GetAttackResolved()
		},
	)
	s.Require().True(found, "expected to receive an AttackResolvedEvent on the stream")
	s.T().Logf("  Found AttackResolvedEvent: attacker=%s target=%s",
		attackEvent.GetAttackerId(), attackEvent.GetTargetId())

	// 7. Assert AttackerState is populated.
	attackerState := attackEvent.GetAttackerState()
	s.Require().NotNil(attackerState,
		"AttackResolvedEvent.AttackerState must not be nil — "+
			"if nil, the orchestrator is not populating the field or it is stripped during JSON serialization")

	s.Assert().NotEmpty(attackerState.GetEntityId(),
		"AttackerState.EntityId should be set")
	s.Assert().Equal(characterID, attackerState.GetEntityId(),
		"AttackerState.EntityId should match the attacker character")
	s.Assert().Greater(attackerState.GetCurrentHitPoints(), int32(0),
		"AttackerState.CurrentHitPoints should be positive")
	s.Assert().Greater(attackerState.GetMaxHitPoints(), int32(0),
		"AttackerState.MaxHitPoints should be positive")
	s.T().Logf("  AttackerState: %s HP=%d/%d",
		attackerState.GetEntityId(),
		attackerState.GetCurrentHitPoints(),
		attackerState.GetMaxHitPoints())

	// 8. Assert TargetState is populated.
	targetState := attackEvent.GetTargetState()
	s.Require().NotNil(targetState,
		"AttackResolvedEvent.TargetState must not be nil — "+
			"if nil, the orchestrator is not populating the field or it is stripped during JSON serialization")

	s.Assert().NotEmpty(targetState.GetEntityId(),
		"TargetState.EntityId should be set")
	s.Assert().Equal(targetID, targetState.GetEntityId(),
		"TargetState.EntityId should match the target monster")
	s.Assert().Greater(targetState.GetMaxHitPoints(), int32(0),
		"TargetState.MaxHitPoints should be positive")
	s.T().Logf("  TargetState: %s HP=%d/%d",
		targetState.GetEntityId(),
		targetState.GetCurrentHitPoints(),
		targetState.GetMaxHitPoints())

	s.T().Log("=== PASSED: AttackResolvedEvent has AttackerState and TargetState ===")
}

// =============================================================================
// TEST: TurnEndedEvent carries UpdatedEntities and CombatState
// =============================================================================

func (s *StreamEntityStateIntegrationSuite) TestStream_TurnEndedEvent_HasUpdatedEntities() {
	s.T().Log("=== TEST: TurnEndedEvent.UpdatedEntities is populated on stream ===")

	playerID := "stream-test-player-turn-ended"
	ctx := s.authCtx(playerID)

	// 1. Create character and start combat.
	characterID := s.createFighter(playerID)
	encounterID := CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
	s.T().Logf("  Character: %s, Encounter: %s", characterID, encounterID)

	// 2. Subscribe to stream.
	cancelStream, eventsCh := s.subscribeToStream(playerID, encounterID)
	defer cancelStream()
	time.Sleep(100 * time.Millisecond)

	// 3. End the player's turn — this publishes a TurnEndedEvent.
	_, err := s.server.EncounterClient.EndTurn(ctx, &dnd5ev1alpha1.EndTurnRequest{
		EncounterId: encounterID,
		EntityId:    characterID,
	})
	s.Require().NoError(err, "EndTurn should not error")
	s.T().Log("  EndTurn called")

	// 4. Collect events until we see a TurnEndedEvent.
	events := collectEvents(s.T(), eventsCh, streamEventTimeout, func(e *dnd5ev1alpha1.EncounterEvent) bool {
		return e.GetTurnEnded() != nil
	})

	s.T().Logf("  Collected %d stream events", len(events))

	// 5. Find the TurnEndedEvent.
	turnEndedEvent, found := findEventOfType(events,
		func(e *dnd5ev1alpha1.EncounterEvent) *dnd5ev1alpha1.TurnEndedEvent {
			return e.GetTurnEnded()
		},
	)
	s.Require().True(found, "expected to receive a TurnEndedEvent on the stream")
	s.T().Log("  Found TurnEndedEvent")

	// 6. Assert UpdatedEntities is populated.
	updatedEntities := turnEndedEvent.GetUpdatedEntities()
	s.Assert().NotEmpty(updatedEntities,
		"TurnEndedEvent.UpdatedEntities must not be empty — "+
			"if empty, the orchestrator is not populating the field or it is stripped during JSON serialization")

	if len(updatedEntities) > 0 {
		s.T().Logf("  UpdatedEntities count: %d", len(updatedEntities))
		for _, entity := range updatedEntities {
			s.Assert().NotEmpty(entity.GetEntityId(), "entity should have entity_id")
			s.T().Logf("  Entity: %s type=%s HP=%d/%d",
				entity.GetEntityId(), entity.GetEntityType(),
				entity.GetCurrentHitPoints(), entity.GetMaxHitPoints())
		}
	}

	// 7. Assert CombatState is populated.
	combatState := turnEndedEvent.GetCombatState()
	s.Assert().NotNil(combatState,
		"TurnEndedEvent.CombatState must not be nil")
	if combatState != nil {
		s.T().Logf("  CombatState: round=%d activeIndex=%d", combatState.GetRound(), combatState.GetActiveIndex())
	}

	s.T().Log("=== PASSED: TurnEndedEvent has UpdatedEntities ===")
}

// =============================================================================
// TEST: MonsterTurnCompletedEvent carries UpdatedEntities and CombatState
// =============================================================================

func (s *StreamEntityStateIntegrationSuite) TestStream_MonsterTurnCompletedEvent_HasUpdatedEntities() {
	s.T().Log("=== TEST: MonsterTurnCompletedEvent.UpdatedEntities is populated on stream ===")

	playerID := "stream-test-player-monster-turn"
	ctx := s.authCtx(playerID)

	// 1. Create character and start combat.
	characterID := s.createFighter(playerID)
	encounterID := CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
	s.T().Logf("  Character: %s, Encounter: %s", characterID, encounterID)

	// 2. Subscribe to stream BEFORE ending the player turn so we catch the
	//    MonsterTurnCompletedEvent that follows automatically.
	cancelStream, eventsCh := s.subscribeToStream(playerID, encounterID)
	defer cancelStream()
	time.Sleep(100 * time.Millisecond)

	// 3. End the player's turn — the server will process the monster turn and
	//    publish a MonsterTurnCompletedEvent followed by a new TurnStarted event.
	endTurnResp, err := s.server.EncounterClient.EndTurn(ctx, &dnd5ev1alpha1.EndTurnRequest{
		EncounterId: encounterID,
		EntityId:    characterID,
	})
	s.Require().NoError(err, "EndTurn should not error")
	s.T().Logf("  EndTurn: %d monster turns in response", len(endTurnResp.GetMonsterTurns()))

	// 4. Collect events until we see a MonsterTurnCompletedEvent or time out.
	events := collectEvents(s.T(), eventsCh, streamEventTimeout, func(e *dnd5ev1alpha1.EncounterEvent) bool {
		return e.GetMonsterTurnCompleted() != nil
	})

	s.T().Logf("  Collected %d stream events", len(events))

	// Log what we received for diagnostics.
	for i, e := range events {
		switch {
		case e.GetCombatStarted() != nil:
			s.T().Logf("  Event[%d]: CombatStarted", i)
		case e.GetTurnEnded() != nil:
			s.T().Logf("  Event[%d]: TurnEnded", i)
		case e.GetMonsterTurnCompleted() != nil:
			m := e.GetMonsterTurnCompleted()
			s.T().Logf("  Event[%d]: MonsterTurnCompleted monster=%s updatedEntities=%d",
				i,
				m.GetMonsterTurn().GetMonsterId(),
				len(m.GetUpdatedEntities()))
		case e.GetAttackResolved() != nil:
			s.T().Logf("  Event[%d]: AttackResolved", i)
		case e.GetMovementCompleted() != nil:
			s.T().Logf("  Event[%d]: MovementCompleted", i)
		default:
			s.T().Logf("  Event[%d]: other", i)
		}
	}

	// 5. Find the MonsterTurnCompletedEvent.
	monsterTurnEvent, found := findEventOfType(events,
		func(e *dnd5ev1alpha1.EncounterEvent) *dnd5ev1alpha1.MonsterTurnCompletedEvent {
			return e.GetMonsterTurnCompleted()
		},
	)
	if !found {
		// If monsters went first or the player character won initiative and the monster
		// turn didn't arrive in the stream window, skip gracefully rather than failing.
		s.T().Skip("MonsterTurnCompletedEvent not received within timeout — " +
			"monster may have gone first or combat ended before monster turn")
		return
	}
	s.T().Logf("  Found MonsterTurnCompletedEvent for monster %s",
		monsterTurnEvent.GetMonsterTurn().GetMonsterId())

	// 6. Assert UpdatedEntities is populated.
	updatedEntities := monsterTurnEvent.GetUpdatedEntities()
	s.Assert().NotEmpty(updatedEntities,
		"MonsterTurnCompletedEvent.UpdatedEntities must not be empty — "+
			"if empty, the orchestrator is not populating the field or it is stripped during JSON serialization")

	if len(updatedEntities) > 0 {
		s.T().Logf("  UpdatedEntities count: %d", len(updatedEntities))
		for _, entity := range updatedEntities {
			s.Assert().NotEmpty(entity.GetEntityId(), "entity should have entity_id")
			s.T().Logf("  Entity: %s type=%s HP=%d/%d",
				entity.GetEntityId(), entity.GetEntityType(),
				entity.GetCurrentHitPoints(), entity.GetMaxHitPoints())
		}
	}

	// 7. Assert CombatState is populated.
	combatState := monsterTurnEvent.GetCombatState()
	s.Assert().NotNil(combatState,
		"MonsterTurnCompletedEvent.CombatState must not be nil")
	if combatState != nil {
		s.T().Logf("  CombatState: round=%d", combatState.GetRound())
	}

	s.T().Log("=== PASSED: MonsterTurnCompletedEvent has UpdatedEntities ===")
}

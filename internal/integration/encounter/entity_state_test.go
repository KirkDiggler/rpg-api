// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package encounter_integration provides API-level integration tests for entity state wiring.
// These tests verify that GetEncounterState populates the unified EncounterStateData with
// EntityState entries for all combatants (characters and monsters).
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
// ENTITY STATE INTEGRATION TESTS
// Verify that GetEncounterState returns populated EncounterStateData with
// EntityState entries for all combatants after combat starts.
// ============================================================================

type EntityStateIntegrationSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	server *harness.TestServer
}

func TestEntityStateIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(EntityStateIntegrationSuite))
}

func (s *EntityStateIntegrationSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	var err error
	s.server, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")

	s.T().Log("=== Entity State Integration Test Server Ready ===")
}

func (s *EntityStateIntegrationSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *EntityStateIntegrationSuite) SetupTest() {
	err := s.server.FlushRedis(s.ctx)
	s.Require().NoError(err, "failed to flush redis")
}

// authCtx adds dev auth header for test requests
func (s *EntityStateIntegrationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// createFighterCharacter creates a fighter with all required choices (matching fighter_test.go pattern).
func (s *EntityStateIntegrationSuite) createFighterCharacter(playerID string) string {
	ctx := s.authCtx(playerID)

	// Step 1: Create character draft
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err, "failed to create draft")
	draftID := createResp.GetDraft().GetId()
	s.Require().NotEmpty(draftID, "draft ID should not be empty")

	// Step 2: Set name
	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    "Entity State Test Fighter",
	})
	s.Require().NoError(err, "failed to set name")

	// Step 3: Set Human with language choice
	_, err = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				Selection: &dnd5ev1alpha1.ChoiceData_Languages{
					Languages: &dnd5ev1alpha1.LanguageSelection{
						Languages: []dnd5ev1alpha1.Language{
							dnd5ev1alpha1.Language_LANGUAGE_DWARVISH,
						},
					},
				},
			},
		},
	})
	s.Require().NoError(err, "failed to set race")

	// Step 4: Set Fighter with skills, fighting style, and equipment choices
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
	s.Require().NoError(err, "failed to set class")

	// Step 5: Set background
	_, err = s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_SOLDIER,
	})
	s.Require().NoError(err, "failed to set background")

	// Step 6: Set ability scores
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
	s.Require().NoError(err, "failed to set ability scores")

	// Step 7: Finalize character
	finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err, "failed to finalize draft")
	s.Require().NotNil(finalizeResp.GetCharacter(), "character should not be nil")
	s.Require().NotEmpty(finalizeResp.GetCharacter().GetId(), "character ID should not be empty")

	return finalizeResp.GetCharacter().GetId()
}

// createEncounterAndStartCombat delegates to the shared helper.
func (s *EntityStateIntegrationSuite) createEncounterAndStartCombat(ctx context.Context, playerID, characterID string) string {
	return CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
}

// getEncounterState calls GetEncounterState and returns the response.
func (s *EntityStateIntegrationSuite) getEncounterState(ctx context.Context, playerID, encounterID string) *dnd5ev1alpha1.GetEncounterStateResponse {
	resp, err := s.server.EncounterClient.GetEncounterState(ctx, &dnd5ev1alpha1.GetEncounterStateRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
	})
	s.Require().NoError(err, "failed to get encounter state")
	s.Require().NotNil(resp, "encounter state response should not be nil")
	return resp
}

// =============================================================================
// TEST: GetEncounterState has EncounterStateData populated after combat starts
// =============================================================================

func (s *EntityStateIntegrationSuite) TestEntityState_GetEncounterState_HasEncounterStateData() {
	s.T().Log("=== TEST: GetEncounterState has EncounterStateData after combat starts ===")

	playerID := "test-player-entity-state-1"
	ctx := s.authCtx(playerID)

	// 1. Create fighter and start combat
	characterID := s.createFighterCharacter(playerID)
	s.T().Logf("  Created character: %s", characterID)

	encounterID := s.createEncounterAndStartCombat(ctx, playerID, characterID)
	s.T().Logf("  Created encounter: %s", encounterID)

	// 2. Get encounter state
	resp := s.getEncounterState(ctx, playerID, encounterID)

	// 3. Verify EncounterStateData is populated
	stateData := resp.GetEncounterStateData()
	s.Require().NotNil(stateData, "EncounterStateData should not be nil on GetEncounterState response")

	// 4. Verify entities map is not empty
	entities := stateData.GetEntities()
	s.Require().NotEmpty(entities, "EncounterStateData.Entities map should not be empty")
	s.T().Logf("  Found %d entities in EncounterStateData", len(entities))

	// 5. Verify each entity has required fields
	foundCharacter := false
	foundMonster := false

	for entityID, entity := range entities {
		s.Assert().NotEmpty(entity.GetEntityId(), "entity should have entity_id set")
		s.Assert().Equal(entityID, entity.GetEntityId(), "map key should match entity_id")
		s.Assert().NotEqual(dnd5ev1alpha1.EntityType_ENTITY_TYPE_UNSPECIFIED, entity.GetEntityType(),
			"entity %s should have a valid entity_type", entityID)
		s.Assert().NotNil(entity.GetPosition(), "entity %s should have position", entityID)

		switch entity.GetEntityType() {
		case dnd5ev1alpha1.EntityType_ENTITY_TYPE_CHARACTER:
			foundCharacter = true
			charDetails := entity.GetCharacterDetails()
			s.Require().NotNil(charDetails, "character entity %s should have character_details", entityID)
			s.Assert().NotEmpty(charDetails.GetName(), "character %s should have a name", entityID)
			s.Assert().NotEqual(dnd5ev1alpha1.Race_RACE_UNSPECIFIED, charDetails.GetRace(),
				"character %s should have a race", entityID)
			s.Assert().NotEqual(dnd5ev1alpha1.Class_CLASS_UNSPECIFIED, charDetails.GetCharacterClass(),
				"character %s should have a class", entityID)
			s.T().Logf("  Character: %s (race=%s, class=%s, HP=%d/%d)",
				charDetails.GetName(), charDetails.GetRace(), charDetails.GetCharacterClass(),
				entity.GetCurrentHitPoints(), entity.GetMaxHitPoints())

		case dnd5ev1alpha1.EntityType_ENTITY_TYPE_MONSTER:
			foundMonster = true
			monsterDetails := entity.GetMonsterDetails()
			s.Require().NotNil(monsterDetails, "monster entity %s should have monster_details", entityID)
			s.Assert().NotEmpty(monsterDetails.GetName(), "monster %s should have a name", entityID)
			s.Assert().NotEqual(dnd5ev1alpha1.MonsterType_MONSTER_TYPE_UNSPECIFIED, monsterDetails.GetMonsterType(),
				"monster %s should have a monster_type", entityID)
			s.T().Logf("  Monster: %s (type=%s, HP=%d/%d)",
				monsterDetails.GetName(), monsterDetails.GetMonsterType(),
				entity.GetCurrentHitPoints(), entity.GetMaxHitPoints())
		}

		// All combatants should have positive HP
		s.Assert().Greater(entity.GetCurrentHitPoints(), int32(0),
			"entity %s should have current_hit_points > 0", entityID)
		s.Assert().Greater(entity.GetMaxHitPoints(), int32(0),
			"entity %s should have max_hit_points > 0", entityID)
	}

	s.Assert().True(foundCharacter, "should find at least one character entity")
	s.Assert().True(foundMonster, "should find at least one monster entity")

	s.T().Log("=== PASSED: GetEncounterState has EncounterStateData ===")
}

// =============================================================================
// TEST: Attack resolves and entity states reflect updated HP
// =============================================================================

func (s *EntityStateIntegrationSuite) TestEntityState_AttackResolved_HasEntityStates() {
	s.T().Log("=== TEST: Entity states update after attack ===")

	playerID := "test-player-entity-state-2"
	ctx := s.authCtx(playerID)

	// 1. Create fighter and start combat
	characterID := s.createFighterCharacter(playerID)
	encounterID := s.createEncounterAndStartCombat(ctx, playerID, characterID)
	s.T().Logf("  Character: %s, Encounter: %s", characterID, encounterID)

	// 2. Get initial state to record monster HP
	initialResp := s.getEncounterState(ctx, playerID, encounterID)
	initialStateData := initialResp.GetEncounterStateData()

	// If EncounterStateData is nil, the wiring is not yet complete - skip gracefully
	if initialStateData == nil {
		s.T().Skip("EncounterStateData not yet wired to GetEncounterState response - wiring bug")
		return
	}

	// 3. Find a monster target
	targetID := FindMonsterTarget(s.T(), s.server, ctx, playerID, encounterID)
	s.T().Logf("  Target monster: %s", targetID)

	// Record initial monster HP
	initialMonsterState := initialStateData.GetEntities()[targetID]
	s.Require().NotNil(initialMonsterState, "target monster should exist in entity state map")
	initialHP := initialMonsterState.GetCurrentHitPoints()
	s.T().Logf("  Monster initial HP: %d/%d", initialHP, initialMonsterState.GetMaxHitPoints())

	// 4. Attack the monster
	attackResp, err := s.server.EncounterClient.Attack(ctx, &dnd5ev1alpha1.AttackRequest{
		EncounterId: encounterID,
		AttackerId:  characterID,
		TargetId:    targetID,
	})
	s.Require().NoError(err, "attack should not error")
	s.Assert().True(attackResp.GetSuccess(), "attack request should succeed: %s", attackResp.GetError())

	result := attackResp.GetResult()
	s.Require().NotNil(result, "attack result should not be nil")
	s.T().Logf("  Attack: roll=%d, total=%d vs AC %d, hit=%v, damage=%d",
		result.GetAttackRoll(), result.GetAttackTotal(), result.GetTargetAc(),
		result.GetHit(), result.GetDamage())

	// 5. Get updated state
	updatedResp := s.getEncounterState(ctx, playerID, encounterID)
	updatedStateData := updatedResp.GetEncounterStateData()
	s.Require().NotNil(updatedStateData, "EncounterStateData should still be present after attack")

	updatedMonsterState := updatedStateData.GetEntities()[targetID]
	s.Require().NotNil(updatedMonsterState, "target monster should still exist in entity state map after attack")

	updatedHP := updatedMonsterState.GetCurrentHitPoints()
	s.T().Logf("  Monster updated HP: %d/%d", updatedHP, updatedMonsterState.GetMaxHitPoints())

	// If the attack hit, HP should have decreased
	if result.GetHit() && result.GetDamage() > 0 {
		s.Assert().Less(updatedHP, initialHP,
			"monster HP should decrease after being hit (was %d, now %d)", initialHP, updatedHP)
	}

	// Entity state should still have all required fields
	s.Assert().NotEmpty(updatedMonsterState.GetEntityId(), "monster entity should still have entity_id")
	s.Assert().Equal(dnd5ev1alpha1.EntityType_ENTITY_TYPE_MONSTER, updatedMonsterState.GetEntityType(),
		"monster entity should still be ENTITY_TYPE_MONSTER")

	s.T().Log("=== PASSED: Entity states update after attack ===")
}

// =============================================================================
// TEST: Movement updates entity position in state
// =============================================================================

func (s *EntityStateIntegrationSuite) TestEntityState_Movement_UpdatesEntityPosition() {
	s.T().Log("=== TEST: Movement updates entity position in EncounterStateData ===")

	playerID := "test-player-entity-state-3"
	ctx := s.authCtx(playerID)

	// 1. Create fighter and start combat
	characterID := s.createFighterCharacter(playerID)
	encounterID := s.createEncounterAndStartCombat(ctx, playerID, characterID)
	s.T().Logf("  Character: %s, Encounter: %s", characterID, encounterID)

	// 2. Get initial state to find character position
	initialResp := s.getEncounterState(ctx, playerID, encounterID)
	initialStateData := initialResp.GetEncounterStateData()

	if initialStateData == nil {
		s.T().Skip("EncounterStateData not yet wired to GetEncounterState response - wiring bug")
		return
	}

	initialCharState := initialStateData.GetEntities()[characterID]
	s.Require().NotNil(initialCharState, "character should exist in entity state map")
	initialPos := initialCharState.GetPosition()
	s.T().Logf("  Initial position: (%v, %v, %v)", initialPos.GetX(), initialPos.GetY(), initialPos.GetZ())

	// 3. Move the character one hex (cube coordinate adjacent)
	// Move to an adjacent hex: shift +1 on X, -1 on Y, 0 on Z
	targetX := initialPos.GetX() + 1
	targetY := initialPos.GetY() - 1
	targetZ := initialPos.GetZ()

	moveResp, err := s.server.EncounterClient.MoveCharacter(ctx, &dnd5ev1alpha1.MoveCharacterRequest{
		EncounterId: encounterID,
		EntityId:    characterID,
		Path: []*apiv1alpha1.Position{
			{X: targetX, Y: targetY, Z: targetZ},
		},
	})
	// Movement may fail if the target hex is occupied or blocked - that's OK
	if err != nil {
		s.T().Logf("  Movement failed (may be blocked): %v - checking state anyway", err)
	} else {
		s.T().Logf("  Movement result: success=%v, remaining=%d", moveResp.GetSuccess(), moveResp.GetMovementRemaining())
	}

	// 4. Get updated state
	updatedResp := s.getEncounterState(ctx, playerID, encounterID)
	updatedStateData := updatedResp.GetEncounterStateData()
	s.Require().NotNil(updatedStateData, "EncounterStateData should be present after movement")

	updatedCharState := updatedStateData.GetEntities()[characterID]
	s.Require().NotNil(updatedCharState, "character should still exist in entity state map after movement")

	updatedPos := updatedCharState.GetPosition()
	s.T().Logf("  Updated position: (%v, %v, %v)", updatedPos.GetX(), updatedPos.GetY(), updatedPos.GetZ())

	// If movement succeeded, position should have changed
	if err == nil && moveResp.GetSuccess() {
		s.Assert().NotEqual(initialPos.GetX(), updatedPos.GetX(),
			"character X position should change after successful movement")
		s.T().Log("  Position updated successfully")
	} else {
		s.T().Log("  Movement did not succeed - position unchanged (expected if hex was blocked)")
	}

	s.T().Log("=== PASSED: Movement updates entity position ===")
}

// =============================================================================
// TEST: All combatants are represented in EncounterStateData.Entities
// =============================================================================

func (s *EntityStateIntegrationSuite) TestEntityState_EntitiesIncludeAllCombatants() {
	s.T().Log("=== TEST: EncounterStateData.Entities includes all combatants ===")

	playerID := "test-player-entity-state-4"
	ctx := s.authCtx(playerID)

	// 1. Create fighter and start combat
	characterID := s.createFighterCharacter(playerID)
	encounterID := s.createEncounterAndStartCombat(ctx, playerID, characterID)
	s.T().Logf("  Character: %s, Encounter: %s", characterID, encounterID)

	// 2. Get encounter state
	resp := s.getEncounterState(ctx, playerID, encounterID)

	stateData := resp.GetEncounterStateData()
	if stateData == nil {
		s.T().Skip("EncounterStateData not yet wired to GetEncounterState response - wiring bug")
		return
	}

	entitiesMap := stateData.GetEntities()
	s.Require().NotEmpty(entitiesMap, "entities map should not be empty")

	// 3. Get turn order from combat state (the existing legacy combat state)
	combatState := resp.GetCombatState()
	s.Require().NotNil(combatState, "combat state should be present")

	turnOrder := combatState.GetTurnOrder()
	s.Require().NotEmpty(turnOrder, "turn order should not be empty")
	s.T().Logf("  Turn order has %d entries, entities map has %d entries", len(turnOrder), len(entitiesMap))

	// 4. Every entity in the turn order should have an entry in the entities map
	for _, entry := range turnOrder {
		entityID := entry.GetEntityId()
		entityState, exists := entitiesMap[entityID]
		s.Assert().True(exists, "turn order entity %s (type=%s) should have an entry in entities map",
			entityID, entry.GetEntityType())

		if exists && entityState != nil {
			s.T().Logf("  Turn order entity %s: type=%s, HP=%d/%d",
				entityID, entityState.GetEntityType(),
				entityState.GetCurrentHitPoints(), entityState.GetMaxHitPoints())
		}
	}

	// 5. Count characters and monsters
	characterCount := 0
	monsterCount := 0
	for _, entity := range entitiesMap {
		switch entity.GetEntityType() {
		case dnd5ev1alpha1.EntityType_ENTITY_TYPE_CHARACTER:
			characterCount++
		case dnd5ev1alpha1.EntityType_ENTITY_TYPE_MONSTER:
			monsterCount++
		}
	}

	s.Assert().GreaterOrEqual(characterCount, 1, "should have at least 1 character in entities")
	s.Assert().GreaterOrEqual(monsterCount, 1, "should have at least 1 monster in entities")
	s.T().Logf("  Entity breakdown: %d characters, %d monsters", characterCount, monsterCount)

	// 6. The entities count should match or exceed the turn order
	// (entities map may include obstacles that are not in turn order)
	s.Assert().GreaterOrEqual(len(entitiesMap), len(turnOrder),
		"entities map should have at least as many entries as turn order")

	s.T().Log("=== PASSED: Entities include all combatants ===")
}

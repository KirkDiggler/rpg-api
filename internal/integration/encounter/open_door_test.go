// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package encounter_integration provides integration tests for the OpenDoor → RoomRevealed pipeline.
//
// # What this tests
//
// The full event pipeline for OpenDoor:
//
//	orchestrator.publishEvent
//	  → eventProcessor.Process
//	    → Redis pub/sub (JSON serialize/deserialize via custom MarshalJSON/UnmarshalJSON)
//	      → stream handler
//	        → convertToProtoEvent
//	          → gRPC stream.Send
//	            → client Recv
//
// # Known concern
//
// RoomRevealedEvent.EncounterStateData is tagged json:"-" on the original struct.
// A custom MarshalJSON/UnmarshalJSON in encounter_events_json.go encodes it as
// base64 proto wire bytes. If that round-trip fails, the field arrives nil and the
// test fails — which is exactly the bug we are trying to catch.
//
// A passing test proves the entire pipeline (including JSON serialization) is intact.
// A failing test identifies where in the pipeline the event or its payload is lost.
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
	dungeonrepo "github.com/KirkDiggler/rpg-api/internal/repositories/dungeons"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// ============================================================================
// OPEN DOOR → ROOM REVEALED INTEGRATION TEST
// ============================================================================

// OpenDoorIntegrationSuite tests that calling OpenDoor causes a RoomRevealedEvent
// to arrive on the encounter event stream with a populated EncounterStateData.
type OpenDoorIntegrationSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	server *harness.TestServer
}

func TestOpenDoorIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(OpenDoorIntegrationSuite))
}

func (s *OpenDoorIntegrationSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 3*time.Minute)

	var err error
	s.server, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")

	s.T().Log("=== OpenDoor Integration Test Server Ready ===")
}

func (s *OpenDoorIntegrationSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *OpenDoorIntegrationSuite) SetupTest() {
	err := s.server.FlushRedis(s.ctx)
	s.Require().NoError(err, "failed to flush redis")
}

func (s *OpenDoorIntegrationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// subscribeToOpenDoorStream opens a StreamEncounterEvents subscription and returns
// a cancel function and a channel of events. The caller MUST call cancel() when done.
func (s *OpenDoorIntegrationSuite) subscribeToOpenDoorStream(
	playerID, encounterID string,
) (cancel context.CancelFunc, eventsCh <-chan *dnd5ev1alpha1.EncounterEvent) {
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
				if streamCtx.Err() != nil {
					return
				}
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

// createFighterForOpenDoor creates a fully-specified fighter character.
// Mirrors the pattern used in stream_entity_state_test.go.
func (s *OpenDoorIntegrationSuite) createFighterForOpenDoor(playerID string) string {
	ctx := s.authCtx(playerID)

	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err, "failed to create draft")
	draftID := createResp.GetDraft().GetId()
	s.Require().NotEmpty(draftID)

	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    "OpenDoor Test Fighter",
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

// =============================================================================
// TEST: OpenDoor publishes RoomRevealedEvent with EncounterStateData
// =============================================================================

func (s *OpenDoorIntegrationSuite) TestOpenDoor_RoomRevealedEvent_HasEncounterStateData() {
	s.T().Log("=== TEST: OpenDoor → RoomRevealedEvent.EncounterStateData is populated on stream ===")

	playerID := "open-door-test-player"
	ctx := s.authCtx(playerID)

	// 1. Create fighter character.
	characterID := s.createFighterForOpenDoor(playerID)
	s.T().Logf("  Created character: %s", characterID)

	// 2. Create encounter and set player ready (but do NOT start combat yet).
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

	// 3. Subscribe BEFORE starting combat to observe the full event stream.
	cancelStream, eventsCh := s.subscribeToOpenDoorStream(playerID, encounterID)
	defer cancelStream()

	// Small delay to ensure the Redis subscription is active before we publish.
	time.Sleep(100 * time.Millisecond)

	// 4. Start combat — this generates a dungeon and returns dungeon_id + doors.
	startResp, err := s.server.EncounterClient.StartCombat(ctx, &dnd5ev1alpha1.StartCombatRequest{
		EncounterId: encounterID,
		Theme:       dnd5ev1alpha1.DungeonTheme_DUNGEON_THEME_CRYPT,
		Difficulty:  dnd5ev1alpha1.DungeonDifficulty_DUNGEON_DIFFICULTY_MEDIUM,
		Length:      dnd5ev1alpha1.DungeonLength_DUNGEON_LENGTH_SHORT,
	})
	s.Require().NoError(err, "StartCombat should succeed")

	dungeonID := startResp.GetDungeonId()
	s.Require().NotEmpty(dungeonID, "StartCombatResponse.DungeonId must not be empty")
	s.T().Logf("  Dungeon: %s", dungeonID)

	doors := startResp.GetDoors()
	s.Require().NotEmpty(doors, "StartCombatResponse.Doors must not be empty — no door to open")
	s.T().Logf("  Available doors: %d", len(doors))

	// Pick the first closed door that has a position.
	var targetDoor *dnd5ev1alpha1.DoorInfo
	for _, d := range doors {
		if !d.GetIsOpen() && d.GetPosition() != nil {
			targetDoor = d
			break
		}
	}
	s.Require().NotNil(targetDoor, "at least one closed door with position must exist after StartCombat")
	s.T().Logf("  Target door: connectionId=%s position=%v physicalHint=%q",
		targetDoor.GetConnectionId(),
		targetDoor.GetPosition(),
		targetDoor.GetPhysicalHint(),
	)

	doorPos := targetDoor.GetPosition()

	// 5. Teleport character adjacent to the door via the encounter repository.
	//
	// Rooms can be 15-20 hexes wide and characters have only 30 feet (6 hexes) of
	// movement per turn. Trying to walk the character across multiple combat turns
	// is fragile: walls can block greedy movement, and monsters can kill the character.
	//
	// Instead we directly update the character's cube position in the encounter repo —
	// the same pattern used in LevelUpMonkToLevel2. This is test infrastructure, not
	// game mechanics, so using the repo directly is appropriate.
	s.teleportCharacterAdjacentToDoor(ctx, encounterID, characterID, doorPos)

	// 6. Call OpenDoor — this is the action that should publish the RoomRevealedEvent.
	openResp, err := s.server.EncounterClient.OpenDoor(ctx, &dnd5ev1alpha1.OpenDoorRequest{
		DungeonId:    dungeonID,
		ConnectionId: targetDoor.GetConnectionId(),
	})
	s.Require().NoError(err, "OpenDoor should succeed after teleporting character adjacent to door")
	s.Require().True(openResp.GetSuccess(), "OpenDoor response should indicate success")
	s.T().Logf("  OpenDoor succeeded: encounterId=%s", openResp.GetEncounterId())

	// 7. Collect events from the stream until we see a RoomRevealedEvent or time out.
	events := collectEvents(s.T(), eventsCh, streamEventTimeout, func(e *dnd5ev1alpha1.EncounterEvent) bool {
		return e.GetRoomRevealed() != nil
	})

	s.T().Logf("  Collected %d stream events", len(events))

	// Log a summary of every event type received for diagnostics.
	for i, e := range events {
		switch {
		case e.GetCombatStarted() != nil:
			s.T().Logf("  Event[%d]: CombatStarted", i)
		case e.GetMovementCompleted() != nil:
			s.T().Logf("  Event[%d]: MovementCompleted entity=%s", i, e.GetMovementCompleted().GetEntityId())
		case e.GetRoomRevealed() != nil:
			r := e.GetRoomRevealed()
			s.T().Logf("  Event[%d]: RoomRevealed dungeonId=%s connectionId=%s encStateDataNil=%v",
				i, r.GetDungeonId(), r.GetConnectionId(), r.GetEncounterStateData() == nil)
		case e.GetMonsterTurnCompleted() != nil:
			s.T().Logf("  Event[%d]: MonsterTurnCompleted", i)
		case e.GetTurnEnded() != nil:
			s.T().Logf("  Event[%d]: TurnEnded", i)
		case e.GetAttackResolved() != nil:
			s.T().Logf("  Event[%d]: AttackResolved", i)
		default:
			s.T().Logf("  Event[%d]: other", i)
		}
	}

	// 8. Assert: a RoomRevealedEvent MUST arrive — this is the critical pipeline assertion.
	roomRevealedEvent, found := findEventOfType(events,
		func(e *dnd5ev1alpha1.EncounterEvent) *dnd5ev1alpha1.RoomRevealedEvent {
			return e.GetRoomRevealed()
		},
	)
	s.Require().True(found,
		"RoomRevealedEvent MUST arrive on the stream after OpenDoor — "+
			"if it did not arrive, the event was silently dropped somewhere in the "+
			"orchestrator → Redis pub/sub → stream handler pipeline")
	s.T().Log("  Found RoomRevealedEvent")

	// 9. Assert: event metadata matches what we requested.
	s.Assert().Equal(dungeonID, roomRevealedEvent.GetDungeonId(),
		"RoomRevealedEvent.DungeonId should match the dungeon we opened")
	s.Assert().Equal(targetDoor.GetConnectionId(), roomRevealedEvent.GetConnectionId(),
		"RoomRevealedEvent.ConnectionId should match the door we opened")

	// 10. Assert: EncounterStateData survives the Redis JSON round-trip.
	//
	// This is THE critical assertion for the bug we are testing.
	// If EncounterStateData is nil here the event was received but the proto payload
	// was lost during JSON serialization/deserialization in the pub/sub round-trip.
	stateData := roomRevealedEvent.GetEncounterStateData()
	s.Require().NotNil(stateData,
		"RoomRevealedEvent.EncounterStateData must not be nil — "+
			"if nil, the custom MarshalJSON/UnmarshalJSON for RoomRevealedEvent failed "+
			"to survive the Redis pub/sub JSON round-trip")
	s.T().Log("  EncounterStateData is non-nil — JSON round-trip PASSED")

	// 11. Assert: rooms are populated (start room + newly revealed room = 2).
	rooms := stateData.GetRooms()
	s.Assert().Len(rooms, 2,
		"EncounterStateData.Rooms should have 2 entries (start room + revealed room)")
	s.T().Logf("  EncounterStateData.Rooms count: %d", len(rooms))

	// 12. Assert: at least one room has a non-zero origin (the revealed room is offset).
	foundNonZeroOrigin := false
	for roomID, room := range rooms {
		origin := room.GetOrigin()
		if origin != nil && (origin.GetX() != 0 || origin.GetY() != 0 || origin.GetZ() != 0) {
			foundNonZeroOrigin = true
			s.T().Logf("  Room %s has non-zero origin: (%.1f, %.1f, %.1f)",
				roomID, origin.GetX(), origin.GetY(), origin.GetZ())
		}
	}
	s.Assert().True(foundNonZeroOrigin,
		"at least one room in EncounterStateData.Rooms should have a non-zero Origin "+
			"(the revealed room is offset from (0,0,0) in dungeon-space)")

	// 13. Assert: entities map is populated.
	entities := stateData.GetEntities()
	s.Assert().NotEmpty(entities,
		"EncounterStateData.Entities should be non-empty (character + monsters from both rooms)")
	s.T().Logf("  EncounterStateData.Entities count: %d", len(entities))

	// 14. Assert: monster entities exist from the newly revealed room.
	//
	// When a door is opened, monsters from the revealed room are added to the
	// encounter. Their positions should be absolute (offset by room origin),
	// not the zero-origin (0,0,0) of the start room.
	foundMonsterInRoom2 := false
	for entityID, entity := range entities {
		if entity.GetEntityType() == dnd5ev1alpha1.EntityType_ENTITY_TYPE_MONSTER {
			pos := entity.GetPosition()
			if pos != nil && (pos.GetX() != 0 || pos.GetY() != 0 || pos.GetZ() != 0) {
				foundMonsterInRoom2 = true
				s.T().Logf("  Monster %s in room 2 at absolute pos (%.1f, %.1f, %.1f)",
					entityID, pos.GetX(), pos.GetY(), pos.GetZ())
			}
		}
	}
	s.Assert().True(foundMonsterInRoom2,
		"at least one monster in EncounterStateData.Entities should have a non-zero position "+
			"(monsters in the revealed room have positions offset by the room origin)")

	s.T().Log("=== PASSED: OpenDoor → RoomRevealedEvent pipeline is working end-to-end ===")
}

// =============================================================================
// HELPERS
// =============================================================================

// teleportCharacterAdjacentToDoor moves the character to be adjacent to the door
// by directly updating the encounter repository's room data.
//
// This is test infrastructure, not game mechanics. We update the cube position
// directly in the same way LevelUpMonkToLevel2 updates character data directly.
// This avoids the fragility of multi-turn API-level movement (wall-blocked paths,
// monster attacks, movement budget constraints).
//
// The door position is in dungeon-absolute coordinates. For the start room the
// origin is (0,0,0), so room-local coordinates equal dungeon-absolute.
// The proximity check uses cube distance <= 1, so placing the character ON the
// door cell (distance 0) satisfies the check.
func (s *OpenDoorIntegrationSuite) teleportCharacterAdjacentToDoor(
	ctx context.Context,
	encounterID, characterID string,
	doorPos *apiv1alpha1.Position,
) {
	// 1. Load the dungeon to confirm the start room and its current state.
	dungeonOutput, err := s.server.DungeonRepo.GetByEncounterID(ctx,
		&dungeonrepo.GetByEncounterIDInput{EncounterID: encounterID},
	)
	s.Require().NoError(err, "teleportCharacterAdjacentToDoor: failed to load dungeon")
	s.Require().NotNil(dungeonOutput.Dungeon, "teleportCharacterAdjacentToDoor: dungeon must not be nil")

	// 2. Load encounter data to get the current room data.
	encOutput, err := s.server.EncounterRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: encounterID,
	})
	s.Require().NoError(err, "teleportCharacterAdjacentToDoor: failed to load encounter")
	s.Require().NotNil(encOutput.Data, "teleportCharacterAdjacentToDoor: encounter data must not be nil")

	// 3. Cast room data to *spatial.RoomData (the type used by the hex grid).
	roomData, ok := encOutput.Data.RoomData.(*spatial.RoomData)
	s.Require().True(ok, "teleportCharacterAdjacentToDoor: RoomData must be *spatial.RoomData")
	s.Require().NotNil(roomData, "teleportCharacterAdjacentToDoor: RoomData must not be nil")

	// 4. Place the character directly on the door position.
	//    Cube distance 0 satisfies the proximity check (distance <= 1).
	//    If the door cell is blocked (occupied by monster), try its neighbors.
	targetX := int(doorPos.GetX())
	targetY := int(doorPos.GetY())
	targetZ := int(doorPos.GetZ())

	// Check if the door cell itself is occupied by a blocking entity.
	doorOccupied := false
	for id, placement := range roomData.CubeEntities {
		if id != characterID && placement.BlocksMovement &&
			placement.CubePosition.X == targetX &&
			placement.CubePosition.Y == targetY &&
			placement.CubePosition.Z == targetZ {
			doorOccupied = true
			break
		}
	}

	if doorOccupied {
		// Try the six hex neighbors of the door (all at distance 1).
		placed := false
		neighbors := [][3]int{
			{targetX + 1, targetY - 1, targetZ},
			{targetX - 1, targetY + 1, targetZ},
			{targetX + 1, targetY, targetZ - 1},
			{targetX - 1, targetY, targetZ + 1},
			{targetX, targetY + 1, targetZ - 1},
			{targetX, targetY - 1, targetZ + 1},
		}
		for _, n := range neighbors {
			nx, ny, nz := n[0], n[1], n[2]
			occupied := false
			for id, placement := range roomData.CubeEntities {
				if id != characterID && placement.BlocksMovement &&
					placement.CubePosition.X == nx &&
					placement.CubePosition.Y == ny &&
					placement.CubePosition.Z == nz {
					occupied = true
					break
				}
			}
			if !occupied {
				targetX, targetY, targetZ = nx, ny, nz
				placed = true
				break
			}
		}
		s.Require().True(placed, "teleportCharacterAdjacentToDoor: no unoccupied cell adjacent to door")
	}

	// 5. Update character's cube position in the room data.
	existing, exists := roomData.CubeEntities[characterID]
	entityType := "character"
	if exists {
		entityType = existing.EntityType
	}
	roomData.CubeEntities[characterID] = spatial.EntityCubePlacement{
		EntityID:       characterID,
		EntityType:     entityType,
		CubePosition:   spatial.CubeCoordinate{X: targetX, Y: targetY, Z: targetZ},
		Size:           1,
		BlocksMovement: true,
	}

	s.T().Logf("  Teleported %s to (%d,%d,%d) — adjacent to door at (%d,%d,%d)",
		characterID, targetX, targetY, targetZ,
		int(doorPos.GetX()), int(doorPos.GetY()), int(doorPos.GetZ()))

	// 6. Persist the updated room data back to the encounter repo.
	_, err = s.server.EncounterRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID: encounterID,
		RoomData:    roomData,
	})
	s.Require().NoError(err, "teleportCharacterAdjacentToDoor: failed to update encounter with new position")
}

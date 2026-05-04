// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package encounter

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	dicemock "github.com/KirkDiggler/rpg-toolkit/dice/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	dungeontoolkit "github.com/KirkDiggler/rpg-api/internal/components/dungeon/toolkit"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	dungeonsrepo "github.com/KirkDiggler/rpg-api/internal/repositories/dungeons"
	encountersrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"

	"go.uber.org/mock/gomock"
)

// Test fixture constants reused across the suite's tests.
const (
	testDungeonID    = "dng-464"
	testEncounterID  = "enc-464"
	testConnectionID = "conn-1-2"
	testRoom1ID      = "room-1"
	testRoom2ID      = "room-2"
	testCharID       = "char-hero"
)

// OpenDoorPerceptionTestSuite verifies that after OpenDoor reveals a new room,
// the persisted encounter RoomData includes the new monsters so that later
// perception (executeMonsterTurns -> buildPerception) can find player
// characters as targets, and that downstream consumers respect the
// post-OpenDoor coordinate-space contract (CubeEntities are dungeon-absolute,
// no re-translation on the read path).
//
// Bug context: KirkDiggler/rpg-api#464 — OpenDoor previously persisted
// Monsters and InitiativeData but NOT RoomData. The next EndTurn would call
// buildPerception with the stale single-room RoomData; the new monsters were
// not in CubeEntities, perception returned an empty enemy list, and the new
// monsters skipped their turns. Copilot review on PR #491 surfaced four
// related double-shift / persistence bugs along the same code path; this
// suite covers the regression checks for all of them.
type OpenDoorPerceptionTestSuite struct {
	suite.Suite
	ctx          context.Context
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	encRepo      *encountersrepo.InMemoryRepository
	dungeonRepo  *dungeonsrepo.InMemoryRepository
	roller       *dicemock.MockRoller
	orchestrator *Orchestrator
}

func TestOpenDoorPerceptionTestSuite(t *testing.T) {
	suite.Run(t, new(OpenDoorPerceptionTestSuite))
}

func (s *OpenDoorPerceptionTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.encRepo = encountersrepo.NewInMemory()
	s.dungeonRepo = dungeonsrepo.NewInMemory()
	s.roller = dicemock.NewMockRoller(s.ctrl)

	orc, err := New(&Config{
		CharacterRepo: s.mockCharRepo,
		EncounterRepo: s.encRepo,
		DungeonRepo:   s.dungeonRepo,
		DungeonGen:    dungeontoolkit.CreateGenerator(&dungeontoolkit.ToolkitConfig{}),
		Roller:        s.roller,
	})
	s.Require().NoError(err)
	s.orchestrator = orc
}

func (s *OpenDoorPerceptionTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// seedTwoRoomScenario sets up a two-room dungeon (room-1 at origin, room-2
// at the supplied absolute origin) connected by a north door, with a single
// skeleton in room-2's encounter and the character standing adjacent to the
// door in room-1. Optionally adds an obstacle to room-2 if obstacleID is
// non-empty; the obstacle is placed at obstacleLocal (room-local cube coords).
func (s *OpenDoorPerceptionTestSuite) seedTwoRoomScenario(
	room2Origin dungeon.AbsolutePosition,
	obstacleID string,
	obstacleLocal dungeon.LocalPosition,
) {
	s.T().Helper()

	dng := &entities.Dungeon{
		ID:          testDungeonID,
		EncounterID: testEncounterID,
		Connections: []*environments.ConnectionEdge{
			{
				ID:            testConnectionID,
				FromRoomID:    testRoom1ID,
				ToRoomID:      testRoom2ID,
				Bidirectional: true,
				Type:          "north door",
			},
		},
		StartRoomID:   testRoom1ID,
		CurrentRoomID: testRoom1ID,
		RevealedRooms: map[string]bool{testRoom1ID: true},
		OpenDoors:     map[string]bool{},
		RoomOrigins: map[string]dungeon.AbsolutePosition{
			testRoom1ID: dungeon.NewAbsolutePosition(0, 0),
			testRoom2ID: room2Origin,
		},
		Rooms: map[string]*dungeon.Room{
			testRoom1ID: {
				ID:    testRoom1ID,
				Shape: &dungeon.Shape{Width: 20, Height: 20},
			},
			testRoom2ID: {
				ID:    testRoom2ID,
				Shape: &dungeon.Shape{Width: 15, Height: 15},
				Encounter: &dungeon.Encounter{
					Monsters: []dungeon.MonsterPlacement{
						{
							ID:        "mp-1",
							MonsterID: "skeleton",
							Position:  dungeon.NewLocalPosition(5, 5),
						},
					},
				},
			},
		},
		State: entities.DungeonStateActive,
	}
	if obstacleID != "" {
		dng.Rooms[testRoom2ID].Features = &dungeon.FeatureLayout{
			Obstacles: []dungeon.Obstacle{
				{
					ID:                obstacleID,
					Type:              dungeon.ObstacleTypePillar,
					Position:          obstacleLocal,
					BlocksMovement:    true,
					BlocksLineOfSight: true,
				},
			},
		}
	}
	_, err := s.dungeonRepo.Save(s.ctx, &dungeonsrepo.SaveInput{Dungeon: dng})
	s.Require().NoError(err)

	// Seed encounter with the character placed adjacent to the north door
	// in room-1 (door at x=10, z=0 — character at x=10, z=1 is one hex away).
	charCube := spatial.CubeCoordinate{X: 10, Y: -11, Z: 1}
	startRoomData := &spatial.RoomData{
		ID:       testEncounterID + "-" + testRoom1ID,
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		CubeEntities: map[string]spatial.EntityCubePlacement{
			testCharID: {
				EntityID:       testCharID,
				EntityType:     entityTypeCharacter,
				CubePosition:   charCube,
				Size:           1,
				BlocksMovement: true,
			},
		},
	}
	charAbs := dungeon.NewAbsolutePosition(charCube.X, charCube.Z)
	_, err = s.encRepo.Save(s.ctx, &encountersrepo.SaveInput{
		EncounterID: testEncounterID,
		RoomData:    startRoomData,
		InitiativeData: &initiative.TrackerData{
			Order: []initiative.EntityData{
				{ID: testCharID, Type: entityTypeCharacter},
			},
			Current: 0,
			Round:   1,
		},
		InitiativeRolls: []initiative.Roll{
			{
				Entity:   initiative.NewParticipant(testCharID, entityTypeCharacter),
				Roll:     13,
				Modifier: 2,
				Total:    15,
			},
		},
		Monsters: []*monster.Data{},
		// Seed the Entities map with the character so addMonstersToEntityMap
		// has a non-nil map to mutate (matches StartCombat's behavior).
		Entities: map[string]*entities.EntityStateData{
			testCharID: {
				EntityID:   testCharID,
				EntityType: entities.EntityTypeCharacter,
				RoomID:     testRoom1ID,
				Position:   &charAbs,
				Size:       1,
			},
		},
		// Seed lobby so GetEncounterState's player-presence check passes.
		State: encountersrepo.StateActive,
		Players: map[string]*encountersrepo.Player{
			"player-1": {PlayerID: "player-1", CharacterID: testCharID},
		},
		HostID: "player-1",
	})
	s.Require().NoError(err)
}

// TestOpenDoor_PersistsRoomDataSoNewMonstersPerceiveCharacters reproduces
// issue #464: it seeds a two-room dungeon with a character in the start room,
// opens the door to room 2, and then asserts that the persisted RoomData
// contains the newly spawned monsters in dungeon-absolute coordinates so
// buildPerception can find the character as an enemy.
//
// Before the fix: OpenDoor's encRepo.Update omits RoomData, so the
// persisted CubeEntities still only contain the character. buildPerception
// for the new monster falls into the "monsterPlacement not found" branch
// and returns zero enemies — the monster appears AFK on its turn.
//
// After the fix: OpenDoor builds a combined RoomData that includes the new
// monsters at their dungeon-absolute positions. buildPerception finds the
// character as an enemy, with a finite distance.
func (s *OpenDoorPerceptionTestSuite) TestOpenDoor_PersistsRoomDataSoNewMonstersPerceiveCharacters() {
	// Room-2 origin is offset in dungeon-absolute space so we can verify
	// translation (room-local 5,5 -> absolute origin + (5,5)).
	room2Origin := dungeon.NewAbsolutePosition(20, 0)
	s.seedTwoRoomScenario(room2Origin, "", dungeon.LocalPosition{})

	// One d20 roll for initiative of the new skeleton.
	s.roller.EXPECT().Roll(gomock.Any(), 20).Return(11, nil)

	// Act: open the door to room-2.
	output, err := s.orchestrator.OpenDoor(s.ctx, &OpenDoorInput{
		DungeonID:    testDungeonID,
		ConnectionID: testConnectionID,
	})
	s.Require().NoError(err, "OpenDoor should succeed")
	s.Require().NotNil(output)
	s.Require().Len(output.Monsters, 1, "should spawn one skeleton in room-2")
	newMonsterID := output.Monsters[0].ID

	// Reload the persisted encounter and verify RoomData carries the new
	// monster so perception can find the character.
	getOut, err := s.encRepo.Get(s.ctx, &encountersrepo.GetInput{EncounterID: testEncounterID})
	s.Require().NoError(err)
	persistedData := getOut.Data
	s.Require().NotNil(persistedData)
	s.Require().NotNil(persistedData.RoomData, "RoomData must be persisted by OpenDoor")

	persistedRoom, ok := persistedData.RoomData.(*spatial.RoomData)
	s.Require().True(ok, "persisted RoomData should be *spatial.RoomData")
	s.Require().NotNil(persistedRoom.CubeEntities)

	// Sanity: the existing character is still in the persisted RoomData.
	charCube := spatial.CubeCoordinate{X: 10, Y: -11, Z: 1}
	charPlacement, hasChar := persistedRoom.CubeEntities[testCharID]
	s.Require().True(hasChar, "persisted RoomData should still contain the character")
	s.Equal(charCube, charPlacement.CubePosition, "character position should be unchanged")

	// Bug check: the new monster must be present in the persisted RoomData
	// using dungeon-absolute coordinates so cross-room perception works.
	monsterPlacement, hasMonster := persistedRoom.CubeEntities[newMonsterID]
	s.Require().True(hasMonster,
		"persisted RoomData must contain the new monster %s — without this, "+
			"buildPerception returns zero enemies and the monster skips its turn",
		newMonsterID)
	s.Equal(entityTypeMonster, monsterPlacement.EntityType)

	// The skeleton was placed at room-2 local (5, 5). Room-2 origin is
	// (20, 0) absolute, so the persisted absolute position should be (25, 5).
	expectedAbs := dungeon.NewAbsolutePosition(
		room2Origin.X+5, // local X + room origin X
		room2Origin.Z+5, // local Z + room origin Z
	)
	s.Equal(spatial.CubeCoordinate{X: expectedAbs.X, Y: expectedAbs.Y, Z: expectedAbs.Z},
		monsterPlacement.CubePosition,
		"new monster must be persisted in dungeon-absolute coords")

	// Behavioral assertion: with the persisted RoomData, buildPerception
	// for the new monster must find the character as an enemy.
	perception := buildPerception(
		persistedRoom,
		newMonsterID,
		[]string{testCharID},
		persistedData.Monsters,
		nil, // walls not relevant for this assertion
	)
	s.Require().NotNil(perception)
	s.Require().Len(perception.Enemies, 1,
		"the new monster must perceive the character as an enemy after OpenDoor — "+
			"if this fails, the monster will appear AFK on its turn (issue #464)")
	s.Equal(testCharID, perception.Enemies[0].Entity.GetID())
	s.Greater(perception.Enemies[0].Distance, 0, "distance to character should be positive")
}

// TestOpenDoor_PersistsEntitiesMapForPostReloadSnapshots covers Copilot review
// item 3184901637: addMonstersToEntityMap mutates encOutput.Data.Entities
// in-memory, but the encRepo.Update call must include Entities so a
// post-reload GetEncounterState -> buildEncounterStateSnapshot still sees the
// newly-revealed monsters. Without persisting Entities, late-join clients
// would receive a snapshot that omits the room-2 monsters.
func (s *OpenDoorPerceptionTestSuite) TestOpenDoor_PersistsEntitiesMapForPostReloadSnapshots() {
	room2Origin := dungeon.NewAbsolutePosition(20, 0)
	s.seedTwoRoomScenario(room2Origin, "", dungeon.LocalPosition{})

	s.roller.EXPECT().Roll(gomock.Any(), 20).Return(11, nil)

	output, err := s.orchestrator.OpenDoor(s.ctx, &OpenDoorInput{
		DungeonID:    testDungeonID,
		ConnectionID: testConnectionID,
	})
	s.Require().NoError(err)
	s.Require().Len(output.Monsters, 1)
	newMonsterID := output.Monsters[0].ID

	// Reload from the repo and verify the Entities map has the new monster.
	getOut, err := s.encRepo.Get(s.ctx, &encountersrepo.GetInput{EncounterID: testEncounterID})
	s.Require().NoError(err)

	esd, ok := getOut.Data.Entities[newMonsterID]
	s.Require().True(ok,
		"newly-revealed monster must be persisted in the Entities map so "+
			"GetEncounterState -> buildEncounterStateSnapshot includes it after reload")
	s.Equal(entities.EntityTypeMonster, esd.EntityType)
	s.Equal(testRoom2ID, esd.RoomID, "EntityStateData.RoomID should be the revealed room")
	s.Require().NotNil(esd.Position, "EntityStateData.Position must be populated")
	// Position is dungeon-absolute (20+5, 0+5) = (25, 5) per NewAbsolutePosition.
	s.Equal(dungeon.NewAbsolutePosition(25, 5), *esd.Position,
		"EntityStateData.Position must be dungeon-absolute")
}

// TestOpenDoor_BringsObstaclesFromRevealedRoom covers Copilot review item
// 3184901660: convertToRoomData copies room.Features.Obstacles into
// CubeEntities at StartCombat; the OpenDoor merge must do the same so that
// movement-collision (executeMove / MoveCharacter, which iterates
// CubeEntities and checks BlocksMovement) sees blocking features in the
// newly-revealed room.
func (s *OpenDoorPerceptionTestSuite) TestOpenDoor_BringsObstaclesFromRevealedRoom() {
	const obstacleID = "pillar-1"
	room2Origin := dungeon.NewAbsolutePosition(20, 0)
	// Obstacle at room-2 local (3, 4); expected dungeon-absolute is (23, 4).
	obstacleLocal := dungeon.NewLocalPosition(3, 4)
	s.seedTwoRoomScenario(room2Origin, obstacleID, obstacleLocal)

	s.roller.EXPECT().Roll(gomock.Any(), 20).Return(11, nil)

	_, err := s.orchestrator.OpenDoor(s.ctx, &OpenDoorInput{
		DungeonID:    testDungeonID,
		ConnectionID: testConnectionID,
	})
	s.Require().NoError(err)

	getOut, err := s.encRepo.Get(s.ctx, &encountersrepo.GetInput{EncounterID: testEncounterID})
	s.Require().NoError(err)
	persistedRoom, ok := getOut.Data.RoomData.(*spatial.RoomData)
	s.Require().True(ok)

	placement, hasObstacle := persistedRoom.CubeEntities[obstacleID]
	s.Require().True(hasObstacle,
		"obstacles from the revealed room must be added to the combined RoomData "+
			"so movement-collision can check BlocksMovement against them")
	s.Equal(entityTypeObstacle, placement.EntityType)
	s.True(placement.BlocksMovement, "obstacle must keep BlocksMovement so collision check fires")
	s.True(placement.BlocksLineOfSight, "obstacle BlocksLineOfSight should also be carried through")

	// Obstacle was at room-2 local (3, 4); room-2 origin is (20, 0) absolute,
	// so the persisted absolute position should be (23, 4).
	expectedAbs := dungeon.NewAbsolutePosition(room2Origin.X+3, room2Origin.Z+4)
	s.Equal(spatial.CubeCoordinate{X: expectedAbs.X, Y: expectedAbs.Y, Z: expectedAbs.Z},
		placement.CubePosition,
		"obstacle must be persisted in dungeon-absolute coords like the new monsters")
}

// TestSyncMonsterPositionFromRoom_DoesNotDoubleShift covers Copilot review
// item 3184901620: under the post-OpenDoor contract CubeEntities are
// dungeon-absolute, so syncMonsterPositionFromRoom must copy the cube
// straight into EntityStateData.Position without LocalToAbsolute. Previously
// the function called localToAbsoluteOrLocal(module, roomID, ...) which
// would shift by the room origin a SECOND time once CurrentRoomID advanced
// to a non-origin room.
func (s *OpenDoorPerceptionTestSuite) TestSyncMonsterPositionFromRoom_DoesNotDoubleShift() {
	// Simulate post-OpenDoor state: a monster lives in room-2, so its
	// CubeEntities position is dungeon-absolute (25, -30, 5).
	const monsterID = "monster-room-2-mp-1"
	absPos := dungeon.NewAbsolutePosition(25, 5) // (25, -30, 5)
	roomData := &spatial.RoomData{
		GridType: spatial.GridTypeHex,
		CubeEntities: map[string]spatial.EntityCubePlacement{
			monsterID: {
				EntityID:     monsterID,
				EntityType:   entityTypeMonster,
				CubePosition: spatial.CubeCoordinate{X: absPos.X, Y: absPos.Y, Z: absPos.Z},
			},
		},
	}
	entityMap := map[string]*entities.EntityStateData{
		monsterID: {
			EntityID:   monsterID,
			EntityType: entities.EntityTypeMonster,
			RoomID:     testRoom2ID,
		},
	}

	// Sync. Under the new contract, no module/roomID is passed and no
	// re-translation is applied.
	syncMonsterPositionFromRoom(entityMap, monsterID, roomData)

	s.Require().NotNil(entityMap[monsterID].Position)
	s.Equal(absPos, *entityMap[monsterID].Position,
		"syncMonsterPositionFromRoom must pass the absolute cube through unchanged — "+
			"no LocalToAbsolute against CurrentRoomID, which would double-shift "+
			"a monster whose CubeEntities position is already absolute (#491)")
}

// TestGetEncounterState_DoesNotDoubleShiftRevealedRoomMonsters covers
// Copilot review item 3184901652: GetEncounterState returns the persisted
// (absolute) RoomData, and the handler previously called applyOriginToEntities
// which would add the room origin a second time. Even though the start room
// is at origin so the regression isn't visible in the simple case, the
// orchestrator return should be in absolute and the handler must preserve
// that without further translation.
//
// We exercise the orchestrator side here (verifying GetEncounterState's Room
// is absolute) so any handler-side double-shift that ships the same proto
// without applyOriginToEntities will keep matching reality. The handler-side
// rule is documented at internal/handlers/dnd5e/v1alpha1/encounter/handler.go
// around line 250 and exercised by handler_test.go's
// TestGetEncounterState_DoesNotDoubleShiftAbsoluteEntities.
func (s *OpenDoorPerceptionTestSuite) TestGetEncounterState_DoesNotDoubleShiftRevealedRoomMonsters() {
	room2Origin := dungeon.NewAbsolutePosition(20, 0)
	s.seedTwoRoomScenario(room2Origin, "", dungeon.LocalPosition{})

	// GetEncounterState calls buildPartyFromPlayers which loads character
	// data. Return an error from charRepo so the party member is built
	// without CharacterData — we don't care about character details here,
	// only the Room contract.
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("not seeded")).
		AnyTimes()

	s.roller.EXPECT().Roll(gomock.Any(), 20).Return(11, nil)
	openOut, err := s.orchestrator.OpenDoor(s.ctx, &OpenDoorInput{
		DungeonID:    testDungeonID,
		ConnectionID: testConnectionID,
	})
	s.Require().NoError(err)
	s.Require().Len(openOut.Monsters, 1)
	newMonsterID := openOut.Monsters[0].ID

	stateOut, err := s.orchestrator.GetEncounterState(s.ctx, &GetEncounterStateInput{
		EncounterID: testEncounterID,
		PlayerID:    "player-1",
	})
	s.Require().NoError(err)
	s.Require().NotNil(stateOut)
	s.Require().NotNil(stateOut.Room, "GetEncounterState must include Room while combat is active")

	stateRoom, ok := stateOut.Room.(*spatial.RoomData)
	s.Require().True(ok)

	monsterPlacement, hasMonster := stateRoom.CubeEntities[newMonsterID]
	s.Require().True(hasMonster, "GetEncounterState's Room must include the revealed-room monster")

	expectedAbs := dungeon.NewAbsolutePosition(room2Origin.X+5, room2Origin.Z+5)
	s.Equal(spatial.CubeCoordinate{X: expectedAbs.X, Y: expectedAbs.Y, Z: expectedAbs.Z},
		monsterPlacement.CubePosition,
		"GetEncounterState must return dungeon-absolute CubeEntities — "+
			"the handler relies on this contract to NOT call applyOriginToEntities, "+
			"which would otherwise double-shift the monster's position (#491)")
}

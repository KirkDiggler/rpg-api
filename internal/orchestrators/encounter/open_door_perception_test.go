// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package encounter

import (
	"context"
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

// OpenDoorPerceptionTestSuite verifies that after OpenDoor reveals a new room,
// the persisted encounter RoomData includes the new monsters so that later
// perception (executeMonsterTurns -> buildPerception) can find player
// characters as targets.
//
// Bug context: KirkDiggler/rpg-api#464 — OpenDoor previously persisted
// Monsters and InitiativeData but NOT RoomData. The next EndTurn would call
// buildPerception with the stale single-room RoomData; the new monsters were
// not in CubeEntities, perception returned an empty enemy list, and the new
// monsters skipped their turns.
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
	const (
		dungeonID    = "dng-464"
		encounterID  = "enc-464"
		connectionID = "conn-1-2"
		room1ID      = "room-1"
		room2ID      = "room-2"
		charID       = "char-hero"
	)

	// Room-2 origin is offset in dungeon-absolute space so we can verify
	// translation (room-local 5,5 -> absolute origin + (5,5)).
	room2Origin := dungeon.NewAbsolutePosition(20, 0)

	// Seed dungeon: two rooms connected by a north door, room-1 revealed.
	dng := &entities.Dungeon{
		ID:          dungeonID,
		EncounterID: encounterID,
		Connections: []*environments.ConnectionEdge{
			{
				ID:            connectionID,
				FromRoomID:    room1ID,
				ToRoomID:      room2ID,
				Bidirectional: true,
				Type:          "north door",
			},
		},
		StartRoomID:   room1ID,
		CurrentRoomID: room1ID,
		RevealedRooms: map[string]bool{room1ID: true},
		OpenDoors:     map[string]bool{},
		RoomOrigins: map[string]dungeon.AbsolutePosition{
			room1ID: dungeon.NewAbsolutePosition(0, 0),
			room2ID: room2Origin,
		},
		Rooms: map[string]*dungeon.Room{
			room1ID: {
				ID:    room1ID,
				Shape: &dungeon.Shape{Width: 20, Height: 20},
			},
			room2ID: {
				ID:    room2ID,
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
	_, err := s.dungeonRepo.Save(s.ctx, &dungeonsrepo.SaveInput{Dungeon: dng})
	s.Require().NoError(err)

	// Seed encounter with the character placed adjacent to the north door
	// in room-1 (door at x=10, z=0 — character at x=10, z=1 is one hex away).
	charCube := spatial.CubeCoordinate{X: 10, Y: -11, Z: 1}
	startRoomData := &spatial.RoomData{
		ID:       encounterID + "-" + room1ID,
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		CubeEntities: map[string]spatial.EntityCubePlacement{
			charID: {
				EntityID:       charID,
				EntityType:     entityTypeCharacter,
				CubePosition:   charCube,
				Size:           1,
				BlocksMovement: true,
			},
		},
	}
	_, err = s.encRepo.Save(s.ctx, &encountersrepo.SaveInput{
		EncounterID: encounterID,
		RoomData:    startRoomData,
		InitiativeData: &initiative.TrackerData{
			Order: []initiative.EntityData{
				{ID: charID, Type: entityTypeCharacter},
			},
			Current: 0,
			Round:   1,
		},
		InitiativeRolls: []initiative.Roll{
			{
				Entity:   initiative.NewParticipant(charID, entityTypeCharacter),
				Roll:     13,
				Modifier: 2,
				Total:    15,
			},
		},
		Monsters: []*monster.Data{},
		Entities: map[string]*entities.EntityStateData{},
	})
	s.Require().NoError(err)

	// One d20 roll for initiative of the new skeleton.
	s.roller.EXPECT().Roll(gomock.Any(), 20).Return(11, nil)

	// Act: open the door to room-2.
	output, err := s.orchestrator.OpenDoor(s.ctx, &OpenDoorInput{
		DungeonID:    dungeonID,
		ConnectionID: connectionID,
	})
	s.Require().NoError(err, "OpenDoor should succeed")
	s.Require().NotNil(output)
	s.Require().Len(output.Monsters, 1, "should spawn one skeleton in room-2")
	newMonsterID := output.Monsters[0].ID

	// Reload the persisted encounter and verify RoomData carries the new
	// monster so perception can find the character.
	getOut, err := s.encRepo.Get(s.ctx, &encountersrepo.GetInput{EncounterID: encounterID})
	s.Require().NoError(err)
	persistedData := getOut.Data
	s.Require().NotNil(persistedData)
	s.Require().NotNil(persistedData.RoomData, "RoomData must be persisted by OpenDoor")

	persistedRoom, ok := persistedData.RoomData.(*spatial.RoomData)
	s.Require().True(ok, "persisted RoomData should be *spatial.RoomData")
	s.Require().NotNil(persistedRoom.CubeEntities)

	// Sanity: the existing character is still in the persisted RoomData.
	charPlacement, hasChar := persistedRoom.CubeEntities[charID]
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
		[]string{charID},
		persistedData.Monsters,
		nil, // walls not relevant for this assertion
	)
	s.Require().NotNil(perception)
	s.Require().Len(perception.Enemies, 1,
		"the new monster must perceive the character as an enemy after OpenDoor — "+
			"if this fails, the monster will appear AFK on its turn (issue #464)")
	s.Equal(charID, perception.Enemies[0].Entity.GetID())
	s.Greater(perception.Enemies[0].Distance, 0, "distance to character should be positive")
}

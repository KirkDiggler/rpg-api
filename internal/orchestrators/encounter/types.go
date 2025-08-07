package encounter

import "github.com/KirkDiggler/rpg-toolkit/tools/spatial"

// DungeonStartInput defines the request for starting a dungeon encounter
type DungeonStartInput struct {
	CharacterIDs []string
}

// DungeonStartOutput defines the response for starting a dungeon encounter
type DungeonStartOutput struct {
	EncounterID string
	RoomData    *spatial.RoomData
}

// Note: All spatial types (Position, EntityPlacement, RoomData) are now provided
// by the github.com/KirkDiggler/rpg-toolkit/tools/spatial package

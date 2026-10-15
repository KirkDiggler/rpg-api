package lobby_test

import (
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
)

// testWorldID is the host's world every launch in these suites resolves in:
// the world the test registries' saves land in.
var testWorldID = dungeonstest.Caller.WorldID

// hostCaller is the contract suites' host asking in that world.
var hostCaller = dungeons.Caller{WorldID: testWorldID, PlayerID: "alice"}

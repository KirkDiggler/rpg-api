package lobby_test

import (
	"bytes"
	"os"
	"path/filepath"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// TestStartEncounter_AnApproachBeyondSpeakReachIsRefused pins the hail's
// launch gate through the lobby (rpg-project#923, ruled 2026-11-11: "the
// reach inside the current speak distance"). An author's copy of the castle
// with `front-gate` at `approach: { within: 5 }` is valid authored content —
// the registry's compile accepts the `approach` key and the guards'
// `on: approached` rows — and the launch refuses it as an invalid world
// whose sentence names the interaction, before any session is written.
func (s *SessionStackSuite) TestStartEncounter_AnApproachBeyondSpeakReachIsRefused() {
	const (
		castleKey = "the-mad-king"
		farKey    = "far-gate-castle"
		approach  = "approach: { within: 2 }"
	)
	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(s.T()), castleKey+".yaml")) //nolint:gosec // the shipped content directory
	s.Require().NoError(err)
	s.Require().Equal(1, bytes.Count(raw, []byte(approach)), "precondition: the castle authors one approach")
	far := dungeonstest.AsKey(s.T(), bytes.Replace(raw, []byte(approach), []byte("approach: { within: 5 }"), 1), castleKey, farKey)

	registry, _ := dungeonstest.Scratch(s.T())
	put, err := registry.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: farKey, YAML: far})
	s.Require().NoError(err)
	s.Require().Empty(put.Errors, "the compiler accepts the approach; reach is the session's to judge")

	orch := s.lobbyOver(registry)
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	_, err = orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		WorldID:  testWorldID,
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: lobbyorch.DungeonKey(farKey),
	})
	s.Require().Error(err)
	s.ErrorIs(err, sdk.ErrInvalidWorld, "the launch refuses the approach")
	s.Contains(err.Error(), `interaction "front-gate" approach within 5 is beyond the reach 3`)

	sessionKeys, err := s.redisClient.Keys(s.ctx, "session:v1alpha1:*").Result()
	s.Require().NoError(err)
	s.Empty(sessionKeys, "no session record was written")
}

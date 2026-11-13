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

// castleWithSharing is the shipped castle under key, its root carrying
// `party: { sharing: word }` (rpg-project#935), saved to a scratch registry.
func (s *SessionStackSuite) castleWithSharing(key, word string) dungeons.Registry {
	const castleKey = "the-mad-king"
	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(s.T()), castleKey+".yaml")) //nolint:gosec // the shipped content directory
	s.Require().NoError(err)
	s.Require().Equal(1, bytes.Count(raw, []byte("\nroom:")), "precondition: the castle has one root room key")
	marked := bytes.Replace(raw, []byte("\nroom:"), []byte("\nparty:\n  sharing: "+word+"\nroom:"), 1)
	doc := dungeonstest.AsKey(s.T(), marked, castleKey, key)

	// A shelf holding only the castle: the registry needs its default
	// dungeon, and nothing else is played here.
	shelf := s.T().TempDir()
	s.Require().NoError(os.WriteFile(filepath.Join(shelf, castleKey+".yaml"), raw, 0o600))
	castleOnly, err := dungeons.NewFileRegistry(shelf, dungeonstest.Projector(s.T()))
	s.Require().NoError(err)
	registry := dungeonstest.Join(s.T(), castleOnly, true)
	put, err := registry.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: key, YAML: doc})
	s.Require().NoError(err)
	s.Require().Empty(put.Errors, "the compiler accepts sharing %q", word)
	return registry
}

// launchSharing launches the castle marked with word through the lobby.
func (s *SessionStackSuite) launchSharing(word string) error {
	key := "sharing-" + word + "-castle"
	orch := s.lobbyOver(s.castleWithSharing(key, word))
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")
	_, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		WorldID: testWorldID, PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: lobbyorch.DungeonKey(key),
	})
	return err
}

// TestStartEncounter_EachTheirOwnLaunches and its view sibling pin
// rpg-project#935 at the lobby door: a castle marked own or view launches.
// The setting is inert in the api; nothing here reads it.
func (s *SessionStackSuite) TestStartEncounter_EachTheirOwnLaunches() {
	s.Require().NoError(s.launchSharing("own"))
}

func (s *SessionStackSuite) TestStartEncounter_SharedViewLaunches() {
	s.Require().NoError(s.launchSharing("view"))
}

// TestStartEncounter_SharedKnowledgeIsRefused: `knowledge` compiles and
// saves, and the launch refuses it with the engine's sentence as the
// session's ErrSharingNotBuilt -- a precondition the author can change, not
// a broken world -- before any session is written.
func (s *SessionStackSuite) TestStartEncounter_SharedKnowledgeIsRefused() {
	err := s.launchSharing("knowledge")
	s.Require().Error(err)
	s.ErrorIs(err, sdk.ErrSharingNotBuilt)
	s.NotErrorIs(err, sdk.ErrInvalidWorld, "the dungeon is valid")
	s.Contains(err.Error(), "shared knowledge is not built yet")

	sessionKeys, err := s.redisClient.Keys(s.ctx, "session:v1alpha1:*").Result()
	s.Require().NoError(err)
	s.Empty(sessionKeys, "no session record was written")
}

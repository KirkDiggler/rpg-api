package lobby_test

import (
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	sessionv1alpha1 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// frontRoomKey is the shipped v2 dungeon this file launches: a second dialect
// from the single-room fixtures, so the key's path is proven on content the
// World Builder did not author.
const frontRoomKey = "reference-front-room"

// TestStartEncounter_GetAtlasNamesTheDungeonTheLaunchResolved is the whole of
// slice 4 end to end (rpg-project#479): a launch names a dungeon, the lobby
// writes the RESOLVED key onto the session record, and the real GetAtlas
// handler hands a client that key so it can fetch the room's appearance from
// the same registry entry this world was compiled from.
//
// The deprecated scene field is asserted empty on the same response. There is
// nothing left on the atlas to fill it from, and a client on the previous
// build must read an empty scene and draw the map alone rather than a room
// this layer invented.
func (s *SessionStackSuite) TestStartEncounter_GetAtlasNamesTheDungeonTheLaunchResolved() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: lobbyorch.DungeonKey(frontRoomKey),
	})
	s.Require().NoError(err)

	handler, err := sessionv1alpha1.New(&sessionv1alpha1.HandlerConfig{
		Manager: s.sessOrch.Manager, Broker: s.sessOrch.Broker, Characters: s.charRepo,
	})
	s.Require().NoError(err)
	resp, err := handler.GetAtlas(auth.WithPlayerID(s.ctx, "alice"),
		&sessionpb.GetAtlasRequest{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)

	s.Equal(frontRoomKey, resp.GetDungeonKey(),
		"the key the launch resolved reaches the client that has to fetch the room's picture by it")
	s.Empty(resp.GetRoomSceneJson(),
		"and no scene rides the wire: the engine stopped carrying one")
	s.NotEmpty(resp.GetCells(), "the map itself is unchanged")
}

// TestStartEncounter_ADefaultedLaunchNamesTheDungeonItActuallyPlayed pins the
// half a naive wiring gets wrong: a launch that names NO dungeon plays the
// default, and the key written down is the RESOLVED one, not the empty string
// the caller sent. A client told "" would go looking for content under no key
// at all and draw nothing.
func (s *SessionStackSuite) TestStartEncounter_ADefaultedLaunchNamesTheDungeonItActuallyPlayed() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().NoError(err)

	atlas, err := s.sessOrch.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)
	s.Equal(dungeons.DefaultKey, atlas.DungeonKey,
		"an unnamed launch names the dungeon it actually played, not the empty request")
}

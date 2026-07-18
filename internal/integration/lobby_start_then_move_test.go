// Package integration_test's lobby_start_then_move_test.go is the rpg-api#656
// regression gate: MoveEntity must actually move the entity in an encounter
// created via the REAL RPC path (LobbyService.StartEncounter, not a direct
// tkenc.New/AddPlayer seed). This is exactly the class of coverage hole
// named in #656 — every existing integration test seeds encounters directly
// via the toolkit SDK (tkenc.New + AddPlayer + repo.Save), never through
// StartEncounter's InitRoom + corner-anchored spawn combination, so none of
// them could have caught a movement-direction bug specific to that spawn
// position.
package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	core "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

type LobbyStartThenMoveSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	srv    *harness.TestServer
}

func TestLobbyStartThenMoveSuite(t *testing.T) {
	suite.Run(t, new(LobbyStartThenMoveSuite))
}

func (s *LobbyStartThenMoveSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	var err error
	s.srv, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")
}

func (s *LobbyStartThenMoveSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *LobbyStartThenMoveSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// sixHexDirections is every unit step in cube-hex space (Q+R+S=0 preserved).
// rpg-api#656's own bug report only exercised ONE of these (Q+1,S-1) — "one
// direction green was exactly how this bug hid for three days" (gate note on
// PR #657). A corner-anchored spawn breaks roughly half of these (any
// direction that decreases offset X or Y); a center-anchored spawn must
// clear all six, so this test asserts all six rather than re-pinning just
// the one direction the original report happened to catch.
var sixHexDirections = []core.Hex{
	{Q: 1, R: -1, S: 0},
	{Q: 1, R: 0, S: -1},
	{Q: 0, R: 1, S: -1},
	{Q: -1, R: 1, S: 0},
	{Q: -1, R: 0, S: 1},
	{Q: 0, R: -1, S: 1},
}

// TestMoveEntity_AfterStartEncounter_ActuallyMoves is the #656 regression
// gate: StartEncounter's InitRoom + player-seeding combination must produce
// an encounter where every movement direction works, not just the ones a
// prior playtest happened to exercise. Each direction gets its own fresh
// lobby/character/encounter (via s.Run sub-tests) rather than chaining moves
// on one encounter, so a failure in one direction is independently visible
// in test output and isn't masked by, or dependent on, any other direction's
// outcome.
func (s *LobbyStartThenMoveSuite) TestMoveEntity_AfterStartEncounter_ActuallyMoves() {
	for i, dir := range sixHexDirections {
		s.Run(fmt.Sprintf("direction_%d_%+v", i, dir), func() {
			s.assertMoveSucceedsInDirection(fmt.Sprintf("656-%d", i), dir)
		})
	}
}

// assertMoveSucceedsInDirection creates a fresh lobby, starts an encounter
// via the real RPC path, reads the player's actual spawn position (no
// hardcoded hex — this pins "movement works from wherever StartEncounter
// spawns the party," not a specific fix shape), moves one step in dir via
// the real MoveEntity RPC, and asserts the entity actually arrived.
func (s *LobbyStartThenMoveSuite) assertMoveSucceedsInDirection(suffix string, dir core.Hex) {
	playerID := "alice-" + suffix
	characterID := "char-alice-" + suffix

	_, err := s.srv.CharacterRepo.Create(s.ctx, characterrepo.CreateInput{
		Character: &entities.Character{
			Data: &toolkitchar.Data{
				ID: characterID, PlayerID: playerID, Name: "Alice",
				HitPoints: 12, MaxHitPoints: 12,
			},
		},
	})
	s.Require().NoError(err)

	createResp, err := s.srv.LobbyClient.CreateLobby(s.authCtx(playerID), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-" + suffix, CharacterId: characterID,
	})
	s.Require().NoError(err)
	lobbyID := createResp.GetLobbyId()

	_, err = s.srv.LobbyClient.SetReady(s.authCtx(playerID), &lobbyv1alpha1.SetReadyRequest{
		LobbyId: lobbyID, Ready: true,
	})
	s.Require().NoError(err)

	startResp, err := s.srv.LobbyClient.StartEncounter(s.authCtx(playerID), &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: lobbyID,
	})
	s.Require().NoError(err, "StartEncounter must succeed via the real RPC path")
	encounterID := startResp.GetEncounterId()
	s.Require().NotEmpty(encounterID)

	before, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	pd, ok := before.Players[core.PlayerID(playerID)]
	s.Require().True(ok, "%q must be seated in the encounter", playerID)
	s.Require().NotNil(pd.View, "%q's view must be set", playerID)
	startHex := pd.View.Position

	destHex := core.Hex{Q: startHex.Q + dir.Q, R: startHex.R + dir.R, S: startHex.S + dir.S}

	_, err = s.srv.EncounterClientV2.MoveEntity(s.authCtx(playerID), &encounterv2pb.MoveEntityRequest{
		EncounterId: encounterID,
		EntityId:    characterID,
		ProposedPath: []*encounterv2pb.Position{
			{X: int32(startHex.Q), Y: int32(startHex.R), Z: int32(startHex.S)},
			{X: int32(destHex.Q), Y: int32(destHex.R), Z: int32(destHex.S)},
		},
	})
	s.Require().NoError(err, "MoveEntity must succeed via the real RPC path")

	after, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	afterPD, ok := after.Players[core.PlayerID(playerID)]
	s.Require().True(ok)
	s.Require().Equal(destHex, afterPD.View.Position,
		"%q must actually be at the destination hex after moving in direction %+v — "+
			"a silently-truncated (no-op) move is the #656 regression", playerID, dir)
}

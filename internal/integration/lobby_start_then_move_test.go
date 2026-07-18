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

// TestMoveEntity_AfterStartEncounter_ActuallyMoves is the #656 regression
// gate: StartEncounter's InitRoom + player-seeding combination must produce
// an encounter where every movement direction works, not just the ones a
// prior playtest happened to exercise. Reproduces the exact bug report
// (rpg-api#656): a fresh StartEncounter-created FREE_ROAM encounter, one
// player, a single-hex move in the +Q/-S direction (offset Y-1 from the
// player's spawn hex) — this is the direction that silently no-opped
// (truncated at the room's offset-coordinate bound) when the player spawned
// at the room's OFFSET CORNER (cube-origin) rather than its center.
func (s *LobbyStartThenMoveSuite) TestMoveEntity_AfterStartEncounter_ActuallyMoves() {
	const playerID = "alice"
	const characterID = "char-alice"

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
		CampaignId: "campaign-656", CharacterId: characterID,
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

	// Read the player's actual spawn position from the persisted encounter —
	// do not assume a specific hex, so this test pins "movement works from
	// wherever StartEncounter spawns the party" rather than a specific fix
	// shape.
	before, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	pd, ok := before.Players[core.PlayerID(playerID)]
	s.Require().True(ok, "alice must be seated in the encounter")
	s.Require().NotNil(pd.View, "alice's view must be set")
	startHex := pd.View.Position

	// The exact relative direction from rpg-api#656's bug report: Q+1, S-1
	// (offset Y-1 from the spawn hex) — the direction that silently no-opped
	// when the party spawned at the room's offset-coordinate corner.
	destHex := core.Hex{Q: startHex.Q + 1, R: startHex.R, S: startHex.S - 1}

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
		"alice must actually be at the destination hex after MoveEntity — "+
			"a silently-truncated (no-op) move is the #656 regression")
}

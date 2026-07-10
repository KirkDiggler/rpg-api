// Package integration_test's lobby suite is the boarded "Party Assembles"
// gate test (rpg-project's lobby-surface.md "Verify"): four dev-authenticated
// players create/join/ready/start and land in one v2 encounter with correct
// entities, driven entirely through the LobbyService RPC surface plus the
// pre-existing StreamEncounter path — proving StartEncounter is now the sole
// encounter construction path end-to-end, not just at the unit level.
package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	core "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// LobbyV1alpha1IntegrationSuite is the Party Assembles gate test suite.
type LobbyV1alpha1IntegrationSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	srv    *harness.TestServer
}

func (s *LobbyV1alpha1IntegrationSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	var err error
	s.srv, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")
}

func (s *LobbyV1alpha1IntegrationSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

// authCtx returns a context carrying dev-mode auth for the given player,
// mirroring EncounterV2IntegrationSuite.authCtx.
func (s *LobbyV1alpha1IntegrationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// seedCharacter creates a minimal playable character owned by playerID, for
// binding to a lobby seat.
func (s *LobbyV1alpha1IntegrationSuite) seedCharacter(characterID, playerID, name string, hp int) {
	_, err := s.srv.CharacterRepo.Create(s.ctx, characterrepo.CreateInput{
		Character: &entities.Character{
			Data: &toolkitchar.Data{
				ID: characterID, PlayerID: playerID, Name: name,
				HitPoints: hp, MaxHitPoints: hp,
			},
		},
	})
	s.Require().NoError(err)
}

// TestPartyAssembles_FourPlayers_CreateJoinReadyStart is the boarded
// verify story: one host creates, three join via the dev join_ref carrier,
// all four ready up, host starts, all four land in the same v2 encounter
// with HP seeded from their bound characters.
func (s *LobbyV1alpha1IntegrationSuite) TestPartyAssembles_FourPlayers_CreateJoinReadyStart() {
	players := []struct {
		id, character, name string
		hp                  int
	}{
		{"alice", "char-alice", "Alice", 12},
		{"bob", "char-bob", "Bob", 10},
		{"carol", "char-carol", "Carol", 14},
		{"dave", "char-dave", "Dave", 8},
	}
	for _, p := range players {
		s.seedCharacter(p.character, p.id, p.name, p.hp)
	}

	// Alice creates the lobby and becomes host.
	createResp, err := s.srv.LobbyClient.CreateLobby(s.authCtx("alice"), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: "char-alice",
	})
	s.Require().NoError(err)
	lobbyID := createResp.GetLobbyId()
	joinRef := createResp.GetJoinRef()
	s.Require().NotEmpty(lobbyID)
	s.Require().NotEmpty(joinRef)
	s.Require().Equal("alice", createResp.GetHostPlayerId())

	// bob, carol, dave join via the dev join_ref carrier — the same opaque
	// ref every player supplies, dev/playtest just passes it explicitly.
	var lastJoinResp *lobbyv1alpha1.JoinLobbyResponse
	for _, p := range players[1:] {
		resp, joinErr := s.srv.LobbyClient.JoinLobby(s.authCtx(p.id), &lobbyv1alpha1.JoinLobbyRequest{
			JoinRef: joinRef, CharacterId: p.character,
		})
		s.Require().NoError(joinErr)
		s.Require().Equal(lobbyID, resp.GetLobbyId())
		lastJoinResp = resp
	}
	s.Require().Len(lastJoinResp.GetMembers(), 4, "all four players must be in the roster after joining")

	// All four ready up.
	for _, p := range players {
		_, readyErr := s.srv.LobbyClient.SetReady(s.authCtx(p.id), &lobbyv1alpha1.SetReadyRequest{
			LobbyId: lobbyID, Ready: true,
		})
		s.Require().NoError(readyErr)
	}

	// Host starts. All-ready gate must pass; encounter_id comes back.
	startResp, err := s.srv.LobbyClient.StartEncounter(s.authCtx("alice"), &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: lobbyID,
	})
	s.Require().NoError(err)
	encounterID := startResp.GetEncounterId()
	s.Require().NotEmpty(encounterID)

	// Persist-then-emit proof at the integration level: the encounter must
	// already be readable from the encounter repo StartEncounter returned
	// success from — no polling/race needed since Save happens before the
	// RPC response.
	encData, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	s.Require().Len(encData.Players, 4, "all four ready members must become encounter players")
	for _, p := range players {
		pd, ok := encData.Players[core.PlayerID(p.id)]
		s.Require().True(ok, "player %q must be seated in the encounter", p.id)
		s.Require().Equal(core.EntityID(p.character), pd.EntityID)
		s.Require().Equal(p.hp, pd.HP, "player %q's HP must be seeded from their bound character", p.id)
		s.Require().Equal(p.hp, pd.MaxHP)
	}

	// rpg-api#632: an unseeded SightRange leaves every member able to see only
	// their own spawn hex — the party never actually sees each other despite
	// spawning adjacent. Assert every member's cumulative reveal covers every
	// OTHER member's spawn hex, so this regression can't pass silently again.
	for _, viewer := range players {
		viewerPD := encData.Players[core.PlayerID(viewer.id)]
		s.Require().NotZero(viewerPD.View.SightRange,
			"player %q must have a seeded SightRange", viewer.id)
		for _, other := range players {
			if other.id == viewer.id {
				continue
			}
			otherPD := encData.Players[core.PlayerID(other.id)]
			s.Require().True(viewerPD.View.RevealedHexes.Has(otherPD.View.Position),
				"player %q must be able to see player %q's spawn hex", viewer.id, other.id)
		}
	}

	// All four land in the SAME encounter via the pre-existing v2
	// StreamEncounter path — the already-proven snapshot-first flow
	// (lobby-surface.md "Verify").
	for _, p := range players {
		stream, streamErr := s.srv.EncounterClientV2.StreamEncounter(s.authCtx(p.id),
			&encounterv2pb.StreamEncounterRequest{EncounterId: encounterID})
		s.Require().NoError(streamErr)
		snap, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
		s.Require().NotNil(snap.GetSnapshotDelivered())
		s.Require().Equal(encounterID, snap.GetSnapshotDelivered().GetEncounter().GetId())
	}
}

// TestJoinLobby_LateJoin_FailedPrecondition proves the "late join" edge
// policy end-to-end: JoinLobby on a STARTED lobby is rejected, not silently
// admitted into a mid-encounter party.
func (s *LobbyV1alpha1IntegrationSuite) TestJoinLobby_LateJoin_FailedPrecondition() {
	s.seedCharacter("char-alice2", "alice", "Alice", 10)
	s.seedCharacter("char-eve", "eve", "Eve", 10)

	createResp, err := s.srv.LobbyClient.CreateLobby(s.authCtx("alice"), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: "char-alice2",
	})
	s.Require().NoError(err)

	_, err = s.srv.LobbyClient.SetReady(s.authCtx("alice"), &lobbyv1alpha1.SetReadyRequest{
		LobbyId: createResp.GetLobbyId(), Ready: true,
	})
	s.Require().NoError(err)
	_, err = s.srv.LobbyClient.StartEncounter(s.authCtx("alice"), &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: createResp.GetLobbyId(),
	})
	s.Require().NoError(err)

	_, err = s.srv.LobbyClient.JoinLobby(s.authCtx("eve"), &lobbyv1alpha1.JoinLobbyRequest{
		JoinRef: createResp.GetJoinRef(), CharacterId: "char-eve",
	})
	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func TestLobbyV1alpha1IntegrationSuite(t *testing.T) {
	suite.Run(t, new(LobbyV1alpha1IntegrationSuite))
}

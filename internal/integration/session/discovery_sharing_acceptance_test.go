package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// DiscoverySharingAcceptanceSuite is the web's switch through the real stack
// (rpg-project#753 R2): the deprecated session SetDiscoverySharing writes
// character configuration, GetKnowledge reports it at once, and a later
// session for the same character starts from it.
type DiscoverySharingAcceptanceSuite struct{ suite.Suite }

func TestDiscoverySharingAcceptanceSuite(t *testing.T) {
	suite.Run(t, new(DiscoverySharingAcceptanceSuite))
}

func (s *DiscoverySharingAcceptanceSuite) sharing(ctx context.Context, h *acceptanceHarness, run string) bool {
	out, err := h.handler.GetKnowledge(ctx, &sessionpb.GetKnowledgeRequest{Session: run, Member: "alice"})
	s.Require().NoError(err)
	s.Require().NotNil(out.DiscoverySharing, "this producer reports discovery sharing")
	return out.GetDiscoverySharing()
}

func (s *DiscoverySharingAcceptanceSuite) TestTheSwitchFlipsAndPersistsAcrossSessions() {
	h := newAcceptanceHarness(s.T())
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	for id, player := range map[string]string{"alice": "player-alice", "bob": "player-bob"} {
		_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter(id, player)}})
		s.Require().NoError(err)
	}

	h.launch(s.T(), "first-run", buildThreeRoomTomb(s.T()), seatAt("alice", 0, 0), seatAt("bob", 1, 0))
	s.False(s.sharing(ctx, h, "first-run"), "a character who never chose is private")

	out, err := h.handler.SetDiscoverySharing(ctx, &sessionpb.SetDiscoverySharingRequest{
		Session: "first-run", Member: "alice", Sharing: proto.Bool(true),
	})
	s.Require().NoError(err)
	s.True(out.Sharing)
	s.True(s.sharing(ctx, h, "first-run"), "the switch reads back flipped before the next write verb")

	stored, err := h.redis.Get(ctx, "character-config:alice").Result()
	s.Require().NoError(err)
	s.JSONEq(`{"discovery_sharing":true}`, stored)
	_, err = h.redis.Get(ctx, "character-exploration:v1:alice").Result()
	s.Error(err, "the legacy profile is never written")

	_, err = h.manager.Manager.Exit(ctx, &sdk.ExitInput{Session: "first-run", Member: "alice"})
	s.Require().NoError(err)
	h.launch(s.T(), "second-run", buildThreeRoomTomb(s.T()), seatAt("alice", 0, 0))
	s.True(s.sharing(ctx, h, "second-run"), "a new session starts from the character's choice")

	encounter, err := h.manager.Manager.Knowledge(ctx, &sdk.KnowledgeInput{Session: "second-run", Member: "alice", Player: "player-alice"})
	s.Require().NoError(err)
	s.Require().NotNil(encounter.DiscoverySharing)
	s.True(*encounter.DiscoverySharing, "the encounter itself restored the choice at launch")
}

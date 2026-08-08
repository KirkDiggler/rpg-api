package lobby_test

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
)

func (s *HandlerSuite) TestStartEncounter_NoAuth_Unauthenticated() {
	_, err := s.handler.StartEncounter(context.Background(), &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: "lobby-1",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestStartEncounter_EmptyLobbyID_InvalidArgument() {
	_, err := s.handler.StartEncounter(s.ctx, &lobbyv1alpha1.StartEncounterRequest{})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestStartEncounter_NotAllReady_FailedPrecondition() {
	lobbyID, _ := s.createLobby("alice", "char-alice", "Alice")

	_, err := s.handler.StartEncounter(s.ctx, &lobbyv1alpha1.StartEncounterRequest{LobbyId: lobbyID})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func (s *HandlerSuite) TestStartEncounter_NotHost_PermissionDenied() {
	lobbyID, joinRef := s.createLobby("alice", "char-alice", "Alice")
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)
	bobCtx := auth.WithPlayerID(context.Background(), "bob")
	_, err := s.handler.JoinLobby(bobCtx, &lobbyv1alpha1.JoinLobbyRequest{
		JoinRef: joinRef, CharacterId: "char-bob",
	})
	s.Require().NoError(err)

	_, err = s.handler.StartEncounter(bobCtx, &lobbyv1alpha1.StartEncounterRequest{LobbyId: lobbyID})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestStartEncounter_Success_ReturnsEncounterID() {
	lobbyID, _ := s.createLobby("alice", "char-alice", "Alice")
	_, err := s.handler.SetReady(s.ctx, &lobbyv1alpha1.SetReadyRequest{LobbyId: lobbyID, Ready: true})
	s.Require().NoError(err)

	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	resp, err := s.handler.StartEncounter(s.ctx, &lobbyv1alpha1.StartEncounterRequest{LobbyId: lobbyID})
	s.Require().NoError(err)
	s.Require().NotEmpty(resp.GetEncounterId())
}

// TestStartEncounter_DungeonKeyFromRequest_SelectsReferenceTomb is S2's
// headline proof: StartEncounterRequest.dungeon_key (on the wire since S0,
// rpg-api-protos v0.1.115) must actually reach StartEncounterInput.DungeonKey
// — the orchestrator's own precedence logic (effectiveKey,
// start_encounter.go:109-121) and content-key resolution are already fully
// tested at the orchestrator layer (start_encounter_test.go's
// TestStartEncounter_DungeonKeyOverride_* suite); this test proves ONLY the
// one-line handler wiring this slice adds. Uses the embedded reference-tomb
// key (not S1's uploaded "smoke-test" key) so this test stands alone with
// no RPG_CONTENT_DIR override needed. Distinguishing signal is structural,
// not a region count (reference-tomb and the legacy crypt default both
// compile to 3 regions): only reference-tomb emits a chamber-archetype
// "hall" region — the crypt template never does (verified against
// tkenc's own crypt_dungeon.go).
func (s *HandlerSuite) TestStartEncounter_DungeonKeyFromRequest_SelectsReferenceTomb() {
	lobbyID, _ := s.createLobby("alice", "char-alice", "Alice")
	_, err := s.handler.SetReady(s.ctx, &lobbyv1alpha1.SetReadyRequest{LobbyId: lobbyID, Ready: true})
	s.Require().NoError(err)

	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	resp, err := s.handler.StartEncounter(s.ctx, &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: lobbyID, DungeonKey: "reference-tomb",
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, resp.GetEncounterId())
	s.Require().NoError(err)
	s.Require().Len(encData.Space.Regions, 3)
	var sawHall bool
	for _, r := range encData.Space.Regions {
		if r.ID == "hall" {
			sawHall = true
		}
	}
	s.Require().True(sawHall, "reference-tomb (not the legacy crypt default) must have been selected")
}

// TestStartEncounter_EmptyDungeonKey_UsesLegacyDefault proves the additive
// half of S0's dungeon_key field: an empty (unset) dungeon_key must leave
// StartEncounterInput.DungeonKey at the zero value, exactly like every
// caller before this field existed — the legacy crypt default (entrance/
// corridor/boss, no chamber-archetype region) must still be what gets
// built, not a decode of "" into some other key. Asserts the crypt
// template's own "corridor" region is actually PRESENT (rpg-toolkit's
// crypt_dungeon.go: cryptRegionIDCorridor = "corridor"), not just that
// "hall" is absent — a positive assertion many wrong outcomes (an error,
// an empty Space, some other unrelated content key) would also satisfy
// the weaker absence-only check.
func (s *HandlerSuite) TestStartEncounter_EmptyDungeonKey_UsesLegacyDefault() {
	lobbyID, _ := s.createLobby("alice", "char-alice", "Alice")
	_, err := s.handler.SetReady(s.ctx, &lobbyv1alpha1.SetReadyRequest{LobbyId: lobbyID, Ready: true})
	s.Require().NoError(err)

	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	resp, err := s.handler.StartEncounter(s.ctx, &lobbyv1alpha1.StartEncounterRequest{LobbyId: lobbyID})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, resp.GetEncounterId())
	s.Require().NoError(err)
	var sawCorridor, sawHall bool
	for _, r := range encData.Space.Regions {
		switch r.ID {
		case "corridor":
			sawCorridor = true
		case "hall":
			sawHall = true
		}
	}
	s.Require().True(sawCorridor, "the legacy crypt default's own \"corridor\" region must be present")
	s.Assert().False(sawHall, "an empty dungeon_key must not accidentally select reference-tomb")
}

package lobby_test

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	lobbyhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/lobby/v1alpha1"
)

func (s *HandlerSuite) TestListDungeons_NoAuth_Unauthenticated() {
	_, err := s.handler.ListDungeons(context.Background(), &lobbyv1alpha1.ListDungeonsRequest{})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

// TestListDungeons_ListsWhatIsShipped: the picker sees what the registry
// holds, with authoring off (the handler suite's registry is read-only):
// every single-room reference, marked SHIPPED with no owner. The version 2
// files are left out of the list and still play by key (design R23).
func (s *HandlerSuite) TestListDungeons_ListsWhatIsShipped() {
	resp, err := s.handler.ListDungeons(s.ctx, &lobbyv1alpha1.ListDungeonsRequest{})
	s.Require().NoError(err)

	keys := make([]string, 0, len(resp.GetDungeons()))
	for _, d := range resp.GetDungeons() {
		s.NotEmpty(d.GetName(), "dungeon %q reaches the picker with no name", d.GetKey())
		s.Equal(lobbyv1alpha1.DungeonSource_DUNGEON_SOURCE_SHIPPED, d.GetSource(), d.GetKey())
		s.Empty(d.GetOwner(), d.GetKey())
		keys = append(keys, d.GetKey())
	}
	s.Equal(dungeonstest.ListedReferenceKeys(s.T()), keys)
	s.NotContains(keys, dungeons.DefaultKey, "the v2 tomb plays by an empty key, unlisted")
}

func (s *HandlerSuite) TestListDungeons_WithoutAWorld_FailedPrecondition() {
	_, err := s.handler.ListDungeons(auth.WithPlayerID(context.Background(), "alice"), &lobbyv1alpha1.ListDungeonsRequest{})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Equal(codes.FailedPrecondition, st.Code())
	s.Equal("trusted world context is required", st.Message())
}

// TestDungeonSummaryToProto_MapsSourceAndOwner: WORLD carries its owner,
// SHIPPED none; ownership rides on owner, not on source.
func (s *HandlerSuite) TestDungeonSummaryToProto_MapsSourceAndOwner() {
	s.Equal(&lobbyv1alpha1.DungeonSummary{
		Key: "my-castle", Name: "My Castle", Owner: "alice", Source: lobbyv1alpha1.DungeonSource_DUNGEON_SOURCE_WORLD,
	}, lobbyhandler.DungeonSummaryToProto(dungeons.Summary{
		Key: "my-castle", Name: "My Castle", Owner: "alice", Source: dungeons.SourceWorld,
	}))
	s.Equal(&lobbyv1alpha1.DungeonSummary{
		Key: "castle-kitchen", Name: "The Castle Kitchen", Source: lobbyv1alpha1.DungeonSource_DUNGEON_SOURCE_SHIPPED,
	}, lobbyhandler.DungeonSummaryToProto(dungeons.Summary{
		Key: "castle-kitchen", Name: "The Castle Kitchen", Source: dungeons.SourceShipped,
	}))
}

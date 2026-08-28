package lobby_test

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
)

func (s *HandlerSuite) TestListDungeons_NoAuth_Unauthenticated() {
	_, err := s.handler.ListDungeons(context.Background(), &lobbyv1alpha1.ListDungeonsRequest{})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

// TestListDungeons_ListsTheShippedTomb: the picker sees what the registry
// holds, with authoring off (the handler suite's registry is read-only).
func (s *HandlerSuite) TestListDungeons_ListsTheShippedTomb() {
	resp, err := s.handler.ListDungeons(s.ctx, &lobbyv1alpha1.ListDungeonsRequest{})
	s.Require().NoError(err)
	s.Require().Len(resp.GetDungeons(), 1)
	s.Equal(dungeons.DefaultKey, resp.GetDungeons()[0].GetKey())
	s.NotEmpty(resp.GetDungeons()[0].GetName())
}

package lobby_test

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
)

func (s *HandlerSuite) TestListDungeons_NoAuth_Unauthenticated() {
	_, err := s.handler.ListDungeons(context.Background(), &lobbyv1alpha1.ListDungeonsRequest{})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

// TestListDungeons_ListsWhatIsShipped: the picker sees what the registry
// holds, with authoring off (the handler suite's registry is read-only).
//
// Every shipped dungeon, not a named one: the content directory grew a
// second file with rpg-project#368 (the heirloom fixture the
// recover-the-artifact scenario is authored against), and the picker showing
// it is the point rather than a side effect -- it is how a party chooses to
// play that scenario.
func (s *HandlerSuite) TestListDungeons_ListsWhatIsShipped() {
	resp, err := s.handler.ListDungeons(s.ctx, &lobbyv1alpha1.ListDungeonsRequest{})
	s.Require().NoError(err)

	keys := make([]string, 0, len(resp.GetDungeons()))
	for _, d := range resp.GetDungeons() {
		s.NotEmpty(d.GetName(), "dungeon %q reaches the picker with no name", d.GetKey())
		keys = append(keys, d.GetKey())
	}
	s.Contains(keys, dungeons.DefaultKey, "the default is always there -- 'no key' means the tomb")
	s.Len(keys, dungeonstest.ShippedCount(s.T()), "and every other file in the content directory beside it")
}

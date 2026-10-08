package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionmock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

type KnowledgeHandlerSuite struct {
	suite.Suite
	manager *sessionmock.MockManager
	handler *Handler
	ctx     context.Context
}

func TestKnowledgeHandlerSuite(t *testing.T) { suite.Run(t, new(KnowledgeHandlerSuite)) }
func (s *KnowledgeHandlerSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())
	s.manager = sessionmock.NewMockManager(ctrl)
	s.handler = &Handler{manager: s.manager, characters: anyMemberOwnedBy(ctrl, "alice"), broker: sessionorch.NewBroker()}
	s.ctx = auth.WithPlayerID(context.Background(), "alice")
}
func (s *KnowledgeHandlerSuite) TestSnapshotBindsIdentityAndCopiesTheProviderAnswers() {
	s.manager.EXPECT().Knowledge(gomock.Any(), &sdk.KnowledgeInput{Session: "run", Member: "hero", Player: "alice"}).Return(&sdk.KnowledgeOutput{
		Seq: 7, Holding: []string{"chest"}, Where: sdk.WhereOutput{Position: spatial.Position{X: 2, Y: 1}},
		View: sdk.ViewOutput{Props: []sdk.PropSighting{{Prop: &sdk.AtlasProp{ID: "vase", At: spatial.Position{X: 9, Y: 2}}, ObservedEmpty: true, Status: "held"}},
			Doors: []sdk.DoorSighting{{Door: sdk.Door{ID: "gate", State: "open"}, Status: "held"}}},
	}, nil)
	out, err := s.handler.GetKnowledge(s.ctx, &sessionpb.GetKnowledgeRequest{Session: "run", Member: "hero"})
	s.Require().NoError(err)
	s.Equal(uint64(7), out.Seq)
	s.Equal([]string{"chest"}, out.Holding)
	s.Equal(2.0, out.Where.Position.X)
	s.Require().Len(out.View.Props, 1)
	s.Equal("vase", out.View.Props[0].GetProp().Id)
	s.Nil(out.View.Props[0].GetProp().At, "unknown placement is not position zero or the stale position")
	s.Empty(out.View.Doors[0].CurrentVia)
	s.Equal(sessionpb.DoorState_DOOR_STATE_OPEN, out.View.Doors[0].Door.State)
}
func (s *KnowledgeHandlerSuite) TestSnapshotCarriesStructuralRowsFromTheOwnedKnowledgeRead() {
	atlas := structuralAtlasFixture()
	s.manager.EXPECT().Knowledge(gomock.Any(), &sdk.KnowledgeInput{Session: "run", Member: "hero", Player: "alice"}).Return(&sdk.KnowledgeOutput{Atlas: atlas}, nil)
	out, err := s.handler.GetKnowledge(s.ctx, &sessionpb.GetKnowledgeRequest{Session: "run", Member: "hero"})
	s.Require().NoError(err)
	s.Require().NotNil(out.Atlas)
	s.Require().Len(out.Atlas.StructuralWalls, 1)
	s.Require().Len(out.Atlas.StructuralDoors, 1)
	s.Equal("wall-a", out.Atlas.StructuralWalls[0].Id)
	s.Equal(13.25, out.Atlas.StructuralWalls[0].From.X)
	s.Equal("keep/gate", out.Atlas.StructuralDoors[0].Id)
	s.Empty(out.View.Doors, "fixed layout must not manufacture a mutable observation")
}

func (s *KnowledgeHandlerSuite) TestAnOwnedCharacterDoesNotGrantAnotherSessionSeat() {
	s.manager.EXPECT().Knowledge(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotSeated)
	out, err := s.handler.GetKnowledge(s.ctx, &sessionpb.GetKnowledgeRequest{Session: "other-run", Member: "hero"})
	s.Nil(out)
	requireCode(s.T(), err, codes.PermissionDenied)
}
func (s *KnowledgeHandlerSuite) TestStreamRefusesAnOwnedButUnseatedMember() {
	s.manager.EXPECT().Roster(gomock.Any(), &sdk.RosterInput{Session: "other-run", Member: "hero", Player: "alice"}).Return(nil, sdk.ErrNotSeated)
	err := s.handler.StreamEvents(&sessionpb.StreamEventsRequest{Session: "other-run", Member: "hero"}, newCapturingStream(s.ctx))
	requireCode(s.T(), err, codes.PermissionDenied)
}
func (s *KnowledgeHandlerSuite) TestEmptyFootprintDoesNotLeakAnOldPlacement() {
	out := viewToProto(&sdk.ViewOutput{Props: []sdk.PropSighting{{Placed: &sdk.AtlasPlacedProp{ID: "table", Placement: sdk.FootprintPlacement{Width: 12, Depth: 8}, Cells: []spatial.Position{{X: 4, Y: 5}}}, ObservedEmpty: true, Status: "held"}}})
	s.Equal("table", out.Props[0].GetPlaced().Id)
	s.Nil(out.Props[0].GetPlaced().Placement)
	s.Empty(out.Props[0].GetPlaced().Cells)
}

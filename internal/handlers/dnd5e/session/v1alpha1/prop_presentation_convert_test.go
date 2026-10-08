package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type PropPresentationConversionSuite struct{ suite.Suite }

func TestPropPresentationConversionSuite(t *testing.T) {
	suite.Run(t, new(PropPresentationConversionSuite))
}
func (s *PropPresentationConversionSuite) TestEveryCarrierPreservesPermittedPoseAndIdentity() {
	in := sdk.PropPresentation{ID: "raw-prop", Ref: "content:vase", Origin: sdk.FootprintPoint{X: 12.25, Y: -7.5}, Elevation: 2.75, FacingDegrees: -37, HeightScale: 1.5, DoorID: "actual/gate", Label: "Authored name", PointLight: &sdk.PropPointLight{Enabled: true, Offset: sdk.FootprintPoint{X: .2, Y: -.3}, OffsetElevation: 1, Color: "#aabbcc", Intensity: 2, Range: 8}}
	want := &sessionpb.PropPresentation{Id: "raw-prop", Ref: "content:vase", Origin: &sessionpb.FootprintPoint{X: 12.25, Y: -7.5}, Elevation: 2.75, FacingDegrees: -37, HeightScale: 1.5, DoorId: "actual/gate", Label: "Authored name", PointLight: &sessionpb.PropPointLight{Enabled: true, Offset: &sessionpb.FootprintPoint{X: .2, Y: -.3}, OffsetElevation: 1, Color: "#aabbcc", Intensity: 2, Range: 8}}
	atlas := AtlasToProto(&sdk.Atlas{DungeonKey: "not-the-door-prefix", PropPresentations: []sdk.PropPresentation{in}})
	s.Require().Len(atlas.PropPresentations, 1)
	s.True(proto.Equal(want, atlas.PropPresentations[0]))
	for _, event := range []sdk.Event{
		{Kind: sdk.EventRoomRevealed, Body: sdk.RoomRevealedBody{Region: sdk.AtlasRegion{ID: "room"}, PropPresentations: []sdk.PropPresentation{in}}},
		{Kind: sdk.EventConcealmentRevealed, Body: sdk.ConcealmentRevealedBody{Concealment: "secret", PropPresentations: []sdk.PropPresentation{in}}},
	} {
		out, err := eventToProto(event)
		s.Require().NoError(err)
		rows := out.GetRoomRevealed().GetPropPresentations()
		if event.Kind == sdk.EventConcealmentRevealed {
			rows = out.GetConcealmentRevealed().GetPropPresentations()
		}
		s.Require().Len(rows, 1)
		s.True(proto.Equal(want, rows[0]))
	}
	view := viewToProto(&sdk.ViewOutput{Props: []sdk.PropSighting{{Placed: &sdk.AtlasPlacedProp{ID: in.ID}, Presentation: &in, Status: "remembered"}}})
	s.Require().Len(view.Props, 1)
	s.True(proto.Equal(want, view.Props[0].Presentation))
	s.Empty(view.Props[0].CurrentVia)
	view.Props[0].Presentation.PointLight.Range = 999
	s.Equal(8.0, in.PointLight.Range)
}

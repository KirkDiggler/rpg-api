package sessionv1alpha2

import (
	"testing"

	"github.com/stretchr/testify/suite"

	materialpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/material/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type MaterialChannelsSuite struct{ suite.Suite }

func TestMaterialChannelsSuite(t *testing.T) { suite.Run(t, new(MaterialChannelsSuite)) }

func (s *MaterialChannelsSuite) TestLiveAppearanceOwnersCarryTheSameExplicitIntent() {
	half := .5
	values := []sdk.MaterialChannelAdjustment{{TargetID: "scope", Normal: &sdk.ChannelAdjustment{Mode: sdk.ChannelScale, Multiplier: &half}, Emissive: &sdk.ChannelAdjustment{Mode: sdk.ChannelOff}}}
	outputs := [][]*materialpb.MaterialChannelAdjustment{
		structuralWallsToProto([]sdk.AtlasStructuralWall{{ID: "wall", MaterialChannels: values}})[0].MaterialChannels,
		propPresentationsToProto([]sdk.PropPresentation{{ID: "prop", Ref: "model", HeightScale: 1, MaterialChannels: values}})[0].MaterialChannels,
		doorFrameToProto(&sdk.DoorFramePresentation{Span: &sdk.WallFrameSpan{Before: 3, After: 3}, MaterialChannels: values}).MaterialChannels,
	}
	half = 7
	for _, out := range outputs {
		s.Require().Len(out, 1)
		s.Require().IsType(&materialpb.ChannelAdjustment_Multiplier{}, out[0].Normal.Value)
		s.Equal(.5, out[0].Normal.GetMultiplier())
		s.NotNil(out[0].Emissive.GetOff())
	}
	s.Nil(doorFrameToProto(nil))
}

package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/suite"

	materialpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/material/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type MaterialChannelsSuite struct{ suite.Suite }

func TestMaterialChannelsSuite(t *testing.T) { suite.Run(t, new(MaterialChannelsSuite)) }

func (s *MaterialChannelsSuite) TestEverySnapshotAppearanceOwnerCarriesChannelPresence() {
	one := float64(1)
	values := []sdk.MaterialChannelAdjustment{{TargetID: "scope", Normal: &sdk.ChannelAdjustment{Mode: sdk.ChannelScale, Multiplier: &one}, Emissive: &sdk.ChannelAdjustment{Mode: sdk.ChannelOff}}}
	frame := &sdk.DoorFramePresentation{Span: &sdk.WallFrameSpan{Before: 3, After: 3}, MaterialChannels: values}
	outputs := [][]*materialpb.MaterialChannelAdjustment{
		atlasStructuralWallsToProto([]sdk.AtlasStructuralWall{{ID: "wall", MaterialChannels: values}})[0].MaterialChannels,
		propPresentationToProto(sdk.PropPresentation{ID: "prop", Ref: "model", HeightScale: 1, MaterialChannels: values}).MaterialChannels,
		doorFrameToProto(frame).MaterialChannels,
		publicMemberToProto(sdk.PublicMember{ID: "npc", Kind: "monster", MaterialChannels: values}).MaterialChannels,
	}
	one = 9
	for _, out := range outputs {
		s.Require().Len(out, 1)
		s.Equal("scope", out[0].TargetId)
		s.Require().IsType(&materialpb.ChannelAdjustment_Multiplier{}, out[0].Normal.Value)
		s.Equal(float64(1), out[0].Normal.GetMultiplier())
		s.NotNil(out[0].Emissive.GetOff())
	}
	s.Empty(publicMemberToProto(sdk.PublicMember{ID: "legacy"}).MaterialChannels)
}

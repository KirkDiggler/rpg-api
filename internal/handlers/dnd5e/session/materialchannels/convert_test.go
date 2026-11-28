package materialchannels

import (
	"testing"

	"github.com/stretchr/testify/suite"

	materialpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/material/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type ConvertSuite struct{ suite.Suite }

func TestMaterialChannelConversion(t *testing.T) { suite.Run(t, new(ConvertSuite)) }

func (s *ConvertSuite) TestAbsentOffAndExplicitOneStayDistinctAndDetached() {
	one := float64(1)
	values := []sdk.MaterialChannelAdjustment{
		{TargetID: "body", Normal: &sdk.ChannelAdjustment{Mode: sdk.ChannelScale, Multiplier: &one}, Emissive: &sdk.ChannelAdjustment{Mode: sdk.ChannelOff}},
		{TargetID: "other", Emissive: &sdk.ChannelAdjustment{Mode: sdk.ChannelScale, Multiplier: &one}},
	}
	out := Convert(Input{Values: values}).Values
	s.Require().Len(out, 2)
	s.Equal("body", out[0].TargetId)
	s.Require().IsType(&materialpb.ChannelAdjustment_Multiplier{}, out[0].Normal.Value)
	s.Equal(float64(1), out[0].Normal.GetMultiplier())
	s.NotNil(out[0].Emissive.GetOff())
	s.Nil(out[1].Normal)
	one = 9
	values[0].TargetID = "changed"
	s.Equal(float64(1), out[0].Normal.GetMultiplier())
	s.Equal("body", out[0].TargetId)
	s.Empty(Convert(Input{}).Values)
}

func (s *ConvertSuite) TestMalformedPresentLeafNeverTurnsIntoDefault() {
	one := float64(1)
	for _, value := range []*sdk.ChannelAdjustment{
		{Mode: "unknown"}, {Mode: sdk.ChannelScale}, {Mode: sdk.ChannelOff, Multiplier: &one},
	} {
		out := Convert(Input{Values: []sdk.MaterialChannelAdjustment{{TargetID: "body", Normal: value}}}).Values
		s.Require().NotNil(out[0].Normal, "only an absent source means default")
		s.Nil(out[0].Normal.Value, "unsupported source stays a present invalid union, not invented valid intent")
	}
}

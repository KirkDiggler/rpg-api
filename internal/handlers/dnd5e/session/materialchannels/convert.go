// Package materialchannels maps the SDK's captured channel values to the shared
// protobuf leaf. It owns no asset interpretation, defaults or game rules.
package materialchannels

import (
	"google.golang.org/protobuf/types/known/emptypb"

	materialpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/material/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// Input supplies values already validated/captured by the SDK owner.
type Input struct {
	Values []sdk.MaterialChannelAdjustment
}

// Output contains detached wire values for snapshot and live delivery.
type Output struct {
	Values []*materialpb.MaterialChannelAdjustment
}

// Convert preserves absence, explicit Off and explicit multipliers. Only nil
// source channels become absent. A malformed present SDK leaf remains a present
// invalid union, rather than being silently converted to default or a valid look;
// normal SDK boundaries reject that input before transport.
func Convert(input Input) Output {
	var out Output
	if len(input.Values) == 0 {
		return out
	}
	out.Values = make([]*materialpb.MaterialChannelAdjustment, len(input.Values))
	for i, source := range input.Values {
		value := &materialpb.MaterialChannelAdjustment{TargetId: source.TargetID}
		for _, channel := range []struct {
			source *sdk.ChannelAdjustment
			target **materialpb.ChannelAdjustment
		}{
			{source.Normal, &value.Normal}, {source.Emissive, &value.Emissive},
		} {
			if channel.source == nil {
				continue
			}
			converted := &materialpb.ChannelAdjustment{}
			switch channel.source.Mode {
			case sdk.ChannelOff:
				if channel.source.Multiplier == nil {
					converted.Value = &materialpb.ChannelAdjustment_Off{Off: &emptypb.Empty{}}
				}
			case sdk.ChannelScale:
				if channel.source.Multiplier != nil {
					converted.Value = &materialpb.ChannelAdjustment_Multiplier{Multiplier: *channel.source.Multiplier}
				}
			}
			*channel.target = converted
		}
		out.Values[i] = value
	}
	return out
}

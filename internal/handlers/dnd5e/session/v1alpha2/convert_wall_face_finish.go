package sessionv1alpha2

import (
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// Preserve the SDK's captured choices; no current-source or resource lookup.
func wallFaceFinishesToProto(values []sdk.WallFaceFinish) []*sessionpb.WallFaceFinish {
	return mapSlice(values, func(value sdk.WallFaceFinish) *sessionpb.WallFaceFinish {
		return &sessionpb.WallFaceFinish{Side: value.Side, MaterialRef: value.MaterialRef}
	})
}
